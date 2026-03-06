package appmeta

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Info struct {
	PID            int
	ExecutablePath string
	CPUPct         *float64
	MemoryPct      *float64
	AppName        string
	BundlePath     string
	BundleID       string
	IsApp          bool
}

func LookupMany(ctx context.Context, pids []int) (map[int]Info, error) {
	result := make(map[int]Info, len(pids))
	if len(pids) == 0 {
		return result, nil
	}

	pidParts := make([]string, 0, len(pids))
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		pidParts = append(pidParts, strconv.Itoa(pid))
	}
	if len(pidParts) == 0 {
		return result, nil
	}

	args := []string{"-p", strings.Join(pidParts, ","), "-o", "pid=", "-o", "%cpu=", "-o", "%mem=", "-o", "comm="}
	out, err := exec.CommandContext(ctx, "ps", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ps lookup: %w", err)
	}

	bundleCache := map[string]bundleInfo{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		info := Info{PID: pid}
		if value, err := strconv.ParseFloat(fields[1], 64); err == nil {
			info.CPUPct = &value
		}
		if value, err := strconv.ParseFloat(fields[2], 64); err == nil {
			info.MemoryPct = &value
		}
		info.ExecutablePath = strings.Join(fields[3:], " ")

		bundlePath := detectBundlePath(info.ExecutablePath)
		if bundlePath != "" {
			info.IsApp = true
			info.BundlePath = bundlePath
			if cached, ok := bundleCache[bundlePath]; ok {
				info.AppName = cached.AppName
				info.BundleID = cached.BundleID
			} else {
				meta := readBundleInfo(ctx, bundlePath)
				bundleCache[bundlePath] = meta
				info.AppName = meta.AppName
				info.BundleID = meta.BundleID
			}
		}
		if info.AppName == "" {
			info.AppName = filepath.Base(strings.TrimSpace(info.ExecutablePath))
		}

		result[pid] = info
	}

	return result, nil
}

type bundleInfo struct {
	AppName  string
	BundleID string
}

func detectBundlePath(executablePath string) string {
	const marker = ".app/Contents/"
	idx := strings.Index(executablePath, marker)
	if idx == -1 {
		return ""
	}
	return executablePath[:idx+len(".app")]
}

func readBundleInfo(ctx context.Context, bundlePath string) bundleInfo {
	infoPlist := filepath.Join(bundlePath, "Contents", "Info.plist")
	out, err := exec.CommandContext(ctx, "plutil", "-convert", "json", "-o", "-", infoPlist).CombinedOutput()
	if err != nil {
		return bundleInfo{AppName: appNameFromBundle(bundlePath)}
	}

	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		return bundleInfo{AppName: appNameFromBundle(bundlePath)}
	}

	appName := stringValue(payload["CFBundleDisplayName"])
	if appName == "" {
		appName = stringValue(payload["CFBundleName"])
	}
	if appName == "" {
		appName = appNameFromBundle(bundlePath)
	}

	return bundleInfo{
		AppName:  appName,
		BundleID: stringValue(payload["CFBundleIdentifier"]),
	}
}

func appNameFromBundle(bundlePath string) string {
	base := filepath.Base(bundlePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

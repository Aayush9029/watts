package service

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/Aayush9029/watts/internal/config"
)

const Label = "com.aayush9029.watts"

var plistTemplate = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>{{ .Label }}</string>
  <key>ProgramArguments</key>
  <array>
    <string>{{ .BinaryPath }}</string>
    <string>daemon</string>
    <string>--config</string>
    <string>{{ .ConfigPath }}</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>{{ .LogPath }}</string>
  <key>StandardErrorPath</key>
  <string>{{ .ErrorLogPath }}</string>
  <key>WorkingDirectory</key>
  <string>{{ .WorkingDirectory }}</string>
</dict>
</plist>
`))

func Install(binaryPath string, cfg config.Config) error {
	if err := cfg.EnsureDirectories(); err != nil {
		return err
	}
	if err := touchFile(cfg.LogPath); err != nil {
		return err
	}
	if err := touchFile(cfg.ErrorLogPath); err != nil {
		return err
	}
	if err := writePlist(binaryPath, cfg); err != nil {
		return err
	}
	return Restart()
}

func Uninstall() error {
	_ = Stop()
	if err := os.Remove(PlistPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func Start() error {
	if _, err := os.Stat(PlistPath()); err != nil {
		return fmt.Errorf("launchd plist not found at %s", PlistPath())
	}
	if loaded, _ := IsLoaded(); loaded {
		return run("launchctl", "kickstart", "-k", "system/"+Label)
	}
	return run("launchctl", "bootstrap", "system", PlistPath())
}

func Stop() error {
	if loaded, _ := IsLoaded(); !loaded {
		return nil
	}
	return run("launchctl", "bootout", "system/"+Label)
}

func Restart() error {
	_ = Stop()
	if err := run("launchctl", "bootstrap", "system", PlistPath()); err != nil {
		return err
	}
	return run("launchctl", "kickstart", "-k", "system/"+Label)
}

func IsLoaded() (bool, string) {
	output, err := exec.Command("launchctl", "print", "system/"+Label).CombinedOutput()
	if err != nil {
		return false, strings.TrimSpace(string(output))
	}
	return true, string(output)
}

func Status() (map[string]string, error) {
	output, err := exec.Command("launchctl", "print", "system/"+Label).CombinedOutput()
	if err != nil {
		return map[string]string{
			"loaded": falseString(),
			"error":  strings.TrimSpace(string(output)),
		}, nil
	}

	status := map[string]string{
		"loaded": trueString(),
	}
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "pid = "):
			status["pid"] = strings.TrimPrefix(trimmed, "pid = ")
		case strings.HasPrefix(trimmed, "state = "):
			status["state"] = strings.TrimPrefix(trimmed, "state = ")
		case strings.HasPrefix(trimmed, "last exit code = "):
			status["last_exit_code"] = strings.TrimPrefix(trimmed, "last exit code = ")
		}
	}
	return status, nil
}

func PlistPath() string {
	return filepath.Join("/Library/LaunchDaemons", Label+".plist")
}

func writePlist(binaryPath string, cfg config.Config) error {
	var content bytes.Buffer
	payload := map[string]string{
		"Label":            Label,
		"BinaryPath":       binaryPath,
		"ConfigPath":       config.ConfigPath(cfg.HomeDir),
		"LogPath":          cfg.LogPath,
		"ErrorLogPath":     cfg.ErrorLogPath,
		"WorkingDirectory": cfg.HomeDir,
	}
	if err := plistTemplate.Execute(&content, payload); err != nil {
		return err
	}
	return os.WriteFile(PlistPath(), content.Bytes(), 0o644)
}

func touchFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func falseString() string {
	return strconv.FormatBool(false)
}

func trueString() string {
	return strconv.FormatBool(true)
}

package power

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	elapsedMSRE    = regexp.MustCompile(`\(([\d.]+)ms elapsed\)`)
	powerLineRE    = regexp.MustCompile(`^(CPU Power|GPU Power|ANE Power|Combined Power .*?):\s+(-?[\d.]+)\s+mW$`)
	batteryPctRE   = regexp.MustCompile(`Battery:\s+percent_charge:\s+([\d.]+)`)
	backlightPctRE = regexp.MustCompile(`(?i)Backlight.*?([\d.]+)`)
	columnSplitRE  = regexp.MustCompile(`\s{2,}`)
	taskEnergyRE   = regexp.MustCompile(`^(.*?)\s+(-?\d+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s*$`)
)

type Snapshot struct {
	DurationMS        *float64
	CPUPowerW         *float64
	GPUPowerW         *float64
	ANEPowerW         *float64
	CombinedPowerW    *float64
	BatteryPercent    *float64
	BrightnessPercent *float64
	Processes         []ProcessSample
}

type ProcessSample struct {
	Name                 string
	PID                  int
	EnergyImpact         *float64
	CPUMsPerSec          *float64
	UserPercent          *float64
	DeadlineLT2MSPerSec  *float64
	Deadline2To5MSPerSec *float64
	WakeupsIntrPerSec    *float64
	WakeupsPkgIdlePerSec *float64
}

func Collect(ctx context.Context) (Snapshot, error) {
	args := []string{
		"--samplers", "tasks,battery,cpu_power,gpu_power,ane_power",
		"--show-process-energy",
		"-i", "1000",
		"-n", "1",
		"-a", "0",
	}
	out, err := exec.CommandContext(ctx, "powermetrics", args...).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "*** Sampled system activity") {
			return ParseText(string(out))
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return Snapshot{}, fmt.Errorf("powermetrics: %s", msg)
	}
	return ParseText(string(out))
}

func ParseText(raw string) (Snapshot, error) {
	snapshot := Snapshot{}

	if matches := elapsedMSRE.FindStringSubmatch(raw); len(matches) == 2 {
		if value, err := strconv.ParseFloat(matches[1], 64); err == nil {
			snapshot.DurationMS = &value
		}
	}

	lines := strings.Split(raw, "\n")
	section := ""
	var headers []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if section == "tasks" {
				headers = nil
			}
			continue
		}

		switch trimmed {
		case "*** Running tasks ***":
			section = "tasks"
			headers = nil
			continue
		case "**** Battery and backlight usage ****":
			section = "battery"
			continue
		case "**** Processor usage ****":
			section = "processor"
			continue
		default:
			if strings.HasPrefix(trimmed, "***") || strings.HasPrefix(trimmed, "****") {
				section = ""
				headers = nil
				continue
			}
		}

		switch section {
		case "tasks":
			if headers == nil {
				headers = splitColumns(trimmed)
				continue
			}
			process, ok := parseProcessLine(headers, trimmed)
			if ok {
				snapshot.Processes = append(snapshot.Processes, process)
			}
		case "battery":
			parseBatterySection(&snapshot, trimmed)
		case "processor":
			parseProcessorSection(&snapshot, trimmed)
		}
	}

	sort.SliceStable(snapshot.Processes, func(i, j int) bool {
		left := processSortValue(snapshot.Processes[i])
		right := processSortValue(snapshot.Processes[j])
		return left > right
	})

	return snapshot, nil
}

func parseBatterySection(snapshot *Snapshot, line string) {
	if matches := batteryPctRE.FindStringSubmatch(line); len(matches) == 2 {
		if value, err := strconv.ParseFloat(matches[1], 64); err == nil {
			snapshot.BatteryPercent = &value
		}
	}
	if matches := backlightPctRE.FindStringSubmatch(line); len(matches) == 2 {
		if value, err := strconv.ParseFloat(matches[1], 64); err == nil {
			snapshot.BrightnessPercent = &value
		}
	}
}

func parseProcessorSection(snapshot *Snapshot, line string) {
	matches := powerLineRE.FindStringSubmatch(line)
	if len(matches) != 3 {
		return
	}
	value, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return
	}
	watts := value / 1000.0
	switch {
	case strings.HasPrefix(matches[1], "CPU Power"):
		snapshot.CPUPowerW = &watts
	case strings.HasPrefix(matches[1], "GPU Power"):
		snapshot.GPUPowerW = &watts
	case strings.HasPrefix(matches[1], "ANE Power"):
		snapshot.ANEPowerW = &watts
	case strings.HasPrefix(matches[1], "Combined Power"):
		snapshot.CombinedPowerW = &watts
	}
}

func parseProcessLine(headers []string, line string) (ProcessSample, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "ALL_TASKS") || strings.HasPrefix(trimmed, "DEAD_TASKS") {
		return ProcessSample{}, false
	}

	if process, ok := parseTaskEnergyLine(trimmed); ok {
		return process, true
	}

	fields := splitColumns(trimmed)
	if len(fields) < len(headers) {
		return ProcessSample{}, false
	}
	if len(fields) > len(headers) {
		diff := len(fields) - len(headers)
		name := strings.Join(fields[:diff+1], " ")
		fields = append([]string{name}, fields[diff+1:]...)
	}

	rawColumns := make(map[string]string, len(headers))
	for idx, header := range headers {
		rawColumns[header] = fields[idx]
	}

	name := fields[0]
	if name == "ALL_TASKS" || name == "DEAD_TASKS" {
		return ProcessSample{}, false
	}

	pid := parsePID(rawColumns)
	process := ProcessSample{
		Name: name,
		PID:  pid,
	}

	if value, ok := parseFloatColumn(rawColumns, "Energy"); ok {
		process.EnergyImpact = &value
	}
	if value, ok := parseFloatColumn(rawColumns, "CPU ms/s"); ok {
		process.CPUMsPerSec = &value
	}
	if value, ok := parseFloatColumn(rawColumns, "User%"); ok {
		process.UserPercent = &value
	}

	return process, true
}

func parseTaskEnergyLine(line string) (ProcessSample, bool) {
	matches := taskEnergyRE.FindStringSubmatch(strings.TrimSpace(line))
	if len(matches) != 10 {
		return ProcessSample{}, false
	}

	name := strings.TrimSpace(matches[1])
	if name == "ALL_TASKS" || name == "DEAD_TASKS" {
		return ProcessSample{}, false
	}

	pid, err := strconv.Atoi(matches[2])
	if err != nil {
		return ProcessSample{}, false
	}

	cpuMs, ok := mustParseFloat(matches[3])
	if !ok {
		return ProcessSample{}, false
	}
	userPct, ok := mustParseFloat(matches[4])
	if !ok {
		return ProcessSample{}, false
	}
	energy, ok := mustParseFloat(matches[9])
	if !ok {
		return ProcessSample{}, false
	}
	deadlineLT2MS, ok := mustParseFloat(matches[5])
	if !ok {
		return ProcessSample{}, false
	}
	deadline2To5MS, ok := mustParseFloat(matches[6])
	if !ok {
		return ProcessSample{}, false
	}
	wakeupsIntr, ok := mustParseFloat(matches[7])
	if !ok {
		return ProcessSample{}, false
	}
	wakeupsPkgIdle, ok := mustParseFloat(matches[8])
	if !ok {
		return ProcessSample{}, false
	}

	return ProcessSample{
		Name:                 name,
		PID:                  pid,
		EnergyImpact:         &energy,
		CPUMsPerSec:          &cpuMs,
		UserPercent:          &userPct,
		DeadlineLT2MSPerSec:  &deadlineLT2MS,
		Deadline2To5MSPerSec: &deadline2To5MS,
		WakeupsIntrPerSec:    &wakeupsIntr,
		WakeupsPkgIdlePerSec: &wakeupsPkgIdle,
	}, true
}

func splitColumns(line string) []string {
	return columnSplitRE.Split(strings.TrimSpace(line), -1)
}

func parsePID(columns map[string]string) int {
	for header, value := range columns {
		lower := strings.ToLower(header)
		if lower == "id" || lower == "pid" {
			pid, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil {
				return pid
			}
		}
	}
	return 0
}

func parseFloatColumn(columns map[string]string, needle string) (float64, bool) {
	for header, value := range columns {
		if !strings.Contains(strings.ToLower(header), strings.ToLower(needle)) {
			continue
		}
		cleaned := strings.TrimSpace(strings.TrimSuffix(value, "%"))
		number, err := strconv.ParseFloat(cleaned, 64)
		if err == nil {
			return number, true
		}
	}
	return 0, false
}

func processSortValue(process ProcessSample) float64 {
	if process.EnergyImpact != nil {
		return *process.EnergyImpact
	}
	if process.CPUMsPerSec != nil {
		return *process.CPUMsPerSec
	}
	return 0
}

func mustParseFloat(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

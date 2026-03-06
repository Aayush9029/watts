package battery

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/Aayush9029/watts/internal/model"
	"howett.net/plist"
)

var battLineRE = regexp.MustCompile(`(?i)-InternalBattery-\d+.*?(\d+)%;\s*([^;]+);\s*([^;]+)`)

type Snapshot struct {
	Sample       model.BatterySample
	PMSetRaw     string
	IORegRawJSON string
}

func Collect(ctx context.Context) (Snapshot, error) {
	pmsetOut, err := exec.CommandContext(ctx, "pmset", "-g", "batt").CombinedOutput()
	if err != nil {
		return Snapshot{}, fmt.Errorf("pmset -g batt: %w", err)
	}

	ioregOut, err := exec.CommandContext(ctx, "ioreg", "-a", "-r", "-c", "AppleSmartBattery").Output()
	if err != nil {
		return Snapshot{}, fmt.Errorf("ioreg battery dump: %w", err)
	}

	props, rawJSON, err := parseIORegPlist(ioregOut)
	if err != nil {
		return Snapshot{}, fmt.Errorf("decode battery plist: %w", err)
	}

	sample, err := parse(pmsetOut, props)
	if err != nil {
		return Snapshot{}, err
	}

	return Snapshot{
		Sample:       sample,
		PMSetRaw:     strings.TrimSpace(string(pmsetOut)),
		IORegRawJSON: rawJSON,
	}, nil
}

func parse(pmsetOut []byte, props map[string]any) (model.BatterySample, error) {
	var sample model.BatterySample

	parsePMSet(&sample, string(pmsetOut))

	if v, ok := intFromAny(props["CycleCount"]); ok {
		sample.CycleCount = &v
	}
	if v, ok := intFromAny(props["Voltage"]); ok {
		sample.VoltageMV = &v
	}
	if v, ok := intFromAny(props["Amperage"]); ok {
		sample.AmperageMA = &v
	}
	if v, ok := intFromAny(props["AppleRawCurrentCapacity"]); ok {
		sample.CurrentCapacityMAh = &v
	}
	if v, ok := intFromAny(props["AppleRawMaxCapacity"]); ok {
		sample.MaxCapacityMAh = &v
	}
	if v, ok := intFromAny(props["DesignCapacity"]); ok {
		sample.DesignCapacityMAh = &v
	}
	if v, ok := boolFromAny(props["ExternalConnected"]); ok {
		sample.ExternalConnected = v
	}
	if v, ok := boolFromAny(props["IsCharging"]); ok {
		sample.IsCharging = v
	}
	if v, ok := floatFromAny(props["CurrentCapacity"]); ok && sample.Percentage == nil {
		sample.Percentage = &v
	}

	if adapterWatts, ok := extractAdapterWatts(props["AppleRawAdapterDetails"]); ok {
		sample.AdapterWatts = &adapterWatts
	}

	assignDerivedState(&sample)
	assignDerivedPower(&sample)

	return sample, nil
}

func parsePMSet(sample *model.BatterySample, raw string) {
	lines := strings.Split(raw, "\n")
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		switch {
		case strings.Contains(first, "AC Power"):
			sample.PowerSource = "ac"
		case strings.Contains(first, "Battery Power"):
			sample.PowerSource = "battery"
		}
	}

	for _, line := range lines {
		matches := battLineRE.FindStringSubmatch(line)
		if len(matches) != 4 {
			continue
		}
		if pct, err := strconv.ParseFloat(matches[1], 64); err == nil {
			sample.Percentage = &pct
		}
		state := strings.ToLower(strings.TrimSpace(matches[2]))
		switch {
		case strings.Contains(state, "discharging"):
			sample.State = "discharging"
		case strings.Contains(state, "charging"):
			sample.State = "charging"
			sample.IsCharging = true
		case strings.Contains(state, "charged"):
			sample.State = "charged"
			sample.IsCharging = true
		default:
			sample.State = state
		}

		remaining := strings.TrimSpace(matches[3])
		if minutes, ok := parseTimeRemainingMinutes(remaining); ok {
			sample.TimeRemainingMinutes = &minutes
		}
	}
}

func parseIORegPlist(data []byte) (map[string]any, string, error) {
	var root []map[string]any
	if _, err := plist.Unmarshal(data, &root); err != nil {
		return nil, "", err
	}
	if len(root) == 0 {
		return nil, "", fmt.Errorf("ioreg returned no battery rows")
	}
	rawJSON, err := json.Marshal(root[0])
	if err != nil {
		return nil, "", err
	}
	return root[0], string(rawJSON), nil
}

func extractAdapterWatts(value any) (float64, bool) {
	rows, ok := value.([]any)
	if !ok {
		return 0, false
	}
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		if watts, ok := floatFromAny(m["Watts"]); ok {
			return watts, true
		}
	}
	return 0, false
}

func assignDerivedState(sample *model.BatterySample) {
	if sample.State != "" {
		return
	}
	switch {
	case sample.IsCharging:
		sample.State = "charging"
	case sample.ExternalConnected:
		sample.State = "charged"
	default:
		sample.State = "discharging"
	}
}

func assignDerivedPower(sample *model.BatterySample) {
	if sample.VoltageMV == nil || sample.AmperageMA == nil {
		return
	}

	power := (float64(*sample.VoltageMV) * float64(*sample.AmperageMA)) / 1_000_000.0
	sample.BatteryPowerW = &power

	absPower := power
	if absPower < 0 {
		absPower *= -1
	}

	switch sample.State {
	case "charging", "charged":
		sample.ChargePowerW = &absPower
	case "discharging":
		sample.DischargePowerW = &absPower
	default:
		if power >= 0 {
			sample.ChargePowerW = &absPower
		} else {
			sample.DischargePowerW = &absPower
		}
	}
}

func parseTimeRemainingMinutes(raw string) (int, bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if idx := strings.IndexByte(raw, ' '); idx != -1 {
		raw = raw[:idx]
	}
	if raw == "" || raw == "no estimate" || raw == "finishing charge" {
		return 0, false
	}

	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, false
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, false
	}
	return (hours * 60) + minutes, true
}

func intFromAny(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int64:
		return int(v), true
	case uint64:
		return int(v), true
	case int:
		return v, true
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return int(i), true
		}
	case string:
		i, err := strconv.Atoi(v)
		if err == nil {
			return i, true
		}
	}
	return 0, false
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int64:
		return float64(v), true
	case uint64:
		return float64(v), true
	case int:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err == nil {
			return f, true
		}
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func boolFromAny(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(v) {
		case "yes", "true", "1":
			return true, true
		case "no", "false", "0":
			return false, true
		}
	}
	return false, false
}

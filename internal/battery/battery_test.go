package battery

import "testing"

func TestParseBatterySnapshot(t *testing.T) {
	pmset := []byte("Now drawing from 'Battery Power'\n -InternalBattery-0 (id=1234567)\t82%; discharging; 2:15 remaining present: true\n")
	props := map[string]any{
		"CycleCount":              120,
		"Voltage":                 12000,
		"Amperage":                -2500,
		"AppleRawCurrentCapacity": 4400,
		"AppleRawMaxCapacity":     5200,
		"DesignCapacity":          6200,
		"ExternalConnected":       false,
		"IsCharging":              false,
		"AppleRawAdapterDetails":  []any{map[string]any{"Watts": 35}},
	}

	sample, err := parse(pmset, props)
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}

	if sample.PowerSource != "battery" {
		t.Fatalf("PowerSource = %q, want battery", sample.PowerSource)
	}
	if sample.State != "discharging" {
		t.Fatalf("State = %q, want discharging", sample.State)
	}
	if sample.Percentage == nil || *sample.Percentage != 82 {
		t.Fatalf("Percentage = %v, want 82", sample.Percentage)
	}
	if sample.TimeRemainingMinutes == nil || *sample.TimeRemainingMinutes != 135 {
		t.Fatalf("TimeRemainingMinutes = %v, want 135", sample.TimeRemainingMinutes)
	}
	if sample.DischargePowerW == nil || *sample.DischargePowerW != 30 {
		t.Fatalf("DischargePowerW = %v, want 30", sample.DischargePowerW)
	}
}

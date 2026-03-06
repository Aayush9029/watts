package power

import "testing"

func TestParseText(t *testing.T) {
	raw := `
*** Sampled system activity (Thu Mar  5 18:18:03 2026 -0500) (1000.00ms elapsed) ***

*** Running tasks ***

Name                               ID     Energy Impact  CPU ms/s  User%
Safari                             123    23.5           65.0      82.0
kernel_task                        0      12.0           40.0      0.0
ALL_TASKS                          -2     35.5           105.0     45.0

**** Battery and backlight usage ****

Battery: percent_charge: 77
Backlight level: 60

**** Processor usage ****

CPU Power: 1500 mW
GPU Power: 200 mW
ANE Power: 0 mW
Combined Power (CPU + GPU + ANE): 1700 mW
`

	snapshot, err := ParseText(raw)
	if err != nil {
		t.Fatalf("ParseText() error = %v", err)
	}

	if snapshot.DurationMS == nil || *snapshot.DurationMS != 1000 {
		t.Fatalf("DurationMS = %v, want 1000", snapshot.DurationMS)
	}
	if len(snapshot.Processes) != 2 {
		t.Fatalf("len(Processes) = %d, want 2", len(snapshot.Processes))
	}
	if snapshot.Processes[0].Name != "Safari" {
		t.Fatalf("first process = %q, want Safari", snapshot.Processes[0].Name)
	}
	if snapshot.CPUPowerW == nil || *snapshot.CPUPowerW != 1.5 {
		t.Fatalf("CPUPowerW = %v, want 1.5", snapshot.CPUPowerW)
	}
	if snapshot.BatteryPercent == nil || *snapshot.BatteryPercent != 77 {
		t.Fatalf("BatteryPercent = %v, want 77", snapshot.BatteryPercent)
	}
}

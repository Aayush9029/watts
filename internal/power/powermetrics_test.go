package power

import "testing"

func TestParseText(t *testing.T) {
	raw := `
*** Sampled system activity (Thu Mar  5 18:18:03 2026 -0500) (1000.00ms elapsed) ***

*** Running tasks ***

Name                               ID     CPU ms/s  User%  Deadlines (<2 ms, 2-5 ms)  Wakeups (Intr, Pkg idle)  Energy Impact
Safari                             123    65.0      82.0   0.0     0.0                0.0     0.0               23.5
kernel_task                        0      40.0      0.0    1.0     0.5                10.0    1.0               12.0
ALL_TASKS                          -2     105.0     45.0   1.0     0.5                10.0    1.0               35.5

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
	if snapshot.Processes[0].PID != 123 {
		t.Fatalf("first pid = %d, want 123", snapshot.Processes[0].PID)
	}
	if snapshot.Processes[0].DeadlineLT2MSPerSec == nil || *snapshot.Processes[0].DeadlineLT2MSPerSec != 0 {
		t.Fatalf("deadline lt2 = %v, want 0", snapshot.Processes[0].DeadlineLT2MSPerSec)
	}
	if snapshot.Processes[0].WakeupsIntrPerSec == nil || *snapshot.Processes[0].WakeupsIntrPerSec != 0 {
		t.Fatalf("wakeups intr = %v, want 0", snapshot.Processes[0].WakeupsIntrPerSec)
	}
	if snapshot.CPUPowerW == nil || *snapshot.CPUPowerW != 1.5 {
		t.Fatalf("CPUPowerW = %v, want 1.5", snapshot.CPUPowerW)
	}
	if snapshot.BatteryPercent == nil || *snapshot.BatteryPercent != 77 {
		t.Fatalf("BatteryPercent = %v, want 77", snapshot.BatteryPercent)
	}
}

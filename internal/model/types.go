package model

import "time"

type Record struct {
	CollectedAt   time.Time
	PowerSource   string
	Battery       BatterySample
	System        SystemSample
	Processes     []ProcessRecord
	CollectorInfo CollectorInfo
}

type BatterySample struct {
	PowerSource          string
	Percentage           *float64
	State                string
	IsCharging           bool
	ExternalConnected    bool
	TimeRemainingMinutes *int
	CycleCount           *int
	VoltageMV            *int
	AmperageMA           *int
	AdapterWatts         *float64
	CurrentCapacityMAh   *int
	MaxCapacityMAh       *int
	DesignCapacityMAh    *int
	BatteryPowerW        *float64
	ChargePowerW         *float64
	DischargePowerW      *float64
	BrightnessPercent    *float64
}

type SystemSample struct {
	PowermetricsDurationMS *float64
	CPUPowerW              *float64
	GPUPowerW              *float64
	ANEPowerW              *float64
	CombinedPowerW         *float64
	BatteryPercent         *float64
	BrightnessPercent      *float64
	TemperatureC           *float64
	MaxTemperatureC        *float64
	TemperatureSensorCount *int
	FanCount               *int
	LeftFanRPM             *float64
	RightFanRPM            *float64
}

type ProcessRecord struct {
	Rank                 int
	PID                  int
	Name                 string
	ExecutablePath       string
	AppName              string
	BundlePath           string
	BundleID             string
	IsApp                bool
	EnergyImpact         *float64
	CPUMsPerSec          *float64
	UserPercent          *float64
	DeadlineLT2MSPerSec  *float64
	Deadline2To5MSPerSec *float64
	WakeupsIntrPerSec    *float64
	WakeupsPkgIdlePerSec *float64
	CPUPct               *float64
	MemoryPct            *float64
}

type CollectorInfo struct {
	Version      string
	TopProcesses int
}

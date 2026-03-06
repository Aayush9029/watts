package hwmon

type Snapshot struct {
	TemperatureC           *float64
	MaxTemperatureC        *float64
	TemperatureSensorCount *int
	FanCount               *int
	LeftFanRPM             *float64
	RightFanRPM            *float64
}

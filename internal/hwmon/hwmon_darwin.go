//go:build darwin

package hwmon

/*
#cgo LDFLAGS: -framework CoreFoundation -framework IOKit
#include <stdlib.h>

typedef struct {
	double avg_temp_c;
	double max_temp_c;
	int temp_sensor_count;
	int fan_count;
	double fan0_rpm;
	double fan1_rpm;
	char error_message[256];
} WattsHWMonSample;

WattsHWMonSample watts_hwmon_sample(void);
*/
import "C"

import (
	"fmt"
	"strings"
)

func Collect() (Snapshot, error) {
	raw := C.watts_hwmon_sample()

	snapshot := Snapshot{}
	if raw.avg_temp_c > 0 {
		value := float64(raw.avg_temp_c)
		snapshot.TemperatureC = &value
	}
	if raw.max_temp_c > 0 {
		value := float64(raw.max_temp_c)
		snapshot.MaxTemperatureC = &value
	}
	if raw.temp_sensor_count > 0 {
		value := int(raw.temp_sensor_count)
		snapshot.TemperatureSensorCount = &value
	}
	if raw.fan_count > 0 {
		value := int(raw.fan_count)
		snapshot.FanCount = &value
	}
	if raw.fan0_rpm > 0 {
		value := float64(raw.fan0_rpm)
		snapshot.LeftFanRPM = &value
	}
	if raw.fan1_rpm > 0 {
		value := float64(raw.fan1_rpm)
		snapshot.RightFanRPM = &value
	}

	msg := strings.TrimSpace(C.GoString(&raw.error_message[0]))
	if msg == "" {
		return snapshot, nil
	}
	if snapshot.FanCount == nil && snapshot.LeftFanRPM == nil && snapshot.RightFanRPM == nil {
		return snapshot, fmt.Errorf("%s", msg)
	}
	return snapshot, nil
}

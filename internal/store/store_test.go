package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Aayush9029/watts/internal/model"
)

func TestInsertSample(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "watts.sqlite")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer st.Close()

	pct := 82.0
	cpuPower := 2.2
	record := model.Record{
		CollectedAt: time.Date(2026, 3, 5, 23, 15, 0, 0, time.UTC),
		PowerSource: "battery",
		Battery: model.BatterySample{
			PowerSource: "battery",
			Percentage:  &pct,
			State:       "discharging",
		},
		System: model.SystemSample{
			CPUPowerW: &cpuPower,
		},
		Processes: []model.ProcessRecord{
			{Rank: 1, PID: 123, Name: "Safari"},
		},
		RawPayloads: []model.RawPayload{
			{Source: "pmset", Format: "text", Payload: "raw"},
		},
		CollectorInfo: model.CollectorInfo{Version: "test"},
	}

	sampleID, err := st.InsertSample(context.Background(), record)
	if err != nil {
		t.Fatalf("InsertSample() error = %v", err)
	}
	if sampleID == 0 {
		t.Fatalf("sampleID = %d, want non-zero", sampleID)
	}

	last, err := st.LastSampleTimestamp(context.Background())
	if err != nil {
		t.Fatalf("LastSampleTimestamp() error = %v", err)
	}
	if last == nil || !last.Equal(record.CollectedAt) {
		t.Fatalf("last timestamp = %v, want %v", last, record.CollectedAt)
	}
}

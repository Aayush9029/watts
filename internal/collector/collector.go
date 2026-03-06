package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/Aayush9029/watts/internal/appmeta"
	"github.com/Aayush9029/watts/internal/battery"
	"github.com/Aayush9029/watts/internal/config"
	"github.com/Aayush9029/watts/internal/model"
	"github.com/Aayush9029/watts/internal/power"
	"github.com/Aayush9029/watts/internal/store"
)

func CollectOnce(ctx context.Context, cfg config.Config, version string) (model.Record, error) {
	batterySnapshot, err := battery.Collect(ctx)
	if err != nil {
		return model.Record{}, err
	}

	powerSnapshot, err := power.Collect(ctx)
	if err != nil {
		return model.Record{}, err
	}

	processes, err := enrichProcesses(ctx, powerSnapshot.Processes, cfg.TopProcesses)
	if err != nil {
		return model.Record{}, err
	}

	record := model.Record{
		CollectedAt: time.Now().UTC(),
		PowerSource: batterySnapshot.Sample.PowerSource,
		Battery:     batterySnapshot.Sample,
		System: model.SystemSample{
			PowermetricsDurationMS: powerSnapshot.DurationMS,
			CPUPowerW:              powerSnapshot.CPUPowerW,
			GPUPowerW:              powerSnapshot.GPUPowerW,
			ANEPowerW:              powerSnapshot.ANEPowerW,
			CombinedPowerW:         powerSnapshot.CombinedPowerW,
			BatteryPercent:         powerSnapshot.BatteryPercent,
			BrightnessPercent:      coalesceFloat(batterySnapshot.Sample.BrightnessPercent, powerSnapshot.BrightnessPercent),
		},
		Processes: processes,
		RawPayloads: []model.RawPayload{
			{Source: "pmset", Format: "text", Payload: batterySnapshot.PMSetRaw},
			{Source: "ioreg", Format: "json", Payload: batterySnapshot.IORegRawJSON},
			{Source: "powermetrics", Format: "text", Payload: powerSnapshot.RawText},
		},
		CollectorInfo: model.CollectorInfo{
			Version:      version,
			TopProcesses: len(processes),
		},
	}
	return record, nil
}

func RunDaemon(ctx context.Context, cfg config.Config, version string) error {
	database, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer database.Close()

	interval, err := cfg.IntervalDuration()
	if err != nil {
		return err
	}

	if err := collectAndPersist(ctx, database, cfg, version); err != nil {
		log.Printf("initial sample failed: %v", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := collectAndPersist(ctx, database, cfg, version); err != nil {
				log.Printf("sample failed: %v", err)
			}
		}
	}
}

func collectAndPersist(ctx context.Context, database *store.Store, cfg config.Config, version string) error {
	record, err := CollectOnce(ctx, cfg, version)
	if err != nil {
		return err
	}

	sampleID, err := database.InsertSample(ctx, record)
	if err != nil {
		return err
	}

	log.Printf("stored sample id=%d battery=%s state=%s processes=%d", sampleID, formatBattery(record.Battery.Percentage), record.Battery.State, len(record.Processes))
	return nil
}

func enrichProcesses(ctx context.Context, input []power.ProcessSample, limit int) ([]model.ProcessRecord, error) {
	selected := make([]power.ProcessSample, len(input))
	copy(selected, input)
	sort.SliceStable(selected, func(i, j int) bool {
		return score(selected[i]) > score(selected[j])
	})
	if limit > 0 && len(selected) > limit {
		selected = selected[:limit]
	}

	pids := make([]int, 0, len(selected))
	for _, process := range selected {
		if process.PID > 0 {
			pids = append(pids, process.PID)
		}
	}

	metaByPID, err := appmeta.LookupMany(ctx, pids)
	if err != nil {
		return nil, err
	}

	records := make([]model.ProcessRecord, 0, len(selected))
	for idx, process := range selected {
		rawJSON, err := json.Marshal(process.RawColumns)
		if err != nil {
			return nil, fmt.Errorf("marshal process raw columns: %w", err)
		}

		record := model.ProcessRecord{
			Rank:           idx + 1,
			PID:            process.PID,
			Name:           process.Name,
			EnergyImpact:   process.EnergyImpact,
			CPUMsPerSec:    process.CPUMsPerSec,
			UserPercent:    process.UserPercent,
			RawColumnsJSON: string(rawJSON),
		}
		if meta, ok := metaByPID[process.PID]; ok {
			record.ExecutablePath = meta.ExecutablePath
			record.AppName = meta.AppName
			record.BundlePath = meta.BundlePath
			record.BundleID = meta.BundleID
			record.IsApp = meta.IsApp
			record.CPUPct = meta.CPUPct
			record.MemoryPct = meta.MemoryPct
		}
		records = append(records, record)
	}

	return records, nil
}

func score(process power.ProcessSample) float64 {
	if process.EnergyImpact != nil {
		return *process.EnergyImpact
	}
	if process.CPUMsPerSec != nil {
		return *process.CPUMsPerSec
	}
	return 0
}

func coalesceFloat(values ...*float64) *float64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func formatBattery(value *float64) string {
	if value == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", *value)
}

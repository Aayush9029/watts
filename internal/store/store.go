package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Aayush9029/watts/internal/model"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	st := &Store{db: db}
	if err := st.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o644); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return st, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init() error {
	stmts := []string{
		`PRAGMA busy_timeout = 5000;`,
		`PRAGMA journal_mode = DELETE;`,
		`CREATE TABLE IF NOT EXISTS samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			collected_at TEXT NOT NULL,
			unix_ts INTEGER NOT NULL,
			power_source TEXT,
			battery_percentage REAL,
			battery_state TEXT,
			is_charging INTEGER NOT NULL DEFAULT 0,
			external_connected INTEGER NOT NULL DEFAULT 0,
			time_remaining_minutes INTEGER,
			cycle_count INTEGER,
			voltage_mv INTEGER,
			amperage_ma INTEGER,
			adapter_watts REAL,
			current_capacity_mah INTEGER,
			max_capacity_mah INTEGER,
			design_capacity_mah INTEGER,
			battery_power_w REAL,
			charge_power_w REAL,
			discharge_power_w REAL,
			brightness_percent REAL,
			cpu_power_w REAL,
			gpu_power_w REAL,
			ane_power_w REAL,
			combined_power_w REAL,
			powermetrics_duration_ms REAL,
			top_process_count INTEGER NOT NULL DEFAULT 0,
			collector_version TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS sample_processes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sample_id INTEGER NOT NULL REFERENCES samples(id) ON DELETE CASCADE,
			rank INTEGER NOT NULL,
			pid INTEGER NOT NULL,
			name TEXT NOT NULL,
			executable_path TEXT,
			app_name TEXT,
			bundle_path TEXT,
			bundle_id TEXT,
			is_app INTEGER NOT NULL DEFAULT 0,
			energy_impact REAL,
			cpu_ms_per_s REAL,
			user_percent REAL,
			cpu_percent REAL,
			memory_percent REAL,
			raw_columns_json TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS sample_raw (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sample_id INTEGER NOT NULL REFERENCES samples(id) ON DELETE CASCADE,
			source TEXT NOT NULL,
			payload_format TEXT NOT NULL,
			payload TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_samples_unix_ts ON samples(unix_ts);`,
		`CREATE INDEX IF NOT EXISTS idx_sample_processes_sample_id ON sample_processes(sample_id);`,
		`INSERT INTO meta(key, value) VALUES('schema_version', '1') ON CONFLICT(key) DO NOTHING;`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) InsertSample(ctx context.Context, record model.Record) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	result, err := tx.ExecContext(ctx, `INSERT INTO samples (
		collected_at, unix_ts, power_source, battery_percentage, battery_state, is_charging,
		external_connected, time_remaining_minutes, cycle_count, voltage_mv, amperage_ma,
		adapter_watts, current_capacity_mah, max_capacity_mah, design_capacity_mah,
		battery_power_w, charge_power_w, discharge_power_w, brightness_percent,
		cpu_power_w, gpu_power_w, ane_power_w, combined_power_w, powermetrics_duration_ms,
		top_process_count, collector_version
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.CollectedAt.UTC().Format(time.RFC3339),
		record.CollectedAt.UTC().Unix(),
		record.PowerSource,
		dbFloat(record.Battery.Percentage),
		record.Battery.State,
		boolToInt(record.Battery.IsCharging),
		boolToInt(record.Battery.ExternalConnected),
		dbInt(record.Battery.TimeRemainingMinutes),
		dbInt(record.Battery.CycleCount),
		dbInt(record.Battery.VoltageMV),
		dbInt(record.Battery.AmperageMA),
		dbFloat(record.Battery.AdapterWatts),
		dbInt(record.Battery.CurrentCapacityMAh),
		dbInt(record.Battery.MaxCapacityMAh),
		dbInt(record.Battery.DesignCapacityMAh),
		dbFloat(record.Battery.BatteryPowerW),
		dbFloat(record.Battery.ChargePowerW),
		dbFloat(record.Battery.DischargePowerW),
		dbFloat(coalesceFloat(record.Battery.BrightnessPercent, record.System.BrightnessPercent)),
		dbFloat(record.System.CPUPowerW),
		dbFloat(record.System.GPUPowerW),
		dbFloat(record.System.ANEPowerW),
		dbFloat(record.System.CombinedPowerW),
		dbFloat(record.System.PowermetricsDurationMS),
		len(record.Processes),
		record.CollectorInfo.Version,
	)
	if err != nil {
		return 0, err
	}

	sampleID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	for _, process := range record.Processes {
		_, err = tx.ExecContext(ctx, `INSERT INTO sample_processes (
			sample_id, rank, pid, name, executable_path, app_name, bundle_path, bundle_id,
			is_app, energy_impact, cpu_ms_per_s, user_percent, cpu_percent, memory_percent,
			raw_columns_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sampleID,
			process.Rank,
			process.PID,
			process.Name,
			process.ExecutablePath,
			process.AppName,
			process.BundlePath,
			process.BundleID,
			boolToInt(process.IsApp),
			dbFloat(process.EnergyImpact),
			dbFloat(process.CPUMsPerSec),
			dbFloat(process.UserPercent),
			dbFloat(process.CPUPct),
			dbFloat(process.MemoryPct),
			process.RawColumnsJSON,
		)
		if err != nil {
			return 0, err
		}
	}

	for _, payload := range record.RawPayloads {
		_, err = tx.ExecContext(ctx, `INSERT INTO sample_raw (sample_id, source, payload_format, payload) VALUES (?, ?, ?, ?)`,
			sampleID, payload.Source, payload.Format, payload.Payload,
		)
		if err != nil {
			return 0, err
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return sampleID, nil
}

func (s *Store) LastSampleTimestamp(ctx context.Context) (*time.Time, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT collected_at FROM samples ORDER BY id DESC LIMIT 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, fmt.Errorf("parse last sample timestamp: %w", err)
	}
	return &ts, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
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

func dbInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func dbFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

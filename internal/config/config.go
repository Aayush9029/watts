package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

const (
	ConfigDirName  = ".config/watts"
	ConfigFileName = "config.json"
)

type Config struct {
	Username     string `json:"username"`
	HomeDir      string `json:"home_dir"`
	Interval     string `json:"interval"`
	DBPath       string `json:"db_path"`
	LogPath      string `json:"log_path"`
	ErrorLogPath string `json:"error_log_path"`
	TopProcesses int    `json:"top_processes"`
}

func DefaultForUser(u *user.User, interval time.Duration, topProcesses int) Config {
	baseDir := ConfigDir(u.HomeDir)
	return Config{
		Username:     u.Username,
		HomeDir:      u.HomeDir,
		Interval:     interval.String(),
		DBPath:       filepath.Join(baseDir, "data.sqlite"),
		LogPath:      filepath.Join(baseDir, "logs", "watts.log"),
		ErrorLogPath: filepath.Join(baseDir, "logs", "watts.err.log"),
		TopProcesses: topProcesses,
	}
}

func ConfigDir(homeDir string) string {
	return filepath.Join(homeDir, ConfigDirName)
}

func ConfigPath(homeDir string) string {
	return filepath.Join(ConfigDir(homeDir), ConfigFileName)
}

func (c Config) IntervalDuration() (time.Duration, error) {
	if c.Interval == "" {
		return 0, fmt.Errorf("config interval is empty")
	}
	return time.ParseDuration(c.Interval)
}

func (c Config) EnsureDirectories() error {
	paths := []string{
		filepath.Dir(c.DBPath),
		filepath.Dir(c.LogPath),
		filepath.Dir(c.ErrorLogPath),
	}
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

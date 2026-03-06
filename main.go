package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Aayush9029/watts/internal/collector"
	"github.com/Aayush9029/watts/internal/config"
	"github.com/Aayush9029/watts/internal/service"
	"github.com/Aayush9029/watts/internal/store"
)

var version = "dev"

const (
	defaultInterval = 30 * time.Second
	defaultTopN     = 10
)

func main() {
	log.SetFlags(log.LstdFlags)

	if len(os.Args) < 2 {
		showHelp()
		return
	}

	switch os.Args[1] {
	case "install":
		exitIfErr(runInstall(os.Args[2:]))
	case "uninstall":
		exitIfErr(runUninstall())
	case "start":
		exitIfErr(runStart())
	case "stop":
		exitIfErr(runStop())
	case "restart":
		exitIfErr(runRestart())
	case "status":
		exitIfErr(runStatus(os.Args[2:]))
	case "tail":
		exitIfErr(runTail(os.Args[2:]))
	case "once":
		exitIfErr(runOnce(os.Args[2:]))
	case "daemon":
		exitIfErr(runDaemon(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Printf("watts %s\n", version)
	case "help", "--help", "-h":
		showHelp()
	default:
		exitIfErr(fmt.Errorf("unknown command: %s", os.Args[1]))
	}
}

func runInstall(args []string) error {
	mustBeRoot()

	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	username := fs.String("user", installDefaultUsername(), "target macOS username")
	interval := fs.Duration("interval", defaultInterval, "sampling interval")
	topN := fs.Int("top", defaultTopN, "number of processes to retain")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" {
		return errors.New("install requires --user or SUDO_USER")
	}

	targetUser, err := user.Lookup(*username)
	if err != nil {
		return err
	}

	cfg := config.DefaultForUser(targetUser, *interval, *topN)
	if err := cfg.EnsureDirectories(); err != nil {
		return err
	}
	if err := config.Save(config.ConfigPath(targetUser.HomeDir), cfg); err != nil {
		return err
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}

	binaryPath, err := executablePath()
	if err != nil {
		return err
	}
	if err := service.Install(binaryPath, cfg); err != nil {
		return err
	}

	fmt.Printf("installed %s\n", service.Label)
	fmt.Printf("config: %s\n", config.ConfigPath(targetUser.HomeDir))
	fmt.Printf("db: %s\n", cfg.DBPath)
	return nil
}

func runUninstall() error {
	mustBeRoot()
	return service.Uninstall()
}

func runStart() error {
	mustBeRoot()
	return service.Start()
}

func runStop() error {
	mustBeRoot()
	return service.Stop()
}

func runRestart() error {
	mustBeRoot()
	return service.Restart()
}

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "config path")
	username := fs.String("user", "", "target macOS username")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, resolvedConfigPath, err := resolveConfig(*cfgPath, *username)
	if err != nil {
		return err
	}

	status, err := service.Status()
	if err != nil {
		return err
	}

	fmt.Printf("service: %s\n", service.Label)
	fmt.Printf("loaded: %s\n", status["loaded"])
	if value := status["state"]; value != "" {
		fmt.Printf("state: %s\n", value)
	}
	if value := status["pid"]; value != "" {
		fmt.Printf("pid: %s\n", value)
	}
	if value := status["last_exit_code"]; value != "" {
		fmt.Printf("last exit code: %s\n", value)
	}
	if value := status["error"]; value != "" {
		fmt.Printf("launchctl: %s\n", value)
	}
	fmt.Printf("config: %s\n", resolvedConfigPath)
	fmt.Printf("db: %s\n", cfg.DBPath)

	if _, err := os.Stat(cfg.DBPath); err == nil {
		db, err := store.Open(cfg.DBPath)
		if err == nil {
			defer db.Close()
			lastSample, lastErr := db.LastSampleTimestamp(context.Background())
			if lastErr == nil && lastSample != nil {
				fmt.Printf("last sample: %s\n", lastSample.Local().Format(time.RFC3339))
			}
		}
	}

	return nil
}

func runTail(args []string) error {
	fs := flag.NewFlagSet("tail", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "config path")
	username := fs.String("user", "", "target macOS username")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, _, err := resolveConfig(*cfgPath, *username)
	if err != nil {
		return err
	}

	cmd := exec.Command("tail", "-n", "50", "-f", cfg.LogPath, cfg.ErrorLogPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runOnce(args []string) error {
	mustBeRoot()

	fs := flag.NewFlagSet("once", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "config path")
	username := fs.String("user", "", "target macOS username")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, _, err := resolveConfig(*cfgPath, *username)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		targetUser, lookupErr := lookupDefaultUser(*username)
		if lookupErr != nil {
			return lookupErr
		}
		cfg = config.DefaultForUser(targetUser, defaultInterval, defaultTopN)
	}

	record, err := collector.CollectOnce(context.Background(), cfg, version)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func runDaemon(args []string) error {
	mustBeRoot()

	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	cfgPath := fs.String("config", "", "config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cfgPath == "" {
		return errors.New("daemon requires --config")
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return collector.RunDaemon(ctx, cfg, version)
}

func resolveConfig(explicitPath string, username string) (config.Config, string, error) {
	if explicitPath != "" {
		cfg, err := config.Load(explicitPath)
		return cfg, explicitPath, err
	}

	targetUser, err := lookupDefaultUser(username)
	if err != nil {
		return config.Config{}, "", err
	}

	cfgPath := config.ConfigPath(targetUser.HomeDir)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, "", err
	}
	return cfg, cfgPath, nil
}

func lookupDefaultUser(username string) (*user.User, error) {
	if username != "" {
		return user.Lookup(username)
	}
	if sudoUser := defaultTargetUsername(); sudoUser != "" {
		return user.Lookup(sudoUser)
	}
	return user.Current()
}

func executablePath() (string, error) {
	if len(os.Args) > 0 {
		if path, err := exec.LookPath(os.Args[0]); err == nil {
			return filepath.Abs(path)
		}
	}
	return os.Executable()
}

func defaultTargetUsername() string {
	if value := strings.TrimSpace(os.Getenv("SUDO_USER")); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("USER"))
}

func installDefaultUsername() string {
	if value := strings.TrimSpace(os.Getenv("SUDO_USER")); value != "" {
		return value
	}
	if current, err := user.Current(); err == nil && current.Uid != "0" {
		return current.Username
	}
	return ""
}

func mustBeRoot() {
	if os.Geteuid() != 0 {
		exitIfErr(errors.New("this command must be run as root"))
	}
}

func exitIfErr(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "watts:", err)
	os.Exit(1)
}

func showHelp() {
	fmt.Println("watts")
	fmt.Println("  background battery and power logger for macOS")
	fmt.Println()
	fmt.Println("USAGE")
	fmt.Println("  watts <command> [options]")
	fmt.Println()
	fmt.Println("COMMANDS")
	fmt.Println("  install     Install config and launchd daemon")
	fmt.Println("  uninstall   Remove launchd daemon")
	fmt.Println("  start       Start daemon")
	fmt.Println("  stop        Stop daemon")
	fmt.Println("  restart     Restart daemon")
	fmt.Println("  status      Show service and database status")
	fmt.Println("  tail        Tail collector logs")
	fmt.Println("  once        Run one foreground sample and print JSON")
	fmt.Println("  version     Show version")
}

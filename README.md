# watts

Figure out what is actually draining your MacBook battery.

## Installation

```bash
brew install aayush9029/tap/watts
```

## Usage

```bash
sudo watts install --user "$USER"     # install + start boot daemon
sudo watts status                     # service state + latest sample
sudo watts tail                       # follow collector logs
sudo watts once                       # run one foreground sample
sudo watts restart                    # reload daemon after updating the binary
sqlite3 ~/.config/watts/data.sqlite ".tables"
```

## Options

| Option | Description |
| --- | --- |
| `--user <name>` | Target macOS username for config and database paths |
| `--interval <duration>` | Sampling interval for `install` (default: `30s`) |
| `--top <n>` | Number of top processes to retain per sample (default: `10`) |
| `--config <path>` | Override config path for `status`, `tail`, `once`, or internal daemon runs |
| `-v`, `--version` | Show version |

## How It Works

1. `watts install` writes `~/.config/watts/config.json` for one user and installs a root `launchd` daemon.
2. The daemon samples `pmset`, `ioreg`, `powermetrics`, and hardware sensors for temperature plus fan RPM every 30 seconds.
3. Each sample is stored as structured SQLite rows for battery state, system power, thermals, fan speed, and the top 10 processes.
4. You can query the database directly later to chart trends and find the culprits.

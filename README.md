# watts

Background battery and power logger for macOS.

## Installation

```bash
brew install aayush9029/tap/watts

# or
brew tap aayush9029/tap
brew install watts
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
2. The daemon samples `pmset`, `ioreg`, and `powermetrics` every 30 seconds.
3. Each sample is normalized into SQLite tables for system power, battery state, and top process activity.
4. Raw command payloads are also stored so later analysis can re-parse or enrich old samples.

## Requirements

- macOS on Apple silicon
- Root privileges for `install`, `start`, `stop`, `restart`, `once`, and the daemon
- `powermetrics`, `pmset`, `ioreg`, `plutil`, and `sqlite3` available from macOS
- Local development: `go build -o watts`

## License

MIT

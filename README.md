<p align="center">
  <img src="assets/icon.png" width="128" alt="watts">
  <h1 align="center">watts</h1>
  <p align="center">Figure out what is actually draining your MacBook battery</p>
</p>

<p align="center">
  <a href="https://github.com/Aayush9029/watts/releases/latest"><img src="https://img.shields.io/github/v/release/Aayush9029/watts" alt="Release"></a>
  <a href="https://github.com/Aayush9029/watts/blob/main/LICENSE"><img src="https://img.shields.io/github/license/Aayush9029/watts" alt="License"></a>
</p>

## Install

```bash
brew install aayush9029/tap/watts
```

Or tap first:

```bash
brew tap aayush9029/tap
brew install watts
```

## Usage

```bash
sudo watts install --user "$USER"     # install + start boot daemon
sudo watts status                     # service state + latest sample
sudo watts tail                       # follow collector logs
sudo watts once                       # run one foreground sample
sudo watts restart                    # reload daemon after updating
```

## How it works

1. Installs a root `launchd` daemon that samples every 30 seconds
2. Collects data from `pmset`, `ioreg`, `powermetrics`, and hardware sensors
3. Stores structured SQLite rows for battery state, power, thermals, fan speed, and top processes
4. Query the database directly to chart trends and find culprits

## License

MIT

# Log Monitor

A terminal-based log monitoring application that allows you to tail and monitor log files across multiple remote servers via SSH connections.

## Features

- **Multi-server monitoring**: Connect to multiple remote servers simultaneously
- **Real-time log tailing**: Stream log files in real-time with configurable refresh intervals
- **Interactive TUI**: Clean terminal user interface built with [tview](https://github.com/rivo/tview)
- **Flexible authentication**: Support for SSH keys, passwords, and SSH agent authentication
- **File browsing**: Browse and select log files from remote directories
- **Fuzzy search**: Quickly find files with fuzzy search functionality
- **Configurable**: YAML-based configuration with server defaults and per-server overrides

## Prerequisites

- Go 1.25 or higher
- SSH access to remote servers you want to monitor

## Installation

### From Source

```bash
git clone <repository-url>
cd log-monitor
go build -o log-monitor
```

### Binary

If you have the compiled binary, simply run:

```bash
./log-monitor
```

## Configuration

Create a `config.yaml` file based on the provided example:

```bash
cp config.yaml.example config.yaml
```

Edit the configuration to match your servers:

```yaml
# Log Monitor Configuration
defaults:
  ssh_key: "~/.ssh/id_rsa"       # default SSH private key path
  ssh_port: 22                    # default SSH port
  tail_lines: 100                 # number of lines to show initially
  poll_interval: 5s               # how often to refresh file metadata

servers:
  - name: "Production Web 1"
    host: "192.168.1.10"
    port: 22
    user: "deploy"
    auth:
      method: "key"               # "key", "password", or "agent"
      key_path: "~/.ssh/prod_key"
    log_path: "/var/log/myapp"
    file_patterns:                # optional glob filters
      - "*.log"
      - "*.log.*"

  - name: "Staging DB"
    host: "10.0.0.50"
    user: "admin"
    auth:
      method: "agent"             # use SSH agent for authentication
    log_path: "/var/log/postgresql"
```

### Configuration Options

#### Defaults
- `ssh_key`: Default SSH private key path (supports `~` expansion)
- `ssh_port`: Default SSH port (22)
- `tail_lines`: Initial number of lines to show when tailing a file
- `poll_interval`: How often to refresh file metadata (e.g., "5s", "1m")

#### Per-Server Configuration
- `name`: Display name for the server
- `host`: Server hostname or IP address
- `port`: SSH port (overrides default)
- `user`: SSH username
- `auth`: Authentication configuration
  - `method`: "key", "password", or "agent"
  - `key_path`: Path to SSH private key (for "key" method)
- `log_path`: Directory containing log files on the remote server
- `file_patterns`: Optional list of glob patterns to filter files

## Usage

### Basic Usage

```bash
# Use default config file (config.yaml)
./log-monitor

# Specify custom config file
./log-monitor -config /path/to/config.yaml

# Enable debug logging
./log-monitor -debug debug.log
```

### Command Line Options

- `-config`: Path to configuration file (default: "config.yaml")
- `-debug`: Path to debug log file for troubleshooting

### Interface Navigation

The application provides an interactive terminal interface with three main panes:

1. **Server Pane** (left): List of configured servers
2. **File Pane** (middle): Files in the selected server's log directory
3. **Viewer Pane** (right): Content of the selected log file

#### Key Bindings

- `Tab` / `Shift+Tab`: Navigate between panes
- `↑` / `↓`: Navigate within lists
- `Enter`: Select server or file
- `/`: Open fuzzy search in file pane
- `Esc`: Clear search or go back
- `Ctrl+C`: Quit application

## Authentication Methods

### SSH Key Authentication

```yaml
auth:
  method: "key"
  key_path: "~/.ssh/id_rsa"  # Path to private key
```

### SSH Agent Authentication

```yaml
auth:
  method: "agent"  # Uses SSH agent
```

### Password Authentication

```yaml
auth:
  method: "password"  # Will prompt for password
```

## Building from Source

```bash
# Clone the repository
git clone <repository-url>
cd log-monitor

# Download dependencies
go mod tidy

# Build the application
go build -o log-monitor

# Run
./log-monitor
```

## Dependencies

- [tview](https://github.com/rivo/tview) - Terminal UI library
- [tcell](https://github.com/gdamore/tcell) - Terminal cell interface
- [golang.org/x/crypto](https://golang.org/x/crypto) - SSH client implementation
- [gopkg.in/yaml.v3](https://gopkg.in/yaml.v3) - YAML configuration parsing

## Troubleshooting

### Debug Logging

Enable debug logging to troubleshoot connection or file access issues:

```bash
./log-monitor -debug debug.log
```

The debug log will contain detailed information about:
- SSH connection attempts
- File operations
- UI events
- Error details

### Common Issues

1. **SSH Connection Failed**
   - Verify host, port, and username in configuration
   - Ensure SSH key has proper permissions (600)
   - Test SSH connection manually: `ssh user@host`

2. **Permission Denied**
   - Verify the user has read access to the log directory
   - Check if the log path exists on the remote server

3. **No Files Found**
   - Verify the `log_path` is correct
   - Check if `file_patterns` are too restrictive
   - Ensure the directory contains log files

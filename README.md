# Log Monitor

A terminal-based log monitoring application that allows you to tail and monitor log files across multiple remote servers via SSH connections.

## Features

- **Multi-server monitoring**: Connect to multiple remote servers via SSH
- **Real-time log tailing**: Stream log files in real-time with live spinner indicator
- **Multi-folder support**: Configure multiple log directories per server
- **Sudo support**: Read privileged log files with sudo (prompts for password)
- **Syntax colorization**: Auto-highlights log levels, timestamps, IPs, HTTP methods/status codes, and key=value pairs
- **Tail filtering**: Filter incoming log lines in real-time (`F7`)
- **File download**: Download remote log files to your local machine (`F5`)
- **Fuzzy search**: Type to filter server and file lists
- **Auto-selection**: CLI flags to jump directly to a server, folder, or file at startup
- **Interactive TUI**: Three-pane terminal interface built with [tview](https://github.com/rivo/tview)
- **Mouse support**: Click to focus panes, scroll to navigate

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

Pre-built binaries for Linux, macOS, and Windows are available via [GoReleaser](https://goreleaser.com/) releases.

## Configuration

Create a `config.yaml` file based on the provided example:

```bash
cp config.yaml.example config.yaml
```

Edit the configuration to match your servers:

```yaml
defaults:
  ssh_key: "~/.ssh/id_rsa"       # default SSH private key path
  ssh_port: 22                    # default SSH port
  tail_lines: 100                 # number of lines to show initially
  poll_interval: 5s               # how often to refresh file metadata

servers:
  # Single log directory with file pattern filtering
  - name: "Production Web 1"
    host: "192.168.1.10"
    port: 22
    user: "deploy"
    auth:
      method: "key"
      key_path: "~/.ssh/prod_key"
    log_path: "/var/log/myapp"
    file_patterns:
      - "*.log"
      - "*.log.*"

  # Sudo access for privileged log files
  - name: "Staging DB"
    host: "10.0.0.50"
    user: "admin"
    auth:
      method: "agent"
    log_path: "/var/log/postgresql"
    sudo: true                    # prompts for password at connect time

  # Multiple log directories on a single server
  - name: "Web Server"
    host: "10.0.0.60"
    user: "deploy"
    log_folders:
      - name: "nginx"
        path: "/var/log/nginx"
        file_patterns:
          - "*.log"
      - name: "laravel"
        path: "/var/log/laravel"
      - name: "mysql"
        path: "/var/log/mysql"
        file_patterns:
          - "*.log"
          - "*.err"
```

### Configuration Options

#### Defaults

| Field | Description | Default |
|-------|-------------|---------|
| `ssh_key` | Default SSH private key path (supports `~`) | `~/.ssh/id_rsa` |
| `ssh_port` | Default SSH port | `22` |
| `tail_lines` | Number of lines to load initially when tailing | `100` |
| `poll_interval` | How often to refresh file metadata | `5s` |

#### Per-Server Configuration

| Field | Description | Required |
|-------|-------------|----------|
| `name` | Display name (defaults to `user@host` if omitted) | No |
| `host` | Server hostname or IP address | Yes |
| `port` | SSH port (overrides default) | No |
| `user` | SSH username | Yes |
| `auth.method` | `"key"`, `"agent"`, or `"password"` | No (auto-detects) |
| `auth.key_path` | Path to SSH private key | No |
| `sudo` | Use sudo for file operations | No |
| `log_path` | Single log directory path | * |
| `file_patterns` | Glob patterns to filter files (with `log_path`) | No |
| `log_folders` | Multiple log directories (see below) | * |

\* Either `log_path` or `log_folders` is required, but not both.

#### Log Folders

When a server has multiple log directories, use `log_folders` instead of `log_path`:

| Field | Description | Required |
|-------|-------------|----------|
| `name` | Display name for the folder | Yes |
| `path` | Absolute path on the remote server | Yes |
| `file_patterns` | Glob patterns to filter files in this folder | No |

If no `auth.method` is specified, authentication defaults to `key` if `ssh_key` is set, otherwise `agent`.

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

### Auto-Selection

Use CLI flags to skip manual navigation at startup. All flags are optional and can be combined — you only need to specify the level of auto-navigation you want:

```bash
# Auto-select a server, then manually pick a folder/file
./log-monitor -server myhost

# Auto-select server and folder, then manually pick a file
./log-monitor -server myhost -folder /var/log

# Auto-select server and file (works for single-folder servers)
./log-monitor -server myhost -file app.log

# Full auto-navigation: jump straight to tailing a file
./log-monitor -server myhost -folder /var/log -file app.log
```

### Command Line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-config` | Path to configuration file | `config.yaml` |
| `-debug` | Path to debug log file | (disabled) |
| `-server` | Auto-select server by name | (none) |
| `-folder` | Auto-select folder by path (requires `-server`) | (none) |
| `-file` | Auto-select file by name (requires `-server`) | (none) |

### Interface

The application has a three-pane layout with a status bar:

```
[Server Pane (30 cols)] [File Pane (1x flex)] [Viewer Pane (2x flex)]
[Status Bar                                                         ]
```

- **Server Pane** (left): List of configured servers. Type to fuzzy-filter.
- **File Pane** (middle): Folders or files on the selected server. Type to fuzzy-filter.
- **Viewer Pane** (right): Log content with live tail and syntax colorization.
- **Status Bar** (bottom): Context info and keybinding hints.

### Key Bindings

#### Global

| Key | Action |
|-----|--------|
| `q` / `Ctrl-C` | Quit |
| `Tab` | Focus next pane |
| `Shift-Tab` | Focus previous pane |
| `1` / `2` / `3` | Jump to pane by number |
| `Esc` | Clear filter, stop tail, or go back |

#### Server and File Panes

| Key | Action |
|-----|--------|
| Type any letter | Fuzzy-filter the list |
| `Backspace` | Delete last filter character |
| `Enter` | Select item |
| `r` | Refresh file list |

#### Viewer Pane

| Key | Action |
|-----|--------|
| `g` / `Home` | Jump to top |
| `G` / `End` | Jump to bottom |
| `F5` | Download current file |
| `F7` | Set tail filter (grep-like) |
| `r` | Refresh file list |
| `Esc` | Stop tail |

## Authentication Methods

### SSH Key (default)

```yaml
auth:
  method: "key"
  key_path: "~/.ssh/id_rsa"
```

### SSH Agent

```yaml
auth:
  method: "agent"
```

### Password

```yaml
auth:
  method: "password"  # will prompt for password
```

## Dependencies

- [tview](https://github.com/rivo/tview) - Terminal UI library
- [tcell](https://github.com/gdamore/tcell) - Terminal cell interface
- [golang.org/x/crypto](https://golang.org/x/crypto) - SSH client implementation
- [gopkg.in/yaml.v3](https://gopkg.in/yaml.v3) - YAML configuration parsing

## Troubleshooting

Enable debug logging to troubleshoot connection or file access issues:

```bash
./log-monitor -debug debug.log
```

The debug log captures SSH connections, file operations, UI events, and error details.

### Common Issues

1. **SSH Connection Failed**
   - Verify host, port, and username in configuration
   - Ensure SSH key has proper permissions (`chmod 600`)
   - Test SSH connection manually: `ssh user@host`

2. **Permission Denied**
   - Verify the user has read access to the log directory
   - Enable `sudo: true` in the server config for privileged files

3. **No Files Found**
   - Verify `log_path` or `log_folders` paths are correct
   - Check if `file_patterns` are too restrictive
   - Ensure the directory contains log files

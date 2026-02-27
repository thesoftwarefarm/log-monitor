# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o log-monitor .        # build binary
./log-monitor                     # run with default config.yaml
./log-monitor -config path.yaml   # run with custom config
```

There are no tests yet. No linter is configured.

## Architecture

Go TUI application that monitors log files on remote servers via SSH. Uses `tview` for the terminal UI and `golang.org/x/crypto/ssh` for connections.

### Layout

Three-pane layout with a status bar:
```
[ServerPane (30 cols)] [FilePane (1x flex)] [ViewerPane (2x flex)]
[StatusBar (2 rows)                                               ]
```

- **ServerPane** (`internal/ui/server_pane.go`): `tview.List` of servers from config. Selecting a server triggers SSH connection and file listing.
- **FilePane** (`internal/ui/file_pane.go`): `tview.Table` showing files (name, size, mod time) in the server's log directory. Selecting a file loads and tails it.
- **ViewerPane** (`internal/ui/viewer_pane.go`): `tview.TextView` displaying log content. Live-tails via `io.Writer` interface—the SSH tailer writes directly into it.
- **StatusBar** (`internal/ui/status_bar.go`): Shows keybinding hints or transient error messages.

### Data Flow

1. `main.go` loads YAML config → creates `ui.App`
2. User selects server → `App.onServerSelected` → `ssh.Pool.GetClient` → `ssh.ListFiles` → populates FilePane
3. User selects file → `App.onFileSelected` → `ssh.ReadFileContent` (initial load) → `ssh.StartTail` (live streaming)
4. Tail output streams into `ViewerPane.Writer()` via `io.Copy` in a goroutine

### SSH Layer (`internal/ssh/`)

- **Pool** (`client.go`): Connection cache keyed by `user@host:port`. Validates connections with keepalive before reuse. Supports key and agent auth (password auth is stubbed out).
- **FileOps** (`fileops.go`): Remote file operations via shell commands (`ls`, `tail`, `stat`). Uses `shellescape` for safe command construction.
- **Tailer** (`tailer.go`): Runs `tail -n N -f` over SSH. Cancellable via context; sends SIGTERM to remote process on stop.

### Concurrency

All SSH operations run in goroutines. UI updates from background goroutines must use `tviewApp.QueueUpdateDraw()`. The `App.mu` mutex protects `currentServer`, `currentFile`, and `tailer` state.

### Keybindings (`internal/ui/keybindings.go`)

Global: `q`/Ctrl-C quit, Tab/Shift-Tab cycle panes, Esc stops tail. When viewer is focused: `1`/`2`/`3` jump to pane, `r` refreshes file list.

## Config

YAML config in `config.yaml` (see `config.yaml.example`). Defines `defaults` (ssh_key, ssh_port, tail_lines, poll_interval) and a `servers` list. Server-level settings override defaults. Auth methods: `key`, `agent`, `password` (password not implemented).

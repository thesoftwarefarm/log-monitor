# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o log-monitor .        # build binary
./log-monitor                     # run with default config.yaml
./log-monitor -config path.yaml   # run with custom config
./log-monitor -debug debug.log    # run with debug logging to file
```

There are no tests yet. No linter is configured. Releases are handled via GoReleaser (`.goreleaser.yaml`).

## Architecture

Go TUI application (Go 1.25+) that monitors log files on remote servers via SSH. Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm-architecture TUI framework), [lipgloss](https://github.com/charmbracelet/lipgloss) for styling, and `golang.org/x/crypto/ssh` for connections.

### Layout

Three-pane layout with a status bar:
```
[ServerPane (30 cols)] [FilePane (1x flex)] [ViewerPane (2x flex)]
[StatusBar (1 row)                                                ]
```

- **ServerPane** (`internal/ui/server_pane.go`): Filterable server list from config. Selecting a server triggers SSH connection and file listing.
- **FilePane** (`internal/ui/file_pane.go`): Shows folders (when multi-folder) or files (name, size, mod time) in the server's log directory. Selecting a file loads and tails it.
- **ViewerPane** (`internal/ui/viewer_pane.go`): Log content display using `bubbles/viewport` with syntax colorization and line numbers. Centered message display for binary files.
- **StatusBar** (`internal/ui/status_bar.go`): Color-coded keybinding hints (keys in blue, descriptions in gray) and transient context/error messages.

### Key Files

- `internal/ui/model.go` — Main Bubble Tea model, `Update` loop, all event handling, modal rendering
- `internal/ui/commands.go` — Async `tea.Cmd` functions for SSH operations (connect, list, read, tail, download)
- `internal/ui/messages.go` — Message types (`tea.Msg`) for async command results
- `internal/ui/styles.go` — All lipgloss styles (colors, borders, modals, status bar)
- `internal/ui/keybindings.go` — Key bindings and pane-specific shortcut hint strings

### Data Flow

1. `main.go` loads YAML config → `ui.Run()` creates `tea.Program`
2. User selects server → `Model.onServerSelected` → `connectAndListCmd` (async) → `ssh.Pool.GetClient` → `ssh.ListFiles` → `FilesLoadedMsg` → populates FilePane
3. User selects file → `Model.onFileSelected` → dispatches `countAndReadFileCmd` + `startTailCmd` in parallel via `tea.Batch`
4. Tail data flows through a `chan []byte` → `TailDataMsg` → `ViewerPane.AppendTailData`

### SSH Layer (`internal/ssh/`)

- **Pool** (`client.go`): Connection cache keyed by `user@host:port`. Validates connections with keepalive before reuse. Stores sudo passwords per server. Supports key and agent auth.
- **FileOps** (`fileops.go`): Remote file operations via shell commands (`ls`, `tail`, `stat`, `wc`). Uses `shellescape` for safe command construction. `CountAndReadFileContent` combines line counting and reading into a single command for sudo performance.
- **Tailer** (`tailer.go`): Runs `tail -n N -f` over SSH. Cancellable via context; sends SIGTERM to remote process on stop.

### Concurrency

All SSH operations run as `tea.Cmd` functions (goroutines managed by Bubble Tea). The Elm architecture ensures only one `Update` runs at a time — no mutex needed for model state. Background results arrive as messages (`tea.Msg`) processed sequentially.

When sudo is required, `countAndReadFileCmd` and `startTailCmd` run in parallel (`tea.Batch`) to minimize the number of sequential sudo authentication rounds.

### Mouse Support

Enabled via `tea.WithMouseCellMotion()` (mode 1002). Supports:
- Click to focus panes
- Click to select items in server/file lists
- Double-click to select (equivalent to Enter)
- Scroll wheel in all panes
- Shift+click/drag for native text selection (terminal-level bypass)

Hit-testing uses stored `serverPaneWidth` and `filePaneWidth`. Double-click detected via 400ms timestamp threshold.

### Modals

Three modal types: sudo password, tail filter, file download. Rendered as overlay on top of background using `runeIndex()` for ANSI-aware splicing. Features double border, drop shadow (`▒`), pill-style buttons, and centered positioning.

### Binary File Protection

Two-layer defense against terminal corruption from binary files:
1. Extension check (`isBinaryExtension`) blocks `.gz`, `.zip`, `.tar`, etc. from being tailed — shows centered warning box in viewer
2. `sanitizeLine()` strips control characters (except tab) from every line before colorization

### Log Colorization (`internal/ui/colorize.go`)

Regex-based ANSI syntax highlighting applied per-line. Rules colorize log levels (ERROR=red, WARN=yellow, INFO=green, DEBUG=gray), timestamps, IP addresses, HTTP methods/status codes, quoted strings, and key=value pairs.

### Other Modules

- **Fuzzy search** (`internal/ui/fuzzy.go`): Case-insensitive subsequence matching for filtering server/file lists. Type characters to filter, Backspace to remove.
- **Logger** (`internal/logger/logger.go`): Optional debug logger writing to file. Guarded by mutex, safe from any goroutine. Uses elapsed-time format with component tags (e.g. `[ssh]`, `[app]`).

### Keybindings (`internal/ui/keybindings.go`)

Global: Ctrl-C quit, Tab/Shift-Tab cycle panes, Esc clears filter or stops tail. File/server panes: type to filter, Enter to select, PgUp/PgDn to page. Viewer: g/G top/bottom, F5 download, F7 filter, r refresh.

## Config

YAML config in `config.yaml` (see `config.yaml.example`). Defines `defaults` (ssh_key, ssh_port, tail_lines) and a `servers` list. Server-level settings override defaults. Auth methods: `key`, `agent`, `password` (password not implemented). If no auth method is specified, defaults to `key` if ssh_key is set, otherwise `agent`.

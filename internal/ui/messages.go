package ui

import (
	"log-monitor/internal/config"
	"log-monitor/internal/ssh"
)

// ConnectedMsg signals a successful SSH connection.
type ConnectedMsg struct {
	Server config.ServerConfig
}

// ConnectErrorMsg signals a failed SSH connection.
type ConnectErrorMsg struct {
	Err    error
	Server config.ServerConfig
}

// FilesLoadedMsg carries the file listing result.
type FilesLoadedMsg struct {
	Files     []ssh.FileInfo
	Dir       string
	ShowUpDir bool
}

// FilesErrorMsg signals a file listing failure.
type FilesErrorMsg struct {
	Err error
}

// SudoRetryMsg signals that sudo auth failed and we should re-prompt.
type SudoRetryMsg struct {
	Server config.ServerConfig
}

// FileContentMsg carries the initial file content.
type FileContentMsg struct {
	Content   string
	StartLine int
}

// FileReadErrorMsg signals a file read failure.
type FileReadErrorMsg struct {
	Err error
}

// TailStartedMsg signals that tailing has begun.
type TailStartedMsg struct {
	Tailer *ssh.Tailer
	Cancel func()
}

// TailDataMsg carries a chunk of tail data.
type TailDataMsg struct {
	Data []byte
}

// TailErrorMsg signals a tail error (disconnect).
type TailErrorMsg struct {
	Err error
}

// TailStoppedMsg signals the tail channel was closed.
type TailStoppedMsg struct{}

// DownloadProgressMsg carries download progress information.
type DownloadProgressMsg struct {
	BytesDownloaded int64
	TotalBytes      int64
}

// DownloadDoneMsg signals a successful download.
type DownloadDoneMsg struct {
	Filename string
	Path     string
	Size     int64
}

// DownloadErrorMsg signals a download failure.
type DownloadErrorMsg struct {
	Err       error
	Cancelled bool
}

// StatusMsg is a generic status update for the status bar.
type StatusMsg struct {
	Context string
	Error   string
}

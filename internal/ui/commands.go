package ui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"log-monitor/internal/config"
	"log-monitor/internal/logger"
	"log-monitor/internal/ssh"

	tea "github.com/charmbracelet/bubbletea"
)

// connectAndListCmd connects to a server and lists files in a folder.
func connectAndListCmd(pool *ssh.Pool, srv config.ServerConfig, folder config.LogFolder) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		logger.Log("cmd", "connecting to %s...", srv.Name)
		client, err := pool.GetClient(ctx, srv)
		if err != nil {
			if strings.Contains(err.Error(), "sudo authentication failed") {
				pool.ClearSudoPassword(srv)
				return SudoRetryMsg{Server: srv}
			}
			return ConnectErrorMsg{Err: err, Server: srv}
		}

		opts := ssh.CommandOpts{}
		if srv.Sudo {
			opts.SudoPassword = pool.GetSudoPassword(srv)
		}

		files, err := ssh.ListFiles(client, folder.Path, folder.FilePatterns, opts)
		if err != nil {
			if strings.Contains(err.Error(), "sudo authentication failed") {
				pool.ClearSudoPassword(srv)
				return SudoRetryMsg{Server: srv}
			}
			return FilesErrorMsg{Err: err}
		}

		showUpDir := len(srv.LogFolders) > 1
		return FilesLoadedMsg{Files: files, Dir: folder.Path, ShowUpDir: showUpDir}
	}
}

// countAndReadFileCmd reads the last N lines and counts total lines in a single command.
func countAndReadFileCmd(pool *ssh.Pool, srv config.ServerConfig, fullPath string, tailLines int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		client, err := pool.GetClient(ctx, srv)
		if err != nil {
			return FileReadErrorMsg{Err: err}
		}

		opts := ssh.CommandOpts{}
		if srv.Sudo {
			opts.SudoPassword = pool.GetSudoPassword(srv)
		}

		totalLines, content, err := ssh.CountAndReadFileContent(client, fullPath, tailLines, opts)
		if err != nil {
			return FileReadErrorMsg{Err: err}
		}

		startLine := 1
		if totalLines > tailLines {
			startLine = totalLines - tailLines + 1
		}

		return FileContentMsg{Content: content, StartLine: startLine}
	}
}

// startTailCmd starts tailing and sends data through a channel.
func startTailCmd(pool *ssh.Pool, srv config.ServerConfig, fullPath string, ch chan<- []byte) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		client, err := pool.GetClient(ctx, srv)
		if err != nil {
			return TailErrorMsg{Err: err}
		}

		opts := ssh.CommandOpts{}
		if srv.Sudo {
			opts.SudoPassword = pool.GetSudoPassword(srv)
		}

		// Create a writer that buffers complete lines and sends them to the channel
		w := &chanWriter{ch: ch}

		tailCtx, tailCancel := context.WithCancel(context.Background())
		tailer, err := ssh.StartTail(tailCtx, client, fullPath, 0, w, opts)
		if err != nil {
			tailCancel()
			return TailErrorMsg{Err: err}
		}

		tailer.SetErrCallback(func(err error) {
			// Send the error as a special message through the channel
			// Close the channel to signal TailStoppedMsg
			close(ch)
		})

		return TailStartedMsg{Tailer: tailer, Cancel: tailCancel}
	}
}

// waitForTailData waits for the next chunk of tail data from the channel.
func waitForTailData(ch <-chan []byte) tea.Cmd {
	return func() tea.Msg {
		data, ok := <-ch
		if !ok {
			return TailStoppedMsg{}
		}
		return TailDataMsg{Data: data}
	}
}

// downloadFileCmd downloads a remote file with progress reporting and cancellation support.
func downloadFileCmd(pool *ssh.Pool, srv config.ServerConfig, remotePath, localDir, localFilename string, dlCtx context.Context, progressCh chan<- int64) tea.Cmd {
	return func() tea.Msg {
		localPath := filepath.Join(localDir, localFilename)

		connCtx, connCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer connCancel()

		client, err := pool.GetClient(connCtx, srv)
		if err != nil {
			return DownloadErrorMsg{Err: fmt.Errorf("download connect: %v", err)}
		}

		opts := ssh.CommandOpts{}
		if srv.Sudo {
			opts.SudoPassword = pool.GetSudoPassword(srv)
		}

		if err := ssh.DownloadFile(client, remotePath, localPath, opts, dlCtx, progressCh); err != nil {
			if dlCtx.Err() != nil {
				return DownloadErrorMsg{Err: fmt.Errorf("download cancelled"), Cancelled: true}
			}
			return DownloadErrorMsg{Err: fmt.Errorf("download: %v", err)}
		}

		var size int64
		if info, err := os.Stat(localPath); err == nil {
			size = info.Size()
		}

		return DownloadDoneMsg{Filename: localFilename, Path: localPath, Size: size}
	}
}

// waitForDownloadProgress reads progress updates from the channel and returns them as messages.
func waitForDownloadProgress(ch <-chan int64, totalSize int64) tea.Cmd {
	return func() tea.Msg {
		bytes, ok := <-ch
		if !ok {
			return nil
		}
		return DownloadProgressMsg{BytesDownloaded: bytes, TotalBytes: totalSize}
	}
}

// chanWriter is an io.Writer that sends complete lines to a channel.
type chanWriter struct {
	ch     chan<- []byte
	buf    bytes.Buffer
	closed bool
}

func (w *chanWriter) Write(p []byte) (int, error) {
	if w.closed {
		return len(p), nil
	}

	w.buf.Write(p)
	data := w.buf.String()

	lastNL := strings.LastIndex(data, "\n")
	if lastNL == -1 {
		return len(p), nil
	}

	complete := data[:lastNL+1]
	remainder := data[lastNL+1:]

	w.buf.Reset()
	w.buf.WriteString(remainder)

	// Send the complete lines — recover from panic if channel was closed
	defer func() {
		if r := recover(); r != nil {
			w.closed = true
		}
	}()
	w.ch <- []byte(complete)

	return len(p), nil
}

package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"log-monitor/internal/config"
	"log-monitor/internal/logger"
	"log-monitor/internal/ssh"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// App is the main application struct tying together all UI panes, SSH pool, and config.
type App struct {
	tviewApp *tview.Application
	config   *config.Config
	pool     *ssh.Pool

	pages *tview.Pages

	serverPane *ServerPane
	filePane   *FilePane
	viewerPane *ViewerPane
	statusBar  *StatusBar

	// Current state
	mu            sync.Mutex
	currentServer *config.ServerConfig
	currentFolder *config.LogFolder
	currentFile   *ssh.FileInfo
	tailer        *ssh.Tailer
	tailCtx       context.Context
	tailCancel    context.CancelFunc
	connectCancel context.CancelFunc // cancels in-progress SSH connection

	// Focus tracking
	panes      []tview.Primitive
	focusIndex int

	// Set to 1 after app.Stop() — background goroutines must not call
	// QueueUpdateDraw after this point (it blocks forever).
	stopped atomic.Int32
}

// NewApp creates and wires the full TUI application.
func NewApp(cfg *config.Config) *App {
	tviewApp := tview.NewApplication()

	a := &App{
		tviewApp: tviewApp,
		config:   cfg,
		pool:     ssh.NewPool(),
	}

	// Create panes
	a.serverPane = NewServerPane(cfg.Servers)
	a.filePane = NewFilePane()
	a.viewerPane = NewViewerPane(tviewApp)
	a.statusBar = NewStatusBar()

	// Pane list for focus cycling
	a.panes = []tview.Primitive{
		a.serverPane.List(),
		a.filePane.Table(),
		a.viewerPane.TextView(),
	}

	// Wire callbacks
	a.serverPane.SetSelectedFunc(a.onServerSelected)
	a.filePane.SetSelectedFunc(a.onFileSelected)
	a.filePane.SetFolderSelectedFunc(a.onFolderSelected)
	a.filePane.SetUpDirFunc(a.onUpDir)
	a.viewerPane.SetSearchStatusFunc(func(msg string) {
		a.statusBar.SetContext(" " + msg)
	})

	// Build layout:
	// Row: [ServerPane(30 cols) | FilePane(1x) | ViewerPane(2x)]
	// Below: StatusBar (1 row)
	panes := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(a.serverPane.Widget(), 30, 0, true).
		AddItem(a.filePane.Widget(), 0, 1, false).
		AddItem(a.viewerPane.Widget(), 0, 2, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(panes, 0, 1, true).
		AddItem(a.statusBar.Widget(), 2, 0, false)

	a.pages = tview.NewPages()
	a.pages.AddPage("main", layout, true, true)
	tviewApp.SetRoot(a.pages, true)

	// Setup keybindings
	SetupKeybindings(tviewApp, a)

	// Enable mouse support so clicks and scrolling work.
	tviewApp.EnableMouse(true)

	// Mouse click to focus pane
	tviewApp.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		if action == tview.MouseLeftDown {
			x, y := event.Position()
			for i, pane := range a.panes {
				px, py, pw, ph := pane.GetRect()
				if x >= px && x < px+pw && y >= py && y < py+ph {
					if a.focusIndex != i {
						a.focusIndex = i
						tviewApp.SetFocus(pane)
						a.updateShortcutsForFocus()
					}
					break
				}
			}
		}
		return event, action
	})

	// Set initial border colors (first pane is focused).
	a.updatePaneBorders()

	return a
}

// queueUpdate sends a UI update to the tview event loop. Unlike
// QueueUpdateDraw this never blocks: it fires and forgets so background
// goroutines can't deadlock when the event loop has already stopped.
func (a *App) queueUpdate(f func()) {
	if a.stopped.Load() != 0 {
		logger.Log("app", "queueUpdate: SKIPPED (app stopped)")
		return
	}
	logger.Log("app", "queueUpdate: launching goroutine for QueueUpdateDraw")
	go func() {
		logger.Log("app", "queueUpdate: goroutine calling QueueUpdateDraw...")
		a.tviewApp.QueueUpdateDraw(f)
		logger.Log("app", "queueUpdate: QueueUpdateDraw returned")
	}()
}

// Run starts the TUI event loop.
func (a *App) Run() error {
	defer a.shutdown()
	return a.tviewApp.Run()
}

// CycleFocus moves focus to the next or previous pane.
func (a *App) CycleFocus(direction int) {
	a.focusIndex = (a.focusIndex + direction + len(a.panes)) % len(a.panes)
	a.tviewApp.SetFocus(a.panes[a.focusIndex])
	a.updateShortcutsForFocus()
}

// FocusPane sets focus to a specific pane by index.
func (a *App) FocusPane(idx int) {
	if idx >= 0 && idx < len(a.panes) {
		a.focusIndex = idx
		a.tviewApp.SetFocus(a.panes[idx])
		a.updateShortcutsForFocus()
	}
}

// updateShortcutsForFocus updates the status bar shortcuts based on the focused pane.
func (a *App) updateShortcutsForFocus() {
	switch {
	case a.focusIndex == 2:
		a.statusBar.SetShortcuts(ShortcutsViewerPane)
	case a.focusIndex == 1 && a.filePane.IsInFolderMode():
		a.statusBar.SetShortcuts(ShortcutsFolderPane)
	default:
		a.statusBar.SetShortcuts(ShortcutsListPane)
	}
	a.updatePaneBorders()
}

// updatePaneBorders highlights the focused pane's border in light green and resets others to white.
func (a *App) updatePaneBorders() {
	boxes := []*tview.Box{
		a.serverPane.List().Box,
		a.filePane.Table().Box,
		a.viewerPane.TextView().Box,
	}
	for i, box := range boxes {
		if i == a.focusIndex {
			box.SetBorderColor(tcell.ColorLightGreen)
			box.SetTitleColor(tcell.ColorLightGreen)
		} else {
			box.SetBorderColor(tcell.ColorWhite)
			box.SetTitleColor(tcell.ColorWhite)
		}
	}
}

// HasModalOpen returns true if a modal overlay (e.g. sudo password prompt) is visible.
func (a *App) HasModalOpen() bool {
	name, _ := a.pages.GetFrontPage()
	return name != "main"
}

// promptSudoPassword shows a modal password prompt. The callback receives the
// entered password, or "" if the user cancelled.
func (a *App) promptSudoPassword(serverName string, callback func(string)) {
	form := tview.NewForm()
	form.AddPasswordField("Password:", "", 0, '*', nil)
	form.AddButton("OK", func() {
		pw := form.GetFormItemByLabel("Password:").(*tview.InputField).GetText()
		a.pages.RemovePage("sudo-prompt")
		a.tviewApp.SetFocus(a.panes[a.focusIndex])
		callback(pw)
	})
	form.AddButton("Cancel", func() {
		a.pages.RemovePage("sudo-prompt")
		a.tviewApp.SetFocus(a.panes[a.focusIndex])
		callback("")
	})
	form.SetCancelFunc(func() {
		a.pages.RemovePage("sudo-prompt")
		a.tviewApp.SetFocus(a.panes[a.focusIndex])
		callback("")
	})
	form.SetBorder(true)
	form.SetTitle(fmt.Sprintf(" Sudo password for %s ", serverName))
	form.SetTitleAlign(tview.AlignCenter)

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(form, 50, 0, true).
			AddItem(nil, 0, 1, false),
			7, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.AddPage("sudo-prompt", modal, true, true)
	a.tviewApp.SetFocus(form)
}

// FocusedOnViewer returns true if the viewer pane currently has focus.
func (a *App) FocusedOnViewer() bool {
	return a.focusIndex == 2
}

// ClearFocusedFilter clears the filter on the currently focused pane, if any.
// Returns true if a filter was active and cleared.
func (a *App) ClearFocusedFilter() bool {
	switch a.focusIndex {
	case 0:
		if a.serverPane.HasActiveFilter() {
			a.serverPane.ClearFilter()
			return true
		}
	case 1:
		if a.filePane.HasActiveFilter() {
			a.filePane.ClearFilter()
			return true
		}
	}
	return false
}

// StopTail stops the current tail operation.
func (a *App) StopTail() {
	a.mu.Lock()
	srv := a.currentServer
	a.stopTailLocked()
	a.mu.Unlock()
	a.viewerPane.StopSpinner()
	a.viewerPane.ResetTitle()
	if srv != nil {
		a.statusBar.SetContext(fmt.Sprintf("[yellow]Tail stopped[-] — %s", srv.Name))
	} else {
		a.statusBar.Reset()
	}
}

// RefreshFiles reloads the file list for the current server and folder.
func (a *App) RefreshFiles() {
	a.mu.Lock()
	srv := a.currentServer
	folder := a.currentFolder
	a.mu.Unlock()

	if srv == nil || folder == nil {
		return
	}
	srvCopy := *srv
	folderCopy := *folder
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	go func() {
		defer cancel()
		a.loadFilesForFolder(ctx, srvCopy, folderCopy)
	}()
}

func (a *App) onServerSelected(idx int, srv config.ServerConfig) {
	logger.Log("app", "onServerSelected: %s (idx=%d)", srv.Name, idx)
	a.mu.Lock()
	a.stopTailLocked()
	a.cancelConnectLocked()
	a.currentServer = &srv
	a.currentFolder = nil
	a.currentFile = nil
	a.mu.Unlock()

	logger.Log("app", "clearing panes")
	a.viewerPane.Clear()
	a.filePane.Clear()
	a.serverPane.MarkSelected(idx)

	folders := srv.EffectiveFolders()

	if len(folders) > 1 {
		// Multi-folder server: show folder list
		a.filePane.SetFolders(folders)
		a.FocusPane(1)
		a.statusBar.SetContext(fmt.Sprintf("[green]%s[-] — select a folder", srv.Name))
		return
	}

	// Single folder: auto-select and connect (backward-compatible)
	folder := folders[0]
	a.mu.Lock()
	a.currentFolder = &folder
	a.mu.Unlock()

	if srv.Sudo && a.pool.GetSudoPassword(srv) == "" {
		a.promptSudoPassword(srv.Name, func(pw string) {
			if pw == "" {
				a.statusBar.SetContext("[yellow]Sudo password cancelled[-]")
				a.FocusPane(0)
				return
			}
			a.pool.SetSudoPassword(srv, pw)
			a.startConnection(srv)
		})
		return
	}

	a.startConnection(srv)
}

func (a *App) onFolderSelected(idx int, folder config.LogFolder) {
	logger.Log("app", "onFolderSelected: %s (idx=%d)", folder.Path, idx)
	a.mu.Lock()
	a.stopTailLocked()
	srv := a.currentServer
	if srv == nil {
		a.mu.Unlock()
		return
	}
	a.currentFolder = &folder
	a.currentFile = nil
	srvCopy := *srv
	a.mu.Unlock()

	a.viewerPane.Clear()

	if srvCopy.Sudo && a.pool.GetSudoPassword(srvCopy) == "" {
		a.promptSudoPassword(srvCopy.Name, func(pw string) {
			if pw == "" {
				a.statusBar.SetContext("[yellow]Sudo password cancelled[-]")
				a.FocusPane(0)
				return
			}
			a.pool.SetSudoPassword(srvCopy, pw)
			a.startConnection(srvCopy)
		})
		return
	}

	a.startConnection(srvCopy)
}

func (a *App) onUpDir() {
	logger.Log("app", "onUpDir: returning to folder list")
	a.mu.Lock()
	a.stopTailLocked()
	srv := a.currentServer
	a.currentFolder = nil
	a.currentFile = nil
	a.mu.Unlock()

	a.viewerPane.Clear()

	if srv != nil {
		folders := srv.EffectiveFolders()
		a.filePane.SetFolders(folders)
		a.statusBar.SetContext(fmt.Sprintf("[green]%s[-] — select a folder", srv.Name))
	}
	a.updateShortcutsForFocus()
}

func (a *App) startConnection(srv config.ServerConfig) {
	a.mu.Lock()
	folder := a.currentFolder
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	a.connectCancel = cancel
	a.mu.Unlock()

	if folder == nil {
		return
	}

	a.FocusPane(1)
	a.statusBar.SetContext(fmt.Sprintf("[yellow]Connecting to[-] %s...", srv.Name))
	logger.Log("app", "startConnection: launching loadFilesForFolder goroutine")

	go a.loadFilesForFolder(ctx, srv, *folder)
}

func (a *App) cancelConnectLocked() {
	if a.connectCancel != nil {
		a.connectCancel()
		a.connectCancel = nil
	}
}

func (a *App) loadFilesForFolder(ctx context.Context, srv config.ServerConfig, folder config.LogFolder) {
	logger.Log("app", "loadFilesForFolder: GetClient for %s (folder=%s)...", srv.Name, folder.Path)
	client, err := a.pool.GetClient(ctx, srv)
	if err != nil {
		logger.Log("app", "loadFilesForFolder: GetClient failed: %v (ctx.Err=%v)", err, ctx.Err())
		if ctx.Err() != nil {
			return
		}
		errMsg := fmt.Sprintf("connect %s: %v", srv.Host, err)
		a.queueUpdate(func() {
			a.statusBar.SetError(errMsg)
			a.filePane.SetMessage("[red]Unable to connect[-]")
			a.viewerPane.SetMessage("[red]Unable to connect[-]\n\n[white]" + errMsg + "[-]")
			a.FocusPane(0)
		})
		logger.Log("app", "loadFilesForFolder: queued error update (goroutine launched)")
		return
	}
	logger.Log("app", "loadFilesForFolder: GetClient succeeded, listing files")

	opts := ssh.CommandOpts{}
	if srv.Sudo {
		opts.SudoPassword = a.pool.GetSudoPassword(srv)
	}

	files, err := ssh.ListFiles(client, folder.Path, folder.FilePatterns, opts)
	if err != nil {
		logger.Log("app", "loadFilesForFolder: ListFiles failed: %v", err)
		if strings.Contains(err.Error(), "sudo authentication failed") {
			a.pool.ClearSudoPassword(srv)
			a.queueUpdate(func() {
				a.statusBar.SetError("Sudo authentication failed — try again")
				a.promptSudoPassword(srv.Name, func(pw string) {
					if pw == "" {
						a.statusBar.SetContext("[yellow]Sudo password cancelled[-]")
						a.FocusPane(0)
						return
					}
					a.pool.SetSudoPassword(srv, pw)
					a.startConnection(srv)
				})
			})
			return
		}
		listErr := fmt.Sprintf("list files: %v", err)
		a.queueUpdate(func() {
			a.statusBar.SetError(listErr)
			a.filePane.SetMessage("[red]Unable to list files[-]")
			a.viewerPane.SetMessage("[red]Unable to list files[-]\n\n[white]" + listErr + "[-]")
			a.FocusPane(0)
		})
		return
	}

	// Determine if this server has multiple folders (show "/ .." row)
	showUpDir := len(srv.EffectiveFolders()) > 1

	logger.Log("app", "loadFilesForFolder: got %d files, queuing UI update", len(files))
	a.queueUpdate(func() {
		a.filePane.SetFiles(folder.Path, files, showUpDir)
		a.statusBar.SetContext(fmt.Sprintf("[green]Connected to[-] %s", srv.Name))
	})
}

func (a *App) onFileSelected(idx int, file ssh.FileInfo) {
	a.mu.Lock()
	srv := a.currentServer
	folder := a.currentFolder
	if srv == nil || folder == nil {
		a.mu.Unlock()
		return
	}
	a.stopTailLocked()
	a.currentFile = &file
	srvCopy := *srv
	folderPath := folder.Path
	a.mu.Unlock()

	fullPath := filepath.Join(folderPath, file.Name)

	a.filePane.MarkSelected(idx)
	a.statusBar.SetContext(fmt.Sprintf("[green]%s[-] %s", srvCopy.Name, fullPath))

	// Already on the main goroutine (SetSelectedFunc callback), so update
	// the UI directly. QueueUpdateDraw would deadlock here because it blocks
	// waiting for the main event loop to process the update.
	a.viewerPane.Clear()

	go a.loadAndTailFile(srvCopy, file, fullPath)
}

func (a *App) loadAndTailFile(srv config.ServerConfig, file ssh.FileInfo, fullPath string) {
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer connectCancel()
	client, err := a.pool.GetClient(connectCtx, srv)
	if err != nil {
		a.queueUpdate(func() {
			a.statusBar.SetError(fmt.Sprintf("connect: %v", err))
		})
		return
	}

	opts := ssh.CommandOpts{}
	if srv.Sudo {
		opts.SudoPassword = a.pool.GetSudoPassword(srv)
	}

	// Read initial content
	content, err := ssh.ReadFileContent(client, fullPath, a.config.Defaults.TailLines, opts)
	if err != nil {
		a.queueUpdate(func() {
			a.statusBar.SetError(fmt.Sprintf("read: %v", err))
		})
		return
	}

	a.queueUpdate(func() {
		a.viewerPane.SetText(content)
	})

	// Start tailing
	ctx, cancel := context.WithCancel(context.Background())
	tailer, err := ssh.StartTail(ctx, client, fullPath, 0, a.viewerPane.Writer(), opts)
	if err != nil {
		cancel()
		a.queueUpdate(func() {
			a.statusBar.SetError(fmt.Sprintf("tail: %v", err))
		})
		return
	}

	tailer.SetErrCallback(func(err error) {
		a.queueUpdate(func() {
			a.viewerPane.StopSpinner()
			a.viewerPane.SetTitle(" Disconnected ")
			a.statusBar.SetError(fmt.Sprintf("connection lost: %v", err))
		})
	})

	a.mu.Lock()
	a.tailer = tailer
	a.tailCtx = ctx
	a.tailCancel = cancel
	a.mu.Unlock()

	a.queueUpdate(func() {
		a.statusBar.SetContext(fmt.Sprintf("[green]Tailing[-] %s:%s", srv.Name, fullPath))
		a.viewerPane.StartSpinner(fmt.Sprintf("Tailing: %s", file.Name))
	})
}

func (a *App) stopTailLocked() {
	if a.tailer != nil {
		// Cancel context and let the tailer goroutine clean up asynchronously.
		// We must NOT call tailer.Stop() here (which blocks on <-done) because
		// this may be called from the main tview goroutine, and the tailer's
		// io.Copy goroutine may be blocked on QueueUpdateDraw — deadlock.
		a.tailCancel()
		a.tailer = nil
		a.tailCancel = nil
		a.tailCtx = nil
	}
}

func (a *App) shutdown() {
	logger.Log("app", "shutdown: start")
	a.stopped.Store(1)

	a.mu.Lock()
	a.cancelConnectLocked()
	if a.tailer != nil {
		a.tailCancel()
		tailer := a.tailer
		a.tailer = nil
		a.tailCancel = nil
		a.tailCtx = nil
		a.mu.Unlock()

		// Wait for the tailer to finish with a timeout so the app never hangs on exit.
		done := make(chan struct{})
		go func() {
			tailer.Stop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	} else {
		a.mu.Unlock()
	}
	a.pool.CloseAll()
}

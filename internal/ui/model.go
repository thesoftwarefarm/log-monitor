package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"log-monitor/internal/config"
	"log-monitor/internal/logger"
	"log-monitor/internal/ssh"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pane int

const (
	paneServer pane = iota
	paneFile
	paneViewer
)

type modalType int

const (
	modalNone modalType = iota
	modalSudo
	modalFilter
	modalDownload
)

// AutoSelect holds CLI flags for automatic selection at startup.
type AutoSelect struct {
	Server string
	Folder string
	File   string
}

// Model is the top-level Bubble Tea model.
type Model struct {
	cfg        *config.Config
	pool       *ssh.Pool
	autoSelect AutoSelect
	width      int
	height     int

	// Sub-models
	serverPane ServerPaneModel
	filePane   FilePaneModel
	viewerPane ViewerPaneModel

	// State
	focused       pane
	currentServer *config.ServerConfig
	currentFolder *config.LogFolder
	currentFile   *ssh.FileInfo
	tailer        *ssh.Tailer
	tailCancel    func()
	tailChan      chan []byte
	tailing       bool

	// Modal state
	modal       modalType
	modalInput  textinput.Model
	modalInput2 textinput.Model // second field for download
	modalFocus  int             // which field focused in multi-field modals
	sudoServer  *config.ServerConfig // server awaiting sudo password

	// Pane widths for mouse hit-testing
	serverPaneWidth int
	filePaneWidth   int

	// Double-click tracking
	lastClickTime time.Time
	lastClickY    int
	lastClickPane pane

	// Status bar
	contextMsg string
	errorMsg   string

	// Last non-filter context message, restored when filter is cleared
	lastContext string

	// Auto-select callback
	onFilesLoaded func(*Model) tea.Cmd

	// Spinner tick state
	spinnerTicking bool
}

// NewModel creates the initial model.
func NewModel(cfg *config.Config, autoSelect AutoSelect) Model {
	return Model{
		cfg:        cfg,
		pool:       ssh.NewPool(),
		autoSelect: autoSelect,
		serverPane: NewServerPaneModel(cfg.Servers),
		filePane:   NewFilePaneModel(),
		viewerPane: NewViewerPaneModel(),
		focused:    paneServer,
	}
}

// spinnerTickMsg is a periodic tick for the spinner animation.
type spinnerTickMsg struct{}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	setTerminalTitle("Log Monitor")

	var cmds []tea.Cmd

	if m.autoSelect.Server != "" {
		cmds = append(cmds, func() tea.Msg {
			// Trigger auto-start after first render
			return autoStartMsg{}
		})
	}

	return tea.Batch(cmds...)
}

type autoStartMsg struct{}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcSizes()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case autoStartMsg:
		return m.autoStart()

	case spinnerTickMsg:
		if m.viewerPane.IsSpinning() {
			m.viewerPane.TickSpinner()
			return m, spinnerTickCmd()
		}
		m.spinnerTicking = false
		return m, nil

	case ConnectedMsg:
		// Not used directly — connectAndListCmd combines connect+list
		return m, nil

	case ConnectErrorMsg:
		errDetail := fmt.Sprintf("connect %s: %v", msg.Server.Host, msg.Err)
		m.filePane.SetMessage("Unable to connect\n\n" + errDetail)
		m.focused = paneServer
		return m, nil

	case SudoRetryMsg:
		m.errorMsg = "Sudo authentication failed — try again"
		m = m.showSudoPrompt(msg.Server)
		return m, nil

	case FilesLoadedMsg:
		m.filePane.SetFiles(msg.Dir, msg.Files, msg.ShowUpDir)
		m.errorMsg = ""
		m.setContext(fmt.Sprintf("\033[32m%s\033[0m — Select a file", m.currentServer.Name))
		// Fire auto-select callback if set
		if m.onFilesLoaded != nil {
			cb := m.onFilesLoaded
			m.onFilesLoaded = nil
			cmd := cb(&m)
			return m, cmd
		}
		return m, nil

	case FilesErrorMsg:
		errDetail := fmt.Sprintf("list files: %v", msg.Err)
		m.filePane.SetMessage("Unable to list files\n\n" + errDetail)
		m.focused = paneServer
		return m, nil

	case FileContentMsg:
		m.viewerPane.SetText(msg.Content, msg.StartLine)
		// Tailing is already started in parallel from onFileSelected
		return m, nil

	case FileReadErrorMsg:
		m.errorMsg = fmt.Sprintf("read: %v", msg.Err)
		return m, nil

	case TailStartedMsg:
		m.tailer = msg.Tailer
		m.tailCancel = msg.Cancel
		m.tailing = true
		if m.currentServer != nil && m.currentFile != nil && m.currentFolder != nil {
			fullPath := filepath.Join(m.currentFolder.Path, m.currentFile.Name)
			m.setContext(fmt.Sprintf("\033[38;2;3;175;255mTailing\033[0m %s:%s", m.currentServer.Name, fullPath))
			m.viewerPane.StartSpinner(fmt.Sprintf("Tailing: %s", m.currentFile.Name))
			var cmds []tea.Cmd
			cmds = append(cmds, waitForTailData(m.tailChan))
			if !m.spinnerTicking {
				m.spinnerTicking = true
				cmds = append(cmds, spinnerTickCmd())
			}
			return m, tea.Batch(cmds...)
		}
		return m, waitForTailData(m.tailChan)

	case TailDataMsg:
		m.viewerPane.AppendTailData(msg.Data)
		return m, waitForTailData(m.tailChan)

	case TailErrorMsg:
		m.errorMsg = fmt.Sprintf("tail: %v", msg.Err)
		m.viewerPane.StopSpinner()
		m.viewerPane.SetTitle(" Disconnected ")
		m.tailing = false
		return m, nil

	case TailStoppedMsg:
		if m.tailing {
			m.viewerPane.StopSpinner()
			m.viewerPane.SetTitle(" Disconnected ")
			m.errorMsg = "connection lost"
			m.tailing = false
		}
		return m, nil

	case DownloadDoneMsg:
		sizeStr := ""
		if msg.Size > 0 {
			sizeStr = fmt.Sprintf(" (%s)", ssh.FormatSize(msg.Size))
		}
		m.setContext(fmt.Sprintf("\033[32mDownloaded\033[0m %s%s → %s", msg.Filename, sizeStr, msg.Path))
		return m, nil

	case DownloadErrorMsg:
		m.errorMsg = msg.Err.Error()
		return m, nil

	case StatusMsg:
		if msg.Context != "" {
			m.setContext(msg.Context)
		}
		if msg.Error != "" {
			m.errorMsg = msg.Error
		}
		return m, nil

	case autoFileSelectMsg:
		return m.onFileSelected(msg.idx, msg.file)
	}

	return m, nil
}

func (m *Model) recalcSizes() {
	// Server pane: fixed 30 cols
	// File pane: 1x flex
	// Viewer pane: 2x flex
	// Status bar: 1 row

	statusHeight := 1
	paneHeight := m.height - statusHeight
	if paneHeight < 3 {
		paneHeight = 3
	}

	serverWidth := 30
	remaining := m.width - serverWidth
	if remaining < 20 {
		remaining = 20
	}
	fileWidth := remaining / 3
	viewerWidth := remaining - fileWidth

	m.serverPaneWidth = serverWidth
	m.filePaneWidth = fileWidth

	m.serverPane.SetSize(serverWidth, paneHeight)
	m.filePane.SetSize(fileWidth, paneHeight)
	m.viewerPane.SetSize(viewerWidth, paneHeight)
}

func (m *Model) setContext(msg string) {
	m.lastContext = msg
	m.contextMsg = msg
	m.errorMsg = ""
}

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Render three panes
	serverView := m.serverPane.View(m.focused == paneServer)
	fileView := m.filePane.View(m.focused == paneFile)
	viewerView := m.viewerPane.View(m.focused == paneViewer)

	// Join panes horizontally
	panes := lipgloss.JoinHorizontal(lipgloss.Top, serverView, fileView, viewerView)

	// Status bar
	shortcuts := m.currentShortcuts()
	statusBar := renderStatusBar(m.width, m.contextMsg, m.errorMsg, shortcuts)

	// Join vertically
	result := lipgloss.JoinVertical(lipgloss.Left, panes, statusBar)

	// Modal overlay
	if m.modal != modalNone {
		result = m.renderModal(result)
	}

	return result
}

func (m *Model) currentShortcuts() string {
	switch m.focused {
	case paneServer:
		return shortcutsListPane
	case paneFile:
		if m.filePane.IsInFolderMode() {
			return shortcutsFolderPane
		}
		return shortcutsFilePane
	case paneViewer:
		return shortcutsViewerPane
	}
	return shortcutsListPane
}

// handleKey processes keyboard events.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Modal input handling
	if m.modal != modalNone {
		return m.handleModalKey(msg)
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "tab":
		m.focused = pane((int(m.focused) + 1) % 3)
		return m, nil

	case "shift+tab":
		m.focused = pane((int(m.focused) + 2) % 3)
		return m, nil

	case "esc":
		switch m.focused {
		case paneServer:
			if m.serverPane.HasActiveFilter() {
				m.serverPane.ClearFilter()
				m.contextMsg = m.lastContext
				return m, nil
			}
		case paneFile:
			if m.filePane.HasActiveFilter() {
				m.filePane.ClearFilter()
				m.contextMsg = m.lastContext
				return m, nil
			}
		}
		// Stop tail
		return m.stopTail(), nil

	case "f5":
		return m.showDownloadDialog()

	case "f7":
		return m.showFilterPrompt(), nil

	case "enter":
		return m.handleEnter()

	case "up":
		return m.handleUp(), nil

	case "down":
		return m.handleDown(), nil

	case "home":
		if m.focused == paneViewer {
			m.viewerPane.GotoTop()
		}
		return m, nil

	case "end":
		if m.focused == paneViewer {
			m.viewerPane.GotoBottom()
		}
		return m, nil

	case "pgup":
		if m.focused == paneViewer {
			m.viewerPane.ScrollUp(m.viewerPane.viewport.Height)
		}
		return m, nil

	case "pgdown":
		if m.focused == paneViewer {
			m.viewerPane.ScrollDown(m.viewerPane.viewport.Height)
		}
		return m, nil

	default:
		// Check for single character keys
		keyStr := msg.String()
		if len(keyStr) == 1 {
			r := rune(keyStr[0])
			return m.handleRune(r)
		}
		if keyStr == "backspace" {
			return m.handleBackspace(), nil
		}
	}

	return m, nil
}

// handleMouse processes mouse events.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Ignore mouse when modal is open
	if m.modal != modalNone {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonLeft:
		if msg.Action == tea.MouseActionPress {
			// Determine which pane was clicked
			var clickedPane pane
			if msg.X < m.serverPaneWidth {
				clickedPane = paneServer
			} else if msg.X < m.serverPaneWidth+m.filePaneWidth {
				clickedPane = paneFile
			} else {
				clickedPane = paneViewer
			}

			// Double-click detection
			now := time.Now()
			isDoubleClick := clickedPane == m.lastClickPane &&
				msg.Y == m.lastClickY &&
				now.Sub(m.lastClickTime) < 400*time.Millisecond

			m.lastClickTime = now
			m.lastClickY = msg.Y
			m.lastClickPane = clickedPane

			m.focused = clickedPane

			// Move cursor to clicked row in server/file panes
			switch clickedPane {
			case paneServer:
				m.serverPane.SetCursorFromY(msg.Y)
				if isDoubleClick {
					return m.handleEnter()
				}
			case paneFile:
				m.filePane.SetCursorFromY(msg.Y)
				if isDoubleClick {
					return m.handleEnter()
				}
			}
		}

	case tea.MouseButtonWheelUp:
		if msg.X < m.serverPaneWidth {
			m.serverPane.MoveUp()
		} else if msg.X < m.serverPaneWidth+m.filePaneWidth {
			m.filePane.MoveUp()
		} else {
			m.viewerPane.ScrollUp(3)
		}

	case tea.MouseButtonWheelDown:
		if msg.X < m.serverPaneWidth {
			m.serverPane.MoveDown()
		} else if msg.X < m.serverPaneWidth+m.filePaneWidth {
			m.filePane.MoveDown()
		} else {
			m.viewerPane.ScrollDown(3)
		}
	}

	return m, nil
}

func (m Model) handleRune(r rune) (tea.Model, tea.Cmd) {
	switch m.focused {
	case paneServer:
		m.serverPane.HandleRune(r)
		if m.serverPane.HasActiveFilter() {
			m.contextMsg = fmt.Sprintf("\033[33mFilter:\033[0m %s", m.serverPane.FilterQuery())
		}
		return m, nil

	case paneFile:
		m.filePane.HandleRune(r)
		if m.filePane.HasActiveFilter() {
			m.contextMsg = fmt.Sprintf("\033[33mFilter:\033[0m %s", m.filePane.FilterQuery())
		}
		return m, nil

	case paneViewer:
		switch r {
		case 'g':
			m.viewerPane.GotoTop()
		case 'G':
			m.viewerPane.GotoBottom()
		case 'r':
			return m.refreshFiles()
		}
	}
	return m, nil
}

func (m Model) handleBackspace() Model {
	switch m.focused {
	case paneServer:
		m.serverPane.HandleBackspace()
		if !m.serverPane.HasActiveFilter() {
			m.contextMsg = m.lastContext
		} else {
			m.contextMsg = fmt.Sprintf("\033[33mFilter:\033[0m %s", m.serverPane.FilterQuery())
		}
	case paneFile:
		m.filePane.HandleBackspace()
		if !m.filePane.HasActiveFilter() {
			m.contextMsg = m.lastContext
		} else {
			m.contextMsg = fmt.Sprintf("\033[33mFilter:\033[0m %s", m.filePane.FilterQuery())
		}
	}
	return m
}

func (m Model) handleUp() Model {
	switch m.focused {
	case paneServer:
		m.serverPane.MoveUp()
	case paneFile:
		m.filePane.MoveUp()
	case paneViewer:
		m.viewerPane.ScrollUp(1)
	}
	return m
}

func (m Model) handleDown() Model {
	switch m.focused {
	case paneServer:
		m.serverPane.MoveDown()
	case paneFile:
		m.filePane.MoveDown()
	case paneViewer:
		m.viewerPane.ScrollDown(1)
	}
	return m
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.focused {
	case paneServer:
		idx, srv := m.serverPane.SelectedServer()
		if srv != nil {
			return m.onServerSelected(idx, *srv)
		}

	case paneFile:
		isUpDir, folderIdx, folder, fileOrigIdx, file := m.filePane.SelectedItem()
		if isUpDir {
			return m.onUpDir()
		}
		if folder != nil {
			return m.onFolderSelected(folderIdx, *folder)
		}
		if file != nil {
			return m.onFileSelected(fileOrigIdx, *file)
		}
	}
	return m, nil
}

// onServerSelected handles server selection.
func (m Model) onServerSelected(idx int, srv config.ServerConfig) (tea.Model, tea.Cmd) {
	logger.Log("app", "onServerSelected: %s (idx=%d)", srv.Name, idx)
	m.stopTailInPlace()
	m.currentServer = &srv
	m.currentFolder = nil
	m.currentFile = nil
	m.viewerPane.Clear()
	m.filePane.Clear()
	m.serverPane.MarkSelected(idx)
	setTerminalTitle(fmt.Sprintf("Log Monitor — %s", srv.Name))

	folders := srv.LogFolders

	if len(folders) > 1 {
		m.filePane.SetFolders(folders)
		m.focused = paneFile
		m.setContext(fmt.Sprintf("\033[32m%s\033[0m — select a folder", srv.Name))
		return m, nil
	}

	// Single folder: auto-select
	folder := folders[0]
	m.currentFolder = &folder

	if srv.Sudo && m.pool.GetSudoPassword(srv) == "" {
		m = m.showSudoPrompt(srv)
		return m, nil
	}

	cmd := m.startConnection(srv)
	return m, cmd
}

// onFolderSelected handles folder selection.
func (m Model) onFolderSelected(idx int, folder config.LogFolder) (tea.Model, tea.Cmd) {
	logger.Log("app", "onFolderSelected: %s (idx=%d)", folder.Path, idx)
	m.stopTailInPlace()
	if m.currentServer == nil {
		return m, nil
	}
	m.currentFolder = &folder
	m.currentFile = nil
	m.filePane.selectedFolderIdx = idx
	m.viewerPane.Clear()

	srv := *m.currentServer

	if srv.Sudo && m.pool.GetSudoPassword(srv) == "" {
		m = m.showSudoPrompt(srv)
		return m, nil
	}

	cmd := m.startConnection(srv)
	return m, cmd
}

// onFileSelected handles file selection.
func (m Model) onFileSelected(idx int, file ssh.FileInfo) (tea.Model, tea.Cmd) {
	if m.currentServer == nil || m.currentFolder == nil {
		return m, nil
	}
	m.stopTailInPlace()
	m.currentFile = &file
	srv := *m.currentServer
	folderPath := m.currentFolder.Path
	fullPath := filepath.Join(folderPath, file.Name)

	m.filePane.MarkSelected(idx)
	m.setContext(fmt.Sprintf("\033[32m%s\033[0m %s", srv.Name, fullPath))
	setTerminalTitle(fmt.Sprintf("Log Monitor — %s:%s", srv.Name, fullPath))
	m.viewerPane.Clear()

	if isBinaryExtension(file.Name) {
		m.viewerPane.SetMessage("Binary file — cannot tail\n\nUse F5 to download instead.")
		return m, nil
	}

	// Start initial read and tail in parallel to avoid sequential sudo delays
	ch := make(chan []byte, 64)
	m.tailChan = ch
	return m, tea.Batch(
		countAndReadFileCmd(m.pool, srv, fullPath, m.cfg.Defaults.TailLines),
		startTailCmd(m.pool, srv, fullPath, ch),
	)
}

var binaryExtensions = map[string]bool{
	".gz": true, ".bz2": true, ".xz": true, ".zst": true,
	".zip": true, ".tar": true, ".7z": true, ".rar": true,
	".lz4": true, ".br": true, ".lzo": true, ".z": true,
	".tgz": true, ".tbz2": true, ".txz": true,
}

func isBinaryExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return binaryExtensions[ext]
}

// onUpDir returns to folder view.
func (m Model) onUpDir() (tea.Model, tea.Cmd) {
	m.stopTailInPlace()
	m.currentFolder = nil
	m.currentFile = nil
	m.viewerPane.Clear()

	if m.currentServer != nil {
		m.filePane.SetFolders(m.currentServer.LogFolders)
		m.setContext(fmt.Sprintf("\033[32m%s\033[0m — select a folder", m.currentServer.Name))
	}
	return m, nil
}

func (m *Model) startConnection(srv config.ServerConfig) tea.Cmd {
	folder := m.currentFolder
	if folder == nil {
		return nil
	}
	m.focused = paneFile
	m.setContext(fmt.Sprintf("\033[33mConnecting to\033[0m %s...", srv.Name))
	return connectAndListCmd(m.pool, srv, *folder)
}

func (m Model) stopTail() Model {
	m.stopTailInPlace()
	m.viewerPane.StopSpinner()
	m.viewerPane.ResetTitle()
	if m.currentServer != nil {
		m.setContext(fmt.Sprintf("\033[33mTail stopped\033[0m — %s", m.currentServer.Name))
	}
	return m
}

func (m *Model) stopTailInPlace() {
	if m.tailCancel != nil {
		m.tailCancel()
		m.tailer = nil
		m.tailCancel = nil
		m.tailChan = nil
		m.tailing = false
	}
}

func (m Model) refreshFiles() (tea.Model, tea.Cmd) {
	if m.currentServer == nil || m.currentFolder == nil {
		return m, nil
	}
	return m, connectAndListCmd(m.pool, *m.currentServer, *m.currentFolder)
}

func (m Model) autoStart() (tea.Model, tea.Cmd) {
	logger.Log("app", "autoStart: server=%q folder=%q file=%q", m.autoSelect.Server, m.autoSelect.Folder, m.autoSelect.File)

	serverIdx := -1
	var srv config.ServerConfig
	for i, s := range m.cfg.Servers {
		if strings.EqualFold(s.Name, m.autoSelect.Server) {
			serverIdx = i
			srv = s
			break
		}
	}
	if serverIdx < 0 {
		m.errorMsg = fmt.Sprintf("Server %q not found", m.autoSelect.Server)
		return m, nil
	}

	// If --file is set, install callback
	if m.autoSelect.File != "" {
		autoFile := m.autoSelect.File
		m.onFilesLoaded = func(model *Model) tea.Cmd {
			files := model.filePane.GetFiles()
			for i, f := range files {
				if strings.EqualFold(f.Name, autoFile) {
					// Return a command that will trigger file selection
					fileCopy := f
					return func() tea.Msg {
						return autoFileSelectMsg{idx: i, file: fileCopy}
					}
				}
			}
			model.errorMsg = fmt.Sprintf("File %q not found", autoFile)
			return nil
		}
	}

	folders := srv.LogFolders

	if len(folders) > 1 && m.autoSelect.Folder != "" {
		// First select the server
		m2, _ := m.onServerSelected(serverIdx, srv)
		m = m2.(Model)

		// Find and select the folder
		for i, f := range folders {
			if f.Path == m.autoSelect.Folder {
				return m.onFolderSelected(i, f)
			}
		}
		m.errorMsg = fmt.Sprintf("Folder %q not found on %s", m.autoSelect.Folder, srv.Name)
		return m, nil
	}

	return m.onServerSelected(serverIdx, srv)
}

type autoFileSelectMsg struct {
	idx  int
	file ssh.FileInfo
}

// handleModalKey handles keyboard input when a modal is open.
func (m Model) handleModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.modal = modalNone
		m.sudoServer = nil
		return m, nil

	case "enter":
		return m.submitModal()

	case "tab":
		if m.modal == modalDownload {
			m.modalFocus = (m.modalFocus + 1) % 2
			if m.modalFocus == 0 {
				m.modalInput.Focus()
				m.modalInput2.Blur()
			} else {
				m.modalInput.Blur()
				m.modalInput2.Focus()
			}
			return m, nil
		}
	}

	// Forward to the focused text input
	var cmd tea.Cmd
	if m.modal == modalDownload && m.modalFocus == 1 {
		m.modalInput2, cmd = m.modalInput2.Update(msg)
	} else {
		m.modalInput, cmd = m.modalInput.Update(msg)
	}
	return m, cmd
}

func (m Model) submitModal() (tea.Model, tea.Cmd) {
	switch m.modal {
	case modalSudo:
		pw := m.modalInput.Value()
		m.modal = modalNone
		if m.sudoServer != nil {
			srv := *m.sudoServer
			m.sudoServer = nil
			if pw == "" {
				m.setContext("\033[33mSudo password cancelled\033[0m")
				m.focused = paneServer
				return m, nil
			}
			m.pool.SetSudoPassword(srv, pw)
			m.focused = paneFile
			if m.currentFolder != nil {
				m.setContext(fmt.Sprintf("\033[33mConnecting to\033[0m %s...", srv.Name))
				return m, connectAndListCmd(m.pool, srv, *m.currentFolder)
			}
			return m, nil
		}

	case modalFilter:
		newFilter := m.modalInput.Value()
		m.modal = modalNone
		// Re-load with filter
		if m.currentServer != nil && m.currentFolder != nil && m.currentFile != nil {
			m.stopTailInPlace()
			m.viewerPane.SetTailFilter(newFilter)
			m.viewerPane.Clear()
			m.viewerPane.SetTailFilter(newFilter) // Clear resets it, set again
			fullPath := filepath.Join(m.currentFolder.Path, m.currentFile.Name)
			m.setContext(fmt.Sprintf("\033[32m%s\033[0m %s", m.currentServer.Name, fullPath))
			return m, countAndReadFileCmd(m.pool, *m.currentServer, fullPath, m.cfg.Defaults.TailLines)
		}

	case modalDownload:
		dir := m.modalInput.Value()
		name := m.modalInput2.Value()
		m.modal = modalNone
		if m.currentServer != nil && m.currentFolder != nil && m.currentFile != nil {
			remotePath := filepath.Join(m.currentFolder.Path, m.currentFile.Name)
			m.setContext(fmt.Sprintf("\033[33mDownloading\033[0m %s...", name))
			return m, downloadFileCmd(m.pool, *m.currentServer, remotePath, dir, name)
		}
	}

	return m, nil
}

// modalInnerWidth is the usable text width inside the modal (Width - horizontal padding).
const modalInnerWidth = 70 - 4 // modal Width(70) minus Padding(1, 2) = 2 left + 2 right

// styledInput creates a textinput with modal-appropriate styling.
func styledInput() textinput.Model {
	ti := textinput.New()
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#03AFFF"))
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#03AFFF"))
	ti.Width = modalInnerWidth - 2 // subtract prompt width "> "
	return ti
}

func (m Model) showSudoPrompt(srv config.ServerConfig) Model {
	ti := styledInput()
	ti.Placeholder = "Password"
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '*'
	ti.Focus()

	m.modal = modalSudo
	m.modalInput = ti
	m.sudoServer = &srv
	return m
}

func (m Model) showFilterPrompt() Model {
	ti := styledInput()
	ti.Placeholder = "Filter term"
	ti.SetValue(m.viewerPane.GetTailFilter())
	ti.Focus()

	m.modal = modalFilter
	m.modalInput = ti
	return m
}

func (m Model) showDownloadDialog() (tea.Model, tea.Cmd) {
	if m.currentServer == nil || m.currentFolder == nil || m.currentFile == nil {
		return m, nil
	}

	defaultDir := "."
	if exe, err := os.Executable(); err == nil {
		defaultDir = filepath.Join(filepath.Dir(exe), "logs")
	}

	ti1 := styledInput()
	ti1.Placeholder = "Local path"
	ti1.SetValue(defaultDir)
	ti1.Focus()

	ti2 := styledInput()
	ti2.Placeholder = "Filename"
	ti2.SetValue(m.currentFile.Name)

	m.modal = modalDownload
	m.modalInput = ti1
	m.modalInput2 = ti2
	m.modalFocus = 0
	return m, nil
}

func (m Model) renderModal(background string) string {
	var title, content string

	buttonOK := modalButtonStyle.Render("Enter OK")
	buttonCancel := modalButtonStyle.Render("Esc Cancel")
	buttonTab := modalButtonStyle.Render("Tab Next")

	switch m.modal {
	case modalSudo:
		title = fmt.Sprintf(" Sudo password for %s ", m.currentServer.Name)
		content = m.modalInput.View() + "\n\n" + buttonOK + "  " + buttonCancel

	case modalFilter:
		title = " Tail Filter "
		content = m.modalInput.View() + "\n\n" + buttonOK + "  " + buttonCancel

	case modalDownload:
		title = " Download File "
		content = modalHintStyle.Render("Download remote file to local machine") +
			"\n\n" + modalHintStyle.Render("Local path:") + "\n" + m.modalInput.View() +
			"\n\n" + modalHintStyle.Render("Filename:") + "\n" + m.modalInput2.View() +
			"\n\n" + buttonOK + "  " + buttonTab + "  " + buttonCancel
	}

	modalBox := modalStyle.Width(70).Render(
		modalTitleStyle.Render(title) + "\n\n" + content,
	)

	// Center the modal by replacing entire background rows
	modalW := lipgloss.Width(modalBox)
	modalH := lipgloss.Height(modalBox)

	bgLines := strings.Split(background, "\n")
	startRow := (m.height - modalH) / 2
	startCol := (m.width - modalW) / 2

	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	shadowChar := modalShadowStyle.Render("▒")

	const ansiRst = "\033[0m"

	modalLines := strings.Split(modalBox, "\n")
	for i, ml := range modalLines {
		row := startRow + i
		if row < len(bgLines) {
			bg := bgLines[row]
			// Preserve background content on both sides of the modal
			leftEnd := runeIndex(bg, startCol)
			bgLeft := bg[:leftEnd]

			shadowW := 0
			shadow := ""
			if startCol+modalW < m.width {
				shadow = shadowChar
				shadowW = 1
			}

			rightStart := runeIndex(bg, startCol+modalW+shadowW)
			bgRight := bg[rightStart:]

			bgLines[row] = bgLeft + ansiRst + ml + ansiRst + shadow + ansiRst + bgRight
		}
	}

	// Bottom shadow row
	shadowRow := startRow + len(modalLines)
	if shadowRow < len(bgLines) {
		bg := bgLines[shadowRow]
		leftEnd := runeIndex(bg, startCol+1)
		bgLeft := bg[:leftEnd]

		shadowLine := strings.Repeat("▒", modalW)
		shadow := modalShadowStyle.Render(shadowLine)

		rightStart := runeIndex(bg, startCol+1+modalW)
		bgRight := bg[rightStart:]

		bgLines[shadowRow] = bgLeft + ansiRst + shadow + ansiRst + bgRight
	}

	return strings.Join(bgLines, "\n")
}

// setTerminalTitle sets the terminal window/tab title via OSC escape.
func setTerminalTitle(title string) {
	fmt.Fprintf(os.Stdout, "\033]0;%s\007", title)
}

// Shutdown cleans up SSH connections and resources.
func (m *Model) Shutdown() {
	logger.Log("app", "shutdown: start")
	m.stopTailInPlace()
	m.pool.CloseAll()
	setTerminalTitle("")
	logger.Log("app", "shutdown: done")
}

// Run creates a tea.Program, runs it, and performs cleanup.
func Run(cfg *config.Config, autoSelect AutoSelect) error {
	m := NewModel(cfg, autoSelect)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if fm, ok := finalModel.(Model); ok {
		fm.Shutdown()
	}
	return err
}

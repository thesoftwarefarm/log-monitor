package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"log-monitor/internal/config"
	"log-monitor/internal/ssh"

	"github.com/charmbracelet/lipgloss"
)

type filePaneMode int

const (
	modeFiles   filePaneMode = iota
	modeFolders
)

// FilePaneModel holds the state for the file list pane.
type FilePaneModel struct {
	mode    filePaneMode
	files   []ssh.FileInfo
	folders []config.LogFolder
	dir     string
	cursor  int
	width   int
	height  int

	selectedFileIdx   int // original index into files[], -1 = none
	selectedFolderIdx int // last selected folder index
	hasUpDir          bool
	folderPath        string
	message           string // error/status message to display

	// Fuzzy filter
	filterQuery    string
	filteredIdxMap []int // maps display index -> original file index
}

// NewFilePaneModel creates a new file pane model.
func NewFilePaneModel() FilePaneModel {
	return FilePaneModel{
		selectedFileIdx:   -1,
		selectedFolderIdx: -1,
	}
}

func (fp *FilePaneModel) rebuildFilter() {
	if fp.mode == modeFolders {
		return
	}
	if fp.filterQuery == "" {
		fp.filteredIdxMap = make([]int, len(fp.files))
		for i := range fp.files {
			fp.filteredIdxMap[i] = i
		}
	} else {
		fp.filteredIdxMap = nil
		for i, f := range fp.files {
			if FuzzyMatch(f.Name, fp.filterQuery) {
				fp.filteredIdxMap = append(fp.filteredIdxMap, i)
			}
		}
	}
	// Clamp cursor
	total := fp.totalRows()
	if fp.cursor >= total {
		fp.cursor = max(0, total-1)
	}
}

// totalRows returns the number of selectable rows.
func (fp *FilePaneModel) totalRows() int {
	if fp.mode == modeFolders {
		return len(fp.folders)
	}
	n := len(fp.filteredIdxMap)
	if fp.hasUpDir {
		n++
	}
	return n
}

// HandleRune adds a filter character (files mode only).
func (fp *FilePaneModel) HandleRune(r rune) {
	if fp.mode == modeFolders {
		return
	}
	fp.filterQuery += string(r)
	fp.rebuildFilter()
}

// HandleBackspace removes the last filter character.
func (fp *FilePaneModel) HandleBackspace() bool {
	if fp.mode == modeFolders {
		return false
	}
	if len(fp.filterQuery) > 0 {
		_, size := utf8.DecodeLastRuneInString(fp.filterQuery)
		fp.filterQuery = fp.filterQuery[:len(fp.filterQuery)-size]
		fp.rebuildFilter()
		return true
	}
	return false
}

// ClearFilter resets the filter.
func (fp *FilePaneModel) ClearFilter() {
	fp.filterQuery = ""
	fp.rebuildFilter()
}

// HasActiveFilter returns true if a filter is active.
func (fp *FilePaneModel) HasActiveFilter() bool {
	return fp.filterQuery != ""
}

// FilterQuery returns the current filter string.
func (fp *FilePaneModel) FilterQuery() string {
	return fp.filterQuery
}

// MoveUp moves cursor up.
func (fp *FilePaneModel) MoveUp() {
	if fp.cursor > 0 {
		fp.cursor--
	}
}

// MoveDown moves cursor down.
func (fp *FilePaneModel) MoveDown() {
	total := fp.totalRows()
	if fp.cursor < total-1 {
		fp.cursor++
	}
}

// SetFolders switches to folder mode.
func (fp *FilePaneModel) SetFolders(folders []config.LogFolder) {
	fp.mode = modeFolders
	fp.folders = folders
	fp.files = nil
	fp.dir = ""
	fp.folderPath = ""
	fp.selectedFileIdx = -1
	fp.filterQuery = ""
	fp.hasUpDir = false
	fp.message = ""
	// Restore cursor to last selected folder
	fp.cursor = 0
	if fp.selectedFolderIdx >= 0 && fp.selectedFolderIdx < len(folders) {
		fp.cursor = fp.selectedFolderIdx
	}
}

// SetFiles switches to files mode and populates file data.
func (fp *FilePaneModel) SetFiles(dir string, files []ssh.FileInfo, showUpDir bool) {
	fp.mode = modeFiles
	fp.folders = nil
	fp.files = files
	fp.dir = dir
	fp.selectedFileIdx = -1
	fp.filterQuery = ""
	fp.hasUpDir = showUpDir
	fp.message = ""
	if showUpDir {
		fp.folderPath = dir
	} else {
		fp.folderPath = ""
	}
	fp.rebuildFilter()
	// Default cursor to first data row (skip updir)
	fp.cursor = 0
	if showUpDir && len(fp.filteredIdxMap) > 0 {
		fp.cursor = 1
	}
}

// Clear resets the pane.
func (fp *FilePaneModel) Clear() {
	fp.mode = modeFiles
	fp.files = nil
	fp.folders = nil
	fp.dir = ""
	fp.folderPath = ""
	fp.selectedFileIdx = -1
	fp.selectedFolderIdx = -1
	fp.filterQuery = ""
	fp.hasUpDir = false
	fp.message = ""
	fp.cursor = 0
	fp.filteredIdxMap = nil
}

// SetMessage sets a message to display (e.g. error).
func (fp *FilePaneModel) SetMessage(msg string) {
	fp.Clear()
	fp.message = msg
}

// MarkSelected marks a file as selected.
func (fp *FilePaneModel) MarkSelected(idx int) {
	fp.selectedFileIdx = idx
}

// IsInFolderMode returns true if showing folders.
func (fp *FilePaneModel) IsInFolderMode() bool {
	return fp.mode == modeFolders
}

// SelectedItem returns what's at the cursor.
// Returns: isUpDir, folderIdx, folder, fileOrigIdx, file
func (fp *FilePaneModel) SelectedItem() (isUpDir bool, folderIdx int, folder *config.LogFolder, fileOrigIdx int, file *ssh.FileInfo) {
	if fp.mode == modeFolders {
		if fp.cursor >= 0 && fp.cursor < len(fp.folders) {
			return false, fp.cursor, &fp.folders[fp.cursor], -1, nil
		}
		return false, -1, nil, -1, nil
	}
	// Files mode
	displayIdx := fp.cursor
	if fp.hasUpDir {
		if displayIdx == 0 {
			return true, -1, nil, -1, nil
		}
		displayIdx--
	}
	if displayIdx >= 0 && displayIdx < len(fp.filteredIdxMap) {
		origIdx := fp.filteredIdxMap[displayIdx]
		return false, -1, nil, origIdx, &fp.files[origIdx]
	}
	return false, -1, nil, -1, nil
}

// GetFiles returns the current file list.
func (fp *FilePaneModel) GetFiles() []ssh.FileInfo {
	return fp.files
}

// SetSize updates dimensions.
func (fp *FilePaneModel) SetSize(w, h int) {
	fp.width = w
	fp.height = h
}

// View renders the file pane.
func (fp *FilePaneModel) View(focused bool) string {
	var paneStyle, titleStyle lipgloss.Style
	if focused {
		paneStyle = focusedPaneStyle
		titleStyle = focusedTitleStyle
	} else {
		paneStyle = unfocusedPaneStyle
		titleStyle = unfocusedTitleStyle
	}

	paneStyle = paneStyle.Width(fp.width - 2).Height(fp.height - 2)

	// Title
	titleText := " Files "
	if fp.mode == modeFolders {
		titleText = " Folders "
	} else if fp.folderPath != "" {
		titleText = " " + fp.folderPath + " "
	}
	if fp.filterQuery != "" {
		titleText = fmt.Sprintf("%s[%s] ", titleText, fp.filterQuery)
	}

	var b strings.Builder

	// If there's a message, just show it
	if fp.message != "" {
		b.WriteString(fp.message)
		content := paneStyle.Render(b.String())
		title := titleStyle.Render(" Files ")
		return placeTitleInBorder(content, title)
	}

	// Column widths
	innerWidth := fp.width - 2
	if innerWidth < 20 {
		innerWidth = 20
	}
	sizeColW := 8
	timeColW := 13
	nameColW := innerWidth - sizeColW - timeColW - 3 // 1 space after name, 2 spaces after size
	if nameColW < 10 {
		nameColW = 10
	}

	// Header row
	header := fmt.Sprintf("%-*s %*s  %s",
		nameColW, "Name",
		sizeColW, "Size",
		padRight("Modify time", timeColW))
	b.WriteString(tableHeaderStyle.Render(header))
	b.WriteByte('\n')

	if fp.mode == modeFolders {
		fp.renderFolders(&b, nameColW, sizeColW, timeColW)
	} else {
		fp.renderFiles(&b, nameColW, sizeColW, timeColW)
	}

	content := paneStyle.Render(b.String())
	title := titleStyle.Render(titleText)
	return placeTitleInBorder(content, title)
}

func (fp *FilePaneModel) renderFolders(b *strings.Builder, nameW, sizeW, timeW int) {
	if len(fp.folders) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("(no folders)"))
		return
	}

	innerHeight := fp.height - 5
	if innerHeight < 1 {
		innerHeight = 1
	}

	startIdx := 0
	if fp.cursor >= innerHeight {
		startIdx = fp.cursor - innerHeight + 1
	}
	endIdx := startIdx + innerHeight
	if endIdx > len(fp.folders) {
		endIdx = len(fp.folders)
	}

	lineWidth := fp.width - 2

	for i := startIdx; i < endIdx; i++ {
		name := truncateString(fp.folders[i].Path, nameW)
		// Build plain-text line with proper column alignment
		line := fmt.Sprintf("%-*s %*s  %s", nameW, name, sizeW, "DIR", strings.Repeat(" ", timeW))

		if i == fp.cursor {
			b.WriteString(selectedRowStyle.Render(padRight(line, lineWidth)))
		} else {
			// Color the DIR part after formatting
			plainLine := fmt.Sprintf("%-*s ", nameW, name)
			dirPart := lipgloss.NewStyle().Foreground(accentColor).Render(fmt.Sprintf("%*s", sizeW, "DIR"))
			b.WriteString(plainLine + dirPart + strings.Repeat(" ", timeW+2))
		}
		if i < endIdx-1 {
			b.WriteByte('\n')
		}
	}
}

func (fp *FilePaneModel) renderFiles(b *strings.Builder, nameW, sizeW, timeW int) {
	total := fp.totalRows()
	if total == 0 && len(fp.files) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("(no files found)"))
		return
	}

	innerHeight := fp.height - 5
	if innerHeight < 1 {
		innerHeight = 1
	}

	startIdx := 0
	if fp.cursor >= innerHeight {
		startIdx = fp.cursor - innerHeight + 1
	}
	endIdx := startIdx + innerHeight
	if endIdx > total {
		endIdx = total
	}

	lineWidth := fp.width - 2

	for di := startIdx; di < endIdx; di++ {
		fileDisplayIdx := di
		if fp.hasUpDir {
			if di == 0 {
				// Up-dir row — build as plain text, apply color after
				upLine := fmt.Sprintf("%-*s %*s  %s", nameW, "/..", sizeW, "UP-DIR", strings.Repeat(" ", timeW))
				if di == fp.cursor {
					b.WriteString(selectedRowStyle.Render(padRight(upLine, lineWidth)))
				} else {
					b.WriteString(lipgloss.NewStyle().Foreground(accentColor).Render(padRight(upLine, lineWidth)))
				}
				if di < endIdx-1 {
					b.WriteByte('\n')
				}
				continue
			}
			fileDisplayIdx--
		}

		if fileDisplayIdx >= 0 && fileDisplayIdx < len(fp.filteredIdxMap) {
			origIdx := fp.filteredIdxMap[fileDisplayIdx]
			f := fp.files[origIdx]
			name := f.Name
			isActive := origIdx == fp.selectedFileIdx

			if isActive {
				name = "* " + name
			}
			name = truncateString(name, nameW)

			if di == fp.cursor {
				// Cursor row — plain text, full-width highlight
				line := fmt.Sprintf("%-*s %*s  %s", nameW, name, sizeW, ssh.FormatSize(f.Size), f.ModTime.Format("Jan _2 15:04"))
				b.WriteString(selectedRowStyle.Render(padRight(line, lineWidth)))
			} else if isActive {
				// Active file (not cursor) — blue marker
				marker := activeMarkerStyle.Render("* ")
				plainName := truncateString(f.Name, nameW-2)
				rest := fmt.Sprintf(" %*s  %s", sizeW, ssh.FormatSize(f.Size), f.ModTime.Format("Jan _2 15:04"))
				b.WriteString(marker + padRight(plainName, nameW-2) + rest)
			} else {
				line := fmt.Sprintf("%-*s %*s  %s", nameW, name, sizeW, ssh.FormatSize(f.Size), f.ModTime.Format("Jan _2 15:04"))
				b.WriteString(line)
			}
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("(no matches)"))
		}

		if di < endIdx-1 {
			b.WriteByte('\n')
		}
	}
}

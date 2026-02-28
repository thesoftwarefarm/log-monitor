package ui

import (
	"fmt"
	"unicode/utf8"

	"log-monitor/internal/config"
	"log-monitor/internal/ssh"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// filePaneMode distinguishes between folder-list and file-list views.
type filePaneMode int

const (
	modeFiles   filePaneMode = iota
	modeFolders filePaneMode = iota
)

// FilePane displays the list of log files on a selected server.
type FilePane struct {
	table           *tview.Table
	files           []ssh.FileInfo
	dir             string
	folderPath      string // folder path shown in title when inside a folder
	onSelect        func(idx int, file ssh.FileInfo)
	selectedFileIdx int // index into files[], -1 means no selection

	// Fuzzy filter state
	filterQuery    string
	filteredIdxMap []int // maps displayed table row (after header) → index into files[]

	onFilterChange func(query string)

	// Folder mode state
	mode              filePaneMode
	folders           []config.LogFolder
	selectedFolderIdx int // last selected folder index, -1 means none
	hasUpDir          bool // true when a "/ .." row is present in file mode
	onFolderSelect    func(idx int, folder config.LogFolder)
	onUpDir           func()
}

func NewFilePane() *FilePane {
	fp := &FilePane{
		table:             tview.NewTable(),
		selectedFileIdx:   -1,
		selectedFolderIdx: -1,
	}

	fp.table.SetTitle(" Files ").SetBorder(true)
	fp.table.SetSelectable(true, false)
	fp.table.SetFixed(1, 0)
	fp.table.SetSeparator('│')
	fp.table.SetSelectedStyle(tcell.StyleDefault.
		Background(tcell.ColorDarkCyan).
		Foreground(tcell.ColorWhite))

	// Show the column header initially
	fp.showHeader()

	// Selection via Enter key
	fp.table.SetSelectedFunc(func(row, col int) {
		displayIdx := row - 1 // row 0 is the fixed header

		if fp.mode == modeFolders {
			if fp.onFolderSelect != nil && displayIdx >= 0 && displayIdx < len(fp.folders) {
				fp.selectedFolderIdx = displayIdx
				fp.onFolderSelect(displayIdx, fp.folders[displayIdx])
			}
			return
		}

		// File mode
		if fp.hasUpDir {
			if displayIdx == 0 {
				// "/ .." row selected
				if fp.onUpDir != nil {
					fp.onUpDir()
				}
				return
			}
			// Adjust for the "/ .." row occupying display position 0
			displayIdx--
		}
		if fp.onSelect != nil && displayIdx >= 0 && displayIdx < len(fp.filteredIdxMap) {
			origIdx := fp.filteredIdxMap[displayIdx]
			fp.onSelect(origIdx, fp.files[origIdx])
		}
	})

	// Input capture for fuzzy filtering (only in file mode)
	fp.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// When the pane is empty (no files/folders loaded), swallow
		// navigation keys so tview doesn't loop looking for a selectable row.
		if fp.files == nil && fp.folders == nil {
			switch event.Key() {
			case tcell.KeyUp, tcell.KeyDown, tcell.KeyEnter:
				return nil
			}
			return event
		}

		if fp.mode == modeFolders {
			return event // no type-to-filter in folder mode
		}

		switch event.Key() {
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if len(fp.filterQuery) > 0 {
				_, size := utf8.DecodeLastRuneInString(fp.filterQuery)
				fp.filterQuery = fp.filterQuery[:len(fp.filterQuery)-size]
				fp.applyFilter()
				return nil
			}
		case tcell.KeyRune:
			r := event.Rune()
			// Don't capture 'q' — that's the global quit key
			if r == 'q' {
				return event
			}
			fp.filterQuery += string(r)
			fp.applyFilter()
			return nil
		}
		return event
	})

	return fp
}

// showHeader renders the column header in row 0.
func (fp *FilePane) showHeader() {
	fp.table.SetCell(0, 0,
		tview.NewTableCell("Name").
			SetSelectable(false).
			SetTextColor(tcell.ColorYellow).
			SetExpansion(1))
	fp.table.SetCell(0, 1,
		tview.NewTableCell("Size").
			SetSelectable(false).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignRight).
			SetExpansion(0).
			SetMaxWidth(8))
	fp.table.SetCell(0, 2,
		tview.NewTableCell("Modify time").
			SetSelectable(false).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetExpansion(0).
			SetMaxWidth(12))
}

// updateTitle sets the pane title, incorporating filter info if applicable.
func (fp *FilePane) updateTitle() {
	if fp.mode == modeFolders {
		fp.table.SetTitle(" Folders ")
		return
	}
	base := " Files "
	if fp.folderPath != "" {
		base = " " + fp.folderPath + " "
	}
	if fp.filterQuery != "" {
		// Strip trailing space from base, append filter, re-add space
		fp.table.SetTitle(fmt.Sprintf("%s[%s] ", base, fp.filterQuery))
	} else {
		fp.table.SetTitle(base)
	}
}

// rebuildTable populates the table from filteredIdxMap.
func (fp *FilePane) rebuildTable() {
	fp.table.Clear()
	fp.showHeader()
	fp.updateTitle()

	if fp.mode == modeFolders {
		fp.rebuildFolderTable()
		return
	}

	// Build index map
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

	// Determine starting data row (after header)
	nextRow := 1

	// Insert "/ .." row if needed
	if fp.hasUpDir {
		fp.table.SetCell(nextRow, 0,
			tview.NewTableCell("/ ..").
				SetExpansion(1).
				SetAlign(tview.AlignLeft).
				SetTextColor(tcell.ColorAqua))
		fp.table.SetCell(nextRow, 1,
			tview.NewTableCell("UP--DIR").
				SetAlign(tview.AlignRight).
				SetExpansion(0).
				SetMaxWidth(8).
				SetTextColor(tcell.ColorAqua))
		fp.table.SetCell(nextRow, 2,
			tview.NewTableCell("").
				SetAlign(tview.AlignLeft).
				SetExpansion(0).
				SetMaxWidth(12))
		nextRow++
	}

	if len(fp.filteredIdxMap) == 0 && len(fp.files) > 0 {
		fp.table.SetSelectable(fp.hasUpDir, false)
		fp.table.SetCell(nextRow, 0,
			tview.NewTableCell("(no matches)").
				SetSelectable(false).
				SetTextColor(tcell.ColorGray).
				SetExpansion(1))
		return
	}

	if len(fp.files) == 0 {
		fp.table.SetSelectable(fp.hasUpDir, false)
		fp.table.SetCell(nextRow, 0,
			tview.NewTableCell("(no files found)").
				SetSelectable(false).
				SetTextColor(tcell.ColorGray).
				SetExpansion(1))
		return
	}

	// Data rows exist — ensure selection is enabled.
	fp.table.SetSelectable(true, false)

	for i, origIdx := range fp.filteredIdxMap {
		row := nextRow + i
		f := fp.files[origIdx]

		// Col 0: Name (with "* " prefix if selected)
		name := f.Name
		if origIdx == fp.selectedFileIdx {
			name = "* " + name
		}
		fp.table.SetCell(row, 0,
			tview.NewTableCell(name).
				SetExpansion(1).
				SetAlign(tview.AlignLeft))

		// Col 1: Size (right-aligned, tight)
		fp.table.SetCell(row, 1,
			tview.NewTableCell(ssh.FormatSize(f.Size)).
				SetAlign(tview.AlignRight).
				SetExpansion(0).
				SetMaxWidth(8))

		// Col 2: Modified (short format)
		fp.table.SetCell(row, 2,
			tview.NewTableCell(f.ModTime.Format("Jan _2 15:04")).
				SetAlign(tview.AlignLeft).
				SetExpansion(0).
				SetMaxWidth(12))
	}

	// Preserve cursor on the selected item, or default to first data row
	targetRow := 1
	if fp.hasUpDir {
		targetRow = 2 // skip "/ .." by default
	}
	if fp.selectedFileIdx >= 0 {
		offset := 1
		if fp.hasUpDir {
			offset = 2
		}
		for i, origIdx := range fp.filteredIdxMap {
			if origIdx == fp.selectedFileIdx {
				targetRow = offset + i
				break
			}
		}
	}
	totalDataRows := len(fp.filteredIdxMap)
	if fp.hasUpDir {
		totalDataRows++
	}
	if totalDataRows > 0 {
		fp.table.Select(targetRow, 0)
	}
}

// rebuildFolderTable renders the folder list.
func (fp *FilePane) rebuildFolderTable() {
	if len(fp.folders) == 0 {
		fp.table.SetSelectable(false, false)
		fp.table.SetCell(1, 0,
			tview.NewTableCell("(no folders)").
				SetSelectable(false).
				SetTextColor(tcell.ColorGray).
				SetExpansion(1))
		return
	}

	fp.table.SetSelectable(true, false)

	for i, f := range fp.folders {
		row := i + 1
		fp.table.SetCell(row, 0,
			tview.NewTableCell(f.Path).
				SetExpansion(1).
				SetAlign(tview.AlignLeft))
		fp.table.SetCell(row, 1,
			tview.NewTableCell("DIR").
				SetAlign(tview.AlignRight).
				SetExpansion(0).
				SetMaxWidth(8).
				SetTextColor(tcell.ColorAqua))
		fp.table.SetCell(row, 2,
			tview.NewTableCell("").
				SetAlign(tview.AlignLeft).
				SetExpansion(0).
				SetMaxWidth(12))
	}

	// Restore cursor to last selected folder, or default to first.
	targetRow := 1
	if fp.selectedFolderIdx >= 0 && fp.selectedFolderIdx < len(fp.folders) {
		targetRow = fp.selectedFolderIdx + 1
	}
	fp.table.Select(targetRow, 0)
}

// applyFilter rebuilds the table based on the current filterQuery.
func (fp *FilePane) applyFilter() {
	fp.rebuildTable()
	if fp.onFilterChange != nil {
		fp.onFilterChange(fp.filterQuery)
	}
}

// SetSelectedFunc sets the callback for when a file is selected.
func (fp *FilePane) SetSelectedFunc(fn func(idx int, file ssh.FileInfo)) {
	fp.onSelect = fn
}

// SetFolderSelectedFunc sets the callback for when a folder is selected.
func (fp *FilePane) SetFolderSelectedFunc(fn func(idx int, folder config.LogFolder)) {
	fp.onFolderSelect = fn
}

// SetUpDirFunc sets the callback for when "/ .." is selected.
func (fp *FilePane) SetUpDirFunc(fn func()) {
	fp.onUpDir = fn
}

// SetFolders switches to folder mode and populates the folder list.
func (fp *FilePane) SetFolders(folders []config.LogFolder) {
	fp.mode = modeFolders
	fp.folders = folders
	fp.files = nil
	fp.dir = ""
	fp.folderPath = ""
	fp.selectedFileIdx = -1
	fp.filterQuery = ""
	fp.hasUpDir = false
	fp.table.SetSelectable(true, false)
	fp.rebuildTable()
}

// SetFiles populates the file table. Files should already be sorted.
// If showUpDir is true, a "/ .." row is added at the top.
func (fp *FilePane) SetFiles(dir string, files []ssh.FileInfo, showUpDir bool) {
	fp.mode = modeFiles
	fp.folders = nil
	fp.files = files
	fp.dir = dir
	fp.selectedFileIdx = -1
	fp.filterQuery = ""
	fp.hasUpDir = showUpDir
	if showUpDir {
		fp.folderPath = dir
	} else {
		fp.folderPath = ""
	}
	// Re-enable row selection (may have been disabled by Clear).
	fp.table.SetSelectable(true, false)
	fp.rebuildTable()

	// Scroll to top and select first data row
	fp.table.ScrollToBeginning()
	firstDataRow := 1
	if showUpDir {
		firstDataRow = 2
	}
	if len(fp.filteredIdxMap) > 0 {
		fp.table.Select(firstDataRow, 0)
	} else if showUpDir {
		fp.table.Select(1, 0)
	} else {
		fp.table.Select(0, 0)
	}
}

// IsInFolderMode returns true if the pane is currently showing folders.
func (fp *FilePane) IsInFolderMode() bool {
	return fp.mode == modeFolders
}

// Clear removes all items from the file table.
func (fp *FilePane) Clear() {
	fp.mode = modeFiles
	fp.files = nil
	fp.folders = nil
	fp.dir = ""
	fp.folderPath = ""
	fp.selectedFileIdx = -1
	fp.selectedFolderIdx = -1
	fp.filterQuery = ""
	fp.hasUpDir = false
	fp.table.Clear()
	// Disable row selection when empty — tview's Table navigation loops
	// infinitely if selectable is true but no selectable rows exist.
	fp.table.SetSelectable(false, false)
	fp.table.SetTitle(" Files ")
	fp.showHeader()
}

// SetMessage displays a centered message in the file pane (e.g. error state).
func (fp *FilePane) SetMessage(msg string) {
	fp.mode = modeFiles
	fp.files = nil
	fp.folders = nil
	fp.dir = ""
	fp.folderPath = ""
	fp.selectedFileIdx = -1
	fp.selectedFolderIdx = -1
	fp.filterQuery = ""
	fp.hasUpDir = false
	fp.table.Clear()
	fp.table.SetSelectable(false, false)
	fp.table.SetTitle(" Files ")
	fp.table.SetCell(0, 0,
		tview.NewTableCell(msg).
			SetSelectable(false).
			SetAlign(tview.AlignCenter).
			SetExpansion(1))
}

// MarkSelected marks a file (by original index) as selected with "* " prefix.
func (fp *FilePane) MarkSelected(fileIdx int) {
	fp.selectedFileIdx = fileIdx
	fp.rebuildTable()
}

// ClearSelection removes the selection mark.
func (fp *FilePane) ClearSelection() {
	fp.selectedFileIdx = -1
	fp.rebuildTable()
}

// HasActiveFilter returns true if there is a non-empty filter query.
func (fp *FilePane) HasActiveFilter() bool {
	return fp.filterQuery != ""
}

// SetFilterChangeFunc sets the callback for when the filter query changes.
func (fp *FilePane) SetFilterChangeFunc(fn func(query string)) {
	fp.onFilterChange = fn
}

// ClearFilter resets the filter query and rebuilds the table.
func (fp *FilePane) ClearFilter() {
	fp.filterQuery = ""
	fp.rebuildTable()
	if fp.onFilterChange != nil {
		fp.onFilterChange("")
	}
}

// GetDir returns the current directory being listed.
func (fp *FilePane) GetDir() string {
	return fp.dir
}

// GetFiles returns the current file list.
func (fp *FilePane) GetFiles() []ssh.FileInfo {
	return fp.files
}

// Widget returns the underlying tview primitive.
func (fp *FilePane) Widget() tview.Primitive {
	return fp.table
}

// Table returns the underlying table widget for focus management.
func (fp *FilePane) Table() *tview.Table {
	return fp.table
}

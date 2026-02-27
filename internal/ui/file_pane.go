package ui

import (
	"fmt"
	"unicode/utf8"

	"log-monitor/internal/ssh"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// FilePane displays the list of log files on a selected server.
type FilePane struct {
	table           *tview.Table
	files           []ssh.FileInfo
	dir             string
	onSelect        func(idx int, file ssh.FileInfo)
	selectedFileIdx int // index into files[], -1 means no selection

	// Fuzzy filter state
	filterQuery    string
	filteredIdxMap []int // maps displayed table row (after header) → index into files[]
}

func NewFilePane() *FilePane {
	fp := &FilePane{
		table:           tview.NewTable(),
		selectedFileIdx: -1,
	}

	fp.table.SetTitle(" Files ").SetBorder(true)
	fp.table.SetSelectable(true, false)
	fp.table.SetFixed(1, 0)
	fp.table.SetSelectedStyle(tcell.StyleDefault.
		Background(tcell.ColorDarkCyan).
		Foreground(tcell.ColorWhite))

	// Show the "Please select" prompt initially
	fp.showPrompt()

	// Selection only on Enter key
	fp.table.SetSelectedFunc(func(row, col int) {
		// Row 0 is the fixed header; data rows start at 1
		displayIdx := row - 1
		if fp.onSelect != nil && displayIdx >= 0 && displayIdx < len(fp.filteredIdxMap) {
			origIdx := fp.filteredIdxMap[displayIdx]
			fp.onSelect(origIdx, fp.files[origIdx])
		}
	})

	// Input capture for fuzzy filtering
	fp.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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

// showPrompt displays the header row (row 0).
func (fp *FilePane) showPrompt() {
	if fp.filterQuery != "" {
		fp.table.SetCell(0, 0,
			tview.NewTableCell(fmt.Sprintf("Filter: %s", fp.filterQuery)).
				SetSelectable(false).
				SetTextColor(tcell.ColorYellow).
				SetExpansion(1))
	} else {
		fp.table.SetCell(0, 0,
			tview.NewTableCell("Please select log file").
				SetSelectable(false).
				SetTextColor(tcell.ColorYellow).
				SetExpansion(1))
	}
	// Clear columns 1 and 2 in the header row
	fp.table.SetCell(0, 1, tview.NewTableCell("").SetSelectable(false))
	fp.table.SetCell(0, 2, tview.NewTableCell("").SetSelectable(false))
}

// rebuildTable populates the table from filteredIdxMap.
func (fp *FilePane) rebuildTable() {
	fp.table.Clear()

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

	// Row 0: header prompt
	fp.showPrompt()

	if len(fp.filteredIdxMap) == 0 && len(fp.files) > 0 {
		fp.table.SetSelectable(false, false)
		fp.table.SetCell(1, 0,
			tview.NewTableCell("(no matches)").
				SetSelectable(false).
				SetTextColor(tcell.ColorGray).
				SetExpansion(1))
		return
	}

	if len(fp.files) == 0 {
		fp.table.SetSelectable(false, false)
		fp.table.SetCell(1, 0,
			tview.NewTableCell("(no files found)").
				SetSelectable(false).
				SetTextColor(tcell.ColorGray).
				SetExpansion(1))
		return
	}

	// Data rows exist — ensure selection is enabled.
	fp.table.SetSelectable(true, false)

	for i, origIdx := range fp.filteredIdxMap {
		row := i + 1 // offset by header row
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

		// Col 1: Size (right-aligned, fixed width with padding)
		fp.table.SetCell(row, 1,
			tview.NewTableCell(fmt.Sprintf("%10s    ", ssh.FormatSize(f.Size))).
				SetAlign(tview.AlignRight).
				SetMaxWidth(14))

		// Col 2: Modified
		fp.table.SetCell(row, 2,
			tview.NewTableCell(f.ModTime.Format("2006-01-02 15:04:05")).
				SetAlign(tview.AlignLeft).
				SetMaxWidth(19))
	}

	// Preserve cursor on the selected item, or default to first data row
	targetRow := 1
	if fp.selectedFileIdx >= 0 {
		for i, origIdx := range fp.filteredIdxMap {
			if origIdx == fp.selectedFileIdx {
				targetRow = i + 1 // +1 for header row
				break
			}
		}
	}
	if len(fp.filteredIdxMap) > 0 {
		fp.table.Select(targetRow, 0)
	}
}

// applyFilter rebuilds the table based on the current filterQuery.
func (fp *FilePane) applyFilter() {
	fp.rebuildTable()
}

// SetSelectedFunc sets the callback for when a file is selected.
func (fp *FilePane) SetSelectedFunc(fn func(idx int, file ssh.FileInfo)) {
	fp.onSelect = fn
}

// SetFiles populates the file table. Files should already be sorted.
func (fp *FilePane) SetFiles(dir string, files []ssh.FileInfo) {
	fp.files = files
	fp.dir = dir
	fp.selectedFileIdx = -1
	fp.filterQuery = ""
	// Re-enable row selection (may have been disabled by Clear).
	fp.table.SetSelectable(true, false)
	fp.rebuildTable()

	// Scroll to top and select first data row
	fp.table.ScrollToBeginning()
	if len(fp.filteredIdxMap) > 0 {
		fp.table.Select(1, 0)
	} else {
		fp.table.Select(0, 0)
	}
}

// Clear removes all items from the file table.
func (fp *FilePane) Clear() {
	fp.files = nil
	fp.dir = ""
	fp.selectedFileIdx = -1
	fp.filterQuery = ""
	fp.table.Clear()
	// Disable row selection when empty — tview's Table navigation loops
	// infinitely if selectable is true but no selectable rows exist.
	fp.table.SetSelectable(false, false)
	fp.showPrompt()
}

// SetMessage displays a centered message in the file pane (e.g. error state).
func (fp *FilePane) SetMessage(msg string) {
	fp.files = nil
	fp.dir = ""
	fp.selectedFileIdx = -1
	fp.filterQuery = ""
	fp.table.Clear()
	fp.table.SetSelectable(false, false)
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

// ClearFilter resets the filter query and rebuilds the table.
func (fp *FilePane) ClearFilter() {
	fp.filterQuery = ""
	fp.rebuildTable()
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

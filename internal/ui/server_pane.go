package ui

import (
	"fmt"
	"unicode/utf8"

	"log-monitor/internal/config"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ServerPane displays the list of configured locations (servers).
type ServerPane struct {
	list        *tview.List
	servers     []config.ServerConfig
	onSelect    func(idx int, srv config.ServerConfig)
	selectedIdx int // index into servers[], -1 means no selection

	// Fuzzy filter state
	filterQuery    string
	filteredIdxMap []int // maps displayed list index (after header) → index into servers[]

	onFilterChange func(query string)
}

func NewServerPane(servers []config.ServerConfig) *ServerPane {
	sp := &ServerPane{
		list:        tview.NewList(),
		servers:     servers,
		selectedIdx: -1,
	}

	sp.list.SetTitle(" Locations ").SetBorder(true)
	sp.list.ShowSecondaryText(false)
	sp.list.SetHighlightFullLine(true)
	sp.list.SetSelectedBackgroundColor(tcell.ColorDarkCyan)

	sp.rebuildList()

	// Selection only on Enter key — no SetChangedFunc
	sp.list.SetSelectedFunc(func(idx int, main, secondary string, shortcut rune) {
		// Index 0 is the header row — skip it
		displayIdx := idx - 1
		if sp.onSelect != nil && displayIdx >= 0 && displayIdx < len(sp.filteredIdxMap) {
			origIdx := sp.filteredIdxMap[displayIdx]
			sp.onSelect(origIdx, sp.servers[origIdx])
		}
	})

	// Input capture for fuzzy filtering
	sp.list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if len(sp.filterQuery) > 0 {
				_, size := utf8.DecodeLastRuneInString(sp.filterQuery)
				sp.filterQuery = sp.filterQuery[:len(sp.filterQuery)-size]
				sp.applyFilter()
				return nil
			}
		case tcell.KeyRune:
			r := event.Rune()
			// Don't capture 'q' — that's the global quit key
			if r == 'q' {
				return event
			}
			sp.filterQuery += string(r)
			sp.applyFilter()
			return nil
		}
		return event
	})

	return sp
}

// rebuildList populates the list from filteredIdxMap, or all servers if no filter.
func (sp *ServerPane) rebuildList() {
	sp.list.Clear()

	// Build index map if empty (no filter active)
	if sp.filterQuery == "" {
		sp.filteredIdxMap = make([]int, len(sp.servers))
		for i := range sp.servers {
			sp.filteredIdxMap[i] = i
		}
		sp.list.AddItem("[yellow]Please select[-]", "", 0, nil)
	} else {
		sp.filteredIdxMap = nil
		for i, s := range sp.servers {
			if FuzzyMatch(s.Name, sp.filterQuery) {
				sp.filteredIdxMap = append(sp.filteredIdxMap, i)
			}
		}
		sp.list.AddItem(fmt.Sprintf("[yellow]Filter: %s[-]", sp.filterQuery), "", 0, nil)
	}

	for _, origIdx := range sp.filteredIdxMap {
		name := sp.servers[origIdx].Name
		if origIdx == sp.selectedIdx {
			name = "* " + name
		}
		sp.list.AddItem(name, "", 0, nil)
	}

	// Preserve cursor on the selected item, or default to first data item
	targetListIdx := 1
	if sp.selectedIdx >= 0 {
		for i, origIdx := range sp.filteredIdxMap {
			if origIdx == sp.selectedIdx {
				targetListIdx = i + 1 // +1 for header row
				break
			}
		}
	}
	if len(sp.filteredIdxMap) > 0 {
		sp.list.SetCurrentItem(targetListIdx)
	}
}

// applyFilter rebuilds the list based on the current filterQuery.
func (sp *ServerPane) applyFilter() {
	sp.rebuildList()
	if sp.onFilterChange != nil {
		sp.onFilterChange(sp.filterQuery)
	}
}

// SetSelectedFunc sets the callback for when a location is selected.
func (sp *ServerPane) SetSelectedFunc(fn func(idx int, srv config.ServerConfig)) {
	sp.onSelect = fn
}

// MarkSelected marks a server (by original index) as selected with "* " prefix.
func (sp *ServerPane) MarkSelected(serverIdx int) {
	sp.selectedIdx = serverIdx
	sp.rebuildList()
}

// ClearSelection removes the selection mark.
func (sp *ServerPane) ClearSelection() {
	sp.selectedIdx = -1
	sp.rebuildList()
}

// HasActiveFilter returns true if there is a non-empty filter query.
func (sp *ServerPane) HasActiveFilter() bool {
	return sp.filterQuery != ""
}

// SetFilterChangeFunc sets the callback for when the filter query changes.
func (sp *ServerPane) SetFilterChangeFunc(fn func(query string)) {
	sp.onFilterChange = fn
}

// ClearFilter resets the filter query and rebuilds the list.
func (sp *ServerPane) ClearFilter() {
	sp.filterQuery = ""
	sp.rebuildList()
	if sp.onFilterChange != nil {
		sp.onFilterChange("")
	}
}

// Widget returns the underlying tview primitive.
func (sp *ServerPane) Widget() tview.Primitive {
	return sp.list
}

// List returns the underlying list widget for focus management.
func (sp *ServerPane) List() *tview.List {
	return sp.list
}

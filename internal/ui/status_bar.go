package ui

import (
	"fmt"

	"github.com/rivo/tview"
)

const defaultShortcuts = " [yellow]q[white]: Exit | [yellow]↑↓[white]: Navigate | [yellow]Enter[white]: Select | [yellow]Tab[white]: Switch pane | [yellow]Esc[white]: Stop tail | [yellow]r[white]: Refresh"

// Pane-specific shortcut hints.
const (
	ShortcutsListPane   = " [yellow]Type[white]: Filter | [yellow]Enter[white]: Select | [yellow]Tab[white]: Switch pane | [yellow]Esc[white]: Clear filter | [yellow]q[white]: Exit"
	ShortcutsFolderPane = " [yellow]Enter[white]: Select folder/file | [yellow]Tab[white]: Switch pane | [yellow]q[white]: Exit"
	ShortcutsFilePane   = " [yellow]Type[white]: Filter | [yellow]Enter[white]: Select file | [yellow]Tab[white]: Switch pane | [yellow]Esc[white]: Clear filter | [yellow]q[white]: Exit"
	ShortcutsViewerPane     = " [yellow]F9[white]: Copy mode | [yellow]F5[white]: Download | [yellow]F7[white]: Filter | [yellow]g/G[white]: Top/Bottom | [yellow]r[white]: Refresh | [yellow]Esc[white]: Stop tail | [yellow]q[white]: Exit"
	ShortcutsViewerCopyMode = " [yellow::r] COPY MODE [-:-:-] Select text with mouse | [yellow]F9[white]/[yellow]Esc[white]: Exit copy mode"
)

// StatusBar is a single-row bar with context messages on the left and
// keybinding hints on the right.
type StatusBar struct {
	contextView   *tview.TextView
	shortcutsView *tview.TextView
	flex          *tview.Flex
}

func NewStatusBar() *StatusBar {
	sb := &StatusBar{
		contextView:   tview.NewTextView(),
		shortcutsView: tview.NewTextView(),
	}

	sb.contextView.SetDynamicColors(true)
	sb.contextView.SetTextAlign(tview.AlignLeft)

	sb.shortcutsView.SetDynamicColors(true)
	sb.shortcutsView.SetTextAlign(tview.AlignRight)
	sb.shortcutsView.SetText(defaultShortcuts)

	sb.flex = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(sb.contextView, 0, 1, false).
		AddItem(sb.shortcutsView, 0, 1, false)

	return sb
}

// SetContext displays a context message (server/file info) in the upper bar.
func (sb *StatusBar) SetContext(msg string) {
	sb.contextView.SetText(" " + msg)
}

// ClearContext clears the context bar.
func (sb *StatusBar) ClearContext() {
	sb.contextView.SetText("")
}

// SetFilter displays the active filter query in the context bar, or clears it.
func (sb *StatusBar) SetFilter(query string) {
	if query == "" {
		sb.contextView.SetText("")
	} else {
		sb.contextView.SetText(fmt.Sprintf(" [yellow]Filter:[-] %s", query))
	}
}

// SetError displays a transient error message in the context bar.
func (sb *StatusBar) SetError(msg string) {
	sb.contextView.SetText(fmt.Sprintf(" [red]Error:[-] %s", msg))
}

// Reset clears the context bar.
func (sb *StatusBar) Reset() {
	sb.contextView.SetText("")
}

// SetShortcuts updates the shortcuts bar with pane-specific hints.
func (sb *StatusBar) SetShortcuts(text string) {
	sb.shortcutsView.SetText(text)
}

// Widget returns the flex layout containing both bars.
func (sb *StatusBar) Widget() tview.Primitive {
	return sb.flex
}

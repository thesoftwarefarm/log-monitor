package ui

import (
	"fmt"

	"github.com/rivo/tview"
)

const defaultShortcuts = " [yellow]q[white]: Exit | [yellow]↑↓[white]: Navigate | [yellow]Enter[white]: Select | [yellow]Tab[white]: Switch pane | [yellow]Esc[white]: Stop tail | [yellow]r[white]: Refresh"

// Pane-specific shortcut hints.
const (
	ShortcutsListPane   = " [yellow]Type[white]: Filter | [yellow]Enter[white]: Select | [yellow]Tab[white]: Switch pane | [yellow]Esc[white]: Clear filter | [yellow]q[white]: Exit"
	ShortcutsFolderPane = " [yellow]Enter[white]: Select folder | [yellow]Tab[white]: Switch pane | [yellow]q[white]: Exit"
	ShortcutsViewerPane = " [yellow]/[white]: Search | [yellow]n/N[white]: Next/Prev | [yellow]g/G[white]: Top/Bottom | [yellow]r[white]: Refresh | [yellow]Esc[white]: Stop tail | [yellow]q[white]: Exit"
)

// StatusBar has two rows: a context bar (server/file info, errors) and a
// shortcuts bar (always shows keybindings).
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
	sb.shortcutsView.SetTextAlign(tview.AlignLeft)
	sb.shortcutsView.SetText(defaultShortcuts)

	sb.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(sb.contextView, 1, 0, false).
		AddItem(sb.shortcutsView, 1, 0, false)

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

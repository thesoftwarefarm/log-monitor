package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Quit       key.Binding
	Tab        key.Binding
	ShiftTab   key.Binding
	Escape     key.Binding
	Enter      key.Binding
	Up         key.Binding
	Down       key.Binding
	Home       key.Binding
	End        key.Binding
	Download    key.Binding
	TailFilter  key.Binding
	Refresh     key.Binding
	ResumeTail  key.Binding
	GotoTop     key.Binding
	GotoBottom  key.Binding
	Wrap        key.Binding
}

var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("Ctrl-C", "Exit"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("Tab", "Next pane"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("Shift-Tab", "Prev pane"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("Esc", "Stop tail/Clear filter"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("Enter", "Select"),
	),
	Up: key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("Up", "Navigate up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("Down", "Navigate down"),
	),
	Home: key.NewBinding(
		key.WithKeys("home"),
		key.WithHelp("Home", "Scroll to top"),
	),
	End: key.NewBinding(
		key.WithKeys("end"),
		key.WithHelp("End", "Scroll to bottom"),
	),
	Download: key.NewBinding(
		key.WithKeys("f5"),
		key.WithHelp("F5", "Download"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("f6"),
		key.WithHelp("F6", "Refresh"),
	),
	TailFilter: key.NewBinding(
		key.WithKeys("f7"),
		key.WithHelp("F7", "Tail filter"),
	),
	ResumeTail: key.NewBinding(
		key.WithKeys("f8"),
		key.WithHelp("F8", "Resume tail"),
	),
	GotoTop: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "Top"),
	),
	GotoBottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "Bottom"),
	),
	Wrap: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "Toggle wrap"),
	),
}

// Pane-specific shortcut hint strings.
const (
	shortcutsListPane   = "Type: Filter | Enter: Select | Tab: Switch pane | Esc: Clear filter | Ctrl-C: Exit"
	shortcutsFolderPane = "Enter: Select folder | Tab: Switch pane | Ctrl-C: Exit"
	shortcutsFilePane   = "Type: Filter | Enter: Select file | F5: Download | F6: Refresh | Tab: Switch pane | Esc: Clear filter | Ctrl-C: Exit"
	shortcutsViewerPane = "F6: Refresh | F7: Filter | g/G: Top/Bottom | w: Wrap | Shift+Click: Select text | Esc: Stop tail | Ctrl-C: Exit"
)

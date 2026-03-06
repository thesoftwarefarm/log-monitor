package ui

import "github.com/charmbracelet/lipgloss"

var (
	// Border styles
	focusedBorder   = lipgloss.RoundedBorder()
	unfocusedBorder = lipgloss.RoundedBorder()

	// Colors
	focusedColor   = lipgloss.Color("#03AFFF") // bright blue
	unfocusedColor = lipgloss.Color("7")       // white/gray
	headerColor    = lipgloss.Color("11")      // yellow
	selectedBg     = lipgloss.Color("#03AFFF")  // same blue for cursor highlight
	errorColor     = lipgloss.Color("9")       // red
	infoColor      = lipgloss.Color("10")      // green
	warnColor      = lipgloss.Color("11")      // yellow
	accentColor    = lipgloss.Color("14")      // aqua/cyan

	// Pane styles
	focusedPaneStyle = lipgloss.NewStyle().
				Border(focusedBorder).
				BorderForeground(focusedColor)

	unfocusedPaneStyle = lipgloss.NewStyle().
				Border(unfocusedBorder).
				BorderForeground(unfocusedColor)

	// Title styles
	focusedTitleStyle = lipgloss.NewStyle().
				Foreground(focusedColor).
				Bold(true)

	unfocusedTitleStyle = lipgloss.NewStyle().
				Foreground(unfocusedColor)

	// Table header style
	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(headerColor).
				Bold(true)

	// Selected row style (white text on blue bg)
	selectedRowStyle = lipgloss.NewStyle().
				Background(selectedBg).
				Foreground(lipgloss.Color("15"))

	// Active selection marker style (the "* " prefix on selected server/file)
	activeMarkerStyle = lipgloss.NewStyle().
				Foreground(focusedColor).
				Bold(true)

	// Modal styles
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#03AFFF")).
			Padding(1, 2).
			Background(lipgloss.Color("#1a1a2e"))

	modalTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#03AFFF")).
			Bold(true)

	modalHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	modalButtonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#03AFFF")).
				Bold(true)

)

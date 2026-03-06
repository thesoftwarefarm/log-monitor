package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// renderStatusBar renders the status bar with context on the left and shortcuts on the right.
func renderStatusBar(width int, contextMsg, errorMsg, shortcuts string) string {
	left := ""
	if errorMsg != "" {
		left = lipgloss.NewStyle().Foreground(errorColor).Render("Error: ") + errorMsg
	} else if contextMsg != "" {
		left = contextMsg
	}

	// Calculate available widths
	rightWidth := lipgloss.Width(shortcuts)
	leftWidth := width - rightWidth - 2
	if leftWidth < 0 {
		leftWidth = 0
	}

	left = " " + truncateString(left, leftWidth)
	left = padRight(left, leftWidth+1)

	right := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(shortcuts)

	return fmt.Sprintf("%s%s", left, right)
}

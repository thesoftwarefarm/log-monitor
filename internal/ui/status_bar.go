package ui

import (
	"fmt"
	"strings"

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

	coloredShortcuts := colorizeShortcuts(shortcuts)

	// Calculate available widths using the plain text length for spacing
	rightWidth := lipgloss.Width(shortcuts)
	leftWidth := width - rightWidth - 2
	if leftWidth < 0 {
		leftWidth = 0
	}

	left = " " + truncateString(left, leftWidth)
	left = padRight(left, leftWidth+1)

	return fmt.Sprintf("%s%s", left, coloredShortcuts)
}

// colorizeShortcuts renders shortcut hints with colored keys.
func colorizeShortcuts(s string) string {
	parts := strings.Split(s, " | ")
	colored := make([]string, len(parts))
	for i, part := range parts {
		if idx := strings.Index(part, ": "); idx >= 0 {
			key := part[:idx]
			desc := part[idx+2:]
			colored[i] = statusKeyStyle.Render(key) + statusSepStyle.Render(": "+desc)
		} else {
			colored[i] = statusSepStyle.Render(part)
		}
	}
	sep := statusSepStyle.Render(" | ")
	return strings.Join(colored, sep)
}

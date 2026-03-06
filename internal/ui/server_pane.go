package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"log-monitor/internal/config"

	"github.com/charmbracelet/lipgloss"
)

// ServerPaneModel holds the state for the server list pane.
type ServerPaneModel struct {
	servers     []config.ServerConfig
	cursor      int
	selectedIdx int // original index of the selected server, -1 means none
	width       int
	height      int

	// Fuzzy filter
	filterQuery    string
	filteredIdxMap []int // maps display index -> original server index
}

// NewServerPaneModel creates a new server pane model.
func NewServerPaneModel(servers []config.ServerConfig) ServerPaneModel {
	sp := ServerPaneModel{
		servers:     servers,
		selectedIdx: -1,
	}
	sp.rebuildFilter()
	return sp
}

func (sp *ServerPaneModel) rebuildFilter() {
	if sp.filterQuery == "" {
		sp.filteredIdxMap = make([]int, len(sp.servers))
		for i := range sp.servers {
			sp.filteredIdxMap[i] = i
		}
	} else {
		sp.filteredIdxMap = nil
		for i, s := range sp.servers {
			if FuzzyMatch(s.Name, sp.filterQuery) {
				sp.filteredIdxMap = append(sp.filteredIdxMap, i)
			}
		}
	}
	// Clamp cursor
	if sp.cursor >= len(sp.filteredIdxMap) {
		sp.cursor = max(0, len(sp.filteredIdxMap)-1)
	}
}

// HandleRune adds a character to the filter.
func (sp *ServerPaneModel) HandleRune(r rune) {
	sp.filterQuery += string(r)
	sp.rebuildFilter()
}

// HandleBackspace removes the last filter character.
func (sp *ServerPaneModel) HandleBackspace() bool {
	if len(sp.filterQuery) > 0 {
		_, size := utf8.DecodeLastRuneInString(sp.filterQuery)
		sp.filterQuery = sp.filterQuery[:len(sp.filterQuery)-size]
		sp.rebuildFilter()
		return true
	}
	return false
}

// ClearFilter resets the filter.
func (sp *ServerPaneModel) ClearFilter() {
	sp.filterQuery = ""
	sp.rebuildFilter()
}

// HasActiveFilter returns true if a filter is active.
func (sp *ServerPaneModel) HasActiveFilter() bool {
	return sp.filterQuery != ""
}

// FilterQuery returns the current filter string.
func (sp *ServerPaneModel) FilterQuery() string {
	return sp.filterQuery
}

// MoveUp moves the cursor up.
func (sp *ServerPaneModel) MoveUp() {
	if sp.cursor > 0 {
		sp.cursor--
	}
}

// MoveDown moves the cursor down.
func (sp *ServerPaneModel) MoveDown() {
	if sp.cursor < len(sp.filteredIdxMap)-1 {
		sp.cursor++
	}
}

// SetCursorFromY moves the cursor based on a mouse Y coordinate within the pane.
// The pane layout is: row 0 = border, row 1 = header, row 2+ = items.
func (sp *ServerPaneModel) SetCursorFromY(y int) {
	if len(sp.filteredIdxMap) == 0 {
		return
	}
	innerHeight := sp.height - 4
	if innerHeight < 1 {
		innerHeight = 1
	}
	startIdx := 0
	if sp.cursor >= innerHeight {
		startIdx = sp.cursor - innerHeight + 1
	}
	itemIdx := startIdx + (y - 2) // row 0=border, row 1=header
	if itemIdx < 0 {
		itemIdx = 0
	}
	if itemIdx >= len(sp.filteredIdxMap) {
		itemIdx = len(sp.filteredIdxMap) - 1
	}
	sp.cursor = itemIdx
}

// SelectedServer returns the server at the cursor, or nil.
func (sp *ServerPaneModel) SelectedServer() (int, *config.ServerConfig) {
	if len(sp.filteredIdxMap) == 0 {
		return -1, nil
	}
	origIdx := sp.filteredIdxMap[sp.cursor]
	return origIdx, &sp.servers[origIdx]
}

// MarkSelected sets the "active" server marker.
func (sp *ServerPaneModel) MarkSelected(idx int) {
	sp.selectedIdx = idx
}

// SetSize updates the available dimensions.
func (sp *ServerPaneModel) SetSize(w, h int) {
	sp.width = w
	sp.height = h
}

// View renders the server list.
func (sp *ServerPaneModel) View(focused bool) string {
	var paneStyle, titleStyle lipgloss.Style
	if focused {
		paneStyle = focusedPaneStyle
		titleStyle = focusedTitleStyle
	} else {
		paneStyle = unfocusedPaneStyle
		titleStyle = unfocusedTitleStyle
	}

	paneStyle = paneStyle.Width(sp.width - 2).Height(sp.height - 2)

	// Build content
	var b strings.Builder

	// Header
	if sp.filterQuery != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(headerColor).Render(fmt.Sprintf("Filter: %s", sp.filterQuery)))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(headerColor).Render("Please select"))
	}
	b.WriteByte('\n')

	// Visible area for items (height minus border, title line, header line)
	innerHeight := sp.height - 4
	if innerHeight < 1 {
		innerHeight = 1
	}

	// Calculate scroll window
	startIdx := 0
	if sp.cursor >= innerHeight {
		startIdx = sp.cursor - innerHeight + 1
	}
	endIdx := startIdx + innerHeight
	if endIdx > len(sp.filteredIdxMap) {
		endIdx = len(sp.filteredIdxMap)
	}

	lineWidth := sp.width - 2
	if lineWidth < 1 {
		lineWidth = 1
	}

	for di := startIdx; di < endIdx; di++ {
		origIdx := sp.filteredIdxMap[di]
		name := sp.servers[origIdx].Name

		if di == sp.cursor {
			// Cursor row — full-width highlight
			display := name
			if origIdx == sp.selectedIdx {
				display = "› " + display
			}
			display = truncateString(display, lineWidth)
			display = padRight(display, lineWidth)
			b.WriteString(selectedRowStyle.Render(display))
		} else if origIdx == sp.selectedIdx {
			// Active server (not cursor) — blue marker
			marker := activeMarkerStyle.Render("› ")
			display := truncateString(name, lineWidth-2)
			b.WriteString(marker + display)
		} else {
			display := truncateString(name, lineWidth)
			b.WriteString(display)
		}
		if di < endIdx-1 {
			b.WriteByte('\n')
		}
	}

	title := titleStyle.Render(" Locations ")
	content := paneStyle.Render(b.String())
	// Place title in top border
	return placeTitleInBorder(content, title)
}

// placeTitleInBorder overlays a title string onto the top border of a bordered box.
func placeTitleInBorder(box, title string) string {
	lines := strings.Split(box, "\n")
	if len(lines) == 0 {
		return box
	}
	topLine := lines[0]
	titleLen := lipgloss.Width(title)
	topWidth := lipgloss.Width(topLine)
	if titleLen+4 > topWidth {
		return box
	}
	// Extract the ANSI color prefix from the top line (the border color).
	// After inserting the title (which ends with a color reset), we must
	// re-emit this prefix so the remaining border characters keep their color.
	borderAnsi := getAnsiPrefix(topLine)

	prefix := topLine[:runeIndex(topLine, 2)]
	suffix := topLine[runeIndex(topLine, 2+titleLen):]
	lines[0] = prefix + title + borderAnsi + suffix
	return strings.Join(lines, "\n")
}

// getAnsiPrefix returns all ANSI escape sequences before the first visible character.
func getAnsiPrefix(s string) string {
	inEsc := false
	for i, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		// First visible character — everything before it is ANSI
		return s[:i]
	}
	return ""
}

// runeIndex returns the byte index of the nth rune in a string (ignoring ANSI).
// For simplicity, we count visible runes.
func runeIndex(s string, n int) int {
	count := 0
	inEsc := false
	for i, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if count == n {
			return i
		}
		count++
	}
	return len(s)
}

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func truncateString(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for i := len(runes); i > 0; i-- {
		if lipgloss.Width(string(runes[:i])) <= maxWidth-1 {
			return string(runes[:i]) + "…"
		}
	}
	return ""
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const defaultViewerTitle = " Log Viewer "
const maxViewerLines = 10000
const gutterWidth = 8 // "NNNNN | " = 5 digits + space + pipe + space
const gutterFmt = "\033[90m%5d |\033[0m "

var blankGutter = strings.Repeat(" ", gutterWidth)
var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// viewerLine stores a line's number separately from its colorized content.
type viewerLine struct {
	num     int    // original file line number
	content string // colorized content (without line number prefix)
}

// ViewerPaneModel holds the state for the log viewer pane.
type ViewerPaneModel struct {
	viewport viewport.Model
	lines    []viewerLine // colorized lines with line numbers
	width    int
	height   int
	title    string

	// Line numbering
	startLineNum int // the file line number of the first line in lines
	nextLineNum  int // next line number to assign from tail data

	// Tail filter
	tailFilter string

	// Spinner
	spinning     bool
	spinnerFrame int
	spinBase     string

	// Line count
	lineCount int

	// Word wrap
	wrapEnabled bool
}

// NewViewerPaneModel creates a new viewer pane model.
func NewViewerPaneModel() ViewerPaneModel {
	vp := ViewerPaneModel{
		title:        defaultViewerTitle,
		startLineNum: 1,
		nextLineNum:  1,
	}
	vp.viewport = viewport.New(0, 0)
	vp.viewport.SetContent("")
	return vp
}

// SetSize updates dimensions and the internal viewport.
func (vp *ViewerPaneModel) SetSize(w, h int) {
	vp.width = w
	vp.height = h
	// Account for border (2) + title line is part of border
	innerW := w - 2
	innerH := h - 2
	if innerW < 1 {
		innerW = 1
	}
	if innerH < 1 {
		innerH = 1
	}
	vp.viewport.Width = innerW
	vp.viewport.Height = innerH
	vp.rebuildContent()
}

// SetText replaces all content with initial file content.
func (vp *ViewerPaneModel) SetText(text string, startLine int) {
	vp.lines = nil
	vp.startLineNum = startLine
	vp.nextLineNum = startLine
	vp.lineCount = 0

	if text == "" {
		vp.rebuildContent()
		return
	}

	rawLines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for _, line := range rawLines {
		line = sanitizeLine(line)
		origNum := vp.nextLineNum
		vp.nextLineNum++

		// Apply filter
		if vp.tailFilter != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(vp.tailFilter)) {
			continue
		}

		colorized := ColorizeLine(line)
		if vp.tailFilter != "" {
			colorized = highlightFilterANSI(colorized, vp.tailFilter)
		}
		vp.lines = append(vp.lines, viewerLine{num: origNum, content: colorized})
		vp.lineCount++
	}

	vp.rebuildContent()
	vp.viewport.GotoBottom()
}

// AppendTailData processes incoming tail data and appends lines.
func (vp *ViewerPaneModel) AppendTailData(data []byte) {
	text := string(data)
	rawLines := strings.Split(text, "\n")

	for i, line := range rawLines {
		// Skip trailing empty from split
		if i == len(rawLines)-1 && line == "" {
			break
		}

		line = sanitizeLine(line)
		origNum := vp.nextLineNum
		vp.nextLineNum++

		// Apply filter
		if vp.tailFilter != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(vp.tailFilter)) {
			continue
		}

		colorized := ColorizeLine(line)
		if vp.tailFilter != "" {
			colorized = highlightFilterANSI(colorized, vp.tailFilter)
		}
		vp.lines = append(vp.lines, viewerLine{num: origNum, content: colorized})
		vp.lineCount++
	}

	// Cap at max lines
	if len(vp.lines) > maxViewerLines {
		excess := len(vp.lines) - maxViewerLines
		vp.lines = vp.lines[excess:]
	}

	wasAtBottom := vp.viewport.AtBottom()
	vp.rebuildContent()
	if wasAtBottom {
		vp.viewport.GotoBottom()
	}
}

// Clear resets the viewer.
func (vp *ViewerPaneModel) Clear() {
	vp.lines = nil
	vp.title = defaultViewerTitle
	vp.tailFilter = ""
	vp.lineCount = 0
	vp.startLineNum = 1
	vp.nextLineNum = 1
	vp.spinning = false
	vp.rebuildContent()
}

// SetMessage displays a message (plain, top-aligned).
func (vp *ViewerPaneModel) SetMessage(msg string) {
	vp.lines = nil
	vp.title = defaultViewerTitle
	vp.tailFilter = ""
	vp.lineCount = 0
	vp.spinning = false
	vp.startLineNum = 1
	vp.nextLineNum = 1
	vp.viewport.SetContent(msg)
}

// SetCenteredMessage displays a pre-rendered block centered in the viewport.
func (vp *ViewerPaneModel) SetCenteredMessage(block string) {
	vp.lines = nil
	vp.title = defaultViewerTitle
	vp.tailFilter = ""
	vp.lineCount = 0
	vp.spinning = false
	vp.startLineNum = 1
	vp.nextLineNum = 1

	centered := lipgloss.Place(vp.viewport.Width, vp.viewport.Height,
		lipgloss.Center, lipgloss.Center, block)
	vp.viewport.SetContent(centered)
}

// SetTitle sets a custom title.
func (vp *ViewerPaneModel) SetTitle(title string) {
	vp.title = title
}

// ResetTitle restores the default title.
func (vp *ViewerPaneModel) ResetTitle() {
	vp.title = defaultViewerTitle
	vp.lineCount = 0
}

// SetTailFilter sets the active tail filter.
func (vp *ViewerPaneModel) SetTailFilter(query string) {
	vp.tailFilter = query
}

// GetTailFilter returns the current tail filter.
func (vp *ViewerPaneModel) GetTailFilter() string {
	return vp.tailFilter
}

// StartSpinner starts the spinner animation.
func (vp *ViewerPaneModel) StartSpinner(base string) {
	vp.spinning = true
	vp.spinnerFrame = 0
	vp.spinBase = base
}

// StopSpinner stops the spinner.
func (vp *ViewerPaneModel) StopSpinner() {
	vp.spinning = false
}

// TickSpinner advances the spinner frame and returns the updated title.
func (vp *ViewerPaneModel) TickSpinner() {
	if !vp.spinning {
		return
	}
	vp.spinnerFrame++
	title := vp.spinBase
	if vp.tailFilter != "" {
		title = fmt.Sprintf("%s [filter: %s]", title, vp.tailFilter)
	}
	if vp.lineCount > 0 {
		title = fmt.Sprintf("%s (%s lines)", title, formatLineCount(vp.lineCount))
	}
	vp.title = fmt.Sprintf(" %c %s ", spinnerFrames[vp.spinnerFrame%len(spinnerFrames)], title)
}

// IsSpinning returns whether the spinner is active.
func (vp *ViewerPaneModel) IsSpinning() bool {
	return vp.spinning
}

// GotoTop scrolls to the top.
func (vp *ViewerPaneModel) GotoTop() {
	vp.viewport.GotoTop()
}

// GotoBottom scrolls to the bottom.
func (vp *ViewerPaneModel) GotoBottom() {
	vp.viewport.GotoBottom()
}

// ScrollUp scrolls up by one line.
func (vp *ViewerPaneModel) ScrollUp(n int) {
	vp.viewport.LineUp(n)
}

// ScrollDown scrolls down by one line.
func (vp *ViewerPaneModel) ScrollDown(n int) {
	vp.viewport.LineDown(n)
}

// ToggleWrap toggles line wrapping and rebuilds content.
func (vp *ViewerPaneModel) ToggleWrap() {
	vp.wrapEnabled = !vp.wrapEnabled
	vp.rebuildContent()
}

// IsWrapEnabled returns whether line wrapping is active.
func (vp *ViewerPaneModel) IsWrapEnabled() bool {
	return vp.wrapEnabled
}

func (vp *ViewerPaneModel) rebuildContent() {
	if len(vp.lines) == 0 {
		vp.viewport.SetContent("")
		return
	}

	var b strings.Builder

	if !vp.wrapEnabled {
		for i, line := range vp.lines {
			if i > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, gutterFmt, line.num)
			b.WriteString(line.content)
		}
		vp.viewport.SetContent(b.String())
		return
	}

	// Wrapping enabled: wrap content at (viewportWidth - gutterWidth)
	contentWidth := vp.viewport.Width - gutterWidth
	if contentWidth < 1 {
		contentWidth = 1
	}

	for i, line := range vp.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		wrapped := ansi.Hardwrap(line.content, contentWidth, true)
		parts := strings.Split(wrapped, "\n")
		for j, part := range parts {
			if j > 0 {
				b.WriteByte('\n')
			}
			if j == 0 {
				fmt.Fprintf(&b, gutterFmt, line.num)
			} else {
				b.WriteString(blankGutter)
			}
			b.WriteString(part)
		}
	}

	vp.viewport.SetContent(b.String())
}

// View renders the viewer pane.
func (vp *ViewerPaneModel) View(focused bool) string {
	var paneStyle, titleStyle lipgloss.Style
	if focused {
		paneStyle = focusedPaneStyle
		titleStyle = focusedTitleStyle
	} else {
		paneStyle = unfocusedPaneStyle
		titleStyle = unfocusedTitleStyle
	}

	paneStyle = paneStyle.Width(vp.width - 2).Height(vp.height - 2)

	content := paneStyle.Render(vp.viewport.View())
	title := titleStyle.Render(vp.title)
	return placeTitleInBorder(content, title)
}

// sanitizeLine strips control characters (except tab) from a line to prevent
// binary data from corrupting the terminal display.
func sanitizeLine(s string) string {
	clean := true
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b < 0x20 && b != '\t' || b == 0x7F {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	buf := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 0x20 && b != 0x7F || b == '\t' {
			buf = append(buf, b)
		}
	}
	return string(buf)
}

// highlightFilterANSI wraps occurrences of query with ANSI highlight (yellow background).
func highlightFilterANSI(text, query string) string {
	if query == "" {
		return text
	}
	lowerQuery := strings.ToLower(query)
	lowerText := strings.ToLower(text)
	var b strings.Builder
	pos := 0
	for {
		idx := strings.Index(lowerText[pos:], lowerQuery)
		if idx == -1 {
			b.WriteString(text[pos:])
			break
		}
		b.WriteString(text[pos : pos+idx])
		b.WriteString("\033[30;43m") // black on yellow
		b.WriteString(text[pos+idx : pos+idx+len(query)])
		b.WriteString("\033[0m")
		pos += idx + len(query)
	}
	return b.String()
}

// formatLineCount returns the line count formatted with commas.
func formatLineCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 1000 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

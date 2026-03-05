package ui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/rivo/tview"
)

const defaultViewerTitle = " Log Viewer "
const maxLines = 10000

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// drawWriter wraps a tview.TextView as an io.Writer and throttles redraws.
// At most one QueueUpdateDraw is queued at a time. ScrollToEnd runs safely
// inside the queued callback on the main goroutine.
//
// Incoming bytes are buffered so that only complete lines (terminated by '\n')
// are colorized and forwarded to the text view. Any trailing partial line is
// kept in lineBuf until more data arrives.
type drawWriter struct {
	tv           *tview.TextView
	app          *tview.Application
	vp           *ViewerPane
	mu           sync.Mutex
	pending      bool
	pendingLines int
	lineBuf      bytes.Buffer
	lineNumber   int // next line number to assign
}

func (dw *drawWriter) Write(p []byte) (int, error) {
	origLen := len(p)

	dw.mu.Lock()
	dw.lineBuf.Write(p)
	data := dw.lineBuf.String()

	// Find last newline — everything up to it can be colorized.
	lastNL := strings.LastIndex(data, "\n")
	if lastNL == -1 {
		// No complete line yet; keep buffering.
		dw.mu.Unlock()
		return origLen, nil
	}

	complete := data[:lastNL+1] // includes the trailing '\n'
	remainder := data[lastNL+1:]

	dw.lineBuf.Reset()
	dw.lineBuf.WriteString(remainder)
	startLine := dw.lineNumber
	dw.mu.Unlock()

	filter := dw.vp.GetTailFilter()
	result, displayedCount, nextLine := processLines(complete, startLine, filter)

	dw.mu.Lock()
	dw.lineNumber = nextLine
	dw.mu.Unlock()

	if result == "" {
		return origLen, nil
	}

	_, err := io.WriteString(dw.tv, result)

	dw.mu.Lock()
	dw.pendingLines += displayedCount
	if !dw.pending {
		dw.pending = true
		dw.mu.Unlock()
		go dw.app.QueueUpdateDraw(func() {
			dw.tv.ScrollToEnd()
			dw.mu.Lock()
			lines := dw.pendingLines
			dw.pendingLines = 0
			dw.pending = false
			dw.mu.Unlock()
			dw.vp.addLines(lines)
		})
	} else {
		dw.mu.Unlock()
	}

	return origLen, err
}

// processLines processes raw text: assigns original line numbers starting from
// startLine, optionally filters by query, escapes for tview, colorizes, and
// prefixes line numbers. Filtered-out lines still consume line numbers so
// displayed lines reflect their actual file position.
//
// Returns the processed text ready for tview, the count of displayed lines,
// and the next line number (startLine + total input lines).
func processLines(text string, startLine int, filter string) (result string, displayedCount int, nextLine int) {
	lines := strings.Split(text, "\n")
	var buf strings.Builder
	lineNum := startLine
	lowerFilter := strings.ToLower(filter)
	displayed := 0

	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			// Trailing empty string from split on trailing '\n'
			break
		}

		origNum := lineNum
		lineNum++

		// If filter is active and line doesn't match, skip display
		if filter != "" && !strings.Contains(strings.ToLower(line), lowerFilter) {
			continue
		}

		// Escape, highlight, prefix
		escaped := tview.Escape(line)
		if filter != "" {
			escaped = highlightFilter(escaped, filter)
		}

		if displayed > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(fmt.Sprintf("[gray]%5d |[-] %s", origNum, escaped))
		displayed++
	}

	if displayed > 0 {
		buf.WriteByte('\n')
	}

	return buf.String(), displayed, lineNum
}

// ViewerPane displays log file content with live tailing support.
type ViewerPane struct {
	textView *tview.TextView
	app      *tview.Application
	writer   *drawWriter
	flex     *tview.Flex

	// Spinner state
	spinMu     sync.Mutex
	spinCancel context.CancelFunc
	spinBase   string

	// Tail filter state
	tailFilterMu sync.Mutex
	tailFilter   string

	// Line count
	lineCount int
}

func NewViewerPane(app *tview.Application) *ViewerPane {
	vp := &ViewerPane{
		textView: tview.NewTextView(),
		app:      app,
	}

	vp.textView.SetTitle(defaultViewerTitle).SetBorder(true)
	vp.textView.SetDynamicColors(true)
	vp.textView.SetScrollable(true)
	vp.textView.SetWordWrap(true)
	vp.textView.SetMaxLines(maxLines)

	vp.writer = &drawWriter{tv: vp.textView, app: app, vp: vp, lineNumber: 1}

	// Build the flex container — just the textView
	vp.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(vp.textView, 0, 1, true)

	return vp
}

// SetTailFilter sets the active tail filter term. Thread-safe.
func (vp *ViewerPane) SetTailFilter(query string) {
	vp.tailFilterMu.Lock()
	vp.tailFilter = query
	vp.tailFilterMu.Unlock()
}

// GetTailFilter returns the current tail filter term. Thread-safe.
func (vp *ViewerPane) GetTailFilter() string {
	vp.tailFilterMu.Lock()
	defer vp.tailFilterMu.Unlock()
	return vp.tailFilter
}

// highlightFilter wraps every occurrence of query in escaped text with
// yellow-background highlight tags. Case-insensitive; preserves original case.
// The query is escaped so it matches against already-escaped text.
func highlightFilter(escaped, query string) string {
	eq := tview.Escape(query)
	if eq == "" {
		return escaped
	}
	lowerEq := strings.ToLower(eq)
	var buf strings.Builder
	rest := escaped
	for {
		idx := strings.Index(strings.ToLower(rest), lowerEq)
		if idx == -1 {
			buf.WriteString(rest)
			break
		}
		buf.WriteString(rest[:idx])
		buf.WriteString("[black:yellow]")
		buf.WriteString(rest[idx : idx+len(eq)])
		buf.WriteString("[white:-]")
		rest = rest[idx+len(eq):]
	}
	return buf.String()
}

// SetTitle sets a custom title on the viewer pane.
func (vp *ViewerPane) SetTitle(title string) {
	vp.textView.SetTitle(title)
}

// ResetTitle restores the default title.
func (vp *ViewerPane) ResetTitle() {
	vp.textView.SetTitle(defaultViewerTitle)
	vp.lineCount = 0
}

// addLines increments the line count by n, capping at maxLines.
// Must be called on the main goroutine (inside QueueUpdateDraw).
func (vp *ViewerPane) addLines(n int) {
	vp.lineCount += n
	if vp.lineCount > maxLines {
		vp.lineCount = maxLines
	}
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

// StartSpinner starts a spinning animation in the viewer title. The baseTitle
// is shown alongside the spinner character (e.g. "Tailing: file.log").
// Safe to call from any goroutine.
func (vp *ViewerPane) StartSpinner(baseTitle string) {
	vp.spinMu.Lock()
	defer vp.spinMu.Unlock()

	// Stop any existing spinner
	if vp.spinCancel != nil {
		vp.spinCancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	vp.spinCancel = cancel
	vp.spinBase = baseTitle

	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		frame := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				f := frame
				go vp.app.QueueUpdateDraw(func() {
					vp.spinMu.Lock()
					// If spinner was stopped, don't overwrite title
					if vp.spinCancel == nil {
						vp.spinMu.Unlock()
						return
					}
					vp.spinMu.Unlock()
					title := baseTitle
					filter := vp.GetTailFilter()
					if filter != "" {
						title = fmt.Sprintf("%s [filter: %s]", baseTitle, filter)
					}
					if vp.lineCount > 0 {
						title = fmt.Sprintf("%s (%s lines)", title, formatLineCount(vp.lineCount))
					}
					vp.textView.SetTitle(fmt.Sprintf(" %c %s ", spinnerFrames[f%len(spinnerFrames)], title))
				})
				frame++
			}
		}
	}()
}

// StopSpinner stops the spinning animation. Safe to call from any goroutine.
func (vp *ViewerPane) StopSpinner() {
	vp.spinMu.Lock()
	defer vp.spinMu.Unlock()
	if vp.spinCancel != nil {
		vp.spinCancel()
		vp.spinCancel = nil
	}
}

// SetText replaces the current content, escaping brackets for tview safety.
// Applies the active tail filter before display. startLine is the file line
// number of the first line in text (e.g. totalLines - tailLines + 1).
func (vp *ViewerPane) SetText(text string, startLine int) {
	vp.textView.SetTextAlign(tview.AlignLeft)
	vp.textView.Clear()

	filter := vp.GetTailFilter()
	result, displayed, nextLine := processLines(text, startLine, filter)

	vp.writer.mu.Lock()
	vp.writer.lineNumber = nextLine
	vp.writer.mu.Unlock()

	vp.textView.SetText(result)
	vp.textView.ScrollToEnd()
	vp.lineCount = displayed
}

// SetMessage displays a centered message in the viewer (e.g. error state).
func (vp *ViewerPane) SetMessage(msg string) {
	vp.StopSpinner()
	vp.ResetTitle()
	vp.textView.Clear()
	vp.tailFilter = ""
	vp.lineCount = 0
	vp.writer.mu.Lock()
	vp.writer.lineNumber = 1
	vp.writer.mu.Unlock()
	vp.textView.SetTextAlign(tview.AlignCenter)
	vp.textView.SetText("\n\n\n" + msg)
}

// Clear removes all content, stops spinner, resets the title, and clears the filter.
func (vp *ViewerPane) Clear() {
	vp.StopSpinner()
	vp.ResetTitle()
	vp.textView.Clear()
	vp.tailFilter = ""
	vp.lineCount = 0
	vp.writer.mu.Lock()
	vp.writer.lineNumber = 1
	vp.writer.mu.Unlock()
}

// Writer returns an io.Writer that appends to the text view with throttled redraws.
func (vp *ViewerPane) Writer() io.Writer {
	return vp.writer
}

// Widget returns the flex container.
func (vp *ViewerPane) Widget() tview.Primitive {
	return vp.flex
}

// TextView returns the underlying tview.TextView for focus management.
func (vp *ViewerPane) TextView() *tview.TextView {
	return vp.textView
}

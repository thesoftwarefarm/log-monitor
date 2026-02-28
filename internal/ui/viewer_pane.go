package ui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const defaultViewerTitle = " Log Viewer "

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// drawWriter wraps a tview.TextView as an io.Writer and throttles redraws.
// At most one QueueUpdateDraw is queued at a time. ScrollToEnd runs safely
// inside the queued callback on the main goroutine.
//
// Incoming bytes are buffered so that only complete lines (terminated by '\n')
// are colorized and forwarded to the text view. Any trailing partial line is
// kept in lineBuf until more data arrives.
type drawWriter struct {
	tv      *tview.TextView
	app     *tview.Application
	vp      *ViewerPane
	mu      sync.Mutex
	pending bool
	lineBuf bytes.Buffer
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
	dw.mu.Unlock()

	// Escape style tags so tview doesn't misparse them as color tags.
	escaped := tview.Escape(complete)
	_, err := io.WriteString(dw.tv, escaped)

	dw.mu.Lock()
	if !dw.pending {
		dw.pending = true
		dw.mu.Unlock()
		go dw.app.QueueUpdateDraw(func() {
			dw.tv.ScrollToEnd()
			dw.mu.Lock()
			dw.pending = false
			dw.mu.Unlock()
			dw.vp.updateLineCount()
		})
	} else {
		dw.mu.Unlock()
	}

	return origLen, err
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

	// Search state
	searchInput   *tview.InputField
	searchVisible bool
	searchQuery   string
	matchLines    []int
	matchIdx      int

	// Line count
	lineCount int

	// Callback for search status updates (e.g. "Match 3/17")
	onSearchStatus func(string)
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
	vp.textView.SetMaxLines(10000)

	vp.writer = &drawWriter{tv: vp.textView, app: app, vp: vp}

	// Build the search input field
	vp.searchInput = tview.NewInputField()
	vp.searchInput.SetLabel(" /")
	vp.searchInput.SetFieldBackgroundColor(tcell.ColorDarkSlateGray)
	vp.searchInput.SetLabelColor(tcell.ColorYellow)

	vp.searchInput.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			query := vp.searchInput.GetText()
			vp.executeSearch(query)
			vp.HideSearch()
		case tcell.KeyEscape:
			vp.HideSearch()
		}
	})

	// Build the flex container — starts with just the textView
	vp.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(vp.textView, 0, 1, true)

	return vp
}

// SetSearchStatusFunc sets a callback for search status messages (e.g. "Match 3/17").
func (vp *ViewerPane) SetSearchStatusFunc(fn func(string)) {
	vp.onSearchStatus = fn
}

// ShowSearch shows the search input bar and focuses it.
func (vp *ViewerPane) ShowSearch() {
	if vp.searchVisible {
		vp.app.SetFocus(vp.searchInput)
		return
	}
	vp.searchVisible = true
	vp.searchInput.SetText("")
	vp.flex.AddItem(vp.searchInput, 1, 0, false)
	vp.app.SetFocus(vp.searchInput)
}

// HideSearch hides the search input bar and returns focus to the text view.
func (vp *ViewerPane) HideSearch() {
	if !vp.searchVisible {
		return
	}
	vp.searchVisible = false
	vp.flex.RemoveItem(vp.searchInput)
	vp.app.SetFocus(vp.textView)
}

// executeSearch finds all lines containing the query (case-insensitive) and jumps to the first match.
func (vp *ViewerPane) executeSearch(query string) {
	vp.searchQuery = query
	vp.matchLines = nil
	vp.matchIdx = 0

	if query == "" {
		return
	}

	text := vp.textView.GetText(true)
	lines := strings.Split(text, "\n")
	lowerQuery := strings.ToLower(query)

	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), lowerQuery) {
			vp.matchLines = append(vp.matchLines, i)
		}
	}

	if len(vp.matchLines) > 0 {
		vp.matchIdx = 0
		vp.textView.ScrollTo(vp.matchLines[0], 0)
		vp.reportSearchStatus()
	} else {
		if vp.onSearchStatus != nil {
			vp.onSearchStatus("No matches")
		}
	}
}

// NextMatch jumps to the next search match.
func (vp *ViewerPane) NextMatch() {
	if len(vp.matchLines) == 0 {
		return
	}
	vp.matchIdx = (vp.matchIdx + 1) % len(vp.matchLines)
	vp.textView.ScrollTo(vp.matchLines[vp.matchIdx], 0)
	vp.reportSearchStatus()
}

// PrevMatch jumps to the previous search match.
func (vp *ViewerPane) PrevMatch() {
	if len(vp.matchLines) == 0 {
		return
	}
	vp.matchIdx = (vp.matchIdx - 1 + len(vp.matchLines)) % len(vp.matchLines)
	vp.textView.ScrollTo(vp.matchLines[vp.matchIdx], 0)
	vp.reportSearchStatus()
}

func (vp *ViewerPane) reportSearchStatus() {
	if vp.onSearchStatus != nil {
		vp.onSearchStatus(fmt.Sprintf("Match %d/%d", vp.matchIdx+1, len(vp.matchLines)))
	}
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

// updateLineCount counts lines in the text view and updates the spinner title if active.
func (vp *ViewerPane) updateLineCount() {
	text := vp.textView.GetText(true)
	if text == "" {
		vp.lineCount = 0
		return
	}
	vp.lineCount = strings.Count(text, "\n") + 1
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
					if vp.lineCount > 0 {
						title = fmt.Sprintf("%s (%s lines)", baseTitle, formatLineCount(vp.lineCount))
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
func (vp *ViewerPane) SetText(text string) {
	vp.textView.SetTextAlign(tview.AlignLeft)
	vp.textView.Clear()
	vp.textView.SetText(tview.Escape(text))
	vp.textView.ScrollToEnd()
	vp.updateLineCount()
}

// SetMessage displays a centered message in the viewer (e.g. error state).
func (vp *ViewerPane) SetMessage(msg string) {
	vp.StopSpinner()
	vp.ResetTitle()
	vp.textView.Clear()
	vp.searchQuery = ""
	vp.matchLines = nil
	vp.matchIdx = 0
	vp.lineCount = 0
	vp.HideSearch()
	vp.textView.SetTextAlign(tview.AlignCenter)
	vp.textView.SetText("\n\n\n" + msg)
}

// Clear removes all content, stops spinner, and resets the title.
func (vp *ViewerPane) Clear() {
	vp.StopSpinner()
	vp.ResetTitle()
	vp.textView.Clear()
	vp.searchQuery = ""
	vp.matchLines = nil
	vp.matchIdx = 0
	vp.lineCount = 0
	vp.HideSearch()
}

// Writer returns an io.Writer that appends to the text view with throttled redraws.
func (vp *ViewerPane) Writer() io.Writer {
	return vp.writer
}

// Widget returns the flex container (textView + optional search bar).
func (vp *ViewerPane) Widget() tview.Primitive {
	return vp.flex
}

// TextView returns the underlying tview.TextView for focus management.
func (vp *ViewerPane) TextView() *tview.TextView {
	return vp.textView
}

// SearchVisible returns whether the search bar is currently shown.
func (vp *ViewerPane) SearchVisible() bool {
	return vp.searchVisible
}

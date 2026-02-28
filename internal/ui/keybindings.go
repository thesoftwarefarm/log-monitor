package ui

import (
	"fmt"

	"log-monitor/internal/logger"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SetupKeybindings configures global keyboard shortcuts for the application.
func SetupKeybindings(app *tview.Application, a *App) {
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Log every keystroke so we can tell if the event loop is alive.
		if event.Key() == tcell.KeyRune {
			logger.Log("keys", "rune=%c focus=%d", event.Rune(), a.focusIndex)
		} else {
			logger.Log("keys", "key=%s focus=%d", fmt.Sprintf("%v", event.Key()), a.focusIndex)
		}

		// Ctrl-C always quits, even with a modal open.
		if event.Key() == tcell.KeyCtrlC {
			logger.Log("keys", "Ctrl-C → app.Stop()")
			app.Stop()
			return nil
		}

		// Let the modal handle its own input when open.
		if a.HasModalOpen() {
			return event
		}

		switch event.Key() {
		case tcell.KeyTab:
			a.CycleFocus(1)
			return nil
		case tcell.KeyBacktab:
			a.CycleFocus(-1)
			return nil
		case tcell.KeyEscape:
			if a.ClearFocusedFilter() {
				return nil
			}
			a.StopTail()
			return nil
		case tcell.KeyF7:
			a.ShowFilterPrompt()
			return nil
		case tcell.KeyHome:
			if a.FocusedOnViewer() {
				a.viewerPane.TextView().ScrollToBeginning()
				return nil
			}
		case tcell.KeyEnd:
			if a.FocusedOnViewer() {
				a.viewerPane.TextView().ScrollToEnd()
				return nil
			}
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q':
				logger.Log("keys", "q → app.Stop()")
				app.Stop()
				return nil
			}
			// Rune shortcuts only when the viewer pane has focus,
			// so they don't interfere with list widget navigation.
			if a.FocusedOnViewer() {
				switch event.Rune() {
				case 'r':
					a.RefreshFiles()
					return nil
				case 'g':
					a.viewerPane.TextView().ScrollToBeginning()
					return nil
				case 'G':
					a.viewerPane.TextView().ScrollToEnd()
					return nil
				}
			}
		}
		return event
	})
}

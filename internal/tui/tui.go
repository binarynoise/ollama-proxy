package tui

import (
	"fmt"
	"log"
	"strings"
	"time"

	"ollama-proxy/internal/tracker"
	"ollama-proxy/internal/types"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type TUI struct {
	app        *tview.Application
	callList   *tview.List
	detailView *tview.TextView
	logView    *tview.TextView
	statusView *tview.TextView
	flex       *tview.Flex

	tracker    *tracker.CallTracker
	selectedID string
	logChan    chan string
}

func NewTUI(tracker *tracker.CallTracker) *TUI {
	app := tview.NewApplication()

	logView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true).
		SetChangedFunc(func() { app.Draw() })

	t := &TUI{
		app:        app,
		callList:   tview.NewList().ShowSecondaryText(false),
		detailView: tview.NewTextView().SetDynamicColors(true),
		logView:    logView,
		statusView: tview.NewTextView().SetTextAlign(tview.AlignCenter),
		tracker:    tracker,
		logChan:    make(chan string, 1000), // Buffered channel to prevent blocking
	}

	t.setupUI()
	return t
}

func (t *TUI) setupUI() {
	// Configure call list
	t.callList.SetBorder(true).SetTitle(" API Calls ")
	t.callList.SetWrapAround(false)

	// Handle selection changes (both arrow keys and Enter)
	handleSelection := func(index int) {
		if index < 0 || index >= t.callList.GetItemCount() {
			return
		}
		// Get the full ID from the secondary text
		_, secondaryText := t.callList.GetItemText(index)
		if secondaryText != "" && t.selectedID != secondaryText {
			t.selectedID = secondaryText
			t.updateDetailView()
		}
	}

	// Handle Enter key press
	t.callList.SetSelectedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		handleSelection(index)
	})

	// Handle arrow key navigation
	t.callList.SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		handleSelection(index)
	})

	// Configure log view
	t.logView.SetBorder(true).SetTitle(" Log ")
	t.logView.SetScrollable(true).SetWrap(false)
	t.logView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Allow scrolling in log view
		switch event.Key() {
		case tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyHome, tcell.KeyEnd:
			return event
		case tcell.KeyEscape:
			t.app.SetFocus(t.callList)
			t.statusView.SetText("↑/↓: Navigate | Enter: Select | q: Quit")
			return nil
		}
		return event
	})

	// Configure detail view
	t.detailView.SetBorder(true).SetTitle(" Details ")
	t.detailView.SetScrollable(true).SetWrap(true)
	t.detailView.SetChangedFunc(func() {
		t.app.Draw()
	})

	// Configure status view
	t.statusView.SetBorder(false)
	t.statusView.SetText("↑/↓: Navigate | Enter: Select | Tab: Next | Esc: Back to calls | q: Quit")

	// Create the layout
	// Top panel contains call list and detail view side by side
	topPanel := tview.NewFlex()
	topPanel.AddItem(t.callList, 0, 1, true)
	topPanel.AddItem(t.detailView, 0, 2, false)

	// Main layout: top panel on top, log view at bottom
	t.flex = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(topPanel, 0, 1, true).
		AddItem(t.logView, 10, 1, false). // Fixed height for log view
		AddItem(t.statusView, 1, 0, false)

	// Setup logger with our custom writer that updates the UI
	log.SetOutput(&logWriter{tui: t})

	// Set input capture for global shortcuts
	t.flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			// Cycle focus between call list, detail view, and log view
			switch t.app.GetFocus() {
			case t.callList:
				t.app.SetFocus(t.detailView)
				t.statusView.SetText("Tab: Next | Esc: Back to calls | q: Quit")
			case t.detailView:
				t.app.SetFocus(t.logView)
				t.statusView.SetText("↑/↓: Scroll | Tab/Shift+Tab: Navigate | Esc: Back to calls | q: Quit")
			case t.logView:
				t.app.SetFocus(t.callList)
				t.statusView.SetText("↑/↓: Navigate | Enter: Select | Tab: Next | q: Quit")
			}
			return nil
		case tcell.KeyBacktab: // Shift+Tab
			// Cycle focus in reverse order
			switch t.app.GetFocus() {
			case t.callList:
				t.app.SetFocus(t.logView)
				t.statusView.SetText("↑/↓: Scroll | Tab/Shift+Tab: Navigate | Esc: Back to calls | q: Quit")
			case t.detailView:
				t.app.SetFocus(t.callList)
				t.statusView.SetText("↑/↓: Navigate | Enter: Select | Tab: Next | q: Quit")
			case t.logView:
				t.app.SetFocus(t.detailView)
				t.statusView.SetText("Tab: Next | Esc: Back to calls | q: Quit")
			}
			return nil
		case tcell.KeyEscape:
			// Always allow escape to return to call list
			if t.app.GetFocus() != t.callList {
				t.app.SetFocus(t.callList)
				t.statusView.SetText("↑/↓: Navigate | Enter: Select | q: Quit")
				return nil
			}
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q':
				t.app.Stop()
				return nil
			}
		}
		return event
	})
}

func (t *TUI) updateCallList() {
	// Store the current selection
	currentIdx := t.callList.GetCurrentItem()
	var currentID string
	if currentIdx >= 0 && currentIdx < t.callList.GetItemCount() {
		// Get the full ID from the secondary text if available
		_, secondaryText := t.callList.GetItemText(currentIdx)
		assert(secondaryText != "")
		currentID = secondaryText
		// if secondaryText != "" {
		// 	currentID = secondaryText
		// } else if item, _ := t.callList.GetItemText(currentIdx); item != "" {
		// 	// Fallback: extract from display text if secondary text is not available
		// 	if strings.HasPrefix(item, "[") {
		// 		end := strings.Index(item, "]")
		// 		if end > 0 {
		// 			currentID = item[1:end]
		// 		}
		// 	} else {
		// 		parts := strings.Fields(item)
		// 		if len(parts) > 0 {
		// 			currentID = parts[0]
		// 		}
		// 	}
		// }
	}

	t.callList.Clear()
	calls := t.tracker.GetCalls()

	// Add all calls to the list
	for i, call := range calls {
		status := " "
		switch call.Status {
		case types.StatusActive:
			status = "🟢"
		case types.StatusDone:
			status = "✅"
		case types.StatusError:
			status = "❌"
		}

		var duration time.Duration
		if call.Status == types.StatusActive {
			duration = time.Since(call.StartTime).Round(time.Millisecond)
		} else if call.EndTime != nil {
			duration = call.EndTime.Sub(call.StartTime).Round(time.Millisecond)
		}

		shortID := call.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		assert(shortID != "")

		itemText := fmt.Sprintf("[%s[] %s %s %s %s", shortID, status, call.Method, call.Endpoint, duration)
		t.callList.AddItem(
			itemText,
			call.ID, // Store full ID as secondary text for reference
			0,
			nil,
		)
		log.Printf("Rendered call %d: ID='%s', ShortID='%s', Text='%s' (%d chars)", i, call.ID, shortID, itemText, len(itemText))

		// If this was the previously selected item, select it again
		if currentID != "" && strings.HasPrefix(call.ID, currentID) {
			t.callList.SetCurrentItem(i)
			t.selectedID = call.ID
		}
	}

	// If we had a selection but it's no longer valid, select the first item
	if t.callList.GetItemCount() > 0 && (currentID == "" || t.callList.GetCurrentItem() < 0) {
		t.callList.SetCurrentItem(0)
		if len(calls) > 0 {
			t.selectedID = calls[0].ID
			t.updateDetailView()
		}
	}
}

func assert(b bool) {
	if !b {
		panic("assertion failed")
	}
}

func (t *TUI) updateDetailView() {
	if t.selectedID == "" {
		t.detailView.Clear()
		return
	}

	call, exists := t.tracker.GetCall(t.selectedID)
	if !exists {
		t.detailView.SetText("Call not found")
		return
	}

	var sb strings.Builder
	sb.WriteString("[yellow]Request:[white]\n")
	sb.WriteString(call.Request)
	sb.WriteString("\n\n[yellow]Response:[white]\n")
	sb.WriteString(call.Response)

	t.detailView.SetText(sb.String())
	t.detailView.ScrollToEnd()
}

// logProcessor handles log messages in a separate goroutine
func (t *TUI) startLogProcessor() {
	for msg := range t.logChan {
		t.app.QueueUpdateDraw(func() {
			fmt.Fprint(t.logView, msg)
			t.logView.ScrollToEnd()
		})
	}
}

// Write implements io.Writer for logging
type logWriter struct {
	tui *TUI
}

func (lw *logWriter) Write(p []byte) (n int, err error) {
	// Send log message to the channel instead of updating UI directly
	lw.tui.logChan <- string(p)
	return len(p), nil
}

// Write implements io.Writer interface for TUI
func (t *TUI) Write(p []byte) (n int, err error) {
	t.logChan <- string(p)
	return len(p), nil
}

func (t *TUI) Run() error {
	// Start the log processor
	go t.startLogProcessor()

	// Set the app root and run
	t.app.SetRoot(t.flex, true).SetFocus(t.callList)

	// Initial update
	t.updateCallList()

	// Start a goroutine to update the UI
	go func() {
		defer func() {
			close(t.logChan)
		}()
		for event := range t.tracker.Events() {
			t.app.QueueUpdateDraw(func() {
				// Update the call list to show the latest calls
				prevSelected := t.selectedID
				t.updateCallList()

				// If this event is for the currently selected call, update the detail view
				if event.ID == prevSelected || prevSelected == "" {
					t.updateDetailView()
				}

				// If we just added a new call and no call is selected, select the first one
				if t.selectedID == "" && t.callList.GetItemCount() > 0 {
					t.callList.SetCurrentItem(0)
					if item, _ := t.callList.GetItemText(0); item != "" {
						parts := strings.Fields(item)
						if len(parts) > 0 {
							t.selectedID = strings.Trim(parts[0], "[]")
							t.updateDetailView()
						}
					}
				}
			})
		}
	}()

	return t.app.Run()
}

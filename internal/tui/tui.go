package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"ollama-proxy/internal/tracker"
	"ollama-proxy/internal/types"
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
	logMu      sync.RWMutex
	logClosed  bool
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
	t.statusView.SetText("↑/↓: Navigate | Enter: Select | Tab/Shift+Tab: Switch Panel | Esc: Back to Calls | q: Quit")

	// Create the layout
	// Top panel contains call list and detail view side by side
	topPanel := tview.NewFlex()
	// Set fixed width of 30 columns for the call list, then let detail view take remaining space
	topPanel.AddItem(t.callList, 40, 0, true)
	topPanel.AddItem(t.detailView, 0, 1, false)

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
		case tcell.KeyBacktab:
			// Cycle focus between call list, detail view, and log view
			switch t.app.GetFocus() {
			case t.callList:
				t.app.SetFocus(t.detailView)
			case t.detailView:
				t.app.SetFocus(t.logView)
			case t.logView:
				t.app.SetFocus(t.callList)
			}
			return nil
		case tcell.KeyTab: // Shift+Tab
			// Cycle focus in reverse order
			switch t.app.GetFocus() {
			case t.callList:
				t.app.SetFocus(t.logView)
			case t.detailView:
				t.app.SetFocus(t.callList)
			case t.logView:
				t.app.SetFocus(t.detailView)
			}
			return nil
		case tcell.KeyEscape:
			// Always allow escape to return to call list
			if t.app.GetFocus() != t.callList {
				t.app.SetFocus(t.callList)
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
	currentID := t.selectedID
	currentIdx := t.callList.GetCurrentItem()
	followLatest := currentIdx <= 0
	if currentIdx >= 0 && currentIdx < t.callList.GetItemCount() {
		if _, secondary := t.callList.GetItemText(currentIdx); secondary != "" {
			currentID = secondary
		}
	}

	t.callList.Clear()

	calls := t.tracker.GetCalls()
	if len(calls) == 0 {
		t.selectedID = ""
		t.detailView.Clear()
		return
	}

	selectedIdx := 0
	matchFound := false
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

		duration := time.Since(call.StartTime).Round(time.Millisecond)
		if call.Status != types.StatusActive && call.EndTime != nil {
			duration = call.EndTime.Sub(call.StartTime).Round(time.Millisecond)
		}

		shortID := call.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		itemText := fmt.Sprintf("[%s[] %s %s %s %s", shortID, status, call.Method, call.Endpoint, duration)
		t.callList.AddItem(itemText, call.ID, 0, nil)

		if !matchFound && currentID != "" && call.ID == currentID {
			selectedIdx = i
			matchFound = true
		}
	}

	if followLatest || !matchFound {
		selectedIdx = 0
	}

	t.callList.SetCurrentItem(selectedIdx)

	if _, secondary := t.callList.GetItemText(selectedIdx); secondary != "" {
		t.selectedID = secondary
	} else {
		t.selectedID = calls[selectedIdx].ID
	}

	t.updateDetailView()
}

func formatGenerateMessages(request, response string) string {
	var sb strings.Builder

	// Parse the request JSON once
	if strings.TrimSpace(request) != "" {
		var reqData map[string]any
		if err := json.Unmarshal([]byte(request), &reqData); err == nil {
			// Display model if available
			if model, ok := reqData["model"].(string); ok && model != "" {
				sb.WriteString(fmt.Sprintf("[blue]Model:[white] %s\n\n", model))
			}

			// Display prompt
			sb.WriteString("[yellow]Prompt:[white]\n")
			if prompt, ok := reqData["prompt"].(string); ok && prompt != "" {
				sb.WriteString(prompt)
				sb.WriteString("\n")
			} else {
				sb.WriteString(request)
			}
		} else {
			sb.WriteString("[yellow]Prompt:[white]\n")
			sb.WriteString(request)
		}
	} else {
		sb.WriteString("[yellow]Prompt:[white]\n")
		sb.WriteString(request)
	}

	// Parse and display the response
	sb.WriteString("\n\n[yellow]Response:[white]\n")
	if strings.TrimSpace(response) != "" {
		// Handle both single response and streamed responses (one JSON object per line)
		lines := strings.Split(strings.TrimSpace(response), "\n")
		var responseBuilder strings.Builder

		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}

			var respData map[string]any
			if err := json.Unmarshal([]byte(line), &respData); err != nil {
				continue
			}

			// Handle the response format for /api/generate
			if chunk, ok := respData["response"].(string); ok && chunk != "" {
				responseBuilder.WriteString(chunk)
			}
		}

		fullResponse := responseBuilder.String()
		if fullResponse != "" {
			sb.WriteString(fullResponse)
			sb.WriteString("\n")
		} else {
			sb.WriteString(response)
		}
	}

	return sb.String()
}

func formatChatMessages(request, response string) string {
	var sb strings.Builder

	// Parse the request JSON once
	if strings.TrimSpace(request) != "" {
		var reqData map[string]any
		if err := json.Unmarshal([]byte(request), &reqData); err == nil {
			// Display model if available
			if model, ok := reqData["model"].(string); ok && model != "" {
				sb.WriteString(fmt.Sprintf("[blue]Model:[white] %s\n\n", model))
			}

			// Display messages
			sb.WriteString("[yellow]Request:[white]\n")
			if messages, ok := reqData["messages"].([]interface{}); ok {
				for _, msg := range messages {
					if msgMap, ok := msg.(map[string]interface{}); ok {
						role, _ := msgMap["role"].(string)
						content, _ := msgMap["content"].(string)
						if role != "" && content != "" {
							sb.WriteString(fmt.Sprintf("\n# %s\n%s\n", strings.Title(role), content))
						}
					}
				}
			} else {
				sb.WriteString(request)
			}
		} else {
			sb.WriteString(request)
		}
	}

	// Add response
	sb.WriteString("\n\n[yellow]Response:[white]\n")
	if strings.TrimSpace(response) != "" {
		// Handle both single response and streamed responses (one JSON object per line)
		lines := strings.Split(strings.TrimSpace(response), "\n")
		var lastResponse string

		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}

			var respData map[string]any
			if err := json.Unmarshal([]byte(line), &respData); err != nil {
				continue
			}

			// Handle Ollama API response format
			if message, ok := respData["message"].(map[string]any); ok {
				if role, roleOk := message["role"].(string); roleOk && role == "assistant" {
					if content, contentOk := message["content"].(string); contentOk && content != "" {
						if lastResponse == "" {
							lastResponse = content
						} else {
							lastResponse += content
						}
					}
				}
			} else if content, ok := respData["response"].(string); ok && content != "" {
				// Fallback for other response formats
				lastResponse = content
			}
		}

		if lastResponse != "" {
			sb.WriteString(fmt.Sprintf("# Assistant\n%s\n", lastResponse))
		} else {
			sb.WriteString(response)
		}
	}

	return sb.String()
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

	var displayText string
	switch {
	case strings.HasSuffix(call.Endpoint, "/api/chat"):
		displayText = formatChatMessages(call.Request, call.Response)
	case strings.HasSuffix(call.Endpoint, "/api/generate"):
		displayText = formatGenerateMessages(call.Request, call.Response)
	default:
		// Fallback to raw display for other endpoints
		var sb strings.Builder
		sb.WriteString("[yellow]Request:[white]\n")
		sb.WriteString(call.Request)
		sb.WriteString("\n\n[yellow]Response:[white]\n")
		sb.WriteString(call.Response)
		displayText = sb.String()
	}

	t.detailView.SetText(displayText)
	t.detailView.ScrollToEnd()
}

type logWriter struct {
	tui *TUI
}

func (lw *logWriter) Write(p []byte) (n int, err error) {
	return lw.tui.enqueueLog(string(p)), nil
}

func (t *TUI) Write(p []byte) (n int, err error) {
	return t.enqueueLog(string(p)), nil
}

func (t *TUI) enqueueLog(msg string) int {
	t.logMu.RLock()
	defer t.logMu.RUnlock()
	if t.logClosed {
		return len(msg)
	}

	t.logChan <- msg
	return len(msg)
}

func (t *TUI) closeLog() {
	t.logMu.Lock()
	defer t.logMu.Unlock()
	if t.logClosed {
		return
	}

	t.logClosed = true
	close(t.logChan)
}

func (t *TUI) startLogProcessor() {
	for msg := range t.logChan {
		t.app.QueueUpdateDraw(func() {
			fmt.Fprint(t.logView, msg)
			t.logView.ScrollToEnd()
		})
	}
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
		defer t.closeLog()
		for event := range t.tracker.Events() {
			t.app.QueueUpdateDraw(func() {
				// Update the call list to show the latest calls
				prevSelected := t.selectedID
				t.updateCallList()

				// If this event is for the currently selected call, update the detail view
				if event.ID == prevSelected || prevSelected == "" {
					t.updateDetailView()
				}
			})
		}
	}()

	return t.app.Run()
}

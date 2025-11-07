package tracker

import (
	"log"
	"sync"
	"time"

	"ollama-proxy/internal/types"

	"github.com/google/uuid"
)

type CallTracker struct {
	calls     map[string]*types.Call
	maxCalls  int
	mu        sync.RWMutex
	eventChan chan types.Event
}

func NewCallTracker(maxCalls int) *CallTracker {
	return &CallTracker{
		calls:     make(map[string]*types.Call),
		maxCalls:  maxCalls,
		eventChan: make(chan types.Event, 100), // Buffered channel to prevent blocking
	}
}

func (t *CallTracker) NewCall(method, endpoint, request string) *types.Call {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Clean up old calls if we're at capacity
	if len(t.calls) >= t.maxCalls {
		// Find and remove the oldest call
		var oldestID string
		var oldestTime time.Time
		for id, call := range t.calls {
			if oldestTime.IsZero() || call.StartTime.Before(oldestTime) {
				oldestTime = call.StartTime
				oldestID = id
			}
		}
		if oldestID != "" {
			delete(t.calls, oldestID)
		}
	}

	call := &types.Call{
		ID:        uuid.New().String(),
		Method:    method,
		Endpoint:  endpoint,
		Status:    types.StatusActive,
		StartTime: time.Now(),
		Request:   request,
	}

	t.calls[call.ID] = call

	// Send initial event
	t.eventChan <- types.Event{
		ID:   call.ID,
		Data: "",
		Done: false,
	}

	log.Printf("Created new call with ID: %s", call.ID)

	return call
}

func (t *CallTracker) UpdateCall(id, data string) {
	t.mu.RLock()
	call, exists := t.calls[id]
	t.mu.RUnlock()

	if exists {
		call.UpdateResponse(data)
		t.eventChan <- types.Event{
			ID:   id,
			Data: data,
			Done: false,
		}
	}
}

func (t *CallTracker) CompleteCall(id string) {
	t.mu.RLock()
	call, exists := t.calls[id]
	t.mu.RUnlock()

	if exists {
		call.MarkDone()
		t.eventChan <- types.Event{
			ID:   id,
			Data: "",
			Done: true,
		}
	}
}

func (t *CallTracker) ErrorCall(id string) {
	t.mu.RLock()
	call, exists := t.calls[id]
	t.mu.RUnlock()

	if exists {
		call.MarkError()
		t.eventChan <- types.Event{
			ID:   id,
			Data: "Error occurred",
			Done: true,
		}
	}
}

func (t *CallTracker) GetCalls() []*types.Call {
	t.mu.RLock()
	defer t.mu.RUnlock()

	calls := make([]*types.Call, 0, len(t.calls))
	for _, call := range t.calls {
		calls = append(calls, call)
	}

	// Sort by most recent first
	for i := 0; i < len(calls); i++ {
		for j := i + 1; j < len(calls); j++ {
			if calls[i].StartTime.Before(calls[j].StartTime) {
				calls[i], calls[j] = calls[j], calls[i]
			}
		}
	}

	return calls
}

func (t *CallTracker) GetCall(id string) (*types.Call, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	call, exists := t.calls[id]
	return call, exists
}

func (t *CallTracker) Events() <-chan types.Event {
	return t.eventChan
}

func (t *CallTracker) Close() {
	close(t.eventChan)
}

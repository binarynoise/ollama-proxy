package interceptor

import (
	"encoding/json"
	"net/http"
	"sync"

	"ollama-proxy/internal/tracker"
)

// responseForwarder ensures only complete JSON objects are sent to the client
type responseForwarder struct {
	http.ResponseWriter
	callID  string
	tracker *tracker.CallTracker

	mu      sync.Mutex
	errored bool
	buffer  []byte
}

func (r *responseForwarder) CallID() string {
	return r.callID
}

func (r *responseForwarder) MarkError() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.errored {
		return
	}
	r.errored = true
	if r.tracker != nil && r.callID != "" {
		r.tracker.ErrorCall(r.callID)
	}
}

func (r *responseForwarder) Errored() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.errored
}

// WriteHeader captures the status code and marks errors for 4xx/5xx responses
func (r *responseForwarder) WriteHeader(statusCode int) {
	if statusCode >= 400 {
		r.MarkError()
	}
	r.ResponseWriter.WriteHeader(statusCode)
}

// Flush flushes any buffered data to the client
func (r *responseForwarder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// If we have any buffered data, write it to the client
	if len(r.buffer) > 0 {
		r.ResponseWriter.Write(r.buffer)
		r.buffer = nil
	}

	// Flush the underlying writer if it supports it
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Write forwards complete JSON objects from the response data and updates the tracker
func (r *responseForwarder) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Combine buffer with new data
	combined := append(r.buffer, data...)

	// Try to parse the combined data as JSON
	var obj json.RawMessage
	err := json.Unmarshal(combined, &obj)

	switch {
	// If it's valid JSON, write it and clear the buffer
	case err == nil:
		if r.tracker != nil && r.callID != "" {
			r.tracker.UpdateCall(r.callID, string(combined))
		}
		r.buffer = nil // Clear the buffer
		return r.ResponseWriter.Write(combined)

	// If we have a JSON syntax error, buffer the data for next time
	case isJSONErrorRecoverable(err):
		r.buffer = combined
		return len(data), nil

	// For other errors, forward the data as-is
	default:
		r.buffer = nil // Clear the buffer on error
		return r.ResponseWriter.Write(data)
	}
}

// isJSONErrorRecoverable checks if a JSON parsing error might be due to incomplete data
func isJSONErrorRecoverable(err error) bool {
	switch err.Error() {
	case "unexpected end of JSON input":
		return true
	case "unexpected EOF":
		return true
	default:
		return false
	}
}

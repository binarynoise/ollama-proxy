package interceptor

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"ollama-proxy/internal/tracker"
)

// CallAwareResponse represents a response writer associated with a tracked call.
type CallAwareResponse interface {
	http.ResponseWriter
	CallID() string
	MarkError()
	Errored() bool
}

// AsCallAwareResponse attempts to extract a CallAwareResponse from a response writer.
func AsCallAwareResponse(w http.ResponseWriter) (CallAwareResponse, bool) {
	if fw, ok := w.(*responseForwarder); ok {
		return fw, true
	}
	return nil, false
}

// Interceptor handles request/response interception and tracking
type Interceptor struct {
	tracker *tracker.CallTracker
}

// NewInterceptor creates a new interceptor instance
func NewInterceptor(tracker *tracker.CallTracker) *Interceptor {
	return &Interceptor{
		tracker: tracker,
	}
}

// ShouldIntercept determines if a request should be intercepted
func (i *Interceptor) ShouldIntercept(r *http.Request) bool {
	return strings.HasSuffix(r.URL.Path, "/api/chat") || strings.HasSuffix(r.URL.Path, "/api/generate")
}

// InterceptRequest processes the request and returns a response writer that tracks the response
func (i *Interceptor) InterceptRequest(w http.ResponseWriter, r *http.Request) (http.ResponseWriter, *http.Request, string) {
	// Read the full request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return nil, nil, ""
	}

	// Restore the request body for the proxy
	req := r.Clone(r.Context())
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Create a call in the tracker with the captured request body
	call := i.tracker.NewCall(r.Method, r.URL.Path, string(bodyBytes))

	// Create a response forwarder that will track the response
	fw := &responseForwarder{
		ResponseWriter: w,
		callID:         call.ID,
		tracker:        i.tracker,
	}

	return fw, req, call.ID
}

// CompleteCall marks a call as completed
func (i *Interceptor) CompleteCall(callID string) {
	i.tracker.CompleteCall(callID)
}

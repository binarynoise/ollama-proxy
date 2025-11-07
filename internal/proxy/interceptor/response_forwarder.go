package interceptor

import (
	"net/http"

	"ollama-proxy/internal/tracker"
)

// responseForwarder is a minimal ResponseWriter that forwards all data
type responseForwarder struct {
	http.ResponseWriter
	callID  string
	tracker *tracker.CallTracker
}

// Write forwards the response data and updates the tracker
func (r *responseForwarder) Write(b []byte) (int, error) {
	// Forward the data to the original writer first
	n, err := r.ResponseWriter.Write(b)

	// Then update the tracker if needed
	if r.tracker != nil && r.callID != "" && n > 0 && err == nil {
		r.tracker.UpdateCall(r.callID, string(b[:n]))
	}

	return n, err
}

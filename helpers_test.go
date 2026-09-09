package cronitor

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// resetSettings restores the package-level settings to their zero/default
// values so tests do not leak state into each other.
func resetSettings() {
	APIKey = ""
	APIVersion = ""
	Environment = ""
	ConfigPath = ""
	Timeout = DefaultTimeout
	PingURL = DefaultPingURL
	MonitorAPIURL = DefaultMonitorAPIURL
	Logger = log.New(io.Discard, "", 0)
}

// recordedRequest is a snapshot of a request received by the test server.
type recordedRequest struct {
	Method string
	Path   string
	Query  map[string][]string
	Header http.Header
	Body   string
}

// testServer wraps an httptest.Server that records every request it receives
// and points the package base URLs at itself.
type testServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
}

func newTestServer(t *testing.T, handler http.HandlerFunc) *testServer {
	t.Helper()
	ts := &testServer{}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts.mu.Lock()
		ts.requests = append(ts.requests, recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Header: r.Header.Clone(),
			Body:   string(body),
		})
		ts.mu.Unlock()
		if handler != nil {
			r.Body = io.NopCloser(bytes.NewReader(body))
			handler(w, r)
		}
	}))
	resetSettings()
	PingURL = ts.URL + "/p"
	MonitorAPIURL = ts.URL + "/api/monitors"
	t.Cleanup(func() {
		ts.Close()
		resetSettings()
	})
	return ts
}

func (ts *testServer) Requests() []recordedRequest {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	out := make([]recordedRequest, len(ts.requests))
	copy(out, ts.requests)
	return out
}

// captureLog swaps Logger for one writing into a buffer for the test's lifetime.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := Logger
	Logger = log.New(buf, "", 0)
	t.Cleanup(func() { Logger = prev })
	return buf
}

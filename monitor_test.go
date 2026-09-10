package cronitor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const fakeMonitorAPIKey = "cb54ac4fd16142469f2d84fc1bbebd84XXXDEADXXX"

func basicAuth(apiKey string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(apiKey+":"))
}

func jsonHandler(status int, body any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			json.NewEncoder(w).Encode(body) //nolint:errcheck
		}
	}
}

func TestFetchMonitor(t *testing.T) {
	srv := newTestServer(t, jsonHandler(200, map[string]any{
		"key":      "a-test_key",
		"name":     "A Test",
		"type":     "job",
		"schedule": "* * * * *",
		"attributes": map[string]any{
			"code":       "ABC123",
			"group_name": "production",
		},
		"latest_events": []any{
			map[string]any{"event": "run"},
			map[string]any{"event": "complete", "metrics": map[string]any{"duration": 1.5}},
		},
	}))
	APIKey = fakeMonitorAPIKey
	APIVersion = "2020-10-01"

	m := NewMonitor("a-test_key")
	data, err := m.Fetch()
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}

	r := srv.Requests()[0]
	if r.Method != http.MethodGet || r.Path != "/api/monitors/a-test_key" {
		t.Errorf("unexpected request %s %s", r.Method, r.Path)
	}
	if got := r.Header.Get("Authorization"); got != basicAuth(fakeMonitorAPIKey) {
		t.Errorf("want basic auth with api key as username, got %q", got)
	}
	if got := r.Header.Get("User-Agent"); got != "cronitor-go/"+Version {
		t.Errorf("want User-Agent cronitor-go/%s, got %q", Version, got)
	}
	if got := r.Header.Get("Cronitor-Version"); got != "2020-10-01" {
		t.Errorf("want Cronitor-Version header, got %q", got)
	}

	if data.Key() != "a-test_key" || data.Name() != "A Test" || data.Type() != "job" || data.Schedule() != "* * * * *" {
		t.Errorf("typed accessors wrong: %v", data)
	}
	if v, ok := data.Get("attributes", "code"); !ok || v != "ABC123" {
		t.Errorf("nested Get failed: %v %v", v, ok)
	}
	if v := data.GetString("attributes", "group_name"); v != "production" {
		t.Errorf("GetString failed: %q", v)
	}
	if v, ok := data.Get("latest_events", "1", "metrics", "duration"); !ok || v != 1.5 {
		t.Errorf("Get through list failed: %v %v", v, ok)
	}
	if _, ok := data.Get("missing", "path"); ok {
		t.Error("Get on missing path should report !ok")
	}
	if raw := map[string]any(data); raw["name"] != "A Test" {
		t.Error("raw document should be accessible as a map")
	}
	if s := data.String(); !strings.Contains(s, "\n") || !strings.Contains(s, `"key": "a-test_key"`) {
		t.Errorf("String should pretty-print JSON, got %s", s)
	}
}

func TestFetchMonitorNoVersionHeaderWhenUnset(t *testing.T) {
	srv := newTestServer(t, jsonHandler(200, map[string]any{"key": "k"}))
	APIKey = fakeMonitorAPIKey
	if _, err := NewMonitor("k").Fetch(); err != nil {
		t.Fatal(err)
	}
	if _, ok := srv.Requests()[0].Header["Cronitor-Version"]; ok {
		t.Error("Cronitor-Version must be omitted when APIVersion is unset")
	}
}

func TestDataIsCachedAndFetchRefreshes(t *testing.T) {
	srv := newTestServer(t, jsonHandler(200, map[string]any{"key": "k", "name": "N"}))
	APIKey = fakeMonitorAPIKey
	m := NewMonitor("k")
	for i := 0; i < 3; i++ {
		if d, err := m.Data(); err != nil || d.Name() != "N" {
			t.Fatalf("data: %v %v", d, err)
		}
	}
	if n := len(srv.Requests()); n != 1 {
		t.Fatalf("Data should fetch once and cache, got %d requests", n)
	}
	if _, err := m.Fetch(); err != nil {
		t.Fatal(err)
	}
	if n := len(srv.Requests()); n != 2 {
		t.Fatalf("Fetch should always hit the API, got %d requests", n)
	}
}

func TestFetchMonitorNotFound(t *testing.T) {
	newTestServer(t, jsonHandler(404, map[string]any{"detail": "not found"}))
	APIKey = fakeMonitorAPIKey
	_, err := NewMonitor("I don't exist").Data()
	if !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("want ErrMonitorNotFound, got %v", err)
	}
	var nf *MonitorNotFoundError
	if !errors.As(err, &nf) || nf.Key != "I don't exist" {
		t.Fatalf("want MonitorNotFoundError with key, got %v", err)
	}
}

func TestFetchMonitorWithoutAPIKey(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = ""
	_, err := NewMonitor("k").Fetch()
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("want ErrAuthentication, got %v", err)
	}
	if len(srv.Requests()) != 0 {
		t.Fatal("no request should be sent without an api key")
	}
}

func TestFetchMonitorAPIError(t *testing.T) {
	newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden")) //nolint:errcheck
	})
	APIKey = fakeMonitorAPIKey
	_, err := NewMonitor("k").Fetch()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 403 || apiErr.Body != "forbidden" {
		t.Fatalf("want APIError 403, got %v", err)
	}
}

func TestDeleteMonitor(t *testing.T) {
	srv := newTestServer(t, jsonHandler(204, nil))
	APIKey = fakeMonitorAPIKey
	if err := NewMonitor("a-test_key").Delete(); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	r := srv.Requests()[0]
	if r.Method != http.MethodDelete || r.Path != "/api/monitors/a-test_key" {
		t.Errorf("unexpected request %s %s", r.Method, r.Path)
	}
	if r.Header.Get("Authorization") != basicAuth(fakeMonitorAPIKey) {
		t.Error("delete must use basic auth")
	}
}

func TestDeleteMonitorErrors(t *testing.T) {
	newTestServer(t, jsonHandler(404, nil))
	APIKey = fakeMonitorAPIKey
	if err := NewMonitor("k").Delete(); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("want ErrMonitorNotFound, got %v", err)
	}

	newTestServer(t, jsonHandler(403, nil))
	APIKey = fakeMonitorAPIKey
	if err := NewMonitor("k").Delete(); !errors.Is(err, ErrAPI) {
		t.Fatalf("want ErrAPI, got %v", err)
	}
}

func TestPauseAndUnpause(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeMonitorAPIKey
	m := NewMonitor("k")
	if err := m.Pause(24); err != nil {
		t.Fatal(err)
	}
	if err := m.Unpause(); err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	if reqs[0].Method != http.MethodGet || reqs[0].Path != "/api/monitors/k/pause/24" {
		t.Errorf("unexpected pause request %s %s", reqs[0].Method, reqs[0].Path)
	}
	if reqs[1].Path != "/api/monitors/k/pause/0" {
		t.Errorf("unpause should pause for 0 hours, got %s", reqs[1].Path)
	}
	if reqs[0].Header.Get("Authorization") != basicAuth(fakeMonitorAPIKey) {
		t.Error("pause must use basic auth")
	}
	if reqs[0].Header.Get("User-Agent") != "cronitor-go/"+Version {
		t.Error("pause must send User-Agent")
	}
}

func TestPauseWithoutAPIKeyIsNoOp(t *testing.T) {
	srv := newTestServer(t, nil)
	logs := captureLog(t)
	APIKey = ""
	if err := NewMonitor("k").Pause(1); err != nil {
		t.Fatalf("pause without api key must not error, got %v", err)
	}
	if len(srv.Requests()) != 0 || !strings.Contains(logs.String(), "No API key") {
		t.Fatal("pause without api key should log and skip the request")
	}
}

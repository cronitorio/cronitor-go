package cronitor

import (
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	fakeKey    = "d3x0c1"
	fakeAPIKey = "ping-api-key"
)

func TestPingURLAndDefaults(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey

	if err := NewMonitor(fakeKey).Ping(PingOptions{}); err != nil {
		t.Fatalf("ping failed: %v", err)
	}

	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	r := reqs[0]
	if r.Method != http.MethodGet {
		t.Errorf("ping must be GET, got %s", r.Method)
	}
	if want := "/p/" + fakeAPIKey + "/" + fakeKey; r.Path != want {
		t.Errorf("want path %s, got %s", want, r.Path)
	}
	if r.Header.Get("User-Agent") != "cronitor-go/"+Version {
		t.Errorf("want User-Agent cronitor-go, got %q", r.Header.Get("User-Agent"))
	}
	if r.Header.Get("Authorization") != "" {
		t.Errorf("ping must not send an Authorization header")
	}
	host, _ := os.Hostname()
	if got := r.Query.Get("host"); got != host {
		t.Errorf("host should default to machine hostname %q, got %q", host, got)
	}
	stamp, err := strconv.ParseFloat(r.Query.Get("stamp"), 64)
	if err != nil || stamp < 1e9 {
		t.Errorf("stamp should be unix time in float seconds, got %q", r.Query.Get("stamp"))
	}
	for _, absent := range []string{"state", "message", "series", "metric", "env"} {
		if _, ok := r.Query[absent]; ok {
			t.Errorf("unset param %q must be omitted, got %v", absent, r.Query[absent])
		}
	}
}

func TestPingWithAllParams(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey

	m := NewMonitor(fakeKey, WithEnv("staging"))
	err := m.Ping(PingOptions{
		State:   StateRun,
		Host:    "foo",
		Message: "test message",
		Series:  "abc",
		Metrics: map[string]float64{"duration": 100, "count": 5, "error_count": 2},
	})
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}

	q := srv.Requests()[0].Query
	checks := map[string]string{
		"state":   "run",
		"host":    "foo",
		"message": "test message",
		"series":  "abc",
		"env":     "staging",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("param %s: want %q, got %q", k, want, got)
		}
	}
	metrics := append([]string(nil), q["metric"]...)
	sort.Strings(metrics)
	if want := []string{"count:5", "duration:100", "error_count:2"}; strings.Join(metrics, ",") != strings.Join(want, ",") {
		t.Errorf("want metrics %v, got %v", want, metrics)
	}
}

func TestPingUsesPackageEnvironment(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey
	Environment = "production"

	if err := NewMonitor(fakeKey).Ping(PingOptions{State: StateComplete}); err != nil {
		t.Fatal(err)
	}
	if got := srv.Requests()[0].Query.Get("env"); got != "production" {
		t.Errorf("want env=production, got %q", got)
	}
}

func TestPingPerMonitorAPIKeyOverride(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey

	if err := NewMonitor(fakeKey, WithAPIKey("other-key")).Ping(PingOptions{}); err != nil {
		t.Fatal(err)
	}
	if want := "/p/other-key/" + fakeKey; srv.Requests()[0].Path != want {
		t.Errorf("want %s, got %s", want, srv.Requests()[0].Path)
	}
}

func TestPingStates(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey
	m := NewMonitor(fakeKey)

	states := []State{StateRun, StateComplete, StateFail, StateOK}
	for _, s := range states {
		if err := m.Ping(PingOptions{State: s}); err != nil {
			t.Fatalf("ping %s: %v", s, err)
		}
	}
	if err := m.OK(); err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests()
	if len(reqs) != 5 {
		t.Fatalf("expected 5 pings, got %d", len(reqs))
	}
	for i, s := range append(states, StateOK) {
		if got := reqs[i].Query.Get("state"); got != string(s) {
			t.Errorf("ping %d: want state %s, got %s", i, s, got)
		}
	}
}

func TestPingWithoutAPIKeyIsNoOp(t *testing.T) {
	srv := newTestServer(t, nil)
	logs := captureLog(t)
	APIKey = ""

	if err := NewMonitor(fakeKey).Ping(PingOptions{State: StateRun}); err != nil {
		t.Fatalf("ping without api key must not return an error, got %v", err)
	}
	if len(srv.Requests()) != 0 {
		t.Fatal("ping without api key must not send a request")
	}
	if !strings.Contains(logs.String(), "No API key") {
		t.Errorf("expected an error to be logged, got %q", logs.String())
	}
}

func TestPingReturnsAPIErrorOnFailure(t *testing.T) {
	newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	APIKey = fakeAPIKey

	err := NewMonitor(fakeKey).Ping(PingOptions{})
	var apiErr *APIError
	if !asError(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("expected APIError with status 404, got %v", err)
	}
}

func TestFormatMetrics(t *testing.T) {
	got := formatMetrics(map[string]float64{"duration": 1.5, "count": 500})
	sort.Strings(got)
	if strings.Join(got, ",") != "count:500,duration:1.5" {
		t.Errorf("unexpected metrics %v", got)
	}
}

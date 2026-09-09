package cronitor

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func pingsFor(reqs []recordedRequest) []recordedRequest {
	var out []recordedRequest
	for _, r := range reqs {
		if strings.HasPrefix(r.Path, "/p/") {
			out = append(out, r)
		}
	}
	return out
}

func TestJobRunAndComplete(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey

	out, err := Job("ping-decorator-test", func() (any, error) {
		return "done: 42", nil
	})
	if err != nil {
		t.Fatalf("job returned error: %v", err)
	}
	if out != "done: 42" {
		t.Errorf("job should return the function's value, got %v", out)
	}

	pings := pingsFor(srv.Requests())
	if len(pings) != 2 {
		t.Fatalf("expected run + complete pings, got %d", len(pings))
	}
	run, complete := pings[0], pings[1]
	if run.Path != "/p/"+fakeAPIKey+"/ping-decorator-test" {
		t.Errorf("unexpected ping path %s", run.Path)
	}
	if run.Query.Get("state") != "run" {
		t.Errorf("first ping should be run, got %q", run.Query.Get("state"))
	}
	if _, ok := run.Query["metric"]; ok {
		t.Error("run ping must not carry metrics")
	}
	if complete.Query.Get("state") != "complete" {
		t.Errorf("second ping should be complete, got %q", complete.Query.Get("state"))
	}
	if complete.Query.Get("message") != "done: 42" {
		t.Errorf("complete ping should carry the stringified output, got %q", complete.Query.Get("message"))
	}
	series := run.Query.Get("series")
	if series == "" || complete.Query.Get("series") != series {
		t.Errorf("run and complete must share the series %q / %q", series, complete.Query.Get("series"))
	}
	if start, err := strconv.ParseFloat(series, 64); err != nil || start < 1e9 {
		t.Errorf("series should be the start timestamp in float seconds, got %q", series)
	}
	metrics := complete.Query["metric"]
	if len(metrics) != 1 || !strings.HasPrefix(metrics[0], "duration:") {
		t.Fatalf("complete ping should carry a duration metric, got %v", metrics)
	}
	if d, err := strconv.ParseFloat(strings.TrimPrefix(metrics[0], "duration:"), 64); err != nil || d < 0 {
		t.Errorf("duration should be a non-negative float, got %q", metrics[0])
	}
}

func TestJobRunAndFail(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey

	boom := errors.New("something broke")
	_, err := Job("ping-decorator-test", func() (any, error) {
		return nil, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("job should return the function's error, got %v", err)
	}

	pings := pingsFor(srv.Requests())
	if len(pings) != 2 {
		t.Fatalf("expected run + fail pings, got %d", len(pings))
	}
	fail := pings[1]
	if fail.Query.Get("state") != "fail" {
		t.Errorf("second ping should be fail, got %q", fail.Query.Get("state"))
	}
	if fail.Query.Get("message") != "something broke" {
		t.Errorf("fail ping should carry the error text, got %q", fail.Query.Get("message"))
	}
	if fail.Query.Get("series") != pings[0].Query.Get("series") {
		t.Error("run and fail must share the series")
	}
	if m := fail.Query["metric"]; len(m) != 1 || !strings.HasPrefix(m[0], "duration:") {
		t.Errorf("fail ping should carry a duration metric, got %v", m)
	}
}

func TestJobPanicSendsFailAndRepanics(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("panic should be re-raised")
		}
		pings := pingsFor(srv.Requests())
		if len(pings) != 2 || pings[1].Query.Get("state") != "fail" || !strings.Contains(pings[1].Query.Get("message"), "kaboom") {
			t.Errorf("panic should send a fail ping with the panic value, got %+v", pings)
		}
	}()
	Job("ping-decorator-test", func() (any, error) { panic("kaboom") }) //nolint:errcheck
}

func TestJobLogOutputDisabled(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey

	if _, err := Job("k", func() (any, error) { return "secret", nil }, WithLogOutput(false)); err != nil {
		t.Fatal(err)
	}
	if _, ok := pingsFor(srv.Requests())[1].Query["message"]; ok {
		t.Error("message must be omitted when log output is disabled")
	}
}

func TestJobNilOutputHasNoMessage(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey
	if _, err := Job("k", func() (any, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if _, ok := pingsFor(srv.Requests())[1].Query["message"]; ok {
		t.Error("nil output should not produce a message")
	}
}

func TestJobWithEnv(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey
	if _, err := Job("k", func() (any, error) { return nil, nil }, WithJobEnv("staging")); err != nil {
		t.Fatal(err)
	}
	for _, p := range pingsFor(srv.Requests()) {
		if p.Query.Get("env") != "staging" {
			t.Errorf("ping should carry env=staging, got %v", p.Query)
		}
	}
}

func TestJobWithoutAPIKeyStillRunsFunction(t *testing.T) {
	srv := newTestServer(t, nil)
	captureLog(t)
	APIKey = ""
	called := false
	if _, err := Job("k", func() (any, error) { called = true; return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if !called || len(srv.Requests()) != 0 {
		t.Error("function should run and no pings should be sent without an api key")
	}
}

func TestRunJobGeneric(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey
	n, err := RunJob("k", func() (int, error) { return 7, nil })
	if err != nil || n != 7 {
		t.Fatalf("RunJob should return typed value, got %v %v", n, err)
	}
	if got := pingsFor(srv.Requests())[1].Query.Get("message"); got != "7" {
		t.Errorf("message should be the stringified value, got %q", got)
	}
}

func TestJobAttributesAreQueuedAndSynced(t *testing.T) {
	srv := newTestServer(t, jsonHandler(200, map[string]any{"monitors": []any{map[string]any{"key": "ping-decorator-test"}}}))
	APIKey = fakeAPIKey
	t.Cleanup(clearMonitorAttributes)

	_, err := Job("ping-decorator-test", func() (any, error) { return nil, nil },
		WithAttributes(map[string]any{"name": "Ping Decorator Test", "schedule": "0 8 * * *"}))
	if err != nil {
		t.Fatal(err)
	}
	if queued := QueuedMonitorAttributes(); len(queued) != 1 || queued[0]["key"] != "ping-decorator-test" {
		t.Fatalf("attributes should be queued with the key, got %v", queued)
	}

	if err := SyncMonitors(); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	var put *recordedRequest
	for i := range srv.Requests() {
		if r := srv.Requests()[i]; r.Method == http.MethodPut {
			put = &r
		}
	}
	if put == nil {
		t.Fatal("SyncMonitors should PUT the queued attributes")
	}
	var payload map[string]any
	json.Unmarshal([]byte(put.Body), &payload) //nolint:errcheck
	list, _ := payload["monitors"].([]any)
	if len(list) != 1 {
		t.Fatalf("unexpected payload %v", payload)
	}
	got := list[0].(map[string]any)
	if got["key"] != "ping-decorator-test" || got["name"] != "Ping Decorator Test" || got["schedule"] != "0 8 * * *" {
		t.Errorf("unexpected monitor attributes %v", got)
	}
	if len(QueuedMonitorAttributes()) != 0 {
		t.Error("queue should be cleared after a successful sync")
	}

	// Nothing queued: no request is made.
	before := len(srv.Requests())
	if err := SyncMonitors(); err != nil {
		t.Fatal(err)
	}
	if len(srv.Requests()) != before {
		t.Error("SyncMonitors with an empty queue must not make a request")
	}
}

func TestJobAttributesAreNotDuplicated(t *testing.T) {
	newTestServer(t, nil)
	APIKey = fakeAPIKey
	t.Cleanup(clearMonitorAttributes)
	for i := 0; i < 3; i++ {
		Job("k", func() (any, error) { return nil, nil }, WithAttributes(map[string]any{"name": "K"})) //nolint:errcheck
	}
	if n := len(QueuedMonitorAttributes()); n != 1 {
		t.Errorf("attributes for the same key should be queued once, got %d", n)
	}
}

func TestNewJobReturnsReusableFunc(t *testing.T) {
	srv := newTestServer(t, nil)
	APIKey = fakeAPIKey
	fn := NewJob("k", func() (any, error) { return "x", nil })
	for i := 0; i < 2; i++ {
		if _, err := fn(); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(pingsFor(srv.Requests())); n != 4 {
		t.Errorf("each invocation should ping run + complete, got %d pings", n)
	}
}

func TestNewJobQueuesAttributesBeforeFirstRun(t *testing.T) {
	t.Cleanup(clearMonitorAttributes)

	_ = NewJob("new-job-attrs", func() (any, error) { return nil, nil },
		WithAttributes(map[string]any{"schedule": "0 8 * * *"}))

	queued := QueuedMonitorAttributes()
	if len(queued) != 1 || queued[0]["key"] != "new-job-attrs" || queued[0]["schedule"] != "0 8 * * *" {
		t.Fatalf("expected attributes queued at wrap time, got %v", queued)
	}
}

package cronitor

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var (
	testMonitor = map[string]any{
		"type":       "job",
		"key":        "a-test_key",
		"schedule":   "* * * * *",
		"assertions": []any{"metric.duration < 10 seconds"},
	}
	testMonitor2 = map[string]any{
		"type":       "job",
		"key":        "another-test-key",
		"schedule":   "* * * * *",
		"assertions": []any{"metric.duration < 10 seconds"},
	}
)

func TestPutMonitors(t *testing.T) {
	srv := newTestServer(t, jsonHandler(200, map[string]any{"monitors": []any{testMonitor, testMonitor2}}))
	APIKey = fakeMonitorAPIKey
	APIVersion = "2020-10-01"

	monitors, err := PutMonitors([]map[string]any{testMonitor, testMonitor2}, PutOptions{})
	if err != nil {
		t.Fatalf("put failed: %v", err)
	}
	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}
	keys := []string{}
	for _, m := range monitors {
		d, _ := m.Data()
		keys = append(keys, m.Key+"="+d.Key())
	}
	if strings.Join(keys, ",") != "a-test_key=a-test_key,another-test-key=another-test-key" {
		t.Errorf("returned monitors should carry their data, got %v", keys)
	}

	r := srv.Requests()[0]
	if r.Method != http.MethodPut || r.Path != "/api/monitors" {
		t.Errorf("unexpected request %s %s", r.Method, r.Path)
	}
	if r.Header.Get("Content-Type") != "application/json" {
		t.Errorf("want json content type, got %q", r.Header.Get("Content-Type"))
	}
	if r.Header.Get("Authorization") != basicAuth(fakeMonitorAPIKey) {
		t.Error("put must use basic auth")
	}
	if r.Header.Get("Cronitor-Version") != "2020-10-01" || r.Header.Get("User-Agent") != "cronitor-go" {
		t.Error("put must send Cronitor-Version and User-Agent")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(r.Body), &payload); err != nil {
		t.Fatalf("body is not json: %v", err)
	}
	if _, ok := payload["rollback"]; ok {
		t.Error("rollback must be omitted unless requested")
	}
	list, _ := payload["monitors"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["key"] != "a-test_key" {
		t.Errorf("unexpected payload %v", payload)
	}
}

func TestPutMonitorsWithRollback(t *testing.T) {
	srv := newTestServer(t, jsonHandler(200, map[string]any{"monitors": []any{testMonitor}}))
	APIKey = fakeMonitorAPIKey

	if _, err := PutMonitors([]map[string]any{testMonitor}, PutOptions{Rollback: true}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	json.Unmarshal([]byte(srv.Requests()[0].Body), &payload) //nolint:errcheck
	if payload["rollback"] != true {
		t.Errorf("rollback: true should be in the payload, got %v", payload)
	}
}

func TestPutMonitorsOptionsOverride(t *testing.T) {
	srv := newTestServer(t, jsonHandler(200, map[string]any{"monitors": []any{}}))
	APIKey = fakeMonitorAPIKey
	if _, err := PutMonitors(nil, PutOptions{APIKey: "override", APIVersion: "2021-01-01"}); err != nil {
		t.Fatal(err)
	}
	r := srv.Requests()[0]
	if r.Header.Get("Authorization") != basicAuth("override") || r.Header.Get("Cronitor-Version") != "2021-01-01" {
		t.Error("PutOptions should override api key and version")
	}
}

func TestPutMonitorsValidationError(t *testing.T) {
	newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"schedule": ["invalid"]}`)) //nolint:errcheck
	})
	APIKey = fakeMonitorAPIKey
	_, err := PutMonitors([]map[string]any{{"schedule": "* * * * *"}}, PutOptions{})
	var ve *APIValidationError
	if !errors.As(err, &ve) || !strings.Contains(ve.Body, "invalid") {
		t.Fatalf("want APIValidationError with body, got %v", err)
	}
	if !errors.Is(err, ErrAPIValidation) {
		t.Fatal("should match ErrAPIValidation")
	}
}

func TestPutMonitorsAPIError(t *testing.T) {
	newTestServer(t, jsonHandler(403, nil))
	APIKey = fakeMonitorAPIKey
	if _, err := PutMonitors([]map[string]any{testMonitor}, PutOptions{}); !errors.Is(err, ErrAPI) {
		t.Fatalf("want ErrAPI, got %v", err)
	}
}

func TestPutMonitorsYAML(t *testing.T) {
	doc := map[string]any{
		"jobs": map[string]any{
			"a-test_key":       map[string]any{"schedule": "* * * * *"},
			"another-test-key": map[string]any{"schedule": "0 * * * *"},
		},
	}
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		yaml.NewEncoder(w).Encode(doc) //nolint:errcheck
	})
	APIKey = fakeMonitorAPIKey

	out, err := PutMonitorsYAML(doc, PutOptions{Rollback: true})
	if err != nil {
		t.Fatalf("put yaml failed: %v", err)
	}
	jobs, _ := out["jobs"].(map[string]any)
	if _, ok := jobs["a-test_key"]; !ok {
		t.Errorf("response yaml should be parsed, got %v", out)
	}
	if _, ok := jobs["another-test-key"]; !ok {
		t.Errorf("response yaml should be parsed, got %v", out)
	}

	r := srv.Requests()[0]
	if r.Method != http.MethodPut || r.Path != "/api/monitors.yaml" {
		t.Errorf("unexpected request %s %s", r.Method, r.Path)
	}
	if r.Header.Get("Content-Type") != "application/yaml" {
		t.Errorf("want yaml content type, got %q", r.Header.Get("Content-Type"))
	}
	var sent map[string]any
	if err := yaml.Unmarshal([]byte(r.Body), &sent); err != nil {
		t.Fatalf("body is not yaml: %v", err)
	}
	if sent["rollback"] != true {
		t.Errorf("rollback: true should be in the yaml body, got %v", sent)
	}
	if _, ok := sent["jobs"].(map[string]any)["a-test_key"]; !ok {
		t.Errorf("jobs missing from yaml body: %v", sent)
	}
}

func TestPutMonitorsYAMLDoesNotMutateInput(t *testing.T) {
	newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("jobs: {}\n")) }) //nolint:errcheck
	APIKey = fakeMonitorAPIKey
	doc := map[string]any{"jobs": map[string]any{}}
	if _, err := PutMonitorsYAML(doc, PutOptions{Rollback: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["rollback"]; ok {
		t.Error("input document must not be mutated")
	}
}

func TestPutMonitorsYAMLValidationError(t *testing.T) {
	newTestServer(t, jsonHandler(400, map[string]any{"detail": "bad"}))
	APIKey = fakeMonitorAPIKey
	if _, err := PutMonitorsYAML(map[string]any{"jobs": map[string]any{}}, PutOptions{}); !errors.Is(err, ErrAPIValidation) {
		t.Fatalf("want ErrAPIValidation, got %v", err)
	}
}

func TestAsYAML(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("jobs:\n  a-test_key:\n    schedule: '* * * * *'\n")) //nolint:errcheck
	})
	APIKey = fakeMonitorAPIKey
	APIVersion = "2020-10-01"

	text, err := AsYAML()
	if err != nil {
		t.Fatalf("as yaml failed: %v", err)
	}
	if !strings.HasPrefix(text, "jobs:") {
		t.Errorf("unexpected yaml %q", text)
	}
	r := srv.Requests()[0]
	if r.Method != http.MethodGet || r.Path != "/api/monitors.yaml" {
		t.Errorf("unexpected request %s %s", r.Method, r.Path)
	}
	if r.Header.Get("Authorization") != basicAuth(fakeMonitorAPIKey) || r.Header.Get("Cronitor-Version") != "2020-10-01" {
		t.Error("as yaml must send basic auth and Cronitor-Version")
	}
}

func TestAsYAMLAPIError(t *testing.T) {
	newTestServer(t, jsonHandler(500, nil))
	APIKey = fakeMonitorAPIKey
	if _, err := AsYAML(); !errors.Is(err, ErrAPI) {
		t.Fatalf("want ErrAPI, got %v", err)
	}
}

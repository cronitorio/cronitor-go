package cronitor

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const yamlPath = "testdata/cronitor.yaml"

func TestReadConfig(t *testing.T) {
	t.Cleanup(resetSettings)
	data, err := ReadConfig(yamlPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	for _, key := range []string{"jobs", "checks", "heartbeats"} {
		if _, ok := data[key]; !ok {
			t.Errorf("config should contain %s: %v", key, data)
		}
	}
	if ConfigPath != yamlPath {
		t.Errorf("ReadConfig should set ConfigPath, got %q", ConfigPath)
	}
}

func TestReadConfigUsesConfigPath(t *testing.T) {
	t.Cleanup(resetSettings)
	ConfigPath = yamlPath
	if _, err := ReadConfig(""); err != nil {
		t.Fatalf("ReadConfig should fall back to ConfigPath: %v", err)
	}
}

func TestReadConfigWithoutPath(t *testing.T) {
	t.Cleanup(resetSettings)
	ConfigPath = ""
	if _, err := ReadConfig(""); !errors.Is(err, ErrConfigValidation) {
		t.Fatalf("want ErrConfigValidation, got %v", err)
	}
}

func TestReadConfigMissingFile(t *testing.T) {
	t.Cleanup(resetSettings)
	_, err := ReadConfig("testdata/does-not-exist.yaml")
	if !errors.Is(err, ErrConfigValidation) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want ErrConfigValidation wrapping os.ErrNotExist, got %v", err)
	}
}

func TestValidateConfig(t *testing.T) {
	raw, _ := os.ReadFile(yamlPath)
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write(raw) }) //nolint:errcheck
	APIKey = fakeMonitorAPIKey
	ConfigPath = yamlPath
	logs := captureLog(t)

	if err := ValidateConfig(); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	r := srv.Requests()[0]
	if r.Method != http.MethodPut || r.Path != "/api/monitors.yaml" || r.Header.Get("Content-Type") != "application/yaml" {
		t.Errorf("unexpected request %s %s %s", r.Method, r.Path, r.Header.Get("Content-Type"))
	}
	var sent map[string]any
	yaml.Unmarshal([]byte(r.Body), &sent) //nolint:errcheck
	if sent["rollback"] != true {
		t.Error("validate must send rollback: true")
	}
	if _, ok := sent["jobs"].(map[string]any)["replenishment-report"]; !ok {
		t.Errorf("config jobs should be sent, got %v", sent)
	}
	if !strings.Contains(logs.String(), "5 monitors validated.") {
		t.Errorf("expected validated count to be logged, got %q", logs.String())
	}
}

func TestApplyConfig(t *testing.T) {
	raw, _ := os.ReadFile(yamlPath)
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write(raw) }) //nolint:errcheck
	APIKey = fakeMonitorAPIKey
	ConfigPath = yamlPath
	logs := captureLog(t)

	if err := ApplyConfig(); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	var sent map[string]any
	yaml.Unmarshal([]byte(srv.Requests()[0].Body), &sent) //nolint:errcheck
	if _, ok := sent["rollback"]; ok {
		t.Error("apply must not send rollback")
	}
	if !strings.Contains(logs.String(), "5 monitors synced.") {
		t.Errorf("expected synced count to be logged, got %q", logs.String())
	}
}

func TestApplyConfigSingularLogMessage(t *testing.T) {
	newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("jobs:\n  only:\n    schedule: '* * * * *'\n"))
	}) //nolint:errcheck
	APIKey = fakeMonitorAPIKey
	ConfigPath = yamlPath
	logs := captureLog(t)
	if err := ApplyConfig(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "1 monitor synced.") {
		t.Errorf("expected singular message, got %q", logs.String())
	}
}

func TestApplyConfigWithoutPath(t *testing.T) {
	newTestServer(t, nil)
	APIKey = fakeMonitorAPIKey
	ConfigPath = ""
	if err := ApplyConfig(); !errors.Is(err, ErrConfigValidation) {
		t.Fatalf("want ErrConfigValidation, got %v", err)
	}
}

func TestApplyConfigValidationError(t *testing.T) {
	newTestServer(t, jsonHandler(400, map[string]any{"detail": "bad schedule"}))
	APIKey = fakeMonitorAPIKey
	ConfigPath = yamlPath
	logs := captureLog(t)
	err := ApplyConfig()
	if !errors.Is(err, ErrAPIValidation) {
		t.Fatalf("want ErrAPIValidation, got %v", err)
	}
	if !strings.Contains(logs.String(), "bad schedule") {
		t.Errorf("error should be logged, got %q", logs.String())
	}
}

func TestGenerateConfig(t *testing.T) {
	body := "jobs:\n  a-test_key:\n    schedule: '* * * * *'\n"
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }) //nolint:errcheck
	APIKey = fakeMonitorAPIKey
	ConfigPath = filepath.Join(t.TempDir(), "cronitor.yaml")

	if err := GenerateConfig(); err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	r := srv.Requests()[0]
	if r.Method != http.MethodGet || r.Path != "/api/monitors.yaml" {
		t.Errorf("unexpected request %s %s", r.Method, r.Path)
	}
	written, err := os.ReadFile(ConfigPath)
	if err != nil || string(written) != body {
		t.Errorf("config file should contain the yaml, got %q (%v)", written, err)
	}
}

func TestGenerateConfigDefaultsToCronitorYAML(t *testing.T) {
	newTestServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("jobs: {}\n")) }) //nolint:errcheck
	APIKey = fakeMonitorAPIKey
	ConfigPath = ""
	dir := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) }) //nolint:errcheck

	if err := GenerateConfig(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cronitor.yaml")); err != nil {
		t.Errorf("expected ./cronitor.yaml to be written: %v", err)
	}
}

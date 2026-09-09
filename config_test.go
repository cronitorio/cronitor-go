package cronitor

import (
	"testing"
	"time"
)

func TestLoadEnvDefaults(t *testing.T) {
	t.Setenv("CRONITOR_API_KEY", "")
	t.Setenv("CRONITOR_API_VERSION", "")
	t.Setenv("CRONITOR_ENVIRONMENT", "")
	t.Setenv("CRONITOR_TIMEOUT", "")
	t.Setenv("CRONITOR_CONFIG", "")
	LoadEnv()
	if APIKey != "" || APIVersion != "" || Environment != "" || ConfigPath != "" {
		t.Fatalf("expected empty settings, got %q %q %q %q", APIKey, APIVersion, Environment, ConfigPath)
	}
	if Timeout != 10*time.Second {
		t.Fatalf("default timeout should be 10s, got %v", Timeout)
	}
}

func TestLoadEnvReadsVariables(t *testing.T) {
	t.Setenv("CRONITOR_API_KEY", "env-key")
	t.Setenv("CRONITOR_API_VERSION", "2020-10-01")
	t.Setenv("CRONITOR_ENVIRONMENT", "staging")
	t.Setenv("CRONITOR_TIMEOUT", "30")
	t.Setenv("CRONITOR_CONFIG", "./testdata/cronitor.yaml")
	LoadEnv()
	t.Cleanup(func() { resetSettings() })

	if APIKey != "env-key" || APIVersion != "2020-10-01" || Environment != "staging" {
		t.Fatalf("settings not loaded from env: %q %q %q", APIKey, APIVersion, Environment)
	}
	if Timeout != 30*time.Second {
		t.Fatalf("timeout should be 30s, got %v", Timeout)
	}
	if ConfigPath != "./testdata/cronitor.yaml" {
		t.Fatalf("config path not loaded: %q", ConfigPath)
	}
}

func TestSettingsWritableInCode(t *testing.T) {
	t.Cleanup(func() { resetSettings() })
	APIKey = "code-key"
	Timeout = 2 * time.Second
	if APIKey != "code-key" || Timeout != 2*time.Second {
		t.Fatal("settings should be writable")
	}
}

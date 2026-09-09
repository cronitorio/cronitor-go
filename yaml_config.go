package cronitor

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath is used by GenerateConfig when ConfigPath is unset.
const DefaultConfigPath = "./cronitor.yaml"

// ReadConfig parses the cronitor.yaml file at path and returns its contents.
// When path is empty the package-level ConfigPath is used; otherwise
// ConfigPath is updated to path. A missing path or unreadable file yields a
// ConfigValidationError.
func ReadConfig(path string) (map[string]any, error) {
	if path == "" {
		path = ConfigPath
	}
	if path == "" {
		return nil, &ConfigValidationError{Message: "must include a path to config file e.g. cronitor.ReadConfig(\"./cronitor.yaml\")"}
	}
	ConfigPath = path

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, &ConfigValidationError{Message: fmt.Sprintf("could not read config file %q", path), Err: err}
	}
	var data map[string]any
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return nil, &ConfigValidationError{Message: fmt.Sprintf("could not parse config file %q", path), Err: err}
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

// ValidateConfig sends the monitors in ConfigPath to Cronitor for validation
// without persisting them (a PUT with rollback: true).
func ValidateConfig() error {
	return syncConfig(true)
}

// ApplyConfig syncs the monitors in ConfigPath to Cronitor.
func ApplyConfig() error {
	return syncConfig(false)
}

func syncConfig(rollback bool) error {
	if ConfigPath == "" {
		return &ConfigValidationError{Message: "must set a path to config file e.g. cronitor.ConfigPath = \"./cronitor.yaml\""}
	}
	config, err := ReadConfig(ConfigPath)
	if err != nil {
		Logger.Print(err)
		return err
	}

	monitors, err := PutMonitorsYAML(config, PutOptions{Rollback: rollback})
	if err != nil {
		Logger.Print(err)
		return err
	}

	total := 0
	for _, section := range []string{"jobs", "checks", "heartbeats"} {
		if entries, ok := monitors[section].(map[string]any); ok {
			total += len(entries)
		}
	}
	plural := "s"
	if total == 1 {
		plural = ""
	}
	verb := "synced"
	if rollback {
		verb = "validated"
	}
	Logger.Printf("%d monitor%s %s.", total, plural, verb)
	return nil
}

// GenerateConfig downloads the account's monitors with AsYAML and writes them
// to ConfigPath (or ./cronitor.yaml when unset).
func GenerateConfig() error {
	path := ConfigPath
	if path == "" {
		path = DefaultConfigPath
	}
	text, err := AsYAML()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return &ConfigValidationError{Message: fmt.Sprintf("could not write config file %q", path), Err: err}
	}
	return nil
}

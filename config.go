package cronitor

import (
	"log"
	"os"
	"strconv"
	"time"
)

// Package-level settings. Each is initialised from the matching environment
// variable when the package is loaded and can be overridden in code.
var (
	// APIKey authenticates requests to Cronitor (CRONITOR_API_KEY).
	APIKey string
	// APIVersion pins the Monitor API version, sent as the Cronitor-Version
	// header when set (CRONITOR_API_VERSION).
	APIVersion string
	// Environment is sent as the `env` ping parameter when set
	// (CRONITOR_ENVIRONMENT).
	Environment string
	// Timeout applies to Monitor API calls (CRONITOR_TIMEOUT, in seconds).
	// Defaults to 10 seconds.
	Timeout = DefaultTimeout
	// ConfigPath is the path to a cronitor.yaml file (CRONITOR_CONFIG).
	ConfigPath string

	// PingURL is the base URL of the telemetry API. Pings are sent to
	// {PingURL}/{api_key}/{monitor_key}. Override it to point at a test server.
	PingURL = DefaultPingURL
	// MonitorAPIURL is the base URL of the Monitor API. Override it to point
	// at a test server.
	MonitorAPIURL = DefaultMonitorAPIURL

	// Logger receives diagnostic messages. Set it to log.New(io.Discard, "", 0)
	// to silence the package.
	Logger = log.New(os.Stderr, "cronitor: ", log.LstdFlags)
)

// Defaults for the package-level settings.
const (
	DefaultTimeout       = 10 * time.Second
	DefaultPingTimeout   = 5 * time.Second
	DefaultPingURL       = "https://cronitor.link/p"
	DefaultMonitorAPIURL = "https://cronitor.io/api/monitors"
)

func init() {
	LoadEnv()
}

// LoadEnv (re)reads the package settings from the CRONITOR_* environment
// variables. It is called automatically when the package is initialised.
func LoadEnv() {
	APIKey = os.Getenv("CRONITOR_API_KEY")
	APIVersion = os.Getenv("CRONITOR_API_VERSION")
	Environment = os.Getenv("CRONITOR_ENVIRONMENT")
	ConfigPath = os.Getenv("CRONITOR_CONFIG")
	Timeout = DefaultTimeout
	if raw := os.Getenv("CRONITOR_TIMEOUT"); raw != "" {
		if secs, err := strconv.ParseFloat(raw, 64); err == nil && secs > 0 {
			Timeout = time.Duration(secs * float64(time.Second))
		} else {
			Logger.Printf("ignoring invalid CRONITOR_TIMEOUT %q", raw)
		}
	}
}

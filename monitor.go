package cronitor

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// State is the lifecycle state reported with a telemetry ping.
type State string

// Ping states. Run, Complete and Fail measure the lifecycle of a job; OK is
// used to manually reset a monitor to a passing state.
const (
	StateRun      State = "run"
	StateComplete State = "complete"
	StateFail     State = "fail"
	StateOK       State = "ok"
)

// PingOptions are the optional parameters of a telemetry ping. Unset fields
// are omitted from the request. See
// https://cronitor.io/docs/telemetry-api#parameters for details.
type PingOptions struct {
	// State is one of StateRun, StateComplete, StateFail or StateOK.
	State State
	// Message is displayed in alerts and in the monitor activity panel.
	Message string
	// Series correlates run/complete/fail pings of the same job invocation.
	Series string
	// Host defaults to the machine hostname.
	Host string
	// Metrics are sent as repeated "metric=name:value" parameters, for
	// example duration, count and error_count.
	Metrics map[string]float64
}

// Monitor is a handle on a single Cronitor monitor identified by its key.
type Monitor struct {
	Key string

	apiKey     string
	apiVersion string
	env        string
}

// MonitorOption customises a Monitor created with NewMonitor.
type MonitorOption func(*Monitor)

// WithAPIKey overrides the package-level APIKey for this monitor.
func WithAPIKey(apiKey string) MonitorOption {
	return func(m *Monitor) { m.apiKey = apiKey }
}

// WithAPIVersion overrides the package-level APIVersion for this monitor.
func WithAPIVersion(apiVersion string) MonitorOption {
	return func(m *Monitor) { m.apiVersion = apiVersion }
}

// WithEnv overrides the package-level Environment for this monitor.
func WithEnv(env string) MonitorOption {
	return func(m *Monitor) { m.env = env }
}

// NewMonitor returns a Monitor for the given key. Options override the
// package-level api key, api version and environment for this instance.
func NewMonitor(key string, opts ...MonitorOption) *Monitor {
	m := &Monitor{Key: key}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// APIKey returns the api key used by this monitor, falling back to the
// package-level setting.
func (m *Monitor) APIKey() string {
	if m.apiKey != "" {
		return m.apiKey
	}
	return APIKey
}

// APIVersion returns the api version used by this monitor, falling back to
// the package-level setting.
func (m *Monitor) APIVersion() string {
	if m.apiVersion != "" {
		return m.apiVersion
	}
	return APIVersion
}

// Env returns the environment used by this monitor, falling back to the
// package-level setting.
func (m *Monitor) Env() string {
	if m.env != "" {
		return m.env
	}
	return Environment
}

const noAPIKeyMessage = "No API key detected. Set cronitor.APIKey or create the Monitor with WithAPIKey."

// Ping sends a telemetry event for this monitor. If no api key is configured
// an error is logged and nil is returned without sending anything.
func (m *Monitor) Ping(opts PingOptions) error {
	apiKey := m.APIKey()
	if apiKey == "" {
		Logger.Print(noAPIKeyMessage)
		return nil
	}

	req, err := http.NewRequest(http.MethodGet, m.pingURL(apiKey), nil)
	if err != nil {
		return &APIError{Err: err}
	}
	req.URL.RawQuery = m.pingParams(opts).Encode()
	req.Header.Set("User-Agent", userAgent)

	resp, err := pingClient().Do(req)
	if err != nil {
		return &APIError{Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	return nil
}

// OK resets the monitor to a passing state. It is shorthand for
// Ping(PingOptions{State: StateOK}).
func (m *Monitor) OK() error {
	return m.Ping(PingOptions{State: StateOK})
}

func (m *Monitor) pingURL(apiKey string) string {
	return fmt.Sprintf("%s/%s/%s", strings.TrimRight(PingURL, "/"), apiKey, m.Key)
}

func (m *Monitor) pingParams(opts PingOptions) url.Values {
	params := url.Values{}
	if opts.State != "" {
		params.Set("state", string(opts.State))
	}
	if opts.Message != "" {
		params.Set("message", opts.Message)
	}
	if opts.Series != "" {
		params.Set("series", opts.Series)
	}
	host := opts.Host
	if host == "" {
		host, _ = os.Hostname()
	}
	if host != "" {
		params.Set("host", host)
	}
	for _, metric := range formatMetrics(opts.Metrics) {
		params.Add("metric", metric)
	}
	params.Set("stamp", formatStamp(time.Now()))
	if env := m.Env(); env != "" {
		params.Set("env", env)
	}
	return params
}

// formatMetrics converts a metrics map to "name:value" strings.
func formatMetrics(metrics map[string]float64) []string {
	if len(metrics) == 0 {
		return nil
	}
	out := make([]string, 0, len(metrics))
	for name, value := range metrics {
		out = append(out, name+":"+strconv.FormatFloat(value, 'f', -1, 64))
	}
	return out
}

// formatStamp renders a time as unix seconds with fractional precision.
func formatStamp(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', 6, 64)
}

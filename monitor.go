package cronitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
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

	mu   sync.Mutex
	data MonitorData
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

// Data returns the monitor document, fetching it from the Monitor API on
// first use and caching it afterwards. Use Fetch to force a refresh.
func (m *Monitor) Data() (MonitorData, error) {
	m.mu.Lock()
	cached := m.data
	m.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	return m.Fetch()
}

// Fetch retrieves the monitor document from the Monitor API and caches it.
// It returns MonitorNotFoundError for an unknown key and AuthenticationError
// when no api key is configured.
func (m *Monitor) Fetch() (MonitorData, error) {
	apiKey := m.APIKey()
	if apiKey == "" {
		return nil, &AuthenticationError{Message: noAPIKeyMessage}
	}

	resp, err := doAPI(apiRequest{
		method:      http.MethodGet,
		url:         monitorAPIURL(m.Key),
		apiKey:      apiKey,
		apiVersion:  m.APIVersion(),
		contentType: "application/json",
	})
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, &MonitorNotFoundError{Key: m.Key}
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, resp.unexpected()
	}

	var data MonitorData
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(resp.Body), Err: err}
	}
	m.setData(data)
	return data, nil
}

// setData replaces the cached monitor document.
func (m *Monitor) setData(data MonitorData) {
	m.mu.Lock()
	m.data = data
	m.mu.Unlock()
}

// Delete destroys the monitor. It returns MonitorNotFoundError if the key is
// unknown and APIError for any other failure.
func (m *Monitor) Delete() error {
	resp, err := doAPI(apiRequest{
		method: http.MethodDelete,
		url:    monitorAPIURL(m.Key),
		apiKey: m.APIKey(),
	})
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return &MonitorNotFoundError{Key: m.Key}
	default:
		return &APIError{StatusCode: resp.StatusCode, Body: fmt.Sprintf("an unexpected error occurred when deleting '%s': %s", m.Key, resp.Body)}
	}
}

// Pause pauses alerting for the monitor for the given number of hours. Pass 0
// to resume alerting. Like Ping, it is a logged no-op without an api key.
func (m *Monitor) Pause(hours int) error {
	apiKey := m.APIKey()
	if apiKey == "" {
		Logger.Print(noAPIKeyMessage)
		return nil
	}
	resp, err := doAPI(apiRequest{
		method: http.MethodGet,
		url:    fmt.Sprintf("%s/pause/%d", monitorAPIURL(m.Key), hours),
		apiKey: apiKey,
		client: pingClient(),
	})
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.unexpected()
	}
	return nil
}

// Unpause resumes alerting for the monitor. It is shorthand for Pause(0).
func (m *Monitor) Unpause() error {
	return m.Pause(0)
}

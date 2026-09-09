package cronitor

import (
	"fmt"
	"sync"
	"time"
)

// jobConfig holds the options of a monitored job.
type jobConfig struct {
	env        string
	logOutput  bool
	attributes map[string]any
}

// JobOption customises Job, NewJob and RunJob.
type JobOption func(*jobConfig)

// WithJobEnv sends the job's pings with the given environment instead of the
// package-level Environment.
func WithJobEnv(env string) JobOption {
	return func(c *jobConfig) { c.env = env }
}

// WithLogOutput controls whether the function's return value is sent as the
// message of the complete ping. It defaults to true.
func WithLogOutput(enabled bool) JobOption {
	return func(c *jobConfig) { c.logOutput = enabled }
}

// WithAttributes queues monitor attributes (schedule, notify, ...) for the
// job's monitor. They are sent to the Monitor API by SyncMonitors; see
// https://cronitor.io/docs/monitor-api#attributes for the available fields.
func WithAttributes(attributes map[string]any) JobOption {
	return func(c *jobConfig) { c.attributes = attributes }
}

// Job runs fn as a monitored job. A run ping is sent before fn is called;
// afterwards a complete ping (with the stringified return value as message
// unless WithLogOutput(false) is set) or, if fn returns an error or panics,
// a fail ping carrying the error text. Both carry a duration metric and share
// a series equal to the start timestamp. The function's result and error are
// returned unchanged; panics are re-raised after the fail ping.
func Job(key string, fn func() (any, error), opts ...JobOption) (any, error) {
	return RunJob(key, fn, opts...)
}

// NewJob wraps fn so that every invocation of the returned function is
// monitored exactly like Job. It is the closest equivalent to a decorator.
func NewJob(key string, fn func() (any, error), opts ...JobOption) func() (any, error) {
	cfg := jobConfig{logOutput: true}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.attributes != nil {
		queueMonitorAttributes(key, cfg.attributes)
	}
	return func() (any, error) { return RunJob(key, fn, opts...) }
}

// RunJob is the generic form of Job, preserving the return type of fn.
func RunJob[T any](key string, fn func() (T, error), opts ...JobOption) (T, error) {
	cfg := jobConfig{logOutput: true}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.attributes != nil {
		queueMonitorAttributes(key, cfg.attributes)
	}

	var monitorOpts []MonitorOption
	if cfg.env != "" {
		monitorOpts = append(monitorOpts, WithEnv(cfg.env))
	}
	monitor := NewMonitor(key, monitorOpts...)

	start := time.Now()
	// The series ties run/fail/complete pings of this invocation together.
	series := formatStamp(start)
	duration := func() map[string]float64 {
		return map[string]float64{"duration": time.Since(start).Seconds()}
	}

	monitor.Ping(PingOptions{State: StateRun, Series: series}) //nolint:errcheck

	var (
		out T
		err error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				monitor.Ping(PingOptions{State: StateFail, Message: fmt.Sprint(r), Metrics: duration(), Series: series}) //nolint:errcheck
				panic(r)
			}
		}()
		out, err = fn()
	}()

	if err != nil {
		monitor.Ping(PingOptions{State: StateFail, Message: err.Error(), Metrics: duration(), Series: series}) //nolint:errcheck
		return out, err
	}

	var message string
	if cfg.logOutput {
		if v := any(out); v != nil {
			message = fmt.Sprint(v)
		}
	}
	monitor.Ping(PingOptions{State: StateComplete, Message: message, Metrics: duration(), Series: series}) //nolint:errcheck
	return out, nil
}

var (
	attributesMu      sync.Mutex
	monitorAttributes []map[string]any
)

// queueMonitorAttributes records attributes for key, replacing any earlier
// entry for the same key.
func queueMonitorAttributes(key string, attributes map[string]any) {
	entry := make(map[string]any, len(attributes)+1)
	for k, v := range attributes {
		entry[k] = v
	}
	entry["key"] = key

	attributesMu.Lock()
	defer attributesMu.Unlock()
	for i, existing := range monitorAttributes {
		if existing["key"] == key {
			monitorAttributes[i] = entry
			return
		}
	}
	monitorAttributes = append(monitorAttributes, entry)
}

// QueuedMonitorAttributes returns a copy of the monitor attributes queued by
// WithAttributes that have not been synced yet.
func QueuedMonitorAttributes() []map[string]any {
	attributesMu.Lock()
	defer attributesMu.Unlock()
	out := make([]map[string]any, len(monitorAttributes))
	copy(out, monitorAttributes)
	return out
}

func clearMonitorAttributes() {
	attributesMu.Lock()
	monitorAttributes = nil
	attributesMu.Unlock()
}

// SyncMonitors sends the monitor attributes queued by WithAttributes to the
// Monitor API with PutMonitors and clears the queue on success. Call it once
// your jobs are registered, for example at process start. It does nothing
// when the queue is empty.
func SyncMonitors() error {
	queued := QueuedMonitorAttributes()
	if len(queued) == 0 {
		return nil
	}
	if _, err := PutMonitors(queued, PutOptions{}); err != nil {
		return err
	}
	clearMonitorAttributes()
	return nil
}

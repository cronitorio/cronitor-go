package cronitor

import (
	"io"
	"net/http"
	"time"
)

const (
	userAgent           = "cronitor-go"
	defaultMaxRetries   = 3
	defaultRetryBackoff = 300 * time.Millisecond
)

// retryBackoff is the base backoff between retries; tests shrink it.
var retryBackoff = defaultRetryBackoff

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// retryTransport retries requests that fail with a transport error or a 5xx
// response, sleeping with exponential backoff between attempts. It mirrors
// the urllib3 Retry configuration used by cronitor-python.
type retryTransport struct {
	base       http.RoundTripper
	maxRetries int
	backoff    time.Duration
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", userAgent)
	}

	var (
		resp *http.Response
		err  error
	)
	for attempt := 0; ; attempt++ {
		if attempt > 0 && req.GetBody != nil {
			body, bodyErr := req.GetBody()
			if bodyErr != nil {
				return nil, bodyErr
			}
			req.Body = body
		}

		resp, err = t.base.RoundTrip(req)
		if !shouldRetry(resp, err) || attempt >= t.maxRetries {
			return resp, err
		}
		if resp != nil {
			// Drain so the connection can be reused, then discard the response.
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
			resp.Body.Close()
		}

		wait := backoffDuration(t.backoff, attempt)
		select {
		case <-time.After(wait):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
}

func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	return resp != nil && resp.StatusCode >= 500
}

// backoffDuration returns base * 2^attempt.
func backoffDuration(base time.Duration, attempt int) time.Duration {
	return base * time.Duration(1<<uint(attempt))
}

// newClient builds an http.Client with the given overall timeout and a
// retrying transport.
func newClient(timeout time.Duration, maxRetries int, backoff time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &retryTransport{
			base:       http.DefaultTransport,
			maxRetries: maxRetries,
			backoff:    backoff,
		},
	}
}

// pingClient returns the client used for telemetry pings (5 second timeout).
func pingClient() *http.Client {
	return newClient(DefaultPingTimeout, defaultMaxRetries, retryBackoff)
}

// apiClient returns the client used for Monitor API calls, honouring the
// package Timeout setting.
func apiClient() *http.Client {
	timeout := Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return newClient(timeout, defaultMaxRetries, retryBackoff)
}

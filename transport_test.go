package cronitor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryTransportRetriesOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "payload" {
			t.Errorf("body should be re-sent on retry, got %q", body)
		}
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newClient(time.Second, 3, time.Millisecond)
	req, _ := http.NewRequest(http.MethodPut, srv.URL, strings.NewReader("payload"))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 after retries, got %d", resp.StatusCode)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}

func TestRetryTransportGivesUpAfterMaxRetries(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newClient(time.Second, 3, time.Millisecond)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected final 500, got %d", resp.StatusCode)
	}
	if calls != 4 {
		t.Fatalf("expected 1 attempt + 3 retries, got %d", calls)
	}
}

func TestRetryTransportDoesNotRetry4xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	client := newClient(time.Second, 3, time.Millisecond)
	resp, _ := client.Get(srv.URL)
	resp.Body.Close()
	if calls != 1 {
		t.Fatalf("4xx must not be retried, got %d attempts", calls)
	}
}

func TestRetryTransportRetriesConnectionErrors(t *testing.T) {
	// Grab a port and close the listener so connections are refused.
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	var attempts int32
	client := newClient(time.Second, 3, time.Millisecond)
	client.Transport.(*retryTransport).base = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return http.DefaultTransport.RoundTrip(r)
	})
	if _, err := client.Get(url); err == nil {
		t.Fatal("expected a connection error")
	}
	if attempts != 4 {
		t.Fatalf("expected 4 attempts on connection error, got %d", attempts)
	}
}

func TestBackoffIsExponential(t *testing.T) {
	base := 300 * time.Millisecond
	want := []time.Duration{300 * time.Millisecond, 600 * time.Millisecond, 1200 * time.Millisecond}
	for i, w := range want {
		if got := backoffDuration(base, i); got != w {
			t.Errorf("attempt %d: want %v, got %v", i, w, got)
		}
	}
}

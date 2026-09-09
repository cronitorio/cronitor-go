package cronitor

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorTypesMatchSentinels(t *testing.T) {
	cases := []struct {
		err      error
		sentinel error
	}{
		{&MonitorNotFoundError{Key: "abc"}, ErrMonitorNotFound},
		{&ConfigValidationError{Message: "no path"}, ErrConfigValidation},
		{&APIValidationError{Body: "bad schedule"}, ErrAPIValidation},
		{&AuthenticationError{Message: "no api key"}, ErrAuthentication},
		{&APIError{StatusCode: 500, Body: "boom"}, ErrAPI},
	}
	for _, c := range cases {
		wrapped := fmt.Errorf("wrapped: %w", c.err)
		if !errors.Is(wrapped, c.sentinel) {
			t.Errorf("%T should match sentinel %v", c.err, c.sentinel)
		}
		if c.err.Error() == "" {
			t.Errorf("%T should have a message", c.err)
		}
	}

	var nf *MonitorNotFoundError
	if !errors.As(fmt.Errorf("x: %w", &MonitorNotFoundError{Key: "abc"}), &nf) || nf.Key != "abc" {
		t.Fatalf("errors.As should extract MonitorNotFoundError")
	}
	var apiErr *APIError
	if !errors.As(&APIError{StatusCode: 503, Body: "down"}, &apiErr) || apiErr.StatusCode != 503 {
		t.Fatalf("errors.As should extract APIError")
	}
	if errors.Is(&APIError{}, ErrAPIValidation) {
		t.Fatalf("APIError must not match ErrAPIValidation")
	}
}

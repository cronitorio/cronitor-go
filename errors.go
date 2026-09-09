package cronitor

import (
	"errors"
	"fmt"
)

// Sentinel errors. Every error returned by this package wraps one of these so
// callers can use errors.Is without caring about the concrete type. Use
// errors.As with the struct types below to inspect details such as the HTTP
// status code or response body.
var (
	ErrMonitorNotFound  = errors.New("cronitor: monitor not found")
	ErrConfigValidation = errors.New("cronitor: config validation error")
	ErrAPIValidation    = errors.New("cronitor: api validation error")
	ErrAuthentication   = errors.New("cronitor: authentication error")
	ErrAPI              = errors.New("cronitor: api error")
)

// MonitorNotFoundError is returned when the Monitor API responds with 404.
type MonitorNotFoundError struct {
	Key string
}

func (e *MonitorNotFoundError) Error() string {
	return fmt.Sprintf("cronitor: monitor '%s' not found", e.Key)
}

// Is reports whether target is ErrMonitorNotFound.
func (e *MonitorNotFoundError) Is(target error) bool { return target == ErrMonitorNotFound }

// ConfigValidationError is returned when a cronitor.yaml config path is not
// set or the config file cannot be read.
type ConfigValidationError struct {
	Message string
	Err     error
}

func (e *ConfigValidationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("cronitor: %s: %v", e.Message, e.Err)
	}
	return "cronitor: " + e.Message
}

// Is reports whether target is ErrConfigValidation.
func (e *ConfigValidationError) Is(target error) bool { return target == ErrConfigValidation }

// Unwrap returns the underlying error, if any.
func (e *ConfigValidationError) Unwrap() error { return e.Err }

// APIValidationError is returned when the Monitor API rejects a request with
// HTTP 400. Body contains the raw response body describing the problem.
type APIValidationError struct {
	Body string
}

func (e *APIValidationError) Error() string {
	return "cronitor: api validation error: " + e.Body
}

// Is reports whether target is ErrAPIValidation.
func (e *APIValidationError) Is(target error) bool { return target == ErrAPIValidation }

// AuthenticationError is returned when an API key is required but not set.
type AuthenticationError struct {
	Message string
}

func (e *AuthenticationError) Error() string {
	return "cronitor: " + e.Message
}

// Is reports whether target is ErrAuthentication.
func (e *AuthenticationError) Is(target error) bool { return target == ErrAuthentication }

// APIError is returned for any unexpected response from the Cronitor API, or
// when the request could not be completed at all (in which case StatusCode is
// 0 and Err holds the transport error).
type APIError struct {
	StatusCode int
	Body       string
	Err        error
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("cronitor: request failed: %v", e.Err)
	}
	return fmt.Sprintf("cronitor: unexpected error (HTTP %d): %s", e.StatusCode, e.Body)
}

// Is reports whether target is ErrAPI.
func (e *APIError) Is(target error) bool { return target == ErrAPI }

// Unwrap returns the underlying transport error, if any.
func (e *APIError) Unwrap() error { return e.Err }

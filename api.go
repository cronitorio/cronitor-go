package cronitor

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

// apiRequest describes a call to the Monitor API.
type apiRequest struct {
	method      string
	url         string
	apiKey      string
	apiVersion  string
	contentType string
	body        []byte
	client      *http.Client
}

// apiResponse is the outcome of an apiRequest with the body fully read.
type apiResponse struct {
	StatusCode int
	Body       []byte
}

// monitorAPIURL builds a Monitor API URL. With no key it returns the
// collection URL, otherwise the URL of the single monitor.
func monitorAPIURL(key string) string {
	base := strings.TrimRight(MonitorAPIURL, "/")
	if key == "" {
		return base
	}
	return base + "/" + key
}

// doAPI performs an authenticated Monitor API request and reads the response.
// Transport failures are wrapped in an APIError.
func doAPI(r apiRequest) (*apiResponse, error) {
	var body io.Reader
	if r.body != nil {
		body = bytes.NewReader(r.body)
	}
	req, err := http.NewRequest(r.method, r.url, body)
	if err != nil {
		return nil, &APIError{Err: err}
	}
	req.SetBasicAuth(r.apiKey, "")
	req.Header.Set("User-Agent", userAgent)
	if r.apiVersion != "" {
		req.Header.Set("Cronitor-Version", r.apiVersion)
	}
	if r.contentType != "" {
		req.Header.Set("Content-Type", r.contentType)
	}

	client := r.client
	if client == nil {
		client = apiClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &APIError{Err: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &APIError{Err: err}
	}
	return &apiResponse{StatusCode: resp.StatusCode, Body: data}, nil
}

// unexpected converts a non-success response into an APIError.
func (r *apiResponse) unexpected() error {
	return &APIError{StatusCode: r.StatusCode, Body: string(r.Body)}
}

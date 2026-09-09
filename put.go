package cronitor

import (
	"encoding/json"
	"net/http"

	"gopkg.in/yaml.v3"
)

// PutOptions customise PutMonitors and PutMonitorsYAML.
type PutOptions struct {
	// Rollback asks the API to validate the request without persisting it.
	Rollback bool
	// APIKey overrides the package-level APIKey for this call.
	APIKey string
	// APIVersion overrides the package-level APIVersion for this call.
	APIVersion string
}

func (o PutOptions) apiKey() string {
	if o.APIKey != "" {
		return o.APIKey
	}
	return APIKey
}

func (o PutOptions) apiVersion() string {
	if o.APIVersion != "" {
		return o.APIVersion
	}
	return APIVersion
}

// PutMonitors creates or updates monitors through the Monitor API. Each map
// is a monitor definition; see https://cronitor.io/docs/monitor-api#attributes
// for the attributes that can be set. It returns a Monitor for each entry of
// the response with its data pre-populated. A 400 response yields an
// APIValidationError; any other failure an APIError.
func PutMonitors(monitors []map[string]any, opts PutOptions) ([]*Monitor, error) {
	if monitors == nil {
		monitors = []map[string]any{}
	}
	payload := map[string]any{"monitors": monitors}
	if opts.Rollback {
		payload["rollback"] = true
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &APIError{Err: err}
	}

	resp, err := putRequest(monitorAPIURL(""), "application/json", body, opts)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Monitors []map[string]any `json:"monitors"`
	}
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(resp.Body), Err: err}
	}
	out := make([]*Monitor, 0, len(parsed.Monitors))
	for _, md := range parsed.Monitors {
		key, _ := md["key"].(string)
		m := NewMonitor(key)
		m.setData(MonitorData(md))
		out = append(out, m)
	}
	return out, nil
}

// PutMonitorsYAML creates or updates monitors from a YAML-style document with
// top-level jobs, checks and heartbeats keys (the shape of cronitor.yaml).
// The document is sent to {MonitorAPIURL}.yaml and the parsed YAML response
// is returned.
func PutMonitorsYAML(document map[string]any, opts PutOptions) (map[string]any, error) {
	payload := make(map[string]any, len(document)+1)
	for k, v := range document {
		payload[k] = v
	}
	if opts.Rollback {
		payload["rollback"] = true
	}
	body, err := yaml.Marshal(payload)
	if err != nil {
		return nil, &APIError{Err: err}
	}

	resp, err := putRequest(monitorAPIURL("")+".yaml", "application/yaml", body, opts)
	if err != nil {
		return nil, err
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(resp.Body), Err: err}
	}
	return parsed, nil
}

// putRequest performs a PUT against the Monitor API and maps error statuses.
func putRequest(url, contentType string, body []byte, opts PutOptions) (*apiResponse, error) {
	resp, err := doAPI(apiRequest{
		method:      http.MethodPut,
		url:         url,
		apiKey:      opts.apiKey(),
		apiVersion:  opts.apiVersion(),
		contentType: contentType,
		body:        body,
	})
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusOK:
		return resp, nil
	case resp.StatusCode == http.StatusBadRequest:
		return nil, &APIValidationError{Body: string(resp.Body)}
	default:
		return nil, resp.unexpected()
	}
}

// AsYAML fetches every monitor in the account as a YAML document, in the
// format used by cronitor.yaml.
func AsYAML() (string, error) {
	resp, err := doAPI(apiRequest{
		method:      http.MethodGet,
		url:         monitorAPIURL("") + ".yaml",
		apiKey:      APIKey,
		apiVersion:  APIVersion,
		contentType: "application/yaml",
	})
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", resp.unexpected()
	}
	return string(resp.Body), nil
}

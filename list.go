package cronitor

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Default pagination for ListMonitors.
const (
	DefaultPage     = 1
	DefaultPageSize = 100
)

// ListOptions control ListMonitors. When Keys is set the named monitors are
// fetched individually and every other field except APIKey and APIVersion is
// ignored; otherwise the filters and pagination settings are sent as query
// parameters.
type ListOptions struct {
	// Keys fetches these specific monitors, one request each.
	Keys []string

	// Filters. Empty values are omitted.
	Type   string
	Group  string
	Tag    string
	State  string
	Env    string
	Search string
	Sort   string

	// Page is the page to fetch (default 1).
	Page int
	// PageSize is the number of results per page (default 100).
	PageSize int
	// AutoPaginate keeps fetching pages until one returns fewer than
	// PageSize results.
	AutoPaginate bool

	// APIKey overrides the package-level APIKey for this call.
	APIKey string
	// APIVersion overrides the package-level APIVersion for this call.
	APIVersion string
}

func (o ListOptions) apiKey() string {
	if o.APIKey != "" {
		return o.APIKey
	}
	return APIKey
}

func (o ListOptions) apiVersion() string {
	if o.APIVersion != "" {
		return o.APIVersion
	}
	return APIVersion
}

func (o ListOptions) filters() url.Values {
	params := url.Values{}
	set := func(name, value string) {
		if value != "" {
			params.Set(name, value)
		}
	}
	set("type", o.Type)
	set("group", o.Group)
	set("tag", o.Tag)
	set("state", o.State)
	set("env", o.Env)
	set("search", o.Search)
	set("sort", o.Sort)
	return params
}

// ListMonitors fetches monitors with optional filtering and pagination. Each
// returned Monitor has its data pre-populated.
func ListMonitors(opts ListOptions) ([]*Monitor, error) {
	if len(opts.Keys) > 0 {
		monitors := make([]*Monitor, 0, len(opts.Keys))
		for _, key := range opts.Keys {
			m := NewMonitor(key, WithAPIKey(opts.apiKey()), WithAPIVersion(opts.apiVersion()))
			if _, err := m.Fetch(); err != nil {
				return nil, err
			}
			monitors = append(monitors, m)
		}
		return monitors, nil
	}

	page := opts.Page
	if page <= 0 {
		page = DefaultPage
	}
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	monitors := []*Monitor{}
	for {
		result, err := fetchMonitorPage(opts, page, pageSize)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, result...)
		if !opts.AutoPaginate || len(result) < pageSize {
			return monitors, nil
		}
		page++
	}
}

// fetchMonitorPage retrieves a single page of monitors from the Monitor API.
func fetchMonitorPage(opts ListOptions, page, pageSize int) ([]*Monitor, error) {
	params := opts.filters()
	params.Set("page", strconv.Itoa(page))
	params.Set("pageSize", strconv.Itoa(pageSize))

	resp, err := doAPI(apiRequest{
		method:     http.MethodGet,
		url:        monitorAPIURL("") + "?" + params.Encode(),
		apiKey:     opts.apiKey(),
		apiVersion: opts.apiVersion(),
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.unexpected()
	}

	var parsed struct {
		Monitors []map[string]any `json:"monitors"`
	}
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(resp.Body), Err: err}
	}
	monitors := make([]*Monitor, 0, len(parsed.Monitors))
	for _, md := range parsed.Monitors {
		key, _ := md["key"].(string)
		m := NewMonitor(key)
		m.setData(MonitorData(md))
		monitors = append(monitors, m)
	}
	return monitors, nil
}

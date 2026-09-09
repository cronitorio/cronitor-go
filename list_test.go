package cronitor

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestListMonitorsWithKeys(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/api/monitors/")
		json.NewEncoder(w).Encode(map[string]any{"key": key, "name": "Monitor " + key}) //nolint:errcheck
	})
	APIKey = fakeMonitorAPIKey

	monitors, err := ListMonitors(ListOptions{Keys: []string{"key1", "key2"}})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}
	for i, want := range []string{"key1", "key2"} {
		d, err := monitors[i].Data()
		if err != nil || d.Key() != want {
			t.Errorf("monitor %d: want %s, got %v (%v)", i, want, d, err)
		}
	}
	reqs := srv.Requests()
	if len(reqs) != 2 || reqs[0].Path != "/api/monitors/key1" || reqs[1].Path != "/api/monitors/key2" {
		t.Errorf("keys should be fetched individually, got %+v", reqs)
	}
}

func TestListMonitorsWithKeysNotFound(t *testing.T) {
	newTestServer(t, jsonHandler(404, nil))
	APIKey = fakeMonitorAPIKey
	if _, err := ListMonitors(ListOptions{Keys: []string{"nope"}}); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("want ErrMonitorNotFound, got %v", err)
	}
}

func TestListMonitorsWithFilters(t *testing.T) {
	srv := newTestServer(t, jsonHandler(200, map[string]any{"monitors": []any{
		map[string]any{"key": "job1", "type": "job"},
		map[string]any{"key": "job2", "type": "job"},
	}}))
	APIKey = fakeMonitorAPIKey
	APIVersion = "2020-10-01"

	monitors, err := ListMonitors(ListOptions{Type: "job", Group: "production", Tag: "t", State: "failing", Env: "prod", Search: "backup", Sort: "name"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(monitors) != 2 || monitors[0].Key != "job1" || monitors[1].Key != "job2" {
		t.Errorf("unexpected monitors %v", monitors)
	}
	d, _ := monitors[0].Data()
	if d.Type() != "job" {
		t.Error("listed monitors should carry data without another fetch")
	}

	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	r := reqs[0]
	if r.Method != http.MethodGet || r.Path != "/api/monitors" {
		t.Errorf("unexpected request %s %s", r.Method, r.Path)
	}
	want := map[string]string{"type": "job", "group": "production", "tag": "t", "state": "failing", "env": "prod", "search": "backup", "sort": "name", "page": "1", "pageSize": "100"}
	for k, v := range want {
		if got := r.Query.Get(k); got != v {
			t.Errorf("param %s: want %q, got %q", k, v, got)
		}
	}
	if len(r.Query) != len(want) {
		t.Errorf("unexpected extra params: %v", r.Query)
	}
	if r.Header.Get("Authorization") != basicAuth(fakeMonitorAPIKey) || r.Header.Get("Cronitor-Version") != "2020-10-01" {
		t.Error("list must send basic auth and Cronitor-Version")
	}
}

func TestListMonitorsPagination(t *testing.T) {
	srv := newTestServer(t, jsonHandler(200, map[string]any{"monitors": []any{map[string]any{"key": "job1"}}}))
	APIKey = fakeMonitorAPIKey

	monitors, err := ListMonitors(ListOptions{Page: 3, PageSize: 25})
	if err != nil || len(monitors) != 1 {
		t.Fatalf("list failed: %v %v", monitors, err)
	}
	q := srv.Requests()[0].Query
	if q.Get("page") != "3" || q.Get("pageSize") != "25" {
		t.Errorf("want page=3 pageSize=25, got %v", q)
	}
}

func TestListMonitorsAutoPaginate(t *testing.T) {
	pages := map[string][]any{
		"1": {map[string]any{"key": "job1"}, map[string]any{"key": "job2"}},
		"2": {map[string]any{"key": "job3"}},
	}
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"monitors": pages[r.URL.Query().Get("page")]}) //nolint:errcheck
	})
	APIKey = fakeMonitorAPIKey

	monitors, err := ListMonitors(ListOptions{PageSize: 2, AutoPaginate: true})
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, m := range monitors {
		keys = append(keys, m.Key)
	}
	if strings.Join(keys, ",") != "job1,job2,job3" {
		t.Errorf("want all pages, got %v", keys)
	}
	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 page requests, got %d", len(reqs))
	}
	for i, r := range reqs {
		if r.Query.Get("page") != strconv.Itoa(i+1) || r.Query.Get("pageSize") != "2" {
			t.Errorf("request %d has wrong pagination params: %v", i, r.Query)
		}
	}
}

func TestListMonitorsWithoutAutoPaginateFetchesOnePage(t *testing.T) {
	srv := newTestServer(t, jsonHandler(200, map[string]any{"monitors": []any{map[string]any{"key": "a"}, map[string]any{"key": "b"}}}))
	APIKey = fakeMonitorAPIKey
	if _, err := ListMonitors(ListOptions{PageSize: 2}); err != nil {
		t.Fatal(err)
	}
	if len(srv.Requests()) != 1 {
		t.Error("only one page should be fetched without AutoPaginate")
	}
}

func TestListMonitorsEmpty(t *testing.T) {
	newTestServer(t, jsonHandler(200, map[string]any{"monitors": []any{}}))
	APIKey = fakeMonitorAPIKey
	monitors, err := ListMonitors(ListOptions{Type: "nonexistent"})
	if err != nil || len(monitors) != 0 {
		t.Fatalf("want empty list, got %v %v", monitors, err)
	}
}

func TestListMonitorsAPIError(t *testing.T) {
	newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Forbidden")) //nolint:errcheck
	})
	APIKey = fakeMonitorAPIKey
	_, err := ListMonitors(ListOptions{Type: "job"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 403 {
		t.Fatalf("want APIError, got %v", err)
	}
}

func TestListMonitorsOptionsOverride(t *testing.T) {
	srv := newTestServer(t, jsonHandler(200, map[string]any{"monitors": []any{}}))
	APIKey = fakeMonitorAPIKey
	if _, err := ListMonitors(ListOptions{APIKey: "override", APIVersion: "2021-01-01"}); err != nil {
		t.Fatal(err)
	}
	r := srv.Requests()[0]
	if r.Header.Get("Authorization") != basicAuth("override") || r.Header.Get("Cronitor-Version") != "2021-01-01" {
		t.Error("ListOptions should override api key and version")
	}
}

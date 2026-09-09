package cronitor

import (
	"os"
	"testing"
)

// Integration tests make real calls to the Cronitor API. They only run when
// CRONITOR_TEST_API_KEY is set:
//
//	CRONITOR_TEST_API_KEY=your_api_key go test -run Integration -v ./...
func integrationSetup(t *testing.T) {
	t.Helper()
	key := os.Getenv("CRONITOR_TEST_API_KEY")
	if key == "" {
		t.Skip("Set CRONITOR_TEST_API_KEY environment variable to run integration tests")
	}
	resetSettings()
	Logger.SetOutput(os.Stderr)
	APIKey = key
	t.Cleanup(resetSettings)
}

func TestIntegrationListAllMonitors(t *testing.T) {
	integrationSetup(t)
	monitors, err := ListMonitors(ListOptions{})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(monitors) > 0 {
		d, err := monitors[0].Data()
		if err != nil || d.Key() == "" || d.Name() == "" {
			t.Errorf("monitor should carry key and name: %v %v", d, err)
		}
		t.Logf("Found %d monitors on first page (default pageSize=100); first: %s (%s)", len(monitors), d.Name(), d.Key())
	}
}

func TestIntegrationListAllMonitorsAutoPaginate(t *testing.T) {
	integrationSetup(t)
	all, err := ListMonitors(ListOptions{AutoPaginate: true})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	page1, err := ListMonitors(ListOptions{PageSize: 100})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(all) < len(page1) {
		t.Errorf("auto-paginate returned %d monitors, fewer than first page %d", len(all), len(page1))
	}
	t.Logf("Auto-paginate fetched %d monitors (first page had %d)", len(all), len(page1))
}

func TestIntegrationListWithPagination(t *testing.T) {
	integrationSetup(t)
	monitors, err := ListMonitors(ListOptions{PageSize: 5})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(monitors) > 5 {
		t.Errorf("expected at most 5 monitors, got %d", len(monitors))
	}
}

func TestIntegrationListWithFilter(t *testing.T) {
	integrationSetup(t)
	monitors, err := ListMonitors(ListOptions{Type: "job"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	for _, m := range monitors {
		d, _ := m.Data()
		if d.Type() != "job" {
			t.Errorf("filter type=job returned %s (%s)", d.Type(), d.Key())
		}
	}
	t.Logf("Filter works, got %d job monitors", len(monitors))
}

func TestIntegrationListWithSearch(t *testing.T) {
	integrationSetup(t)
	monitors, err := ListMonitors(ListOptions{Search: "test job"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	t.Logf("Search works, found %d monitors matching 'test job'", len(monitors))
}

func TestIntegrationListSpecificKeys(t *testing.T) {
	integrationSetup(t)
	all, err := ListMonitors(ListOptions{PageSize: 2})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(all) == 0 {
		t.Skip("No monitors found in account")
	}
	keys := make([]string, 0, len(all))
	for _, m := range all {
		keys = append(keys, m.Key)
	}
	monitors, err := ListMonitors(ListOptions{Keys: keys})
	if err != nil {
		t.Fatalf("list by keys failed: %v", err)
	}
	if len(monitors) != len(keys) {
		t.Fatalf("expected %d monitors, got %d", len(keys), len(monitors))
	}
	for i, m := range monitors {
		d, _ := m.Data()
		if d.Key() != keys[i] {
			t.Errorf("want key %s, got %s", keys[i], d.Key())
		}
	}
}

func TestIntegrationMonitorDataStructure(t *testing.T) {
	integrationSetup(t)
	monitors, err := ListMonitors(ListOptions{PageSize: 1})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(monitors) == 0 {
		t.Skip("No monitors found in account")
	}
	d, err := monitors[0].Data()
	if err != nil {
		t.Fatal(err)
	}
	if d.Key() == "" || d.Name() == "" || d.Type() == "" {
		t.Errorf("basic fields missing: %v", d)
	}
	if d.GetString("attributes", "code") == "" {
		t.Errorf("nested attributes.code missing: %v", d)
	}
	if s := d.String(); s == "" {
		t.Error("String should pretty-print the document")
	}
}

func TestIntegrationPing(t *testing.T) {
	integrationSetup(t)
	// d3x0c1 is a reserved monitor key for running integration tests.
	m := NewMonitor("d3x0c1")
	for _, state := range []State{"", StateRun, StateComplete, StateFail, StateOK} {
		if err := m.Ping(PingOptions{State: state}); err != nil {
			t.Errorf("ping state=%q failed: %v", state, err)
		}
	}
}

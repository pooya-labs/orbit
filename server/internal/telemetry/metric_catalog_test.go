package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMetricCatalogContainsOnlyCuratedWorkspaceMetrics(t *testing.T) {
	catalog := ListMetricCatalog()
	if len(catalog.Data) != 3 {
		t.Fatalf("metric catalog length = %d, want 3", len(catalog.Data))
	}

	want := map[string]string{
		MetricRequestRate:           "requests/second",
		MetricSuccessfulRequestRate: "requests/second",
		MetricFailedRequestRate:     "requests/second",
	}
	for _, definition := range catalog.Data {
		unit, ok := want[definition.ID]
		if !ok {
			t.Fatalf("unexpected metric in catalog: %+v", definition)
		}
		if definition.Unit != unit || definition.Description == "" {
			t.Fatalf("incomplete metric definition: %+v", definition)
		}
		if len(definition.GroupBy) != maxMetricGroupDimensions {
			t.Fatalf("metric %s group_by = %v, want %d curated dimensions", definition.ID, definition.GroupBy, maxMetricGroupDimensions)
		}
		delete(want, definition.ID)
	}
	if len(want) != 0 {
		t.Fatalf("metric catalog omitted definitions: %v", want)
	}

	// Callers cannot mutate the server-owned catalogue through returned slices.
	catalog.Data[0].GroupBy[0] = "caller-controlled"
	next := ListMetricCatalog()
	if next.Data[0].GroupBy[0] == "caller-controlled" {
		t.Fatal("metric catalog leaked its internal group_by slice")
	}
}

func TestParseMetricIDUsesServerAllowList(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "", want: MetricRequestRate},
		{input: " request_rate ", want: MetricRequestRate},
		{input: MetricSuccessfulRequestRate, want: MetricSuccessfulRequestRate},
		{input: MetricFailedRequestRate, want: MetricFailedRequestRate},
		{input: "orbit_demo_requests_total", wantErr: true},
		{input: "request_rate{orbit_workspace_id=~\".*\"}", wantErr: true},
		{input: "up", wantErr: true},
	}
	for _, test := range tests {
		got, err := ParseMetricID(test.input)
		if (err != nil) != test.wantErr {
			t.Fatalf("ParseMetricID(%q) error = %v, wantErr=%t", test.input, err, test.wantErr)
		}
		if err == nil && got != test.want {
			t.Fatalf("ParseMetricID(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestWorkspaceMetricsBuildOnlyCuratedTenantScopedPromQL(t *testing.T) {
	const workspaceID = "00000000-0000-4000-8000-000000000001"
	tests := []struct {
		metric string
		query  string
	}{
		{
			metric: MetricRequestRate,
			query:  `sum by (orbit_workspace_id, service_name) (rate(orbit_demo_requests_total{service_name="api-gateway",orbit_workspace_id="00000000-0000-4000-8000-000000000001"}[5m]))`,
		},
		{
			metric: MetricSuccessfulRequestRate,
			query:  `sum by (orbit_workspace_id, service_name) (rate(orbit_demo_requests_total{service_name="api-gateway",orbit_workspace_id="00000000-0000-4000-8000-000000000001",slo_outcome="good"}[5m]))`,
		},
		{
			metric: MetricFailedRequestRate,
			query:  `sum by (orbit_workspace_id, service_name) (rate(orbit_demo_requests_total{service_name="api-gateway",orbit_workspace_id="00000000-0000-4000-8000-000000000001",slo_outcome="bad"}[5m]))`,
		},
	}

	for _, test := range tests {
		t.Run(test.metric, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/api/v1/query_range" {
					t.Fatalf("Prometheus path = %q", r.URL.Path)
				}
				query := r.URL.Query()
				if query.Get("query") != test.query {
					t.Errorf("PromQL = %q, want %q", query.Get("query"), test.query)
				}
				if query.Get("step") != "1m" || query.Get("timeout") != "5s" || query.Get("limit") != "10" {
					t.Errorf("unbounded Prometheus parameters: %v", query)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
			}))
			defer server.Close()

			client, err := NewClient(server.URL, "", "")
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.WorkspaceMetrics(context.Background(), workspaceID, QueryWindow{Name: "1h", Duration: time.Hour, Step: "1m"}, "api-gateway", test.metric, nil)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || result.Metric != test.metric {
				t.Fatalf("calls=%d result metric=%q, want one query for %q", calls, result.Metric, test.metric)
			}
			var series []map[string]json.RawMessage
			if err := json.Unmarshal(result.Data, &series); err != nil {
				t.Fatalf("decode series: %v", err)
			}
			if len(series) != 0 {
				t.Fatalf("unexpected result series: %s", result.Data)
			}
		})
	}
}

func TestWorkspaceMetricsRejectsUnknownMetricBeforeBackendCall(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "unexpected Prometheus request", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WorkspaceMetrics(
		context.Background(),
		"00000000-0000-4000-8000-000000000001",
		QueryWindow{Name: "1h", Duration: time.Hour, Step: "1m"},
		"api-gateway",
		"orbit_demo_requests_total[1m]",
		nil,
	)
	if err == nil {
		t.Fatal("expected unknown metric to be rejected")
	}
	if calls != 0 {
		t.Fatalf("invalid metric reached Prometheus %d times", calls)
	}
}

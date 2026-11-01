package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseWindowUsesBoundedChoices(t *testing.T) {
	for _, name := range []string{"15m", "1h", "6h", "24h"} {
		if _, err := ParseWindow(name); err != nil {
			t.Fatalf("ParseWindow(%q): %v", name, err)
		}
	}
	if got, err := ParseWindow(""); err != nil || got.Name != "1h" {
		t.Fatalf("ParseWindow(empty) = %+v, %v; want 1h", got, err)
	}
	if _, err := ParseWindow("7d"); err == nil {
		t.Fatal("expected unsupported window to be rejected")
	}
}

func TestParseLimitEnforcesMaximum(t *testing.T) {
	if got, err := ParseLimit("", 25); err != nil || got != 25 {
		t.Fatalf("default limit = %d, %v; want 25", got, err)
	}
	if got, err := ParseLimit("100", 20); err != nil || got != 100 {
		t.Fatalf("maximum limit = %d, %v; want 100", got, err)
	}
	for _, value := range []string{"0", "101", "-1", "many"} {
		if _, err := ParseLimit(value, 20); err == nil {
			t.Fatalf("expected invalid limit %q to be rejected", value)
		}
	}
}

func TestValidateServiceName(t *testing.T) {
	for _, name := range []string{"orbit-telemetry-demo", "api.gateway_v2:blue"} {
		if err := ValidateServiceName(name); err != nil {
			t.Fatalf("ValidateServiceName(%q): %v", name, err)
		}
	}
	for _, name := range []string{"", "bad service", "foo{bar}", "a/../b", strings.Repeat("a", 129)} {
		if err := ValidateServiceName(name); err == nil {
			t.Fatalf("expected service name %q to be rejected", name)
		}
	}
}

func TestNewClientRejectsUnsafeEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"file:///etc/passwd",
		"https://user:password@example.com",
		"https://example.com/path",
		"https://example.com?token=secret",
	} {
		if _, err := NewClient(endpoint, "", ""); err == nil {
			t.Fatalf("expected endpoint %q to be rejected", endpoint)
		}
	}
	if _, err := NewClient("", "", ""); err != nil {
		t.Fatalf("empty optional endpoints should be allowed: %v", err)
	}
}

func TestMetricsQueriesOnlyFixedPromQLAndReturnsFreshness(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("query") != `sum by (service_name) (rate(orbit_demo_requests_total{service_name="orbit-telemetry-demo"}[5m]))` {
			t.Fatalf("unexpected service-scoped PromQL: %q", query.Get("query"))
		}
		if query.Get("step") != "1m" || query.Get("timeout") != "5s" || query.Get("limit") != "10" {
			t.Fatalf("unexpected bounded query parameters: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1,"0.2"]]}]}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Metrics(context.Background(), QueryWindow{Name: "1h", Duration: time.Hour, Step: "1m"}, "orbit-telemetry-demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "prometheus" || result.Window != "1h" || result.Step != "1m" || result.Query != `sum by (service_name) (rate(orbit_demo_requests_total{service_name="orbit-telemetry-demo"}[5m]))` {
		t.Fatalf("unexpected query metadata: %+v", result)
	}
	var data []map[string]json.RawMessage
	if err := json.Unmarshal(result.Data, &data); err != nil || len(data) != 1 {
		t.Fatalf("unexpected Prometheus result %s: %v", result.Data, err)
	}
	if result.QueriedAt.IsZero() {
		t.Fatal("expected queried_at freshness timestamp")
	}
}

func TestLogsQueryEscapesServiceAndBoundsResultCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("query") != `{service_name="orbit-telemetry-demo"}` {
			t.Fatalf("LogQL selector = %q", query.Get("query"))
		}
		if query.Get("limit") != "12" || query.Get("direction") != "backward" {
			t.Fatalf("unexpected bounded Loki query parameters: %v", query)
		}
		if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
			t.Fatalf("invalid encoded query: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"service_name":"orbit-telemetry-demo"},"values":[["1","hello"]]}]}}`))
	}))
	defer server.Close()

	client, err := NewClient("", server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Logs(context.Background(), QueryWindow{Name: "15m", Duration: 15 * time.Minute}, "orbit-telemetry-demo", 12)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "loki" || result.Limit != 12 || result.Service != "orbit-telemetry-demo" {
		t.Fatalf("unexpected Loki query metadata: %+v", result)
	}
}

func TestTracesUsesTempoTagSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("tags") != "service.name=orbit-telemetry-demo" || query.Get("limit") != "7" {
			t.Fatalf("unexpected Tempo search parameters: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"traces":[{"traceID":"1234567890abcdef1234567890abcdef","rootServiceName":"orbit-telemetry-demo","durationMs":8}]}`))
	}))
	defer server.Close()

	client, err := NewClient("", "", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Traces(context.Background(), QueryWindow{Name: "6h", Duration: 6 * time.Hour}, "orbit-telemetry-demo", 7)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "tempo" || result.Limit != 7 {
		t.Fatalf("unexpected Tempo query metadata: %+v", result)
	}
}

func TestUnconfiguredBackendReturnsTypedError(t *testing.T) {
	client, err := NewClient("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Metrics(context.Background(), QueryWindow{Name: "1h", Duration: time.Hour, Step: "1m"}, "orbit-telemetry-demo", nil); err != ErrNotConfigured {
		t.Fatalf("error = %v, want ErrNotConfigured", err)
	}
}

func TestUpstreamErrorDoesNotBecomeAResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Metrics(context.Background(), QueryWindow{Name: "1h", Duration: time.Hour, Step: "1m"}, "orbit-telemetry-demo", nil); err == nil {
		t.Fatal("expected upstream HTTP error")
	}
}


func TestServicesMergesSourcesAndRejectsInvalidLabels(t *testing.T) {
	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/label/service_name/values" || r.URL.Query().Get("limit") != "250" {
			t.Errorf("unexpected Prometheus service discovery request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":["api-gateway","shared-service","bad service"]}`))
	}))
	defer prometheus.Close()

	loki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/label/service_name/values" {
			t.Errorf("unexpected Loki service discovery request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":["worker","shared-service"]}`))
	}))
	defer loki.Close()

	tempo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search/tag/service.name/values" || r.URL.Query().Get("limit") != "250" {
			t.Errorf("unexpected Tempo service discovery request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tagValues":["trace-worker","shared-service"]}`))
	}))
	defer tempo.Close()

	client, err := NewClient(prometheus.URL, loki.URL, tempo.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Services(context.Background(), QueryWindow{Name: "1h", Duration: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "telemetry_backends" || result.Window != "1h" || result.QueriedAt.IsZero() || len(result.Warnings) != 0 {
		t.Fatalf("unexpected discovery metadata: %+v", result)
	}
	if len(result.Data) != 4 {
		t.Fatalf("discovered %d services, want 4: %+v", len(result.Data), result.Data)
	}
	if result.Data[0].Name != "api-gateway" || result.Data[1].Name != "shared-service" {
		t.Fatalf("services should be sorted and invalid names skipped: %+v", result.Data)
	}
	shared := result.Data[1]
	if len(shared.Sources) != 3 || shared.Sources[0] != "loki" || shared.Sources[1] != "prometheus" || shared.Sources[2] != "tempo" {
		t.Fatalf("expected merged source provenance for shared-service, got %+v", shared)
	}
}

func TestServicesReturnsPartialResultsAndWarnings(t *testing.T) {
	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":["api-gateway"]}`))
	}))
	defer prometheus.Close()

	failingLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend detail must not leak", http.StatusBadGateway)
	}))
	defer failingLoki.Close()

	client, err := NewClient(prometheus.URL, failingLoki.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Services(context.Background(), QueryWindow{Name: "15m", Duration: 15 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Data) != 1 || result.Data[0].Name != "api-gateway" {
		t.Fatalf("expected partial service catalog: %+v", result.Data)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "loki service discovery unavailable" {
		t.Fatalf("unexpected safe warnings: %+v", result.Warnings)
	}
}

func TestServicesRequiresAtLeastOneConfiguredBackend(t *testing.T) {
	client, err := NewClient("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Services(context.Background(), QueryWindow{Name: "1h", Duration: time.Hour}); err != ErrNotConfigured {
		t.Fatalf("error = %v, want ErrNotConfigured", err)
	}
}

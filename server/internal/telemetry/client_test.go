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
		t.Fatalf("ParseWindow("") = %+v, %v; want 1h", got, err)
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
		if query.Get("query") != metricsQuery {
			t.Fatalf("query = %q, want fixed query %q", query.Get("query"), metricsQuery)
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
	result, err := client.Metrics(context.Background(), QueryWindow{Name: "1h", Duration: time.Hour, Step: "1m"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "prometheus" || result.Window != "1h" || result.Step != "1m" || result.Query != metricsQuery {
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
	if _, err := client.Metrics(context.Background(), QueryWindow{Name: "1h", Duration: time.Hour, Step: "1m"}); err != ErrNotConfigured {
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
	if _, err := client.Metrics(context.Background(), QueryWindow{Name: "1h", Duration: time.Hour, Step: "1m"}); err == nil {
		t.Fatal("expected upstream HTTP error")
	}
}

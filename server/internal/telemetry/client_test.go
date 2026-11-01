package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseMetricGroupByUsesSafeAllowList(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "default dimensions",
			input: "",
			want:  []string{"deployment_environment", "instance", "service_version"},
		},
		{
			name:  "aggregate only",
			input: "none",
			want:  []string{},
		},
		{
			name:  "trims sorts and removes duplicates",
			input: " service_version,cloud_region,cloud_region ",
			want:  []string{"cloud_region", "service_version"},
		},
		{
			name:    "rejects arbitrary Prometheus labels",
			input:   "__name__",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseMetricGroupBy(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("ParseMetricGroupBy(%q) error = %v, wantErr %v", test.input, err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("ParseMetricGroupBy(%q) = %v, want %v", test.input, got, test.want)
			}
		})
	}
}

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

func TestResourceAttributeProvenanceOnlyIncludesBackendsThatObservedService(t *testing.T) {
	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/series" {
			t.Fatalf("unexpected Prometheus path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":[]}`))
	}))
	defer prometheus.Close()

	loki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/series" {
			t.Fatalf("unexpected Loki path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":[{"service_name":"api-gateway","service_version":"2.1"}]}`))
	}))
	defer loki.Close()

	client, err := NewClient(prometheus.URL, loki.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.DiscoverResourceAttributes(context.Background(), QueryWindow{
		Name: "1h", Duration: time.Hour, Step: "1m",
	}, "api-gateway")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SourcesQueried) != 2 || result.SourcesQueried[0] != "loki" || result.SourcesQueried[1] != "prometheus" {
		t.Fatalf("expected both configured backends to be queried, got %v", result.SourcesQueried)
	}
	if result.Attributes["service.name"] != "api-gateway" {
		t.Fatalf("service.name = %#v, want api-gateway", result.Attributes["service.name"])
	}
	if got := result.AttributeSources["service.name"]; len(got) != 1 || got[0] != "loki" {
		t.Fatalf("empty Prometheus result must not claim service.name provenance, got %v", got)
	}
	if got := result.AttributeSources["service.version"]; len(got) != 1 || got[0] != "loki" {
		t.Fatalf("unexpected service.version provenance: %v", got)
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


func TestWorkspaceQueriesAlwaysConstrainBackendTenant(t *testing.T) {
	const workspaceID = "00000000-0000-4000-8000-000000000001"
	t.Run("prometheus",func(t *testing.T){
		server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
			query:=r.URL.Query().Get("query")
			if !strings.Contains(query,`orbit_workspace_id="00000000-0000-4000-8000-000000000001"`) { t.Errorf("PromQL missing workspace matcher: %s",query) }
			w.Header().Set("Content-Type","application/json"); _,_=w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
		})); defer server.Close()
		client,_:=NewClient(server.URL,"","")
		if _,err:=client.WorkspaceMetrics(context.Background(),workspaceID,QueryWindow{Name:"1h",Duration:time.Hour,Step:"1m"},"api-gateway",nil);err!=nil{t.Fatal(err)}
	})
	t.Run("loki",func(t *testing.T){
		server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
			query:=r.URL.Query().Get("query")
			if query!=`{orbit_workspace_id="00000000-0000-4000-8000-000000000001",service_name="api-gateway"}` { t.Errorf("LogQL missing exact workspace selector: %s",query) }
			w.Header().Set("Content-Type","application/json"); _,_=w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
		})); defer server.Close()
		client,_:=NewClient("",server.URL,"")
		if _,err:=client.WorkspaceLogs(context.Background(),workspaceID,QueryWindow{Name:"1h",Duration:time.Hour},"api-gateway",10);err!=nil{t.Fatal(err)}
	})
	t.Run("tempo",func(t *testing.T){
		server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
			query:=r.URL.Query().Get("q")
			if !strings.Contains(query,`resource.orbit.workspace.id = "00000000-0000-4000-8000-000000000001"`) { t.Errorf("TraceQL missing workspace matcher: %s",query) }
			w.Header().Set("Content-Type","application/json"); _,_=w.Write([]byte(`{"traces":[]}`))
		})); defer server.Close()
		client,_:=NewClient("","",server.URL)
		if _,err:=client.WorkspaceTraces(context.Background(),workspaceID,QueryWindow{Name:"1h",Duration:time.Hour},"api-gateway",10);err!=nil{t.Fatal(err)}
	})
}

func TestWorkspaceQueriesRejectMalformedWorkspaceIDBeforeBackendCall(t *testing.T) {
	client,_:=NewClient("http://127.0.0.1:1","http://127.0.0.1:1","http://127.0.0.1:1")
	window:=QueryWindow{Name:"1h",Duration:time.Hour,Step:"1m"}
	if _,err:=client.WorkspaceMetrics(context.Background(),"not-a-uuid",window,"api-gateway",nil);err==nil{t.Fatal("expected invalid workspace ID")}
	if _,err:=client.WorkspaceLogs(context.Background(),"not-a-uuid",window,"api-gateway",10);err==nil{t.Fatal("expected invalid workspace ID")}
	if _,err:=client.WorkspaceTraces(context.Background(),"not-a-uuid",window,"api-gateway",10);err==nil{t.Fatal("expected invalid workspace ID")}
}


func TestWorkspaceBackendQueriesRemainIsolatedAcrossSequentialWorkspaceReads(t *testing.T) {
	const (
		workspaceA = "00000000-0000-4000-8000-000000000001"
		workspaceB = "00000000-0000-4000-8000-000000000002"
	)
	expectedWorkspace := workspaceA
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var query string
		switch r.URL.Path {
		case "/api/v1/query_range":
			query = r.URL.Query().Get("query")
			if !strings.Contains(query, "orbit_workspace_id="+strconv.Quote(expectedWorkspace)) {
				t.Errorf("Prometheus query for workspace %s lacks exact tenant matcher: %s", expectedWorkspace, query)
			}
			if strings.Contains(query, "orbit_workspace_id="+strconv.Quote(otherWorkspaceID(expectedWorkspace, workspaceA, workspaceB))) {
				t.Errorf("Prometheus query for workspace %s contains the other tenant matcher: %s", expectedWorkspace, query)
			}
			calls["prometheus"]++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
		case "/loki/api/v1/query_range":
			query = r.URL.Query().Get("query")
			if !strings.Contains(query, "orbit_workspace_id="+strconv.Quote(expectedWorkspace)) {
				t.Errorf("Loki query for workspace %s lacks exact tenant matcher: %s", expectedWorkspace, query)
			}
			if strings.Contains(query, "orbit_workspace_id="+strconv.Quote(otherWorkspaceID(expectedWorkspace, workspaceA, workspaceB))) {
				t.Errorf("Loki query for workspace %s contains the other tenant matcher: %s", expectedWorkspace, query)
			}
			calls["loki"]++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
		case "/api/search":
			query = r.URL.Query().Get("q")
			if !strings.Contains(query, "resource.orbit.workspace.id = "+strconv.Quote(expectedWorkspace)) {
				t.Errorf("Tempo query for workspace %s lacks exact tenant matcher: %s", expectedWorkspace, query)
			}
			if strings.Contains(query, "resource.orbit.workspace.id = "+strconv.Quote(otherWorkspaceID(expectedWorkspace, workspaceA, workspaceB))) {
				t.Errorf("Tempo query for workspace %s contains the other tenant matcher: %s", expectedWorkspace, query)
			}
			calls["tempo"]++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"traces":[]}`))
		default:
			t.Errorf("unexpected backend path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	window := QueryWindow{Name: "1h", Duration: time.Hour, Step: "1m"}
	for _, workspaceID := range []string{workspaceA, workspaceB} {
		expectedWorkspace = workspaceID
		if _, err := client.WorkspaceMetrics(context.Background(), workspaceID, window, "api-gateway", nil); err != nil {
			t.Fatalf("workspace %s metrics query: %v", workspaceID, err)
		}
		if _, err := client.WorkspaceLogs(context.Background(), workspaceID, window, "api-gateway", 10); err != nil {
			t.Fatalf("workspace %s logs query: %v", workspaceID, err)
		}
		if _, err := client.WorkspaceTraces(context.Background(), workspaceID, window, "api-gateway", 10); err != nil {
			t.Fatalf("workspace %s traces query: %v", workspaceID, err)
		}
	}
	if calls["prometheus"] != 2 || calls["loki"] != 2 || calls["tempo"] != 2 {
		t.Fatalf("backend call counts = %+v; want two tenant-scoped reads per backend", calls)
	}
}

func otherWorkspaceID(current, first, second string) string {
	if current == first {
		return second
	}
	return first
}

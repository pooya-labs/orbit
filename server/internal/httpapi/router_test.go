package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/telemetry"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type fakeRepository struct {
	services  []store.Service
	incidents []store.Incident
	catalog   []store.TelemetryService
	pingErr   error
	listErr   error
	catalogErr error
}

func (f *fakeRepository) Ping(context.Context) error { return f.pingErr }

func (f *fakeRepository) UpsertTelemetryServices(_ context.Context, services []store.TelemetryService) ([]store.TelemetryService, error) {
	if f.catalogErr != nil {
		return nil, f.catalogErr
	}
	now := time.Now().UTC()
	f.catalog = append([]store.TelemetryService(nil), services...)
	for index := range f.catalog {
		item := &f.catalog[index]
		if item.ID == "" {
			item.ID = "catalog-" + item.Name
		}
		if item.FirstSeenAt.IsZero() {
			item.FirstSeenAt = now
		}
		item.LastSeenAt = now
		item.LastDiscoveredAt = now
		if item.ResourceAttributes == nil {
			item.ResourceAttributes = map[string]any{}
		}
	}
	return f.catalog, nil
}
func (f *fakeRepository) ListServices(context.Context) ([]store.Service, error) {
	return f.services, f.listErr
}
func (f *fakeRepository) ListIncidents(_ context.Context, status string) ([]store.Incident, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if status == "" {
		return f.incidents, nil
	}
	result := make([]store.Incident, 0)
	for _, incident := range f.incidents {
		if incident.Status == status {
			result = append(result, incident)
		}
	}
	return result, nil
}
func (f *fakeRepository) Close() {}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHealthEndpoints(t *testing.T) {
	repository := &fakeRepository{}
	handler := NewHandler(Config{Version: "test", Environment: "test"}, repository, testLogger())

	for _, test := range []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "liveness", path: "/health/live", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{name: "readiness", path: "/health/ready", wantStatus: http.StatusOK, wantBody: `{"status":"ready"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if body := strings.TrimSpace(recorder.Body.String()); body != test.wantBody {
				t.Fatalf("body = %q, want %q", body, test.wantBody)
			}
			if recorder.Header().Get("X-Request-ID") == "" {
				t.Fatal("expected X-Request-ID response header")
			}
			if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Fatalf("content type = %q, want JSON", got)
			}
		})
	}
}

func TestReadinessWithoutDatabase(t *testing.T) {
	handler := NewHandler(Config{}, nil, testLogger())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestMetadataEndpoint(t *testing.T) {
	handler := NewHandler(Config{Version: "0.3.0", Environment: "test"}, &fakeRepository{}, testLogger())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var payload metadataResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Name != "orbit-api" || payload.Version != "0.3.0" || payload.Environment != "test" {
		t.Fatalf("unexpected metadata response: %+v", payload)
	}
}

func TestListServices(t *testing.T) {
	repository := &fakeRepository{services: []store.Service{{ID: "svc-1", Name: "api-gateway", Status: "operational", Source: "demo_seed"}}}
	handler := NewHandler(Config{}, repository, testLogger())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/services", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var payload servicesResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].Name != "api-gateway" {
		t.Fatalf("unexpected services response: %+v", payload)
	}
}

func TestListIncidentsAndFilter(t *testing.T) {
	repository := &fakeRepository{incidents: []store.Incident{
		{ID: "inc-1", Title: "Search stalled", Status: "investigating", StartedAt: time.Now()},
		{ID: "inc-2", Title: "Payment latency", Status: "monitoring", StartedAt: time.Now()},
	}}
	handler := NewHandler(Config{}, repository, testLogger())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/incidents?status=monitoring", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var payload incidentsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].Status != "monitoring" {
		t.Fatalf("unexpected incident response: %+v", payload)
	}
}

func TestInvalidIncidentStatus(t *testing.T) {
	handler := NewHandler(Config{}, &fakeRepository{}, testLogger())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/incidents?status=everything", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestRepositoryFailureIsSafe(t *testing.T) {
	handler := NewHandler(Config{}, &fakeRepository{listErr: errors.New("sensitive database detail")}, testLogger())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/services", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(recorder.Body.String(), "sensitive database detail") {
		t.Fatal("internal database error leaked to client")
	}
}

func TestCORSPreflightAllowsConfiguredOrigin(t *testing.T) {
	handler := NewHandler(Config{AllowedOrigins: []string{"https://example.com"}}, nil, testLogger())
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/services", nil)
	request.Header.Set("Origin", "https://example.com")
	request.Header.Set("Access-Control-Request-Method", "GET")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("allow origin = %q, want configured origin", got)
	}
	if !strings.Contains(recorder.Header().Get("Access-Control-Allow-Methods"), "GET") {
		t.Fatal("expected GET to be allowed")
	}
}

func TestCORSPreflightRejectsUnknownOrigin(t *testing.T) {
	handler := NewHandler(Config{AllowedOrigins: []string{"https://example.com"}}, nil, testLogger())
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/services", nil)
	request.Header.Set("Origin", "https://attacker.example")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected allow origin header %q", got)
	}
}

func TestUnknownRouteIsNotFound(t *testing.T) {
	handler := NewHandler(Config{}, nil, testLogger())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/not-a-route", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}


func TestTelemetryQueryEndpoints(t *testing.T) {
	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/label/service_name/values" {
			_, _ = w.Write([]byte(`{"status":"success","data":["orbit-telemetry-demo"]}`))
			return
		}
		if r.URL.Path != "/api/v1/query_range" || r.URL.Query().Get("query") != `sum(rate(orbit_demo_requests_total{service_name="orbit-telemetry-demo"}[5m]))` {
			t.Errorf("unexpected Prometheus request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1,"0.2"]]}]}}`))
	}))
	defer prometheus.Close()

	loki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/loki/api/v1/label/service_name/values" {
			_, _ = w.Write([]byte(`{"status":"success","data":["orbit-telemetry-demo"]}`))
			return
		}
		if r.URL.Path != "/loki/api/v1/query_range" || r.URL.Query().Get("query") != `{service_name="orbit-telemetry-demo"}` {
			t.Errorf("unexpected Loki request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		if r.URL.Query().Get("limit") != "9" {
			t.Errorf("Loki limit = %q, want 9", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[{"stream":{"service_name":"orbit-telemetry-demo"},"values":[["1","hello"]]}]}}`))
	}))
	defer loki.Close()

	tempo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/search/tag/service.name/values" {
			_, _ = w.Write([]byte(`{"tagValues":["orbit-telemetry-demo"]}`))
			return
		}
		if r.URL.Path != "/api/search" || r.URL.Query().Get("tags") != "service.name=orbit-telemetry-demo" {
			t.Errorf("unexpected Tempo request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"traces":[{"traceID":"1234567890abcdef1234567890abcdef","rootServiceName":"orbit-telemetry-demo","durationMs":8}]}`))
	}))
	defer tempo.Close()

	client, err := telemetry.NewClient(prometheus.URL, loki.URL, tempo.URL)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(Config{Telemetry: client}, &fakeRepository{}, testLogger())

	for _, path := range []string{
		"/api/v1/telemetry/metrics?window=1h&service=orbit-telemetry-demo",
		"/api/v1/telemetry/logs?window=1h&service=orbit-telemetry-demo&limit=9",
		"/api/v1/telemetry/traces?window=1h&service=orbit-telemetry-demo&limit=7",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", path, recorder.Code, recorder.Body.String())
		}
		var payload telemetry.QueryResult
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("%s response JSON: %v", path, err)
		}
		if payload.Source == "" || payload.Window != "1h" || payload.QueriedAt.IsZero() || len(payload.Data) == 0 {
			t.Fatalf("%s returned incomplete telemetry metadata: %+v", path, payload)
		}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/services?window=1h", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("service discovery status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var catalog telemetry.ServiceDiscoveryResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("decode service discovery response: %v", err)
	}
	if len(catalog.Data) != 1 || catalog.Data[0].Name != "orbit-telemetry-demo" || len(catalog.Data[0].Sources) != 3 {
		t.Fatalf("unexpected merged service catalog: %+v", catalog)
	}
	if len(repository.catalog) != 1 || repository.catalog[0].Name != "orbit-telemetry-demo" || len(repository.catalog[0].Sources) != 3 {
		t.Fatalf("expected service discovery to persist merged backend observations, got %+v", repository.catalog)
	}
	if catalog.Data[0].ID == "" || catalog.Data[0].FirstSeenAt.IsZero() || catalog.Data[0].LastDiscoveredAt.IsZero() {
		t.Fatalf("expected persisted catalog metadata in response, got %+v", catalog.Data[0])
	}
}

func TestTelemetryQueryValidationAndUnavailableResponses(t *testing.T) {
	handler := NewHandler(Config{}, &fakeRepository{}, testLogger())

	for _, test := range []struct {
		path       string
		wantStatus int
		wantCode   string
	}{
		{path: "/api/v1/telemetry/metrics?window=7d", wantStatus: http.StatusBadRequest, wantCode: "invalid_window"},
		{path: "/api/v1/telemetry/metrics?service=bad%20service", wantStatus: http.StatusBadRequest, wantCode: "invalid_service"},
		{path: "/api/v1/telemetry/logs?service=bad%20service", wantStatus: http.StatusBadRequest, wantCode: "invalid_service"},
		{path: "/api/v1/telemetry/traces?limit=101", wantStatus: http.StatusBadRequest, wantCode: "invalid_limit"},
		{path: "/api/v1/telemetry/metrics?window=1h", wantStatus: http.StatusServiceUnavailable, wantCode: "telemetry_unavailable"},
	} {
		t.Run(test.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			var payload errorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if payload.Error.Code != test.wantCode || payload.Error.RequestID == "" {
				t.Fatalf("unexpected error payload: %+v", payload.Error)
			}
		})
	}
}

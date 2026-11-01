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

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type fakeRepository struct {
	services  []store.Service
	incidents []store.Incident
	pingErr   error
	listErr   error
}

func (f *fakeRepository) Ping(context.Context) error { return f.pingErr }
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

func TestUnknownRouteIsNotFound(t *testing.T) {
	handler := NewHandler(Config{}, nil, testLogger())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/not-a-route", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

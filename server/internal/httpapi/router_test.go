package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	handler := NewHandler(Config{Version: "test", Environment: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "liveness", path: "/health/live", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{name: "readiness", path: "/health/ready", wantStatus: http.StatusOK, wantBody: `{"status":"ready"}`},
	}

	for _, test := range tests {
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

func TestMetadataEndpoint(t *testing.T) {
	handler := NewHandler(Config{Version: "0.3.0", Environment: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

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

func TestUnknownRouteIsNotFound(t *testing.T) {
	handler := NewHandler(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/not-a-route", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

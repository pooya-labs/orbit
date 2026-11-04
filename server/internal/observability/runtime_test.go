package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracesv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

type capturedOTLP struct {
	mu sync.Mutex
	payloads map[string][][]byte
}

func (c *capturedOTLP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	c.payloads[r.URL.Path] = append(c.payloads[r.URL.Path], body)
	c.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (c *capturedOTLP) all() map[string][][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string][][]byte, len(c.payloads))
	for path, items := range c.payloads {
		result[path] = append([][]byte(nil), items...)
	}
	return result
}

func TestHTTPMiddlewareExportsBoundedOTLPSignals(t *testing.T) {
	capture := &capturedOTLP{payloads: make(map[string][][]byte)}
	collector := httptest.NewServer(capture)
	defer collector.Close()

	ctx := context.Background()
	runtime, err := New(ctx, collector.URL, "test-version", "test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if runtime == nil || !runtime.Enabled() {
		t.Fatal("expected enabled OTLP runtime")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/telemetry/metrics", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	})
	handler := runtime.HTTPMiddleware(mux, func(request *http.Request) string {
		_, pattern := mux.Handler(request)
		return pattern
	})

	runtime.EmitStartupLog(ctx)
	request := httptest.NewRequest(http.MethodGet,
		"http://orbit.example/api/v1/workspaces/workspace-secret-123/telemetry/metrics?token=query-secret",
		nil,
	)
	request.Header.Set("Authorization", "Bearer authorization-secret")
	request.Header.Set("X-Request-ID", "request-id-secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("HTTP status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runtime.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	payloads := capture.all()
	tracePayloads := payloads["/v1/traces"]
	metricPayloads := payloads["/v1/metrics"]
	logPayloads := payloads["/v1/logs"]
	if len(tracePayloads) == 0 || len(metricPayloads) == 0 || len(logPayloads) == 0 {
		t.Fatalf("expected all three OTLP signals; got trace=%d metric=%d log=%d", len(tracePayloads), len(metricPayloads), len(logPayloads))
	}

	var traceRequest tracesv1.ExportTraceServiceRequest
	if err := proto.Unmarshal(tracePayloads[len(tracePayloads)-1], &traceRequest); err != nil {
		t.Fatalf("decode OTLP trace request: %v", err)
	}
	if len(traceRequest.ResourceSpans) == 0 || len(traceRequest.ResourceSpans[0].ScopeSpans) == 0 ||
		len(traceRequest.ResourceSpans[0].ScopeSpans[0].Spans) == 0 {
		t.Fatal("expected an API request span")
	}

	var metricRequest metricsv1.ExportMetricsServiceRequest
	if err := proto.Unmarshal(metricPayloads[len(metricPayloads)-1], &metricRequest); err != nil {
		t.Fatalf("decode OTLP metric request: %v", err)
	}
	if len(metricRequest.ResourceMetrics) == 0 || len(metricRequest.ResourceMetrics[0].ScopeMetrics) == 0 ||
		len(metricRequest.ResourceMetrics[0].ScopeMetrics[0].Metrics) == 0 {
		t.Fatal("expected API request count and duration metrics")
	}

	var logRequest logsv1.ExportLogsServiceRequest
	if err := proto.Unmarshal(logPayloads[len(logPayloads)-1], &logRequest); err != nil {
		t.Fatalf("decode OTLP log request: %v", err)
	}
	if len(logRequest.ResourceLogs) == 0 || len(logRequest.ResourceLogs[0].ScopeLogs) == 0 ||
		len(logRequest.ResourceLogs[0].ScopeLogs[0].LogRecords) == 0 {
		t.Fatal("expected bounded API lifecycle/error log records")
	}

	var exported strings.Builder
	for _, group := range [][]byte{tracePayloads[len(tracePayloads)-1], metricPayloads[len(metricPayloads)-1], logPayloads[len(logPayloads)-1]} {
		exported.Write(group)
	}
	for _, secret := range []string{
		"workspace-secret-123",
		"query-secret",
		"authorization-secret",
		"request-id-secret",
	} {
		if strings.Contains(exported.String(), secret) {
			t.Fatalf("OTLP payload unexpectedly contains sensitive request data %q", secret)
		}
	}
	for _, expected := range []string{"orbit-api", "http.route", "/api/v1/workspaces/{workspaceID}/telemetry/metrics", "http_server_request_failed", "orbit_api_runtime_started"} {
		if !strings.Contains(exported.String(), expected) {
			t.Fatalf("OTLP payload does not include expected bounded field %q", expected)
		}
	}
}

func TestNewWithoutEndpointDisablesInstrumentation(t *testing.T) {
	runtime, err := New(context.Background(), "", "test-version", "test")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if runtime != nil && runtime.Enabled() {
		t.Fatal("empty endpoint must disable self-observability")
	}
}

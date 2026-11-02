package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/auth"
	"github.com/pooya16pg-spec/orbit/server/internal/httpapi"
	"github.com/pooya16pg-spec/orbit/server/internal/telemetry"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracesv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracespb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

const (
	workspacePipelineService = "orbit-tenant-e2e"
	workspacePipelineToken   = "integration-workload-token"
)

type workspacePipelineHumanVerifier struct {
	issuer  string
	subject string
}

func (v workspacePipelineHumanVerifier) Verify(_ context.Context, token string) (auth.Principal, error) {
	if token != "integration-human-token" {
		return auth.Principal{}, errors.New("invalid integration human token")
	}
	return auth.Principal{Issuer: v.issuer, Subject: v.subject, Roles: []string{"viewer"}}, nil
}

// TestWorkspaceOTLPWriteToQueryIsolation exercises the complete tenant data
// path against real PostgreSQL, the authenticated HTTP gateway, the local
// Collector, and all three telemetry backends. The API read path is served by
// the real workspace router and uses the same PostgreSQL membership checks as
// the running API. Only the token verifiers are deterministic test fixtures;
// the workload grant lookup and all data-plane hops are real.
func TestWorkspaceOTLPWriteToQueryIsolation(t *testing.T) {
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	collectorURL := os.Getenv("ORBIT_TEST_COLLECTOR_OTLP_HTTP_URL")
	prometheusURL := os.Getenv("ORBIT_TEST_PROMETHEUS_URL")
	lokiURL := os.Getenv("ORBIT_TEST_LOKI_URL")
	tempoURL := os.Getenv("ORBIT_TEST_TEMPO_URL")
	if databaseURL == "" || collectorURL == "" || prometheusURL == "" || lokiURL == "" || tempoURL == "" {
		t.Skip("workspace OTLP pipeline test requires PostgreSQL, Collector, Prometheus, Loki, and Tempo URLs")
	}

	repository, workspaceA, workspaceB, _, workloadVerifier := openGatewayIntegrationRepository(t)
	if workspaceA.CreatedByIssuer == "" || workspaceA.CreatedBySubject == "" {
		t.Fatal("workspace fixture does not expose its owning principal to the integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	authorizer, err := NewAuthorizer(workloadVerifier, repository)
	if err != nil {
		t.Fatalf("create workload authorizer: %v", err)
	}
	forwarder, err := NewCollectorHTTPForwarder(collectorURL, 10*time.Second)
	if err != nil {
		t.Fatalf("create real Collector forwarder: %v", err)
	}
	gateway, err := NewHTTPGateway(authorizer, forwarder, HTTPGatewayConfig{
		RequestsPerMinute: 200,
		ForwardTimeout:    10 * time.Second,
		RateLimitStore:    repository,
	})
	if err != nil {
		t.Fatalf("create authenticated HTTP gateway: %v", err)
	}

	telemetryClient, err := telemetry.NewClient(prometheusURL, lokiURL, tempoURL)
	if err != nil {
		t.Fatalf("create real telemetry query client: %v", err)
	}
	humanVerifier := workspacePipelineHumanVerifier{
		issuer: workspaceA.CreatedByIssuer, subject: workspaceA.CreatedBySubject,
	}
	api := httpapi.NewHandler(httpapi.Config{
		Version: "workspace-tenant-e2e",
		Environment: "staging",
		Telemetry: telemetryClient,
		AuthVerifier: humanVerifier,
		RateLimitPerMinute: 200,
	}, repository, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// A workload granted only to A must be denied when the route asks it to
	// write directly to B. No bytes may reach the real Collector.
	unauthorizedPayload, err := proto.Marshal(workspacePipelineMetricsRequest(1))
	if err != nil {
		t.Fatal(err)
	}
	denied := workspacePipelineExport(gateway, workspaceB.ID, "metrics", unauthorizedPayload, workspacePipelineToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("workspace B export status = %d, want 403; body=%s", denied.Code, denied.Body.String())
	}

	// The same producer cannot smuggle B's tenant identifier through a
	// producer-controlled resource attribute while targeting workspace A.
	forged := workspacePipelineMetricsRequest(1)
	forged.ResourceMetrics[0].Resource.Attributes = append(
		forged.ResourceMetrics[0].Resource.Attributes,
		stringAttribute(WorkspaceAttribute, workspaceB.ID),
	)
	forgedPayload, err := proto.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	forgedResponse := workspacePipelineExport(gateway, workspaceA.ID, "metrics", forgedPayload, workspacePipelineToken)
	if forgedResponse.Code != http.StatusBadRequest {
		t.Fatalf("producer-forged tenant attribute status = %d, want 400; body=%s", forgedResponse.Code, forgedResponse.Body.String())
	}

	// Send all three signals through the real authorization/gateway/Collector
	// path. Repeated cumulative metric exports span at least one Prometheus
	// scrape interval so rate() has more than one sample to evaluate.
	for value := int64(10); value <= 30; value += 10 {
		payload, marshalErr := proto.Marshal(workspacePipelineMetricsRequest(value))
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		response := workspacePipelineExport(gateway, workspaceA.ID, "metrics", payload, workspacePipelineToken)
		if response.Code != http.StatusOK {
			t.Fatalf("workspace A metrics export status = %d; body=%s", response.Code, response.Body.String())
		}
		if value != 30 {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(8 * time.Second):
			}
		}
	}

	logPayload, err := proto.Marshal(workspacePipelineLogsRequest("workspace-a-only-log-marker"))
	if err != nil {
		t.Fatal(err)
	}
	logResponse := workspacePipelineExport(gateway, workspaceA.ID, "logs", logPayload, workspacePipelineToken)
	if logResponse.Code != http.StatusOK {
		t.Fatalf("workspace A logs export status = %d; body=%s", logResponse.Code, logResponse.Body.String())
	}

	tracePayload, err := proto.Marshal(workspacePipelineTracesRequest())
	if err != nil {
		t.Fatal(err)
	}
	traceResponse := workspacePipelineExport(gateway, workspaceA.ID, "traces", tracePayload, workspacePipelineToken)
	if traceResponse.Code != http.StatusOK {
		t.Fatalf("workspace A traces export status = %d; body=%s", traceResponse.Code, traceResponse.Body.String())
	}

	window, err := telemetry.ParseWindow("15m")
	if err != nil {
		t.Fatal(err)
	}

	for _, signal := range []struct {
		name string
		path string
	}{
		{name: "metrics", path: "/metrics?window=15m&service=" + workspacePipelineService + "&group_by=none"},
		{name: "logs", path: "/logs?window=15m&service=" + workspacePipelineService + "&limit=50"},
		{name: "traces", path: "/traces?window=15m&service=" + workspacePipelineService + "&limit=50"},
	} {
		signal := signal
		t.Run(signal.name, func(t *testing.T) {
			workspacePath := "/api/v1/workspaces/%s/telemetry" + signal.path
			waitForWorkspacePipelineData(t, ctx, api, fmt.Sprintf(workspacePath, workspaceA.ID), true)
			waitForWorkspacePipelineData(t, ctx, api, fmt.Sprintf(workspacePath, workspaceB.ID), false)
		})
	}

	// Keep a direct client assertion alongside the HTTP route assertions so
	// failures distinguish a backend matcher problem from router/membership code.
	metricsA, err := telemetryClient.WorkspaceMetrics(ctx, workspaceA.ID, window, workspacePipelineService, []string{})
	if err != nil {
		t.Fatalf("workspace A Prometheus query: %v", err)
	}
	metricsB, err := telemetryClient.WorkspaceMetrics(ctx, workspaceB.ID, window, workspacePipelineService, []string{})
	if err != nil {
		t.Fatalf("workspace B Prometheus query: %v", err)
	}
	if !workspacePipelineHasData(metricsA.Data) || workspacePipelineHasData(metricsB.Data) {
		t.Fatalf("Prometheus tenant isolation failed: A=%s B=%s", metricsA.Data, metricsB.Data)
	}
}

func workspacePipelineExport(gateway http.Handler, workspaceID, signal string, payload []byte, token string) *httptest.ResponseRecorder {
	path := "/v1/workspaces/" + workspaceID + "/v1/" + signal
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	return response
}

func waitForWorkspacePipelineData(t *testing.T, ctx context.Context, handler http.Handler, path string, wantData bool) {
	t.Helper()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(100 * time.Second)
	defer deadline.Stop()

	for {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer integration-human-token")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code == http.StatusOK {
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode workspace telemetry %s response: %v; body=%s", path, err, response.Body.String())
			}
			raw := envelope["data"]
			hasData := workspacePipelineHasData(raw)
			if hasData == wantData {
				return
			}
		} else if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("workspace telemetry query %s returned HTTP %d: %s", path, response.Code, response.Body.String())
		}

		select {
		case <-ctx.Done():
			t.Fatalf("workspace telemetry query deadline: %v", ctx.Err())
		case <-deadline.C:
			t.Fatalf("workspace telemetry query %s did not reach expected data=%t; last response HTTP %d: %s",
				path, wantData, response.Code, response.Body.String())
		case <-ticker.C:
		}
	}
}

func workspacePipelineHasData(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == "[]" || trimmed == "{}" {
		return false
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) == nil {
		if result, ok := envelope["result"]; ok {
			return workspacePipelineHasData(result)
		}
	}
	return true
}

func workspacePipelineResource() *resourcev1.Resource {
	return &resourcev1.Resource{Attributes: []*commonv1.KeyValue{
		stringAttribute("service.name", workspacePipelineService),
		stringAttribute("service.version", "e2e-1"),
		stringAttribute("deployment.environment", "ci"),
	}}
}

func stringAttribute(key, value string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key: key,
		Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}},
	}
}

func workspacePipelineMetricsRequest(value int64) *metricsv1.ExportMetricsServiceRequest {
	return &metricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource: workspacePipelineResource(),
			ScopeMetrics: []*metricspb.ScopeMetrics{{
				Scope: &commonv1.InstrumentationScope{Name: "orbit.workspace.pipeline.e2e"},
				Metrics: []*metricspb.Metric{{
					Name: "orbit_demo_requests_total",
					Unit: "1",
					Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
						AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
						IsMonotonic: true,
						DataPoints: []*metricspb.NumberDataPoint{{
							TimeUnixNano: uint64(time.Now().UnixNano()),
							Value: &metricspb.NumberDataPoint_AsInt{AsInt: value},
						}},
					}},
				}},
			}},
		}},
	}
}

func workspacePipelineLogsRequest(marker string) *logsv1.ExportLogsServiceRequest {
	return &logsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			Resource: workspacePipelineResource(),
			ScopeLogs: []*logspb.ScopeLogs{{
				Scope: &commonv1.InstrumentationScope{Name: "orbit.workspace.pipeline.e2e"},
				LogRecords: []*logspb.LogRecord{{
					TimeUnixNano: uint64(time.Now().UnixNano()),
					SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_INFO,
					SeverityText: "INFO",
					Body: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: marker}},
				}},
			}},
		}},
	}
}

func workspacePipelineTracesRequest() *tracesv1.ExportTraceServiceRequest {
	traceID := make([]byte, 16)
	spanID := make([]byte, 8)
	for index := range traceID {
		traceID[index] = byte(index + 1)
	}
	for index := range spanID {
		spanID[index] = byte(index + 17)
	}
	start := uint64(time.Now().Add(-time.Second).UnixNano())
	end := uint64(time.Now().UnixNano())
	return &tracesv1.ExportTraceServiceRequest{
		ResourceSpans: []*tracespb.ResourceSpans{{
			Resource: workspacePipelineResource(),
			ScopeSpans: []*tracespb.ScopeSpans{{
				Scope: &commonv1.InstrumentationScope{Name: "orbit.workspace.pipeline.e2e"},
				Spans: []*tracespb.Span{{
					TraceId: traceID,
					SpanId: spanID,
					Name: "workspace-a-only-trace-marker",
					Kind: tracespb.Span_SPAN_KIND_INTERNAL,
					StartTimeUnixNano: start,
					EndTimeUnixNano: end,
				}},
			}},
		}},
	}
}


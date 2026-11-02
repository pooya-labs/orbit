package ingest

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
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

type workloadResourceRecorderStub struct {
	calls int
	workspaceID string
	observations []store.WorkloadResourceObservation
	err error
}

func (r *workloadResourceRecorderStub) RecordWorkloadResourceAttributes(
	_ context.Context,
	workspaceID string,
	observations []store.WorkloadResourceObservation,
) error {
	r.calls++
	r.workspaceID = workspaceID
	r.observations = append([]store.WorkloadResourceObservation(nil), observations...)
	return r.err
}

func testOTLPResource() *resourcev1.Resource {
	return &resourcev1.Resource{Attributes: []*commonv1.KeyValue{
		stringAttribute("service.name", "payments"),
		stringAttribute("service.version", "1.2.3"),
		stringAttribute("deployment.environment", "production"),
		stringAttribute("cloud.region", "eu-central-1"),
		stringAttribute("user.email", "private@example.invalid"),
		stringAttribute("process.command_line", "secret --token abc"),
		stringAttribute("k8s.pod.uid", "high-cardinality-id"),
		stringAttribute(WorkspaceAttribute, "producer-forged-workspace"),
	}}
}

func resourceStringValues(t *testing.T, observation store.WorkloadResourceObservation, key string) []any {
	t.Helper()
	raw, ok := observation.Attributes[key]
	if !ok {
		t.Fatalf("resource attribute %q is missing from %+v", key, observation.Attributes)
	}
	values, ok := raw.([]any)
	if !ok {
		t.Fatalf("resource attribute %q has type %T, want []any", key, raw)
	}
	return values
}

func TestExtractWorkloadResourceAttributesSupportsAllSignalsAndFiltersUnsafeFields(t *testing.T) {
	cases := []struct {
		name string
		request proto.Message
	}{
		{
			name: "metrics",
			request: &metricsv1.ExportMetricsServiceRequest{
				ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: testOTLPResource()}},
			},
		},
		{
			name: "logs",
			request: &logsv1.ExportLogsServiceRequest{
				ResourceLogs: []*logspb.ResourceLogs{{Resource: testOTLPResource()}},
			},
		},
		{
			name: "traces",
			request: &tracesv1.ExportTraceServiceRequest{
				ResourceSpans: []*tracespb.ResourceSpans{{Resource: testOTLPResource()}},
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			observations := ExtractWorkloadResourceAttributes(test.request)
			if len(observations) != 1 || observations[0].ServiceName != "payments" {
				t.Fatalf("observations = %+v, want one payments service", observations)
			}
			values := observations[0].Attributes
			if got := resourceStringValues(t, observations[0], "service.name"); len(got) != 1 || got[0] != "payments" {
				t.Fatalf("service.name values = %#v", got)
			}
			if got := resourceStringValues(t, observations[0], "service.version"); len(got) != 1 || got[0] != "1.2.3" {
				t.Fatalf("service.version values = %#v", got)
			}
			if got := resourceStringValues(t, observations[0], "deployment.environment.name"); len(got) != 1 || got[0] != "production" {
				t.Fatalf("normalized deployment environment = %#v", got)
			}
			for _, excluded := range []string{"user.email", "process.command_line", "k8s.pod.uid", WorkspaceAttribute} {
				if _, exists := values[excluded]; exists {
					t.Errorf("unsafe or tenant attribute %q was persisted", excluded)
				}
			}
		})
	}
}

func TestExtractWorkloadResourceAttributesGroupsServicesAndCapsValues(t *testing.T) {
	first := testOTLPResource()
	second := &resourcev1.Resource{Attributes: []*commonv1.KeyValue{
		stringAttribute("service.name", "payments"),
		stringAttribute("service.version", "1.2.3"),
		stringAttribute("host.name", "node-a"),
	}}
	third := &resourcev1.Resource{Attributes: []*commonv1.KeyValue{
		stringAttribute("service.name", "orders"),
		stringAttribute("service.version", "4.5.6"),
	}}
	request := &metricsv1.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{
		{Resource: first}, {Resource: second}, {Resource: third},
	}}
	observations := ExtractWorkloadResourceAttributes(request)
	if len(observations) != 2 {
		t.Fatalf("observations = %+v, want two distinct services", observations)
	}
	if observations[0].ServiceName != "orders" || observations[1].ServiceName != "payments" {
		t.Fatalf("service observations are not deterministically ordered: %+v", observations)
	}
	paymentVersions := resourceStringValues(t, observations[1], "service.version")
	if len(paymentVersions) != 1 || paymentVersions[0] != "1.2.3" {
		t.Fatalf("duplicate resource version should be deduplicated: %#v", paymentVersions)
	}
}

func TestHTTPGatewayRecordsResourceMetadataOnlyAfterSuccessfulExportAndFailsOpen(t *testing.T) {
	verifier, grants := validStubs()
	authorizer, err := NewAuthorizer(verifier, grants)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &workloadResourceRecorderStub{}
	forwarder := &forwarderStub{}
	var recordError error
	handler, err := NewHTTPGateway(authorizer, forwarder, HTTPGatewayConfig{
		RequestsPerMinute: 10,
		ForwardTimeout: 2 * time.Second,
		ResourceAttributeRecorder: recorder,
		OnResourceAttributeError: func(err error) { recordError = err },
	})
	if err != nil {
		t.Fatal(err)
	}
	send := func() *httptest.ResponseRecorder {
		payload, marshalErr := proto.Marshal(&metricsv1.ExportMetricsServiceRequest{
			ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: testOTLPResource()}},
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+testWorkspace+"/v1/metrics", bytes.NewReader(payload))
		request.Header.Set("Authorization", "Bearer signed-token")
		request.Header.Set("Content-Type", "application/x-protobuf")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	response := send()
	if response.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, body=%s", response.Code, response.Body.String())
	}
	if recorder.calls != 1 || recorder.workspaceID != testWorkspace {
		t.Fatalf("resource recorder = %+v, want one call for authorized workspace", recorder)
	}
	if len(recorder.observations) != 1 || recorder.observations[0].ServiceName != "payments" {
		t.Fatalf("recorded observations = %+v", recorder.observations)
	}

	forwarder.err = errors.New("collector unavailable")
	response = send()
	if response.Code != http.StatusBadGateway {
		t.Fatalf("Collector failure status = %d, want 502", response.Code)
	}
	if recorder.calls != 1 {
		t.Fatalf("resource metadata was recorded despite a failed export; calls=%d", recorder.calls)
	}

	forwarder.err = nil
	recorder.err = errors.New("metadata database temporarily unavailable")
	response = send()
	if response.Code != http.StatusOK {
		t.Fatalf("metadata persistence failure changed successful OTLP response: %d", response.Code)
	}
	if recorder.calls != 2 || !errors.Is(recordError, recorder.err) {
		t.Fatalf("recorder failure was not reported: calls=%d error=%v", recorder.calls, recordError)
	}
}

func TestGRPCGatewayRecordsAuthorizedWorkspaceResourceMetadata(t *testing.T) {
	gateway, forwarder := newTestGRPCGateway()
	recorder := &workloadResourceRecorderStub{}
	gateway.resourceAttributeRecorder = recorder
	request := &metricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: testOTLPResource()}},
	}
	if _, err := (&metricsGRPCService{gateway: gateway}).Export(
		grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer signed-token"}),
		request,
	); err != nil {
		t.Fatalf("gRPC metrics export failed: %v", err)
	}
	if forwarder.metricsCalls != 1 || recorder.calls != 1 || recorder.workspaceID != testWorkspace {
		t.Fatalf("forwarder=%d recorder=%+v", forwarder.metricsCalls, recorder)
	}
	if len(recorder.observations) != 1 || recorder.observations[0].ServiceName != "payments" {
		t.Fatalf("recorded observations = %+v", recorder.observations)
	}
}

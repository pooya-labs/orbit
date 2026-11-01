package ingest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracesv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracespb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type grpcForwarderStub struct {
	metricsCalls int
	logsCalls int
	tracesCalls int
	metricsRequest *metricsv1.ExportMetricsServiceRequest
	logsRequest *logsv1.ExportLogsServiceRequest
	tracesRequest *tracesv1.ExportTraceServiceRequest
	err error
}

func (f *grpcForwarderStub) ExportMetrics(_ context.Context, request *metricsv1.ExportMetricsServiceRequest) (*metricsv1.ExportMetricsServiceResponse, error) {
	f.metricsCalls++
	f.metricsRequest = proto.Clone(request).(*metricsv1.ExportMetricsServiceRequest)
	if f.err != nil { return nil, f.err }
	return &metricsv1.ExportMetricsServiceResponse{}, nil
}

func (f *grpcForwarderStub) ExportLogs(_ context.Context, request *logsv1.ExportLogsServiceRequest) (*logsv1.ExportLogsServiceResponse, error) {
	f.logsCalls++
	f.logsRequest = proto.Clone(request).(*logsv1.ExportLogsServiceRequest)
	if f.err != nil { return nil, f.err }
	return &logsv1.ExportLogsServiceResponse{}, nil
}

func (f *grpcForwarderStub) ExportTraces(_ context.Context, request *tracesv1.ExportTraceServiceRequest) (*tracesv1.ExportTraceServiceResponse, error) {
	f.tracesCalls++
	f.tracesRequest = proto.Clone(request).(*tracesv1.ExportTraceServiceRequest)
	if f.err != nil { return nil, f.err }
	return &tracesv1.ExportTraceServiceResponse{}, nil
}

func newTestGRPCGateway() (*GRPCGateway, *grpcForwarderStub) {
	verifier, grants := validStubs()
	authorizer, _ := NewAuthorizer(verifier, grants)
	forwarder := &grpcForwarderStub{}
	return &GRPCGateway{
		authorizer: authorizer,
		forwarder: forwarder,
		limiter: newWorkspaceRateLimiter(10),
		timeout: 2 * time.Second,
		concurrency: make(chan struct{}, maxConcurrentGRPCExports),
	}, forwarder
}

func grpcWorkloadContext(workspaces, authorizations []string) context.Context {
	md := metadata.MD{}
	for _, value := range workspaces { md.Append(workspaceMetadataKey, value) }
	for _, value := range authorizations { md.Append("authorization", value) }
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestGRPCGatewayForwardsAllThreeSignalsWithCanonicalWorkspace(t *testing.T) {
	gateway, forwarder := newTestGRPCGateway()
	ctx := grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer signed-token"})

	metricsRequest := &metricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcev1.Resource{}}},
	}
	if _, err := (&metricsGRPCService{gateway: gateway}).Export(ctx, metricsRequest); err != nil {
		t.Fatalf("metrics Export() error = %v", err)
	}
	if forwarder.metricsCalls != 1 || attributeValue(forwarder.metricsRequest.ResourceMetrics[0].Resource, WorkspaceAttribute) != testWorkspace {
		t.Fatal("metrics were not forwarded with canonical workspace attribution")
	}
	if attributeValue(metricsRequest.ResourceMetrics[0].Resource, WorkspaceAttribute) != "" {
		t.Fatal("gateway mutated the caller's metrics request")
	}

	logsRequest := &logsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{Resource: &resourcev1.Resource{}}},
	}
	if _, err := (&logsGRPCService{gateway: gateway}).Export(ctx, logsRequest); err != nil {
		t.Fatalf("logs Export() error = %v", err)
	}
	if forwarder.logsCalls != 1 || attributeValue(forwarder.logsRequest.ResourceLogs[0].Resource, WorkspaceAttribute) != testWorkspace {
		t.Fatal("logs were not forwarded with canonical workspace attribution")
	}

	tracesRequest := &tracesv1.ExportTraceServiceRequest{
		ResourceSpans: []*tracespb.ResourceSpans{{Resource: &resourcev1.Resource{}}},
	}
	if _, err := (&tracesGRPCService{gateway: gateway}).Export(ctx, tracesRequest); err != nil {
		t.Fatalf("traces Export() error = %v", err)
	}
	if forwarder.tracesCalls != 1 || attributeValue(forwarder.tracesRequest.ResourceSpans[0].Resource, WorkspaceAttribute) != testWorkspace {
		t.Fatal("traces were not forwarded with canonical workspace attribution")
	}
}

func TestGRPCGatewayRequiresExactlyOneWorkspaceAndBearerMetadata(t *testing.T) {
	for _, test := range []struct {
		name string
		workspaces []string
		authorizations []string
		want codes.Code
	}{
		{name: "missing workspace", authorizations: []string{"Bearer token"}, want: codes.InvalidArgument},
		{name: "duplicate workspace", workspaces: []string{testWorkspace, testWorkspace}, authorizations: []string{"Bearer token"}, want: codes.InvalidArgument},
		{name: "missing authorization", workspaces: []string{testWorkspace}, want: codes.Unauthenticated},
		{name: "duplicate authorization", workspaces: []string{testWorkspace}, authorizations: []string{"Bearer one", "Bearer two"}, want: codes.Unauthenticated},
		{name: "invalid workspace", workspaces: []string{"not-a-uuid"}, authorizations: []string{"Bearer token"}, want: codes.InvalidArgument},
	} {
		t.Run(test.name, func(t *testing.T) {
			gateway, forwarder := newTestGRPCGateway()
			_, err := (&metricsGRPCService{gateway: gateway}).Export(
				grpcWorkloadContext(test.workspaces, test.authorizations),
				&metricsv1.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcev1.Resource{}}}},
			)
			if status.Code(err) != test.want {
				t.Fatalf("gRPC code = %s, want %s; err=%v", status.Code(err), test.want, err)
			}
			if forwarder.metricsCalls != 0 {
				t.Fatal("invalid metadata reached Collector")
			}
		})
	}
}

func TestGRPCGatewayRejectsForgedTenantAttributes(t *testing.T) {
	gateway, forwarder := newTestGRPCGateway()
	request := &metricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{
				Key: WorkspaceAttribute,
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "forged"}},
			}}},
		}},
	}
	_, err := (&metricsGRPCService{gateway: gateway}).Export(
		grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer signed-token"}), request,
	)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("gRPC code = %s, want InvalidArgument", status.Code(err))
	}
	if forwarder.metricsCalls != 0 {
		t.Fatal("forged tenant payload reached Collector")
	}
	if attributeValue(request.ResourceMetrics[0].Resource, WorkspaceAttribute) != "forged" {
		t.Fatal("rejected request was mutated")
	}
}

func TestGRPCGatewayMapsAuthorizationAndDependencyFailures(t *testing.T) {
	gateway, forwarder := newTestGRPCGateway()
	gateway.authorizer.grants.(*grantStub).err = errors.New("database unavailable")
	request := &metricsv1.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcev1.Resource{}}}}
	_, err := (&metricsGRPCService{gateway: gateway}).Export(
		grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer signed-token"}), request,
	)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("database error code = %s, want Unavailable", status.Code(err))
	}
	if forwarder.metricsCalls != 0 {
		t.Fatal("database failure reached Collector")
	}

	gateway, forwarder = newTestGRPCGateway()
	gateway.authorizer.grants.(*grantStub).err = store.ErrWorkloadAuthorizationDenied
	_, err = (&metricsGRPCService{gateway: gateway}).Export(
		grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer signed-token"}), request,
	)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("grant denial code = %s, want PermissionDenied", status.Code(err))
	}
	if forwarder.metricsCalls != 0 {
		t.Fatal("denied workload reached Collector")
	}
}


func TestGRPCGatewayBoundsAdmissionBeforeTokenAndDatabaseWork(t *testing.T) {
	gateway, forwarder := newTestGRPCGateway()
	verifier := gateway.authorizer.verifier.(*verifierStub)
	grants := gateway.authorizer.grants.(*grantStub)
	gateway.concurrency = make(chan struct{}, 1)
	gateway.concurrency <- struct{}{}
	request := &metricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcev1.Resource{}}},
	}
	_, err := (&metricsGRPCService{gateway: gateway}).Export(
		grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer signed-token"}),
		request,
	)
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("gRPC code = %s, want ResourceExhausted; err=%v", status.Code(err), err)
	}
	if verifier.calls != 0 || grants.calls != 0 || forwarder.metricsCalls != 0 {
		t.Fatalf("saturated gateway performed auth/database/forwarding work: verifier=%d grants=%d forwarder=%d", verifier.calls, grants.calls, forwarder.metricsCalls)
	}
}

func TestGRPCGatewayEnforcesPayloadSizeAndSharedRateLimit(t *testing.T) {
	gateway, forwarder := newTestGRPCGateway()
	oversized := &metricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{
				Key: "large.attribute",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: strings.Repeat("x", maxOTLPRequestBytes)}},
			}}},
		}},
	}
	_, err := (&metricsGRPCService{gateway: gateway}).Export(
		grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer signed-token"}), oversized,
	)
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("oversized request code = %s, want ResourceExhausted", status.Code(err))
	}
	if forwarder.metricsCalls != 0 {
		t.Fatal("oversized request reached Collector")
	}

	verifier, grants := validStubs()
	authorizer, _ := NewAuthorizer(verifier, grants)
	shared := &rateLimitStoreStub{}
	forwarder = &grpcForwarderStub{}
	gateway = &GRPCGateway{
		authorizer: authorizer, forwarder: forwarder,
		limiter: newWorkspaceRateLimiter(1), sharedLimiter: shared,
		timeout: time.Second, concurrency: make(chan struct{}, maxConcurrentGRPCExports),
	}
	validRequest := &metricsv1.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcev1.Resource{}}}}
	ctx := grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer signed-token"})
	if _, err := (&metricsGRPCService{gateway: gateway}).Export(ctx, validRequest); err != nil {
		t.Fatalf("first request error = %v", err)
	}
	if _, err := (&metricsGRPCService{gateway: gateway}).Export(ctx, validRequest); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("second request code = %s, want ResourceExhausted", status.Code(err))
	}
	if len(shared.key) != 64 || shared.key == "workload:"+testIdentity+":"+testWorkspace+":metrics" {
		t.Fatalf("shared limiter key was not hashed: %q", shared.key)
	}
	if forwarder.metricsCalls != 1 {
		t.Fatalf("Collector calls = %d, want 1", forwarder.metricsCalls)
	}
}

func TestNewGRPCGatewayServerRegistersOTLPServicesAndValidatesConfig(t *testing.T) {
	gateway, forwarder := newTestGRPCGateway()
	server, err := NewGRPCGatewayServer(gateway.authorizer, forwarder, GRPCGatewayConfig{RequestsPerMinute: 100})
	if err != nil {
		t.Fatal(err)
	}
	services := server.GetServiceInfo()
	for _, name := range []string{
		"opentelemetry.proto.collector.metrics.v1.MetricsService",
		"opentelemetry.proto.collector.logs.v1.LogsService",
		"opentelemetry.proto.collector.trace.v1.TraceService",
	} {
		if _, ok := services[name]; !ok {
			t.Fatalf("gRPC server did not register %s", name)
		}
	}
	server.Stop()
	if _, err := NewGRPCGatewayServer(gateway.authorizer, forwarder, GRPCGatewayConfig{RequestsPerMinute: 0}); err == nil {
		t.Fatal("server accepted a zero rate limit")
	}
}

func TestNewCollectorGRPCForwarderValidatesEndpoint(t *testing.T) {
	for _, endpoint := range []string{"", "http://collector:4317", "collector", "collector:0", "collector:70000"} {
		if _, err := NewCollectorGRPCForwarder(endpoint); err == nil {
			t.Fatalf("NewCollectorGRPCForwarder(%q) unexpectedly succeeded", endpoint)
		}
	}
}

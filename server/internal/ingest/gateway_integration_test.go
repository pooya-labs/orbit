package ingest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/auth"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
	"github.com/pooya16pg-spec/orbit/server/internal/store/postgres"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracesv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracespb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type integrationWorkloadVerifier struct {
	issuer string
	subject string
}

func (v integrationWorkloadVerifier) Verify(_ context.Context, token string) (auth.Principal, error) {
	if token != "integration-workload-token" {
		return auth.Principal{}, errors.New("invalid test credential")
	}
	return auth.Principal{Issuer: v.issuer, Subject: v.subject}, nil
}

func openGatewayIntegrationRepository(t *testing.T) (*postgres.Repository, store.Workspace, store.Workspace, store.WorkloadIdentity, integrationWorkloadVerifier) {
	t.Helper()
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open integration PostgreSQL repository: %v", err)
	}
	t.Cleanup(repository.Close)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	actorIssuer := "https://identity.example/"
	actorSubject := "otlp-gateway-integration-" + suffix
	workspaceA, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "Gateway workspace A " + suffix,
		Slug: "gateway-a-" + suffix,
		OwnerIssuer: actorIssuer,
		OwnerSubject: actorSubject,
	})
	if err != nil {
		t.Fatalf("create workspace A: %v", err)
	}
	workspaceB, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "Gateway workspace B " + suffix,
		Slug: "gateway-b-" + suffix,
		OwnerIssuer: actorIssuer,
		OwnerSubject: actorSubject,
	})
	if err != nil {
		t.Fatalf("create workspace B: %v", err)
	}

	verifier := integrationWorkloadVerifier{
		issuer: "https://workloads.example/",
		subject: "collector-" + suffix,
	}
	identity, err := repository.CreateWorkloadIdentity(ctx, store.CreateWorkloadIdentityInput{
		Issuer: verifier.issuer,
		Subject: verifier.subject,
		DisplayName: "OTLP gateway integration collector",
		ActorIssuer: actorIssuer,
		ActorSubject: actorSubject,
	})
	if err != nil {
		t.Fatalf("create workload identity: %v", err)
	}
	for _, signal := range []string{"metrics", "logs", "traces"} {
		if err := repository.SetWorkloadWorkspaceGrant(ctx, store.WorkloadGrantInput{
			IdentityID: identity.ID,
			WorkspaceID: workspaceA.ID,
			Signal: signal,
			ActorIssuer: actorIssuer,
			ActorSubject: actorSubject,
		}, true); err != nil {
			t.Fatalf("grant %s in workspace A: %v", signal, err)
		}
	}
	return repository, workspaceA, workspaceB, identity, verifier
}

func integrationOTLPRequest(t *testing.T, signal string) proto.Message {
	t.Helper()
	message, _, err := newSignalMessages(signal)
	if err != nil {
		t.Fatal(err)
	}
	switch request := message.(type) {
	case *metricsv1.ExportMetricsServiceRequest:
		request.ResourceMetrics = []*metricspb.ResourceMetrics{{Resource: &resourcev1.Resource{}}}
	case *logsv1.ExportLogsServiceRequest:
		request.ResourceLogs = []*logspb.ResourceLogs{{Resource: &resourcev1.Resource{}}}
	case *tracesv1.ExportTraceServiceRequest:
		request.ResourceSpans = []*tracespb.ResourceSpans{{Resource: &resourcev1.Resource{}}}
	default:
		t.Fatalf("unexpected OTLP request type %T", message)
	}
	return message
}

func callIntegrationGRPCExport(gateway *GRPCGateway, workspaceID, signal string) error {
	ctx := grpcWorkloadContext([]string{workspaceID}, []string{"Bearer integration-workload-token"})
	switch signal {
	case "metrics":
		_, err := (&metricsGRPCService{gateway: gateway}).Export(ctx, integrationOTLPRequestForGRPC(signal).(*metricsv1.ExportMetricsServiceRequest))
		return err
	case "logs":
		_, err := (&logsGRPCService{gateway: gateway}).Export(ctx, integrationOTLPRequestForGRPC(signal).(*logsv1.ExportLogsServiceRequest))
		return err
	case "traces":
		_, err := (&tracesGRPCService{gateway: gateway}).Export(ctx, integrationOTLPRequestForGRPC(signal).(*tracesv1.ExportTraceServiceRequest))
		return err
	default:
		return errors.New("unsupported test signal")
	}
}

func integrationOTLPRequestForGRPC(signal string) proto.Message {
	message, _, _ := newSignalMessages(signal)
	switch request := message.(type) {
	case *metricsv1.ExportMetricsServiceRequest:
		request.ResourceMetrics = []*metricspb.ResourceMetrics{{Resource: &resourcev1.Resource{}}}
	case *logsv1.ExportLogsServiceRequest:
		request.ResourceLogs = []*logspb.ResourceLogs{{Resource: &resourcev1.Resource{}}}
	case *tracesv1.ExportTraceServiceRequest:
		request.ResourceSpans = []*tracespb.ResourceSpans{{Resource: &resourcev1.Resource{}}}
	}
	return message
}

func TestAuthenticatedOTLPGatewaysEnforceRealWorkspaceGrants(t *testing.T) {
	repository, workspaceA, workspaceB, identity, verifier := openGatewayIntegrationRepository(t)
	authorizer, err := NewAuthorizer(verifier, repository)
	if err != nil {
		t.Fatal(err)
	}

	httpForwarder := &forwarderStub{}
	httpHandler, err := NewHTTPGateway(authorizer, httpForwarder, HTTPGatewayConfig{
		RequestsPerMinute: 100,
		ForwardTimeout: 2 * time.Second,
		RateLimitStore: repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	grpcForwarder := &grpcForwarderStub{}
	grpcGateway := &GRPCGateway{
		authorizer: authorizer,
		forwarder: grpcForwarder,
		limiter: newWorkspaceRateLimiter(100),
		sharedLimiter: repository,
		timeout: 2 * time.Second,
		concurrency: make(chan struct{}, maxConcurrentGRPCExports),
	}

	for _, signal := range []string{"metrics", "logs", "traces"} {
		t.Run(signal, func(t *testing.T) {
			payload, err := proto.Marshal(integrationOTLPRequest(t, signal))
			if err != nil {
				t.Fatal(err)
			}
			sendHTTP := func(workspaceID string) *httptest.ResponseRecorder {
				request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+workspaceID+"/v1/"+signal, bytes.NewReader(payload))
				request.Header.Set("Authorization", "Bearer integration-workload-token")
				request.Header.Set("Content-Type", "application/x-protobuf")
				response := httptest.NewRecorder()
				httpHandler.ServeHTTP(response, request)
				return response
			}
			accepted := sendHTTP(workspaceA.ID)
			if accepted.Code != http.StatusOK {
				t.Fatalf("workspace A HTTP export status = %d, body=%s", accepted.Code, accepted.Body.String())
			}
			denied := sendHTTP(workspaceB.ID)
			if denied.Code != http.StatusForbidden {
				t.Fatalf("workspace B HTTP export status = %d, want 403; body=%s", denied.Code, denied.Body.String())
			}

			if err := callIntegrationGRPCExport(grpcGateway, workspaceA.ID, signal); err != nil {
				t.Fatalf("workspace A gRPC export: %v", err)
			}
			if err := callIntegrationGRPCExport(grpcGateway, workspaceB.ID, signal); status.Code(err) != codes.PermissionDenied {
				t.Fatalf("workspace B gRPC code = %s, want PermissionDenied; err=%v", status.Code(err), err)
			}
		})
	}

	actorIssuer := "https://identity.example/"
	actorSubject := "otlp-gateway-integration-" + strings.TrimPrefix(identity.Subject, "collector-")
	revokeCtx, revokeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer revokeCancel()
	if err := repository.SetWorkloadIdentityStatus(revokeCtx, identity.ID, actorIssuer, actorSubject, "revoked"); err != nil {
		t.Fatalf("revoke workload identity: %v", err)
	}
	payload, err := proto.Marshal(integrationOTLPRequest(t, "metrics"))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+workspaceA.ID+"/v1/metrics", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer integration-workload-token")
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	httpHandler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("revoked workload HTTP status = %d, want 403", response.Code)
	}
	if err := callIntegrationGRPCExport(grpcGateway, workspaceA.ID, "metrics"); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("revoked workload gRPC code = %s, want PermissionDenied; err=%v", status.Code(err), err)
	}
}


// addForgedWorkspaceAttribute simulates an untrusted producer attempting to
// choose its own tenant. The gateway must reject the payload before forwarding.
func addForgedWorkspaceAttribute(t *testing.T, message proto.Message, forgedWorkspaceID string) {
	t.Helper()
	attribute := &commonv1.KeyValue{
		Key: WorkspaceAttribute,
		Value: &commonv1.AnyValue{
			Value: &commonv1.AnyValue_StringValue{StringValue: forgedWorkspaceID},
		},
	}
	switch request := message.(type) {
	case *metricsv1.ExportMetricsServiceRequest:
		request.ResourceMetrics[0].Resource.Attributes = append(request.ResourceMetrics[0].Resource.Attributes, attribute)
	case *logsv1.ExportLogsServiceRequest:
		request.ResourceLogs[0].Resource.Attributes = append(request.ResourceLogs[0].Resource.Attributes, attribute)
	case *tracesv1.ExportTraceServiceRequest:
		request.ResourceSpans[0].Resource.Attributes = append(request.ResourceSpans[0].Resource.Attributes, attribute)
	default:
		t.Fatalf("unsupported OTLP request type %T", message)
	}
}

func callIntegrationGRPCMessage(gateway *GRPCGateway, workspaceID, signal string, message proto.Message) error {
	ctx := grpcWorkloadContext([]string{workspaceID}, []string{"Bearer integration-workload-token"})
	switch request := message.(type) {
	case *metricsv1.ExportMetricsServiceRequest:
		_, err := (&metricsGRPCService{gateway: gateway}).Export(ctx, request)
		return err
	case *logsv1.ExportLogsServiceRequest:
		_, err := (&logsGRPCService{gateway: gateway}).Export(ctx, request)
		return err
	case *tracesv1.ExportTraceServiceRequest:
		_, err := (&tracesGRPCService{gateway: gateway}).Export(ctx, request)
		return err
	default:
		return fmt.Errorf("unsupported OTLP signal %q (%T)", signal, message)
	}
}

func TestAuthenticatedOTLPGatewaysRejectProducerWorkspaceSpoofingForEverySignal(t *testing.T) {
	repository, workspaceA, workspaceB, _, verifier := openGatewayIntegrationRepository(t)
	authorizer, err := NewAuthorizer(verifier, repository)
	if err != nil {
		t.Fatal(err)
	}

	httpForwarder := &forwarderStub{}
	httpHandler, err := NewHTTPGateway(authorizer, httpForwarder, HTTPGatewayConfig{
		RequestsPerMinute: 100,
		ForwardTimeout: 2 * time.Second,
		RateLimitStore: repository,
	})
	if err != nil {
		t.Fatal(err)
	}

	grpcForwarder := &grpcForwarderStub{}
	grpcGateway := &GRPCGateway{
		authorizer: authorizer,
		forwarder: grpcForwarder,
		limiter: newWorkspaceRateLimiter(100),
		sharedLimiter: repository,
		timeout: 2 * time.Second,
		concurrency: make(chan struct{}, maxConcurrentGRPCExports),
	}

	for _, signal := range []string{"metrics", "logs", "traces"} {
		t.Run(signal, func(t *testing.T) {
			message := integrationOTLPRequest(t, signal)
			addForgedWorkspaceAttribute(t, message, workspaceB.ID)
			payload, err := proto.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}

			request := httptest.NewRequest(
				http.MethodPost,
				"/v1/workspaces/"+workspaceA.ID+"/v1/"+signal,
				bytes.NewReader(payload),
			)
			request.Header.Set("Authorization", "Bearer integration-workload-token")
			request.Header.Set("Content-Type", "application/x-protobuf")
			response := httptest.NewRecorder()
			httpHandler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("HTTP spoofed export status = %d, want 400; body=%s", response.Code, response.Body.String())
			}
			if httpForwarder.calls != 0 {
				t.Fatalf("spoofed %s payload reached HTTP Collector forwarder", signal)
			}

			grpcErr := callIntegrationGRPCMessage(grpcGateway, workspaceA.ID, signal, message)
			if status.Code(grpcErr) != codes.InvalidArgument {
				t.Fatalf("gRPC spoofed export code = %s, want InvalidArgument; err=%v", status.Code(grpcErr), grpcErr)
			}
			if grpcForwarder.metricsCalls != 0 || grpcForwarder.logsCalls != 0 || grpcForwarder.tracesCalls != 0 {
				t.Fatalf("spoofed %s payload reached gRPC Collector forwarder: metrics=%d logs=%d traces=%d",
					signal, grpcForwarder.metricsCalls, grpcForwarder.logsCalls, grpcForwarder.tracesCalls)
			}
		})
	}
}


// addMixedResourceWithForgedWorkspaceAttribute builds a multi-resource OTLP
// batch whose first resource is ordinary but whose later resource attempts to
// claim a different workspace. The gateway must reject the entire batch.
func addMixedResourceWithForgedWorkspaceAttribute(t *testing.T, message proto.Message, forgedWorkspaceID string) {
	t.Helper()
	attribute := &commonv1.KeyValue{
		Key: WorkspaceAttribute,
		Value: &commonv1.AnyValue{
			Value: &commonv1.AnyValue_StringValue{StringValue: forgedWorkspaceID},
		},
	}
	switch request := message.(type) {
	case *metricsv1.ExportMetricsServiceRequest:
		second := proto.Clone(request.ResourceMetrics[0]).(*metricspb.ResourceMetrics)
		second.Resource = &resourcev1.Resource{Attributes: []*commonv1.KeyValue{attribute}}
		request.ResourceMetrics = append(request.ResourceMetrics, second)
	case *logsv1.ExportLogsServiceRequest:
		second := proto.Clone(request.ResourceLogs[0]).(*logspb.ResourceLogs)
		second.Resource = &resourcev1.Resource{Attributes: []*commonv1.KeyValue{attribute}}
		request.ResourceLogs = append(request.ResourceLogs, second)
	case *tracesv1.ExportTraceServiceRequest:
		second := proto.Clone(request.ResourceSpans[0]).(*tracespb.ResourceSpans)
		second.Resource = &resourcev1.Resource{Attributes: []*commonv1.KeyValue{attribute}}
		request.ResourceSpans = append(request.ResourceSpans, second)
	default:
		t.Fatalf("unsupported OTLP request type %T", message)
	}
}

func TestAuthenticatedOTLPGatewaysRejectMixedWorkspaceResourceBatches(t *testing.T) {
	repository, workspaceA, workspaceB, _, verifier := openGatewayIntegrationRepository(t)
	authorizer, err := NewAuthorizer(verifier, repository)
	if err != nil {
		t.Fatal(err)
	}

	httpForwarder := &forwarderStub{}
	httpHandler, err := NewHTTPGateway(authorizer, httpForwarder, HTTPGatewayConfig{
		RequestsPerMinute: 100,
		ForwardTimeout: 2 * time.Second,
		RateLimitStore: repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	grpcForwarder := &grpcForwarderStub{}
	grpcGateway := &GRPCGateway{
		authorizer: authorizer,
		forwarder: grpcForwarder,
		limiter: newWorkspaceRateLimiter(100),
		sharedLimiter: repository,
		timeout: 2 * time.Second,
		concurrency: make(chan struct{}, maxConcurrentGRPCExports),
	}

	for _, signal := range []string{"metrics", "logs", "traces"} {
		t.Run(signal, func(t *testing.T) {
			message := integrationOTLPRequest(t, signal)
			addMixedResourceWithForgedWorkspaceAttribute(t, message, workspaceB.ID)
			payload, err := proto.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}

			request := httptest.NewRequest(
				http.MethodPost,
				"/v1/workspaces/"+workspaceA.ID+"/v1/"+signal,
				bytes.NewReader(payload),
			)
			request.Header.Set("Authorization", "Bearer integration-workload-token")
			request.Header.Set("Content-Type", "application/x-protobuf")
			response := httptest.NewRecorder()
			httpHandler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("HTTP mixed-resource export status = %d, want 400; body=%s", response.Code, response.Body.String())
			}
			if httpForwarder.calls != 0 {
				t.Fatalf("mixed-resource %s payload reached HTTP Collector forwarder", signal)
			}

			grpcErr := callIntegrationGRPCMessage(grpcGateway, workspaceA.ID, signal, message)
			if status.Code(grpcErr) != codes.InvalidArgument {
				t.Fatalf("gRPC mixed-resource export code = %s, want InvalidArgument; err=%v", status.Code(grpcErr), grpcErr)
			}
			if grpcForwarder.metricsCalls != 0 || grpcForwarder.logsCalls != 0 || grpcForwarder.tracesCalls != 0 {
				t.Fatalf("mixed-resource %s payload reached gRPC Collector forwarder: metrics=%d logs=%d traces=%d",
					signal, grpcForwarder.metricsCalls, grpcForwarder.logsCalls, grpcForwarder.tracesCalls)
			}
		})
	}
}


func addOversizedResourceAttribute(t *testing.T, message proto.Message) {
	t.Helper()
	attribute := &commonv1.KeyValue{
		Key: "integration.oversized.payload",
		Value: &commonv1.AnyValue{
			Value: &commonv1.AnyValue_StringValue{StringValue: strings.Repeat("x", maxOTLPRequestBytes+1)},
		},
	}
	switch request := message.(type) {
	case *metricsv1.ExportMetricsServiceRequest:
		request.ResourceMetrics[0].Resource.Attributes = append(request.ResourceMetrics[0].Resource.Attributes, attribute)
	case *logsv1.ExportLogsServiceRequest:
		request.ResourceLogs[0].Resource.Attributes = append(request.ResourceLogs[0].Resource.Attributes, attribute)
	case *tracesv1.ExportTraceServiceRequest:
		request.ResourceSpans[0].Resource.Attributes = append(request.ResourceSpans[0].Resource.Attributes, attribute)
	default:
		t.Fatalf("unsupported OTLP request type %T", message)
	}
}

func TestAuthenticatedOTLPGatewaysRejectOversizedRequestsForEverySignal(t *testing.T) {
	repository, workspaceA, _, _, verifier := openGatewayIntegrationRepository(t)
	authorizer, err := NewAuthorizer(verifier, repository)
	if err != nil {
		t.Fatal(err)
	}

	httpForwarder := &forwarderStub{}
	httpHandler, err := NewHTTPGateway(authorizer, httpForwarder, HTTPGatewayConfig{
		RequestsPerMinute: 100,
		ForwardTimeout: 2 * time.Second,
		RateLimitStore: repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	grpcForwarder := &grpcForwarderStub{}
	grpcGateway := &GRPCGateway{
		authorizer: authorizer,
		forwarder: grpcForwarder,
		limiter: newWorkspaceRateLimiter(100),
		sharedLimiter: repository,
		timeout: 2 * time.Second,
		concurrency: make(chan struct{}, maxConcurrentGRPCExports),
	}

	for _, signal := range []string{"metrics", "logs", "traces"} {
		t.Run(signal, func(t *testing.T) {
			message := integrationOTLPRequest(t, signal)
			addOversizedResourceAttribute(t, message)
			payload, err := proto.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}
			if len(payload) <= maxOTLPRequestBytes || proto.Size(message) <= maxOTLPRequestBytes {
				t.Fatalf("test payload is not oversized: HTTP bytes=%d gRPC proto size=%d limit=%d", len(payload), proto.Size(message), maxOTLPRequestBytes)
			}

			request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+workspaceA.ID+"/v1/"+signal, bytes.NewReader(payload))
			request.Header.Set("Authorization", "Bearer integration-workload-token")
			request.Header.Set("Content-Type", "application/x-protobuf")
			response := httptest.NewRecorder()
			httpHandler.ServeHTTP(response, request)
			if response.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("HTTP oversized export status = %d, want 413; body=%s", response.Code, response.Body.String())
			}
			if httpForwarder.calls != 0 {
				t.Fatalf("oversized %s payload reached HTTP Collector forwarder", signal)
			}

			grpcErr := callIntegrationGRPCMessage(grpcGateway, workspaceA.ID, signal, message)
			if status.Code(grpcErr) != codes.ResourceExhausted {
				t.Fatalf("gRPC oversized export code = %s, want ResourceExhausted; err=%v", status.Code(grpcErr), grpcErr)
			}
			if grpcForwarder.metricsCalls != 0 || grpcForwarder.logsCalls != 0 || grpcForwarder.tracesCalls != 0 {
				t.Fatalf("oversized %s payload reached gRPC Collector forwarder: metrics=%d logs=%d traces=%d",
					grpcForwarder.metricsCalls, grpcForwarder.logsCalls, grpcForwarder.tracesCalls)
			}
		})
	}
}

package ingest

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/httptest"
	"testing"
	"time"

	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func startRecoveryHTTPCollector(t *testing.T, listener net.Listener) *http.Server {
	t.Helper()
	responseBody, err := proto.Marshal(&metricsv1.ExportMetricsServiceResponse{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(responseBody)
	})
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	return server
}

func sendRecoveryHTTPExport(t *testing.T, handler http.Handler) int {
	t.Helper()
	payload, err := proto.Marshal(metricRequest())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+testWorkspace+"/v1/metrics", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer signed-token")
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response.Code
}

func TestHTTPGatewayRecoversAfterRealCollectorListenerRestart(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	server := startRecoveryHTTPCollector(t, listener)
	t.Cleanup(func() { _ = server.Close() })

	forwarder, err := NewCollectorHTTPForwarder("http://"+address, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHTTPGateway(burstAuthorizer(t), forwarder, HTTPGatewayConfig{
		RequestsPerMinute: 100,
		ForwardTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := sendRecoveryHTTPExport(t, handler); got != http.StatusOK {
		t.Fatalf("initial Collector response = %d, want 200", got)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("stop Collector listener: %v", err)
	}
	if got := sendRecoveryHTTPExport(t, handler); got != http.StatusBadGateway {
		t.Fatalf("request while Collector is offline = %d, want 502", got)
	}

	restartedListener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("restart Collector on the same address: %v", err)
	}
	restartedServer := startRecoveryHTTPCollector(t, restartedListener)
	t.Cleanup(func() { _ = restartedServer.Close() })

	if got := sendRecoveryHTTPExport(t, handler); got != http.StatusOK {
		t.Fatalf("request after Collector restart = %d, want 200", got)
	}
}

type recoveryMetricsCollector struct {
	metricsv1.UnimplementedMetricsServiceServer
}

func (recoveryMetricsCollector) Export(context.Context, *metricsv1.ExportMetricsServiceRequest) (*metricsv1.ExportMetricsServiceResponse, error) {
	return &metricsv1.ExportMetricsServiceResponse{}, nil
}

func startRecoveryGRPCCollector(t *testing.T, listener net.Listener) *grpc.Server {
	t.Helper()
	server := grpc.NewServer()
	metricsv1.RegisterMetricsServiceServer(server, recoveryMetricsCollector{})
	go func() { _ = server.Serve(listener) }()
	return server
}

func callRecoveryGRPCExport(service *metricsGRPCService) error {
	ctx, cancel := context.WithTimeout(
		grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer signed-token"}),
		2*time.Second,
	)
	defer cancel()
	request := &metricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcev1.Resource{}}},
	}
	_, err := service.Export(ctx, request)
	return err
}

func TestGRPCGatewayRecoversAfterRealCollectorListenerRestart(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	server := startRecoveryGRPCCollector(t, listener)
	t.Cleanup(server.Stop)

	forwarder, err := NewCollectorGRPCForwarder(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = forwarder.Close() })
	gateway := &GRPCGateway{
		authorizer: burstAuthorizer(t),
		forwarder: forwarder,
		limiter: newWorkspaceRateLimiter(100),
		timeout: 2 * time.Second,
		concurrency: make(chan struct{}, maxConcurrentGRPCExports),
	}
	service := &metricsGRPCService{gateway: gateway}

	if err := callRecoveryGRPCExport(service); err != nil {
		t.Fatalf("initial Collector export failed: %v", err)
	}
	server.Stop()
	if err := callRecoveryGRPCExport(service); err == nil {
		t.Fatal("export unexpectedly succeeded while Collector was offline")
	} else if code := status.Code(err); code != codes.Unavailable && code != codes.DeadlineExceeded {
		t.Fatalf("offline Collector status = %s, want Unavailable or DeadlineExceeded; err=%v", code, err)
	}

	restartedListener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("restart gRPC Collector on the same address: %v", err)
	}
	restartedServer := startRecoveryGRPCCollector(t, restartedListener)
	t.Cleanup(restartedServer.Stop)

	deadline := time.Now().Add(10 * time.Second)
	var recovered bool
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = callRecoveryGRPCExport(service)
		if lastErr == nil {
			recovered = true
			break
		}
		code := status.Code(lastErr)
		if code != codes.Unavailable && code != codes.DeadlineExceeded {
			t.Fatalf("unexpected status while gRPC reconnects: %s; err=%v", code, lastErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !recovered {
		t.Fatalf("same gRPC forwarder did not recover after Collector restart: %v", lastErr)
	}
}

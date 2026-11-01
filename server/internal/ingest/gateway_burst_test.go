package ingest

import (
	"bytes"\n\t"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/auth"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracesv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type burstVerifier struct{}

func (burstVerifier) Verify(context.Context, string) (auth.Principal, error) {
	return auth.Principal{Issuer: testIssuer, Subject: testSubject}, nil
}

type burstGrantRepository struct{}

func (burstGrantRepository) AuthorizeWorkloadTelemetry(_ context.Context, issuer, subject, workspace, _ string) (store.WorkloadIdentity, error) {
	if issuer != testIssuer || subject != testSubject || workspace != testWorkspace {
		return store.WorkloadIdentity{}, store.ErrWorkloadAuthorizationDenied
	}
	return store.WorkloadIdentity{
		ID: testIdentity, Issuer: testIssuer, Subject: testSubject, Status: "active",
	}, nil
}

func burstAuthorizer(t *testing.T) *Authorizer {
	t.Helper()
	authorizer, err := NewAuthorizer(burstVerifier{}, burstGrantRepository{})
	if err != nil {
		t.Fatal(err)
	}
	return authorizer
}

func recordBurstPeak(active, peak *atomic.Int64) {
	current := active.Add(1)
	for {
		previous := peak.Load()
		if current <= previous || peak.CompareAndSwap(previous, current) {
			return
		}
	}
}

func waitForBurstEntries(t *testing.T, entered <-chan struct{}, expected int) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for i := 0; i < expected; i++ {
		select {
		case <-entered:
		case <-timer.C:
			t.Fatalf("only %d of %d requests reached the blocking forwarder", i, expected)
		}
	}
}

type burstHTTPForwarder struct {
	active  atomic.Int64
	peak    atomic.Int64
	entered chan struct{}
	release chan struct{}
}

func (f *burstHTTPForwarder) Export(ctx context.Context, signal string, _ []byte) ([]byte, error) {
	if signal != "metrics" {
		return nil, errors.New("unexpected signal")
	}
	recordBurstPeak(&f.active, &f.peak)
	defer f.active.Add(-1)
	f.entered <- struct{}{}
	select {
	case <-f.release:
		return proto.Marshal(&metricsv1.ExportMetricsServiceResponse{})
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestHTTPGatewayBoundsDefaultConcurrencyDuringBurst(t *testing.T) {
	const extraRequests = 16
	forwarder := &burstHTTPForwarder{
		entered: make(chan struct{}, maxConcurrentExports),
		release: make(chan struct{}, maxConcurrentExports),
	}
	handler, err := NewHTTPGateway(burstAuthorizer(t), forwarder, HTTPGatewayConfig{
		RequestsPerMinute: 10000,
		ForwardTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := proto.Marshal(metricRequest())
	if err != nil {
		t.Fatal(err)
	}
	send := func() int {
		request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+testWorkspace+"/v1/metrics", bytes.NewReader(payload))
		request.Header.Set("Authorization", "Bearer burst-token")
		request.Header.Set("Content-Type", "application/x-protobuf")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Code
	}

	start := make(chan struct{})
	statuses := make(chan int, maxConcurrentExports)
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrentExports; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			statuses <- send()
		}()
	}
	close(start)
	waitForBurstEntries(t, forwarder.entered, maxConcurrentExports)

	for i := 0; i < extraRequests; i++ {
		if got := send(); got != http.StatusServiceUnavailable {
			t.Fatalf("over-capacity request %d status = %d, want 503", i, got)
		}
	}
	if got := forwarder.active.Load(); got != maxConcurrentExports {
		t.Fatalf("active forwards = %d, want exactly configured cap %d", got, maxConcurrentExports)
	}

	for i := 0; i < maxConcurrentExports; i++ {
		forwarder.release <- struct{}{}
	}
	wg.Wait()
	close(statuses)
	for got := range statuses {
		if got != http.StatusOK {
			t.Fatalf("admitted HTTP request status = %d, want 200", got)
		}
	}
	if got := forwarder.active.Load(); got != 0 {
		t.Fatalf("active forwards after release = %d, want 0", got)
	}
	if got := forwarder.peak.Load(); got != maxConcurrentExports {
		t.Fatalf("peak active forwards = %d, want %d", got, maxConcurrentExports)
	}
}

type burstGRPCForwarder struct {
	active  atomic.Int64
	peak    atomic.Int64
	entered chan struct{}
	release chan struct{}
}

func (f *burstGRPCForwarder) wait(ctx context.Context) error {
	recordBurstPeak(&f.active, &f.peak)
	defer f.active.Add(-1)
	f.entered <- struct{}{}
	select {
	case <-f.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *burstGRPCForwarder) ExportMetrics(ctx context.Context, _ *metricsv1.ExportMetricsServiceRequest) (*metricsv1.ExportMetricsServiceResponse, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	return &metricsv1.ExportMetricsServiceResponse{}, nil
}

func (f *burstGRPCForwarder) ExportLogs(context.Context, *logsv1.ExportLogsServiceRequest) (*logsv1.ExportLogsServiceResponse, error) {
	return nil, errors.New("unexpected logs export")
}

func (f *burstGRPCForwarder) ExportTraces(context.Context, *tracesv1.ExportTraceServiceRequest) (*tracesv1.ExportTraceServiceResponse, error) {
	return nil, errors.New("unexpected traces export")
}

func TestGRPCGatewayBoundsDefaultConcurrencyDuringBurst(t *testing.T) {
	forwarder := &burstGRPCForwarder{
		entered: make(chan struct{}, maxConcurrentGRPCExports),
		release: make(chan struct{}, maxConcurrentGRPCExports),
	}
	gateway := &GRPCGateway{
		authorizer: burstAuthorizer(t),
		forwarder: forwarder,
		limiter: newWorkspaceRateLimiter(10000),
		timeout: 5 * time.Second,
		concurrency: make(chan struct{}, maxConcurrentGRPCExports),
	}
	service := &metricsGRPCService{gateway: gateway}
	request := &metricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcev1.Resource{}}},
	}
	start := make(chan struct{})
	results := make(chan error, maxConcurrentGRPCExports)
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrentGRPCExports; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ctx := grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer burst-token"})
			_, err := service.Export(ctx, request)
			results <- err
		}()
	}
	close(start)
	waitForBurstEntries(t, forwarder.entered, maxConcurrentGRPCExports)

	ctx := grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer burst-token"})
	if _, err := service.Export(ctx, request); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("over-capacity gRPC code = %s, want ResourceExhausted; err=%v", status.Code(err), err)
	}
	if got := forwarder.active.Load(); got != maxConcurrentGRPCExports {
		t.Fatalf("active forwards = %d, want exactly configured cap %d", got, maxConcurrentGRPCExports)
	}

	for i := 0; i < maxConcurrentGRPCExports; i++ {
		forwarder.release <- struct{}{}
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("admitted gRPC request failed: %v", err)
		}
	}
	if got := forwarder.active.Load(); got != 0 {
		t.Fatalf("active forwards after release = %d, want 0", got)
	}
	if got := forwarder.peak.Load(); got != maxConcurrentGRPCExports {
		t.Fatalf("peak active forwards = %d, want %d", got, maxConcurrentGRPCExports)
	}
}

package ingest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracesv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

// fixedWorkloadRateLimitStore deliberately removes rate-limit contention from
// these tests. They exercise gateway admission, authorization, sanitization,
// and forwarding paths; PostgreSQL rate-limit behavior has separate tests.
type fixedWorkloadRateLimitStore struct{}

func (fixedWorkloadRateLimitStore) IncrementRateLimitWindow(context.Context, string) (int64, error) {
	return 1, nil
}

func newLoadTestAuthorizer(tb testing.TB) *Authorizer {
	tb.Helper()
	authorizer, err := NewAuthorizer(burstVerifier{}, burstGrantRepository{})
	if err != nil {
		tb.Fatal(err)
	}
	return authorizer
}

func newLoadTestHTTPGateway(tb testing.TB, forwarder OTLPForwarder) http.Handler {
	tb.Helper()
	handler, err := NewHTTPGateway(newLoadTestAuthorizer(tb), forwarder, HTTPGatewayConfig{
		RequestsPerMinute: 10000,
		ForwardTimeout:    5 * time.Second,
		RateLimitStore:    fixedWorkloadRateLimitStore{},
	})
	if err != nil {
		tb.Fatal(err)
	}
	return handler
}

func newLoadTestGRPCGateway(tb testing.TB, forwarder GRPCForwarder) *GRPCGateway {
	tb.Helper()
	return &GRPCGateway{
		authorizer:    newLoadTestAuthorizer(tb),
		forwarder:     forwarder,
		limiter:       newWorkspaceRateLimiter(10000),
		sharedLimiter: fixedWorkloadRateLimitStore{},
		timeout:       5 * time.Second,
		concurrency:   make(chan struct{}, maxConcurrentGRPCExports),
	}
}

func recordLoadPeak(active, peak *atomic.Int64) {
	current := active.Add(1)
	for {
		previous := peak.Load()
		if current <= previous || peak.CompareAndSwap(previous, current) {
			return
		}
	}
}

func waitForLoadDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type measuredHTTPForwarder struct {
	response []byte
	delay    time.Duration
	active   atomic.Int64
	peak     atomic.Int64
	calls    atomic.Int64
}

func (f *measuredHTTPForwarder) Export(ctx context.Context, signal string, _ []byte) ([]byte, error) {
	if signal != "metrics" {
		return nil, fmt.Errorf("unexpected signal %q", signal)
	}
	recordLoadPeak(&f.active, &f.peak)
	defer f.active.Add(-1)
	if err := waitForLoadDelay(ctx, f.delay); err != nil {
		return nil, err
	}
	f.calls.Add(1)
	return f.response, nil
}

type measuredGRPCForwarder struct {
	delay  time.Duration
	active atomic.Int64
	peak   atomic.Int64
	calls  atomic.Int64
}

func (f *measuredGRPCForwarder) ExportMetrics(ctx context.Context, _ *metricsv1.ExportMetricsServiceRequest) (*metricsv1.ExportMetricsServiceResponse, error) {
	recordLoadPeak(&f.active, &f.peak)
	defer f.active.Add(-1)
	if err := waitForLoadDelay(ctx, f.delay); err != nil {
		return nil, err
	}
	f.calls.Add(1)
	return &metricsv1.ExportMetricsServiceResponse{}, nil
}

func (*measuredGRPCForwarder) ExportLogs(context.Context, *logsv1.ExportLogsServiceRequest) (*logsv1.ExportLogsServiceResponse, error) {
	return nil, errors.New("logs are not part of the metrics load baseline")
}

func (*measuredGRPCForwarder) ExportTraces(context.Context, *tracesv1.ExportTraceServiceRequest) (*tracesv1.ExportTraceServiceResponse, error) {
	return nil, errors.New("traces are not part of the metrics load baseline")
}

func newLoadTestMetricPayload(tb testing.TB) []byte {
	tb.Helper()
	payload, err := proto.Marshal(metricRequest())
	if err != nil {
		tb.Fatal(err)
	}
	return payload
}

func sendLoadTestHTTPExport(handler http.Handler, payload []byte) int {
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/workspaces/"+testWorkspace+"/v1/metrics",
		bytes.NewReader(payload),
	)
	request.Header.Set("Authorization", "Bearer burst-token")
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response.Code
}

func runConcurrentGatewayCalls(t *testing.T, workers, callsPerWorker int, call func() error) {
	t.Helper()
	start := make(chan struct{})
	failures := make(chan error, workers*callsPerWorker)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for callIndex := 0; callIndex < callsPerWorker; callIndex++ {
				if err := call(); err != nil {
					failures <- err
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Errorf("concurrent gateway export failed: %v", err)
	}
}

func TestHTTPGatewaySustainedConcurrentMetricsExports(t *testing.T) {
	const workers = 16
	const callsPerWorker = 32
	const totalCalls = workers * callsPerWorker

	response, err := proto.Marshal(&metricsv1.ExportMetricsServiceResponse{})
	if err != nil {
		t.Fatal(err)
	}
	forwarder := &measuredHTTPForwarder{response: response, delay: 250 * time.Microsecond}
	handler := newLoadTestHTTPGateway(t, forwarder)
	payload := newLoadTestMetricPayload(t)

	runConcurrentGatewayCalls(t, workers, callsPerWorker, func() error {
		if statusCode := sendLoadTestHTTPExport(handler, payload); statusCode != http.StatusOK {
			return fmt.Errorf("HTTP status = %d, want 200", statusCode)
		}
		return nil
	})

	if got := forwarder.calls.Load(); got != totalCalls {
		t.Fatalf("successful Collector forwards = %d, want %d", got, totalCalls)
	}
	if got := forwarder.active.Load(); got != 0 {
		t.Fatalf("active Collector forwards after load = %d, want 0", got)
	}
	if got := forwarder.peak.Load(); got < 2 || got > maxConcurrentExports {
		t.Fatalf("peak concurrent HTTP forwards = %d, want 2..%d", got, maxConcurrentExports)
	}
}

func TestGRPCGatewaySustainedConcurrentMetricsExports(t *testing.T) {
	const workers = 16
	const callsPerWorker = 32
	const totalCalls = workers * callsPerWorker

	forwarder := &measuredGRPCForwarder{delay: 250 * time.Microsecond}
	gateway := newLoadTestGRPCGateway(t, forwarder)
	service := &metricsGRPCService{gateway: gateway}
	request := metricRequest()

	runConcurrentGatewayCalls(t, workers, callsPerWorker, func() error {
		ctx := grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer burst-token"})
		_, err := service.Export(ctx, request)
		return err
	})

	if got := forwarder.calls.Load(); got != totalCalls {
		t.Fatalf("successful Collector forwards = %d, want %d", got, totalCalls)
	}
	if got := forwarder.active.Load(); got != 0 {
		t.Fatalf("active Collector forwards after load = %d, want 0", got)
	}
	if got := forwarder.peak.Load(); got < 2 || got > maxConcurrentGRPCExports {
		t.Fatalf("peak concurrent gRPC forwards = %d, want 2..%d", got, maxConcurrentGRPCExports)
	}
}

func BenchmarkHTTPGatewayMetricsHandlerPath(b *testing.B) {
	payload := newLoadTestMetricPayload(b)
	response, err := proto.Marshal(&metricsv1.ExportMetricsServiceResponse{})
	if err != nil {
		b.Fatal(err)
	}
	handler := newLoadTestHTTPGateway(b, &measuredHTTPForwarder{response: response})
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			if statusCode := sendLoadTestHTTPExport(handler, payload); statusCode != http.StatusOK {
				b.Errorf("HTTP status = %d, want 200", statusCode)
				return
			}
		}
	})
}

func BenchmarkGRPCGatewayMetricsHandlerPath(b *testing.B) {
	forwarder := &measuredGRPCForwarder{}
	gateway := newLoadTestGRPCGateway(b, forwarder)
	service := &metricsGRPCService{gateway: gateway}
	request := metricRequest()
	b.ReportAllocs()
	b.SetBytes(int64(proto.Size(request)))
	b.ResetTimer()
	b.RunParallel(func(parallel *testing.PB) {
		for parallel.Next() {
			ctx := grpcWorkloadContext([]string{testWorkspace}, []string{"Bearer burst-token"})
			if _, err := service.Export(ctx, request); err != nil {
				b.Errorf("gRPC metrics export failed: %v", err)
				return
			}
		}
	})
}

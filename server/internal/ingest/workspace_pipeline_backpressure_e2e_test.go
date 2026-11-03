package ingest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

// workspacePipelineCollectorGate delays requests at the network boundary
// before forwarding them to the real Collector. It makes the gateway's
// admission limit deterministic without replacing PostgreSQL authorization,
// the shared PostgreSQL rate limiter, or the Collector itself.
type workspacePipelineCollectorGate struct {
	target    string
	client    *http.Client
	hold      atomic.Bool
	entered   chan struct{}
	release   chan struct{}
	releaseMu sync.Once
	active    atomic.Int64
	peak      atomic.Int64
	forwarded atomic.Int64
}

func (g *workspacePipelineCollectorGate) releaseHeldRequests() {
	g.hold.Store(false)
	g.releaseMu.Do(func() { close(g.release) })
}

func (g *workspacePipelineCollectorGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.hold.Load() {
		current := g.active.Add(1)
		for {
			previous := g.peak.Load()
			if current <= previous || g.peak.CompareAndSwap(previous, current) {
				break
			}
		}
		select {
		case g.entered <- struct{}{}:
		case <-r.Context().Done():
			g.active.Add(-1)
			return
		}
		select {
		case <-g.release:
		case <-r.Context().Done():
			g.active.Add(-1)
			http.Error(w, "test Collector gate request canceled", http.StatusRequestTimeout)
			return
		}
		g.active.Add(-1)
	}

	upstreamURL := strings.TrimRight(g.target, "/") + r.URL.RequestURI()
	upstreamRequest, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, r.Body)
	if err != nil {
		http.Error(w, "create upstream Collector request", http.StatusBadGateway)
		return
	}
	for _, header := range []string{"Accept", "Content-Type", "Content-Encoding"} {
		if value := r.Header.Get(header); value != "" {
			upstreamRequest.Header.Set(header, value)
		}
	}

	upstreamResponse, err := g.client.Do(upstreamRequest)
	if err != nil {
		http.Error(w, "real Collector request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer upstreamResponse.Body.Close()
	for _, header := range []string{"Content-Type", "Content-Encoding"} {
		if value := upstreamResponse.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.WriteHeader(upstreamResponse.StatusCode)
	if _, err := io.Copy(w, upstreamResponse.Body); err != nil {
		return
	}
	g.forwarded.Add(1)
}

// TestWorkspaceOTLPSustainedBackpressureAndRecovery verifies bounded
// admission and recovery across the real PostgreSQL authorization/rate-limit
// path and the real OTLP Collector. The local proxy only holds accepted
// requests briefly so the 64-slot gateway bound can be exercised
// deterministically; it forwards every admitted request to the real Collector.
func TestWorkspaceOTLPSustainedBackpressureAndRecovery(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("ORBIT_TEST_DATABASE_URL"))
	collectorURL := strings.TrimSpace(os.Getenv("ORBIT_TEST_COLLECTOR_OTLP_HTTP_URL"))
	if databaseURL == "" || collectorURL == "" {
		t.Skip("backpressure integration test requires PostgreSQL and a real OTLP Collector URL")
	}

	repository, workspaceA, _, _, workloadVerifier := openGatewayIntegrationRepository(t)
	authorizer, err := NewAuthorizer(workloadVerifier, repository)
	if err != nil {
		t.Fatalf("create workload authorizer: %v", err)
	}

	gate := &workspacePipelineCollectorGate{
		target:  collectorURL,
		client:  &http.Client{Timeout: 20 * time.Second},
		entered: make(chan struct{}, maxConcurrentExports),
		release: make(chan struct{}),
	}
	gate.hold.Store(true)
	proxy := httptest.NewServer(gate)
	t.Cleanup(proxy.Close)
	t.Cleanup(gate.releaseHeldRequests)

	forwarder, err := NewCollectorHTTPForwarder(proxy.URL, 25*time.Second)
	if err != nil {
		t.Fatalf("create gated real-Collector forwarder: %v", err)
	}
	handler, err := NewHTTPGateway(authorizer, forwarder, HTTPGatewayConfig{
		RequestsPerMinute: 10000,
		ForwardTimeout:    25 * time.Second,
		RateLimitStore:    repository,
	})
	if err != nil {
		t.Fatalf("create authenticated OTLP gateway: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	payloads := make([][]byte, maxConcurrentExports+3*16)
	for index := range payloads {
		request := workspacePipelineMetricsRequest(int64(index + 1))
		request.ResourceMetrics[0].Resource.Attributes[0] = workspacePipelineStringAttribute(
			"service.name",
			fmt.Sprintf("orbit-backpressure-%03d", index),
		)
		payload, marshalErr := proto.Marshal(request)
		if marshalErr != nil {
			t.Fatalf("marshal metrics payload %d: %v", index, marshalErr)
		}
		payloads[index] = payload
	}

	send := func(requestContext context.Context, payload []byte) int {
		request := httptest.NewRequest(
			http.MethodPost,
			"/v1/workspaces/"+workspaceA.ID+"/v1/metrics",
			bytes.NewReader(payload),
		).WithContext(requestContext)
		request.Header.Set("Authorization", "Bearer "+workspacePipelineToken)
		request.Header.Set("Content-Type", "application/x-protobuf")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Code
	}

	const admitted = maxConcurrentExports
	const overflow = 8
	start := make(chan struct{})
	results := make(chan int, admitted)
	var workers sync.WaitGroup
	for index := 0; index < admitted; index++ {
		workers.Add(1)
		go func(payload []byte) {
			defer workers.Done()
			<-start
			results <- send(ctx, payload)
		}(payloads[index])
	}
	defer func() {
		gate.releaseHeldRequests()
		workers.Wait()
	}()
	close(start)

	entryDeadline := time.NewTimer(20 * time.Second)
	defer entryDeadline.Stop()
	for index := 0; index < admitted; index++ {
		select {
		case <-gate.entered:
		case <-entryDeadline.C:
			t.Fatalf("only %d/%d admitted requests reached the real-Collector gate", index, admitted)
		case <-ctx.Done():
			t.Fatalf("timed out filling the gateway admission window: %v", ctx.Err())
		}
	}

	for index := 0; index < overflow; index++ {
		if got := send(ctx, payloads[admitted+index]); got != http.StatusServiceUnavailable {
			t.Fatalf("over-capacity request %d returned HTTP %d, want 503", index, got)
		}
	}
	if got := gate.active.Load(); got != admitted {
		t.Fatalf("requests held at the Collector boundary = %d, want %d", got, admitted)
	}

	gate.releaseHeldRequests()
	workers.Wait()
	close(results)
	for got := range results {
		if got != http.StatusOK {
			t.Errorf("admitted request returned HTTP %d, want 200", got)
		}
	}
	if got := gate.active.Load(); got != 0 {
		t.Fatalf("held Collector requests after release = %d, want 0", got)
	}
	if got := gate.peak.Load(); got != admitted {
		t.Fatalf("peak requests at the Collector boundary = %d, want %d", got, admitted)
	}
	if got := gate.forwarded.Load(); got != admitted {
		t.Fatalf("requests forwarded to the real Collector = %d, want %d", got, admitted)
	}

	// Keep sending concurrent exports after the forced saturation has cleared.
	// Every request must again traverse PostgreSQL auth/rate limiting and reach
	// the real Collector successfully.
	for round := 0; round < 3; round++ {
		const recoveryWorkers = 16
		startRecovery := make(chan struct{})
		recoveryResults := make(chan int, recoveryWorkers)
		var recovery sync.WaitGroup
		for index := 0; index < recoveryWorkers; index++ {
			payload := payloads[admitted+round*recoveryWorkers+index]
			recovery.Add(1)
			go func(payload []byte) {
				defer recovery.Done()
				<-startRecovery
				recoveryResults <- send(ctx, payload)
			}(payload)
		}
		close(startRecovery)
		recovery.Wait()
		close(recoveryResults)
		for got := range recoveryResults {
			if got != http.StatusOK {
				t.Errorf("recovery round %d returned HTTP %d, want 200", round+1, got)
			}
		}
	}

	// Follow the burst with a 20-second steady stream. Eight independent
	// producer streams each send at most four exports per second, keeping the
	// shared 10,000/minute database-backed rate limit well above the test rate.
	const sustainedWorkers = 8
	sustainedStop := make(chan struct{})
	stopSustained := time.AfterFunc(20*time.Second, func() { close(sustainedStop) })
	defer stopSustained.Stop()
	var sustainedSuccesses atomic.Int64
	sustainedFailures := make(chan error, sustainedWorkers)
	var sustained sync.WaitGroup
	for worker := 0; worker < sustainedWorkers; worker++ {
		worker := worker
		sustained.Add(1)
		go func() {
			defer sustained.Done()
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			var sample int64
			for {
				select {
				case <-sustainedStop:
					return
				case <-ctx.Done():
					sustainedFailures <- fmt.Errorf("sustained load context ended: %w", ctx.Err())
					return
				case <-ticker.C:
					sample++
					request := workspacePipelineMetricsRequest(sample)
					request.ResourceMetrics[0].Resource.Attributes[0] = workspacePipelineStringAttribute(
						"service.name",
						fmt.Sprintf("orbit-backpressure-steady-%02d", worker),
					)
					payload, marshalErr := proto.Marshal(request)
					if marshalErr != nil {
						sustainedFailures <- fmt.Errorf("marshal sustained metrics sample: %w", marshalErr)
						return
					}
					if statusCode := send(ctx, payload); statusCode != http.StatusOK {
						sustainedFailures <- fmt.Errorf("sustained worker %d returned HTTP %d", worker, statusCode)
						return
					}
					sustainedSuccesses.Add(1)
				}
			}
		}()
	}
	sustained.Wait()
	close(sustainedFailures)
	for err := range sustainedFailures {
		t.Error(err)
	}
	if got := sustainedSuccesses.Load(); got < 100 {
		t.Fatalf("sustained real-dependency exports = %d, want at least 100 over 20 seconds", got)
	}
	t.Logf("sustained PostgreSQL + Collector exports: %d successful over 20 seconds", sustainedSuccesses.Load())

	expectedForwarded := int64(admitted + 3*16) + sustainedSuccesses.Load()
	if got := gate.forwarded.Load(); got != expectedForwarded {
		t.Fatalf("total requests forwarded to real Collector = %d, want %d", got, expectedForwarded)
	}
}

package ingest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracesv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	maxOTLPRequestBytes = 4 << 20
	defaultForwardTimeout = 10 * time.Second
	maxConcurrentExports = 64
	maxRateLimitEntries = 10000
)

var (
	errUnsupportedEncoding = errors.New("unsupported OTLP content encoding")
	errInvalidMediaType = errors.New("unsupported OTLP content type")
)

type OTLPForwarder interface {
	Export(context.Context, string, []byte) ([]byte, error)
}

// WorkloadRateLimitStore is implemented by the shared PostgreSQL rate-limit
// repository. The gateway hashes keys before they leave process memory.
type WorkloadRateLimitStore interface {
	IncrementRateLimitWindow(context.Context, string) (int64, error)
}

type HTTPGatewayConfig struct {
	RequestsPerMinute int
	ForwardTimeout time.Duration
	RateLimitStore WorkloadRateLimitStore
}

type HTTPGateway struct {
	authorizer *Authorizer
	forwarder OTLPForwarder
	limiter *workspaceRateLimiter
	sharedLimiter WorkloadRateLimitStore
	timeout time.Duration
	concurrency chan struct{}
}

func NewHTTPGateway(authorizer *Authorizer, forwarder OTLPForwarder, cfg HTTPGatewayConfig) (http.Handler, error) {
	if authorizer == nil || forwarder == nil {
		return nil, errors.New("workload authorizer and OTLP forwarder are required")
	}
	if cfg.RequestsPerMinute < 1 || cfg.RequestsPerMinute > 10000 {
		return nil, errors.New("OTLP requests-per-minute limit must be between 1 and 10000")
	}
	if cfg.ForwardTimeout <= 0 {
		cfg.ForwardTimeout = defaultForwardTimeout
	}
	if cfg.ForwardTimeout > 30*time.Second {
		return nil, errors.New("OTLP forward timeout must not exceed 30 seconds")
	}
	gateway := &HTTPGateway{
		authorizer: authorizer,
		forwarder: forwarder,
		limiter: newWorkspaceRateLimiter(cfg.RequestsPerMinute),
		sharedLimiter: cfg.RateLimitStore,
		timeout: cfg.ForwardTimeout,
		concurrency: make(chan struct{}, maxConcurrentExports),
	}
	return http.HandlerFunc(gateway.serveHTTP), nil
}

func (g *HTTPGateway) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspaceID, signal, ok := parseWorkspaceSignalPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Reserve bounded capacity before token verification or database access so
	// concurrent invalid/valid requests cannot fan out unbounded auth work.
	select {
	case g.concurrency <- struct{}{}:
		defer func() { <-g.concurrency }()
	case <-r.Context().Done():
		http.Error(w, "request canceled", http.StatusRequestTimeout)
		return
	default:
		http.Error(w, "OTLP gateway is at capacity", http.StatusServiceUnavailable)
		return
	}
	decision, err := g.authorizer.Authorize(r.Context(), r.Header.Values("Authorization"), workspaceID, signal)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidTarget):
			http.Error(w, "invalid workspace or telemetry target", http.StatusBadRequest)
		case errors.Is(err, ErrInvalidBearer):
			http.Error(w, "invalid workload credentials", http.StatusUnauthorized)
		case errors.Is(err, ErrDenied):
			http.Error(w, "workload is not authorized for this workspace and signal", http.StatusForbidden)
		default:
			http.Error(w, "workload authorization is temporarily unavailable", http.StatusServiceUnavailable)
		}
		return
	}

	rateKey := "workload:" + decision.IdentityID + ":" + decision.WorkspaceID + ":" + decision.Signal
	rateCtx, rateCancel := context.WithTimeout(r.Context(), 750*time.Millisecond)
	allowed, rateErr := allowWorkloadRate(rateCtx, g.sharedLimiter, g.limiter, g.limiter.limit, rateKey, time.Now())
	rateCancel()
	if rateErr != nil {
		http.Error(w, "workload rate-limit service is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "workload telemetry rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	contentTypes := r.Header.Values("Content-Type")
	if len(contentTypes) != 1 {
		http.Error(w, errInvalidMediaType.Error(), http.StatusUnsupportedMediaType)
		return
	}
	mediaType, _, err := mime.ParseMediaType(contentTypes[0])
	if err != nil || (mediaType != "application/x-protobuf" && mediaType != "application/json") {
		http.Error(w, errInvalidMediaType.Error(), http.StatusUnsupportedMediaType)
		return
	}
	contentEncodings := r.Header.Values("Content-Encoding")
	if len(contentEncodings) > 1 {
		http.Error(w, errUnsupportedEncoding.Error(), http.StatusUnsupportedMediaType)
		return
	}
	if len(contentEncodings) == 1 {
		encoding := strings.TrimSpace(strings.ToLower(contentEncodings[0]))
		if encoding != "" && encoding != "identity" {
			http.Error(w, errUnsupportedEncoding.Error(), http.StatusUnsupportedMediaType)
			return
		}
	}
	if r.ContentLength > maxOTLPRequestBytes {
		http.Error(w, "OTLP request exceeds the 4 MiB limit", http.StatusRequestEntityTooLarge)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxOTLPRequestBytes+1))
	if err != nil {
		http.Error(w, "could not read OTLP request", http.StatusBadRequest)
		return
	}
	if len(body) == 0 {
		http.Error(w, "OTLP request body is required", http.StatusBadRequest)
		return
	}
	if len(body) > maxOTLPRequestBytes {
		http.Error(w, "OTLP request exceeds the 4 MiB limit", http.StatusRequestEntityTooLarge)
		return
	}

	message, response, err := newSignalMessages(signal)
	if err != nil {
		http.Error(w, "unsupported OTLP signal", http.StatusNotFound)
		return
	}
	if mediaType == "application/x-protobuf" {
		err = proto.Unmarshal(body, message)
	} else {
		err = (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(body, message)
	}
	if err != nil {
		http.Error(w, "malformed OTLP payload", http.StatusBadRequest)
		return
	}
	if err := AttributeWorkspace(message, decision.WorkspaceID); err != nil {
		if errors.Is(err, ErrReservedTenantAttribute) {
			http.Error(w, "producer supplied a reserved tenant attribute", http.StatusBadRequest)
		} else {
			http.Error(w, "invalid OTLP payload", http.StatusBadRequest)
		}
		return
	}

	forwardCtx, cancel := context.WithTimeout(r.Context(), g.timeout)
	defer cancel()
	encoded, err := proto.Marshal(message)
	if err != nil {
		http.Error(w, "could not encode OTLP request", http.StatusBadRequest)
		return
	}
	responseBody, err := g.forwarder.Export(forwardCtx, signal, encoded)
	if err != nil {
		if errors.Is(forwardCtx.Err(), context.DeadlineExceeded) {
			http.Error(w, "OTLP collector export timed out", http.StatusGatewayTimeout)
		} else {
			http.Error(w, "OTLP collector is temporarily unavailable", http.StatusBadGateway)
		}
		return
	}
	if len(responseBody) > maxOTLPRequestBytes {
		http.Error(w, "OTLP collector returned an oversized response", http.StatusBadGateway)
		return
	}
	if len(responseBody) > 0 {
		if err := proto.Unmarshal(responseBody, response); err != nil {
			http.Error(w, "OTLP collector returned an invalid response", http.StatusBadGateway)
			return
		}
	}
	var output []byte
	if mediaType == "application/x-protobuf" {
		output, err = proto.Marshal(response)
	} else {
		output, err = protojson.Marshal(response)
	}
	if err != nil {
		http.Error(w, "could not encode OTLP response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Length", fmt.Sprint(len(output)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output)
}

func parseWorkspaceSignalPath(path string) (workspaceID, signal string, ok bool) {
	parts := strings.Split(path, "/")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "v1" ||
		parts[2] != "workspaces" || parts[3] == "" || parts[4] != "v1" {
		return "", "", false
	}
	switch parts[5] {
	case "metrics", "logs", "traces":
		return parts[3], parts[5], true
	default:
		return "", "", false
	}
}

func newSignalMessages(signal string) (proto.Message, proto.Message, error) {
	switch signal {
	case "metrics":
		return &metricsv1.ExportMetricsServiceRequest{}, &metricsv1.ExportMetricsServiceResponse{}, nil
	case "logs":
		return &logsv1.ExportLogsServiceRequest{}, &logsv1.ExportLogsServiceResponse{}, nil
	case "traces":
		return &tracesv1.ExportTraceServiceRequest{}, &tracesv1.ExportTraceServiceResponse{}, nil
	default:
		return nil, nil, errors.New("unsupported signal")
	}
}

type collectorHTTPForwarder struct {
	baseURL string
	client *http.Client
}

func NewCollectorHTTPForwarder(baseURL string, timeout time.Duration) (OTLPForwarder, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("Collector OTLP HTTP endpoint must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	if timeout <= 0 {
		timeout = defaultForwardTimeout
	}
	if timeout > 30*time.Second {
		return nil, errors.New("Collector OTLP HTTP timeout must not exceed 30 seconds")
	}
	return &collectorHTTPForwarder{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (f *collectorHTTPForwarder) Export(ctx context.Context, signal string, payload []byte) ([]byte, error) {
	if signal != "metrics" && signal != "logs" && signal != "traces" {
		return nil, errors.New("unsupported OTLP signal")
	}
	endpoint := f.baseURL + "/v1/" + signal
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create Collector request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("Accept", "application/x-protobuf")
	response, err := f.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send OTLP request to Collector: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 8<<10))
		return nil, fmt.Errorf("Collector returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxOTLPRequestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Collector response: %w", err)
	}
	if len(body) > maxOTLPRequestBytes {
		return nil, errors.New("Collector response exceeds the 4 MiB limit")
	}
	return body, nil
}

type rateWindow struct {
	start time.Time
	count int
}

type workspaceRateLimiter struct {
	mu sync.Mutex
	limit int
	windows map[string]rateWindow
}

func newWorkspaceRateLimiter(limit int) *workspaceRateLimiter {
	return &workspaceRateLimiter{limit: limit, windows: make(map[string]rateWindow)}
}

func (l *workspaceRateLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	window, exists := l.windows[key]
	if !exists && len(l.windows) >= maxRateLimitEntries {
		for existingKey, existing := range l.windows {
			if now.Sub(existing.start) >= time.Minute {
				delete(l.windows, existingKey)
			}
		}
		if len(l.windows) >= maxRateLimitEntries {
			return false
		}
	}
	if !exists || now.Sub(window.start) >= time.Minute || now.Before(window.start) {
		l.windows[key] = rateWindow{start: now, count: 1}
		return true
	}
	if window.count >= l.limit {
		return false
	}
	window.count++
	l.windows[key] = window
	return true
}

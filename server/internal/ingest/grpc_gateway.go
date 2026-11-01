package ingest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracesv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	defaultGRPCForwardTimeout = 10 * time.Second
	maxConcurrentGRPCExports = 64
	workspaceMetadataKey = "orbit-workspace-id"
)

type GRPCForwarder interface {
	ExportMetrics(context.Context, *metricsv1.ExportMetricsServiceRequest) (*metricsv1.ExportMetricsServiceResponse, error)
	ExportLogs(context.Context, *logsv1.ExportLogsServiceRequest) (*logsv1.ExportLogsServiceResponse, error)
	ExportTraces(context.Context, *tracesv1.ExportTraceServiceRequest) (*tracesv1.ExportTraceServiceResponse, error)
}

type GRPCGatewayConfig struct {
	RequestsPerMinute int
	ForwardTimeout time.Duration
	RateLimitStore WorkloadRateLimitStore
}

type GRPCGateway struct {
	authorizer *Authorizer
	forwarder GRPCForwarder
	limiter *workspaceRateLimiter
	sharedLimiter WorkloadRateLimitStore
	timeout time.Duration
	concurrency chan struct{}
}

func NewGRPCGatewayServer(authorizer *Authorizer, forwarder GRPCForwarder, cfg GRPCGatewayConfig) (*grpc.Server, error) {
	if authorizer == nil || forwarder == nil {
		return nil, errors.New("workload authorizer and gRPC Collector forwarder are required")
	}
	if cfg.RequestsPerMinute < 1 || cfg.RequestsPerMinute > 10000 {
		return nil, errors.New("OTLP requests-per-minute limit must be between 1 and 10000")
	}
	if cfg.ForwardTimeout <= 0 {
		cfg.ForwardTimeout = defaultGRPCForwardTimeout
	}
	if cfg.ForwardTimeout > 30*time.Second {
		return nil, errors.New("OTLP gRPC forward timeout must not exceed 30 seconds")
	}
	gateway := &GRPCGateway{
		authorizer: authorizer,
		forwarder: forwarder,
		limiter: newWorkspaceRateLimiter(cfg.RequestsPerMinute),
		sharedLimiter: cfg.RateLimitStore,
		timeout: cfg.ForwardTimeout,
		concurrency: make(chan struct{}, maxConcurrentGRPCExports),
	}
	server := grpc.NewServer(
		grpc.MaxRecvMsgSize(maxOTLPRequestBytes),
		grpc.MaxSendMsgSize(maxOTLPRequestBytes),
		grpc.MaxConcurrentStreams(maxConcurrentGRPCExports),
	)
	metricsv1.RegisterMetricsServiceServer(server, &metricsGRPCService{gateway: gateway})
	logsv1.RegisterLogsServiceServer(server, &logsGRPCService{gateway: gateway})
	tracesv1.RegisterTraceServiceServer(server, &tracesGRPCService{gateway: gateway})
	return server, nil
}

func (g *GRPCGateway) authorize(ctx context.Context, signal string) (Decision, error) {
	incoming, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return Decision{}, status.Error(codes.Unauthenticated, "workload authorization metadata is required")
	}
	workspaceValues := incoming.Get(workspaceMetadataKey)
	if len(workspaceValues) != 1 || strings.TrimSpace(workspaceValues[0]) != workspaceValues[0] {
		return Decision{}, status.Error(codes.InvalidArgument, "exactly one orbit-workspace-id metadata value is required")
	}
	decision, err := g.authorizer.Authorize(ctx, incoming.Get("authorization"), workspaceValues[0], signal)
	if err == nil {
		return decision, nil
	}
	switch {
	case errors.Is(err, ErrInvalidTarget):
		return Decision{}, status.Error(codes.InvalidArgument, "invalid workspace or telemetry target")
	case errors.Is(err, ErrInvalidBearer):
		return Decision{}, status.Error(codes.Unauthenticated, "invalid workload credentials")
	case errors.Is(err, ErrDenied):
		return Decision{}, status.Error(codes.PermissionDenied, "workload is not authorized for this workspace and signal")
	default:
		return Decision{}, status.Error(codes.Unavailable, "workload authorization is temporarily unavailable")
	}
}

func (g *GRPCGateway) acquire(ctx context.Context) (func(), error) {
	select {
	case g.concurrency <- struct{}{}:
		return func() { <-g.concurrency }, nil
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	default:
		return nil, status.Error(codes.ResourceExhausted, "OTLP gRPC gateway is at capacity")
	}
}

func (g *GRPCGateway) beginExport(ctx context.Context, decision Decision) (context.Context, context.CancelFunc, error) {
	rateKey := "workload:" + decision.IdentityID + ":" + decision.WorkspaceID + ":" + decision.Signal
	rateCtx, rateCancel := context.WithTimeout(ctx, 750*time.Millisecond)
	allowed, err := allowWorkloadRate(rateCtx, g.sharedLimiter, g.limiter, g.limiter.limit, rateKey, time.Now())
	rateCancel()
	if err != nil {
		return nil, nil, status.Error(codes.Unavailable, "workload rate-limit service is temporarily unavailable")
	}
	if !allowed {
		return nil, nil, status.Error(codes.ResourceExhausted, "workload telemetry rate limit exceeded")
	}
	forwardCtx, cancel := context.WithTimeout(ctx, g.timeout)
	return forwardCtx, cancel, nil
}

type metricsGRPCService struct {
	metricsv1.UnimplementedMetricsServiceServer
	gateway *GRPCGateway
}

func (s *metricsGRPCService) Export(ctx context.Context, request *metricsv1.ExportMetricsServiceRequest) (*metricsv1.ExportMetricsServiceResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "OTLP metrics request is required")
	}
	release, err := s.gateway.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	decision, err := s.gateway.authorize(ctx, "metrics")
	if err != nil {
		return nil, err
	}
	forwardCtx, finish, err := s.gateway.beginExport(ctx, decision)
	if err != nil {
		return nil, err
	}
	defer finish()
	if proto.Size(request) > maxOTLPRequestBytes {
		return nil, status.Error(codes.ResourceExhausted, "OTLP request exceeds the 4 MiB limit")
	}
	sanitized := proto.Clone(request).(*metricsv1.ExportMetricsServiceRequest)
	if err := AttributeWorkspace(sanitized, decision.WorkspaceID); err != nil {
		return nil, invalidOTLPMessageError(err)
	}
	if proto.Size(sanitized) > maxOTLPRequestBytes {
		return nil, status.Error(codes.ResourceExhausted, "canonical OTLP request exceeds the 4 MiB limit")
	}
	response, err := s.gateway.forwarder.ExportMetrics(forwardCtx, sanitized)
	if err != nil {
		if errors.Is(forwardCtx.Err(), context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "OTLP Collector export timed out")
		}
		return nil, status.Error(codes.Unavailable, "OTLP Collector is temporarily unavailable")
	}
	return response, nil
}

type logsGRPCService struct {
	logsv1.UnimplementedLogsServiceServer
	gateway *GRPCGateway
}

func (s *logsGRPCService) Export(ctx context.Context, request *logsv1.ExportLogsServiceRequest) (*logsv1.ExportLogsServiceResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "OTLP logs request is required")
	}
	release, err := s.gateway.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	decision, err := s.gateway.authorize(ctx, "logs")
	if err != nil {
		return nil, err
	}
	forwardCtx, finish, err := s.gateway.beginExport(ctx, decision)
	if err != nil {
		return nil, err
	}
	defer finish()
	if proto.Size(request) > maxOTLPRequestBytes {
		return nil, status.Error(codes.ResourceExhausted, "OTLP request exceeds the 4 MiB limit")
	}
	sanitized := proto.Clone(request).(*logsv1.ExportLogsServiceRequest)
	if err := AttributeWorkspace(sanitized, decision.WorkspaceID); err != nil {
		return nil, invalidOTLPMessageError(err)
	}
	if proto.Size(sanitized) > maxOTLPRequestBytes {
		return nil, status.Error(codes.ResourceExhausted, "canonical OTLP request exceeds the 4 MiB limit")
	}
	response, err := s.gateway.forwarder.ExportLogs(forwardCtx, sanitized)
	if err != nil {
		if errors.Is(forwardCtx.Err(), context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "OTLP Collector export timed out")
		}
		return nil, status.Error(codes.Unavailable, "OTLP Collector is temporarily unavailable")
	}
	return response, nil
}

type tracesGRPCService struct {
	tracesv1.UnimplementedTraceServiceServer
	gateway *GRPCGateway
}

func (s *tracesGRPCService) Export(ctx context.Context, request *tracesv1.ExportTraceServiceRequest) (*tracesv1.ExportTraceServiceResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "OTLP traces request is required")
	}
	release, err := s.gateway.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	decision, err := s.gateway.authorize(ctx, "traces")
	if err != nil {
		return nil, err
	}
	forwardCtx, finish, err := s.gateway.beginExport(ctx, decision)
	if err != nil {
		return nil, err
	}
	defer finish()
	if proto.Size(request) > maxOTLPRequestBytes {
		return nil, status.Error(codes.ResourceExhausted, "OTLP request exceeds the 4 MiB limit")
	}
	sanitized := proto.Clone(request).(*tracesv1.ExportTraceServiceRequest)
	if err := AttributeWorkspace(sanitized, decision.WorkspaceID); err != nil {
		return nil, invalidOTLPMessageError(err)
	}
	if proto.Size(sanitized) > maxOTLPRequestBytes {
		return nil, status.Error(codes.ResourceExhausted, "canonical OTLP request exceeds the 4 MiB limit")
	}
	response, err := s.gateway.forwarder.ExportTraces(forwardCtx, sanitized)
	if err != nil {
		if errors.Is(forwardCtx.Err(), context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "OTLP Collector export timed out")
		}
		return nil, status.Error(codes.Unavailable, "OTLP Collector is temporarily unavailable")
	}
	return response, nil
}

func invalidOTLPMessageError(err error) error {
	if errors.Is(err, ErrReservedTenantAttribute) {
		return status.Error(codes.InvalidArgument, "producer supplied a reserved tenant attribute")
	}
	return status.Error(codes.InvalidArgument, "invalid OTLP payload")
}

type CollectorGRPCForwarder struct {
	connection *grpc.ClientConn
	metrics metricsv1.MetricsServiceClient
	logs logsv1.LogsServiceClient
	traces tracesv1.TraceServiceClient
}

func NewCollectorGRPCForwarder(endpoint string) (*CollectorGRPCForwarder, error) {
	endpoint = strings.TrimSpace(endpoint)
	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil || host == "" {
		return nil, errors.New("Collector OTLP gRPC endpoint must be host:port without a URL scheme")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("Collector OTLP gRPC endpoint has an invalid port")
	}
	connection, err := grpc.NewClient(
		endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxOTLPRequestBytes),
			grpc.MaxCallSendMsgSize(maxOTLPRequestBytes),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create private Collector gRPC client: %w", err)
	}
	return &CollectorGRPCForwarder{
		connection: connection,
		metrics: metricsv1.NewMetricsServiceClient(connection),
		logs: logsv1.NewLogsServiceClient(connection),
		traces: tracesv1.NewTraceServiceClient(connection),
	}, nil
}

func (f *CollectorGRPCForwarder) ExportMetrics(ctx context.Context, request *metricsv1.ExportMetricsServiceRequest) (*metricsv1.ExportMetricsServiceResponse, error) {
	return f.metrics.Export(ctx, request)
}

func (f *CollectorGRPCForwarder) ExportLogs(ctx context.Context, request *logsv1.ExportLogsServiceRequest) (*logsv1.ExportLogsServiceResponse, error) {
	return f.logs.Export(ctx, request)
}

func (f *CollectorGRPCForwarder) ExportTraces(ctx context.Context, request *tracesv1.ExportTraceServiceRequest) (*tracesv1.ExportTraceServiceResponse, error) {
	return f.traces.Export(ctx, request)
}

func (f *CollectorGRPCForwarder) Close() error {
	if f == nil || f.connection == nil {
		return nil
	}
	return f.connection.Close()
}

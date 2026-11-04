package observability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelLog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	httpRequestsMetric = "orbit_api_http_requests"
	httpDurationMetric = "orbit_api_http_request_duration"
	workerRunsMetric = "orbit_worker_runs"
	workerItemsMetric = "orbit_worker_items"
	workerDurationMetric = "orbit_worker_duration"
)

type Runtime struct {
	traces  *sdktrace.TracerProvider
	metrics *sdkmetric.MeterProvider
	logs    *sdklog.LoggerProvider

	tracer  trace.Tracer
	logger  otelLog.Logger
	requests metric.Int64Counter
	duration metric.Float64Histogram

	workerRuns metric.Int64Counter
	workerItems metric.Int64Counter
	workerDuration metric.Float64Histogram
}

// New creates an optional, fail-open OTLP runtime. The endpoint is a private
// Collector base URL; per-signal paths are explicit so exporter defaults cannot
// silently send data to the wrong path.
func New(ctx context.Context, endpoint, version, environment string) (*Runtime, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return nil, nil
	}

	res := resourceFor(version, environment)
	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint+"/v1/traces"))
	if err != nil {
		return nil, fmt.Errorf("create ORBIT API OTLP trace exporter: %w", err)
	}
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter, sdktrace.WithBatchTimeout(5*time.Second), sdktrace.WithMaxQueueSize(1024)),
	)

	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint+"/v1/metrics"))
	if err != nil {
		_ = traceProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("create ORBIT API OTLP metric exporter: %w", err)
	}
	metricProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(15*time.Second),
			sdkmetric.WithTimeout(5*time.Second),
		)),
	)

	logExporter, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(endpoint+"/v1/logs"))
	if err != nil {
		_ = metricProvider.Shutdown(context.Background())
		_ = traceProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("create ORBIT API OTLP log exporter: %w", err)
	}
	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter,
			sdklog.WithExportInterval(2*time.Second),
			sdklog.WithExportTimeout(5*time.Second),
			sdklog.WithMaxQueueSize(512),
		)),
	)

	meter := metricProvider.Meter("github.com/pooya16pg-spec/orbit/server/internal/observability")
	requests, err := meter.Int64Counter(httpRequestsMetric,
		metric.WithDescription("HTTP requests handled by the ORBIT API."),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		_ = logProvider.Shutdown(context.Background())
		_ = metricProvider.Shutdown(context.Background())
		_ = traceProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("create ORBIT API HTTP request counter: %w", err)
	}
	duration, err := meter.Float64Histogram(httpDurationMetric,
		metric.WithDescription("ORBIT API HTTP request duration."),
		metric.WithUnit("s"),
	)
	if err != nil {
		_ = logProvider.Shutdown(context.Background())
		_ = metricProvider.Shutdown(context.Background())
		_ = traceProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("create ORBIT API HTTP duration histogram: %w", err)
	}
	workerRuns, err := meter.Int64Counter(workerRunsMetric,
		metric.WithDescription("Background worker ticks completed by ORBIT."),
		metric.WithUnit("{tick}"),
	)
	if err != nil {
		_ = logProvider.Shutdown(context.Background())
		_ = metricProvider.Shutdown(context.Background())
		_ = traceProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("create ORBIT worker tick counter: %w", err)
	}
	workerItems, err := meter.Int64Counter(workerItemsMetric,
		metric.WithDescription("Items processed by ORBIT background workers."),
		metric.WithUnit("{item}"),
	)
	if err != nil {
		_ = logProvider.Shutdown(context.Background())
		_ = metricProvider.Shutdown(context.Background())
		_ = traceProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("create ORBIT worker item counter: %w", err)
	}
	workerDuration, err := meter.Float64Histogram(workerDurationMetric,
		metric.WithDescription("ORBIT background worker tick duration."),
		metric.WithUnit("s"),
	)
	if err != nil {
		_ = logProvider.Shutdown(context.Background())
		_ = metricProvider.Shutdown(context.Background())
		_ = traceProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("create ORBIT worker duration histogram: %w", err)
	}

	return &Runtime{
		traces: traceProvider, metrics: metricProvider, logs: logProvider,
		tracer: traceProvider.Tracer("github.com/pooya16pg-spec/orbit/server/internal/observability"),
		logger: logProvider.Logger("orbit-api"),
		requests: requests, duration: duration,
		workerRuns: workerRuns, workerItems: workerItems, workerDuration: workerDuration,
	}, nil
}

func resourceFor(version, environment string) *resource.Resource {
	attributes := []attribute.KeyValue{
		attribute.String("service.name", "orbit-api"),
	}
	if version != "" {
		attributes = append(attributes, attribute.String("service.version", version))
	}
	if environment != "" {
		attributes = append(attributes, attribute.String("deployment.environment", environment))
	}
	return resource.NewWithAttributes("", attributes...)
}

func (r *Runtime) Enabled() bool { return r != nil && r.traces != nil }

// EmitStartupLog emits a static, non-sensitive lifecycle event. No environment
// variables, endpoints, database URLs, tokens, request IDs, or tenant IDs are included.
func (r *Runtime) EmitStartupLog(ctx context.Context) {
	if !r.Enabled() {
		return
	}
	now := time.Now().UTC()
	var record otelLog.Record
	record.SetTimestamp(now)
	record.SetObservedTimestamp(now)
	record.SetSeverity(otelLog.SeverityInfo)
	record.SetSeverityText("INFO")
	record.SetBody(attribute.StringValue("orbit_api_runtime_started"))
	r.logger.Emit(ctx, record)
}

// HTTPMiddleware records only normalized methods, ServeMux route templates, and
// status classes. It never exports raw paths, query strings, headers, identities,
// workspace identifiers, request IDs, or request/response bodies.
func (r *Runtime) HTTPMiddleware(next http.Handler, routePattern func(*http.Request) string) http.Handler {
	if !r.Enabled() || next == nil || routePattern == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		route := normalizedRoute(routePattern(request))
		method := normalizedMethod(request.Method)
		parent := propagation.TraceContext{}.Extract(request.Context(), propagation.HeaderCarrier(request.Header))
		ctx, span := r.tracer.Start(parent, "HTTP "+method+" "+route,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.request.method", method),
				attribute.String("http.route", route),
			),
		)

		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, request.WithContext(ctx))
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		class := statusClass(status)
		attributes := []attribute.KeyValue{
			attribute.String("http.request.method", method),
			attribute.String("http.route", route),
			attribute.String("http.response.status_class", class),
		}
		duration := time.Since(started)
		r.requests.Add(ctx, 1, metric.WithAttributes(attributes...))
		r.duration.Record(ctx, duration.Seconds(), metric.WithAttributes(attributes...))
		span.SetAttributes(attribute.Int("http.response.status_code", status))
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, "HTTP server error")
			r.emitServerError(ctx, attributes)
		}
		span.End()
	})
}

func (r *Runtime) emitServerError(ctx context.Context, attributes []attribute.KeyValue) {
	now := time.Now().UTC()
	var record otelLog.Record
	record.SetTimestamp(now)
	record.SetObservedTimestamp(now)
	record.SetSeverity(otelLog.SeverityError)
	record.SetSeverityText("ERROR")
	record.SetBody(attribute.StringValue("http_server_request_failed"))
	record.AddAttributes(attributes...)
	r.logger.Emit(ctx, record)
}

// StartWorker opens one bounded worker-tick span. Worker names and outcomes are
// allow-listed; error text and all per-tenant or per-delivery identifiers are omitted.
func (r *Runtime) StartWorker(ctx context.Context, worker string) (context.Context, func(string, int64, error)) {
	if !r.Enabled() || !validWorkerName(worker) {
		return ctx, func(string, int64, error) {}
	}
	ctx, span := r.tracer.Start(ctx, "worker."+worker+".tick",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String("worker.name", worker)),
	)
	started := time.Now()
	finish := func(outcome string, items int64, err error) {
		outcome = normalizedWorkerOutcome(outcome)
		attributes := []attribute.KeyValue{
			attribute.String("worker.name", worker),
			attribute.String("worker.outcome", outcome),
		}
		r.workerRuns.Add(ctx, 1, metric.WithAttributes(attributes...))
		if items > 0 {
			r.workerItems.Add(ctx, items, metric.WithAttributes(attributes...))
		}
		r.workerDuration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(attributes...))
		span.SetAttributes(attribute.String("worker.outcome", outcome), attribute.Int64("worker.items", items))
		if err != nil {
			span.SetStatus(codes.Error, "worker tick failed")
			r.emitWorkerError(ctx, worker, outcome)
		}
		span.End()
	}
	return ctx, finish
}

func validWorkerName(worker string) bool {
	switch worker {
	case "alert_evaluator", "notification_delivery":
		return true
	default:
		return false
	}
}

func normalizedWorkerOutcome(outcome string) string {
	switch outcome {
	case "ok", "skipped", "error", "cancelled":
		return outcome
	default:
		return "other"
	}
}

func (r *Runtime) emitWorkerError(ctx context.Context, worker, outcome string) {
	now := time.Now().UTC()
	var record otelLog.Record
	record.SetTimestamp(now)
	record.SetObservedTimestamp(now)
	record.SetSeverity(otelLog.SeverityError)
	record.SetSeverityText("ERROR")
	record.SetBody(attribute.StringValue("worker_tick_failed"))
	record.AddAttributes(
		attribute.String("worker.name", worker),
		attribute.String("worker.outcome", outcome),
	)
	r.logger.Emit(ctx, record)
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if !r.Enabled() {
		return nil
	}
	var errs []error
	if err := r.traces.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown trace provider: %w", err))
	}
	if err := r.metrics.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown metric provider: %w", err))
	}
	if err := r.logs.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown log provider: %w", err))
	}
	return errors.Join(errs...)
}

func normalizedMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

func statusClass(status int) string {
	switch {
	case status >= 100 && status < 200:
		return "1xx"
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500 && status < 600:
		return "5xx"
	default:
		return "other"
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (w *responseRecorder) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *responseRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func normalizedRoute(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "unmatched"
	}
	if _, path, ok := strings.Cut(pattern, " "); ok && strings.HasPrefix(path, "/") {
		return path
	}
	return pattern
}

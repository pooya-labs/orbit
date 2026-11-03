package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	metricapi "go.opentelemetry.io/otel/metric"
	traceapi "go.opentelemetry.io/otel/trace"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	serviceName    = "orbit-telemetry-demo"
	serviceVersion = "0.5.0"
	listenAddress  = ":8081"
)

type telemetry struct {
	tracer   traceapi.Tracer
	requests metricapi.Int64Counter
	logger   otellog.Logger

	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	loggerProvider *sdklog.LoggerProvider
}

type emitResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	TraceID string `json:"trace_id"`
	Message string `json:"message"`
}

func newTelemetry(ctx context.Context) (*telemetry, error) {
	res := resource.NewWithAttributes(
		"",
		attribute.String("service.name", serviceName),
		attribute.String("service.version", serviceVersion),
		attribute.String("deployment.environment", "development"),
	)

	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	metricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		_ = tracerProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			metricExporter,
			sdkmetric.WithInterval(2*time.Second),
		)),
	)

	logExporter, err := otlploghttp.New(ctx)
	if err != nil {
		_ = meterProvider.Shutdown(context.Background())
		_ = tracerProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("create OTLP log exporter: %w", err)
	}
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(
			logExporter,
			sdklog.WithExportInterval(time.Second),
		)),
	)

	requests, err := meterProvider.Meter("orbit.telemetry.demo").Int64Counter(
		"orbit_demo_requests",
		metricapi.WithDescription("Requests emitted by the ORBIT telemetry demo service."),
	)
	if err != nil {
		_ = loggerProvider.Shutdown(context.Background())
		_ = meterProvider.Shutdown(context.Background())
		_ = tracerProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("create request counter: %w", err)
	}

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)

	return &telemetry{
		tracer:         tracerProvider.Tracer("orbit.telemetry.demo"),
		requests:       requests,
		logger:         loggerProvider.Logger("orbit.telemetry.demo"),
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
		loggerProvider: loggerProvider,
	}, nil
}

func (t *telemetry) shutdown(ctx context.Context) error {
	return errors.Join(
		t.loggerProvider.Shutdown(ctx),
		t.meterProvider.Shutdown(ctx),
		t.tracerProvider.Shutdown(ctx),
	)
}

func sloOutcomeForHTTPStatus(statusCode int) string {
	if statusCode >= http.StatusOK && statusCode < 400 {
		return "good"
	}
	return "bad"
}

func (t *telemetry) recordSLORequest(ctx context.Context, route, method string, statusCode int) {
	outcome := sloOutcomeForHTTPStatus(statusCode)
	t.requests.Add(ctx, 1, metricapi.WithAttributes(
		attribute.String("http.route", route),
		attribute.String("http.request.method", method),
		attribute.Int("http.response.status_code", statusCode),
		attribute.String("slo_route", route),
		attribute.String("slo_outcome", outcome),
	))
}

func (t *telemetry) emitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		t.recordSLORequest(r.Context(), "/emit", r.Method, http.StatusMethodNotAllowed)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, span := t.tracer.Start(r.Context(), "GET /emit")
	defer span.End()

	traceID := span.SpanContext().TraceID().String()
	now := time.Now().UTC()
	span.SetAttributes(
		attribute.String("http.request.method", http.MethodGet),
		attribute.String("http.route", "/emit"),
		attribute.String("service.name", serviceName),
		attribute.String("demo.signal", "smoke-test"),
	)

	t.recordSLORequest(ctx, "/emit", http.MethodGet, http.StatusAccepted)

	var record otellog.Record
	record.SetTimestamp(now)
	record.SetObservedTimestamp(now)
	record.SetSeverity(otellog.SeverityInfo)
	record.SetSeverityText("INFO")
	record.SetBody(attribute.StringValue(fmt.Sprintf("trace_id=%s event=orbit-telemetry-smoke", traceID)))
	record.AddAttributes(
		attribute.String("http.route", "/emit"),
		attribute.String("trace_id", traceID),
		attribute.String("demo.signal", "smoke-test"),
	)
	t.logger.Emit(ctx, record)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(emitResponse{
		Status:  "accepted",
		Service: serviceName,
		TraceID: traceID,
		Message: "one metric, one trace, and one log record submitted through OTLP",
	})
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	telemetryProviders, err := newTelemetry(ctx)
	if err != nil {
		log.Fatalf("initialize telemetry: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/emit", telemetryProviders.emitHandler)

	server := &http.Server{
		Addr:              listenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("%s listening on %s", serviceName, listenAddress)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server stopped: %v", err)
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown: %v", err)
	}
	if err := telemetryProviders.shutdown(shutdownCtx); err != nil {
		log.Printf("telemetry shutdown: %v", err)
	}
}

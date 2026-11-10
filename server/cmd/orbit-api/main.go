package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/alerts"
	"github.com/pooya16pg-spec/orbit/server/internal/auth"
	"github.com/pooya16pg-spec/orbit/server/internal/config"
	"github.com/pooya16pg-spec/orbit/server/internal/httpapi"
	"github.com/pooya16pg-spec/orbit/server/internal/ingest"
	"github.com/pooya16pg-spec/orbit/server/internal/notifications"
	"github.com/pooya16pg-spec/orbit/server/internal/observability"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
	"github.com/pooya16pg-spec/orbit/server/internal/store/postgres"
	"github.com/pooya16pg-spec/orbit/server/internal/telemetry"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := cfg.Validate(); err != nil {
		logger.Error("API configuration is invalid", "error", err)
		os.Exit(1)
	}

	var authVerifier httpapi.TokenVerifier
	if cfg.LocalWorkEnabled {
		authVerifier, err = auth.NewStaticTokenVerifier(cfg.LocalHumanToken, auth.Principal{
			Issuer: "urn:orbit:local-work", Subject: "local-owner", Roles: []string{"administrator"},
		})
		if err != nil {
			logger.Error("Local Work authentication initialization failed", "error", err)
			os.Exit(1)
		}
		logger.Info("explicit Local Work single-user authentication enabled; keep all listeners on loopback")
	} else if cfg.OIDCIssuer != "" {
		authCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		verifier, err := auth.NewVerifier(authCtx, auth.Config{
			Issuer: cfg.OIDCIssuer, Audience: cfg.OIDCAudience,
			RolesClaim: cfg.OIDCRolesClaim, AllowInsecureHTTP: cfg.Environment == "development",
		})
		cancel()
		if err != nil {
			logger.Error("OIDC authentication initialization failed", "error", err)
			os.Exit(1)
		}
		authVerifier = verifier
		defer verifier.Close()
	}

	telemetryClient, err := telemetry.NewClient(cfg.PrometheusURL, cfg.LokiURL, cfg.TempoURL)
	if err != nil {
		logger.Error("telemetry backend configuration is invalid", "error", err)
		os.Exit(1)
	}

	var webhookSender *notifications.WebhookSender
	if len(cfg.AlertWebhookSecrets) > 0 {
		webhookSender, err = notifications.NewWebhookSender(cfg.AlertWebhookSecrets, cfg.AlertWebhookAllowedHosts)
		if err != nil {
			logger.Error("alert webhook sender configuration is invalid", "error", err)
			os.Exit(1)
		}
	}

	var repository store.Repository
	if cfg.DatabaseURL != "" {
		startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		db, err := postgres.Open(startupCtx, cfg.DatabaseURL)
		cancel()
		if err != nil {
			logger.Error("database initialization failed", "error", err)
			os.Exit(1)
		}
		repository = db
		defer repository.Close()
	} else {
		logger.Warn("PostgreSQL is not configured; readiness and data endpoints will be unavailable")
	}

	apiHandler := httpapi.NewHandler(httpapi.Config{
		Version: cfg.Version, Environment: cfg.Environment, AllowedOrigins: cfg.AllowedOrigins,
		Telemetry: telemetryClient, AuthVerifier: authVerifier,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		AlertWebhookSecretRefs: func() []string {
			if webhookSender == nil { return []string{} }
			return webhookSender.ConfiguredSecretRefs()
		}(),
	}, repository, logger)
	var apiObservability *observability.Runtime
	var apiHTTPHandler http.Handler = apiHandler
	if cfg.SelfTelemetryOTLPHTTPURL != "" {
		observabilityCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		runtime, runtimeErr := observability.New(observabilityCtx, cfg.SelfTelemetryOTLPHTTPURL, cfg.Version, cfg.Environment)
		cancel()
		if runtimeErr != nil {
			logger.Warn("API self-observability disabled; application startup will continue", "error", runtimeErr)
		} else if runtime != nil {
			apiObservability = runtime
			apiHTTPHandler = runtime.HTTPMiddleware(apiHandler, apiHandler.RoutePattern)
			runtime.EmitStartupLog(context.Background())
			logger.Info("API self-observability enabled", "service", "orbit-api")
		}
	}

	apiServer := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: apiHTTPHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	var ingestionHTTPServer *http.Server
	var ingestionGRPCServer *grpc.Server
	var ingestionHTTPListener net.Listener
	var ingestionGRPCListener net.Listener
	var workloadVerifier ingest.TokenVerifier
	var collectorGRPCForwarder *ingest.CollectorGRPCForwarder

	if cfg.WorkloadOIDCIssuer != "" || cfg.LocalWorkEnabled {
		workloadRepository, ok := repository.(store.WorkloadIdentityRepository)
		if !ok || workloadRepository == nil {
			logger.Error("workload ingestion requires a repository with workload identity authorization support")
			os.Exit(1)
		}
		rateLimitStore, ok := repository.(ingest.WorkloadRateLimitStore)
		if !ok || rateLimitStore == nil {
			logger.Error("workload ingestion requires the shared PostgreSQL rate-limit store")
			os.Exit(1)
		}

		if cfg.LocalWorkEnabled {
			workloadVerifier, err = auth.NewStaticTokenVerifier(cfg.LocalWorkloadToken, auth.Principal{
				Issuer: "urn:orbit:local-workload", Subject: "orbit-telemetry-demo",
			})
			if err != nil {
				logger.Error("Local Work telemetry token initialization failed", "error", err)
				os.Exit(1)
			}
		} else {
			authCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			var oidcVerifier *auth.Verifier
			oidcVerifier, err = auth.NewIngestionVerifier(authCtx, auth.Config{
				Issuer: cfg.WorkloadOIDCIssuer,
				Audience: cfg.WorkloadOIDCAudience,
				AllowInsecureHTTP: cfg.Environment == "development",
			}, cfg.OIDCAudience)
			cancel()
			if err != nil {
				logger.Error("workload OIDC verifier initialization failed", "error", err)
				os.Exit(1)
			}
			workloadVerifier = oidcVerifier
			defer oidcVerifier.Close()
		}

		resourceAttributeRecorder, ok := repository.(store.WorkloadResourceAttributeRepository)
		if !ok {
			resourceAttributeRecorder = nil
		}
		onResourceAttributeError := func(err error) {
			logger.Warn("persist workload OTLP resource attributes failed", "error", err)
		}

		authorizer, err := ingest.NewAuthorizer(workloadVerifier, workloadRepository)
		if err != nil {
			logger.Error("workload authorization initialization failed", "error", err)
			os.Exit(1)
		}
		httpForwarder, err := ingest.NewCollectorHTTPForwarder(cfg.CollectorOTLPHTTPURL, 10*time.Second)
		if err != nil {
			logger.Error("Collector OTLP HTTP client configuration is invalid", "error", err)
			os.Exit(1)
		}
		collectorGRPCForwarder, err = ingest.NewCollectorGRPCForwarder(cfg.CollectorOTLPGRPCEndpoint)
		if err != nil {
			logger.Error("Collector OTLP gRPC client configuration is invalid", "error", err)
			os.Exit(1)
		}
		defer collectorGRPCForwarder.Close()

		httpHandler, err := ingest.NewHTTPGateway(authorizer, httpForwarder, ingest.HTTPGatewayConfig{
			RequestsPerMinute: cfg.RateLimitPerMinute,
			ForwardTimeout: 10 * time.Second,
			RateLimitStore: rateLimitStore,
			ResourceAttributeRecorder: resourceAttributeRecorder,
			OnResourceAttributeError: onResourceAttributeError,
		})
		if err != nil {
			logger.Error("OTLP HTTP gateway initialization failed", "error", err)
			os.Exit(1)
		}
		ingestionGRPCServer, err = ingest.NewGRPCGatewayServer(authorizer, collectorGRPCForwarder, ingest.GRPCGatewayConfig{
			RequestsPerMinute: cfg.RateLimitPerMinute,
			ForwardTimeout: 10 * time.Second,
			RateLimitStore: rateLimitStore,
			ResourceAttributeRecorder: resourceAttributeRecorder,
			OnResourceAttributeError: onResourceAttributeError,
		})
		if err != nil {
			logger.Error("OTLP gRPC gateway initialization failed", "error", err)
			os.Exit(1)
		}

		ingestionHTTPListener, err = net.Listen("tcp", cfg.IngestionHTTPAddr)
		if err != nil {
			logger.Error("could not bind OTLP HTTP gateway listener", "address", cfg.IngestionHTTPAddr, "error", err)
			os.Exit(1)
		}
		ingestionGRPCListener, err = net.Listen("tcp", cfg.IngestionGRPCAddr)
		if err != nil {
			_ = ingestionHTTPListener.Close()
			logger.Error("could not bind OTLP gRPC gateway listener", "address", cfg.IngestionGRPCAddr, "error", err)
			os.Exit(1)
		}
		ingestionHTTPServer = &http.Server{
			Handler: httpHandler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout: 10 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout: 60 * time.Second,
			MaxHeaderBytes: 64 << 10,
		}
		logger.Info(
			"authenticated OTLP ingestion gateways enabled",
			"http_address", cfg.IngestionHTTPAddr,
			"grpc_address", cfg.IngestionGRPCAddr,
			"collector_http", cfg.CollectorOTLPHTTPURL,
			"collector_grpc", cfg.CollectorOTLPGRPCEndpoint,
		)
	} else {
		logger.Warn("authenticated OTLP ingestion gateways are disabled; workload OIDC and Local Work are not configured")
	}

	serverErrors := make(chan error, 3)
	go func() {
		logger.Info("starting ORBIT API", "address", cfg.HTTPAddr, "environment", cfg.Environment, "version", cfg.Version)
		serverErrors <- apiServer.ListenAndServe()
	}()
	if ingestionHTTPServer != nil {
		go func() {
			logger.Info("starting authenticated OTLP HTTP gateway", "address", cfg.IngestionHTTPAddr)
			serverErrors <- ingestionHTTPServer.Serve(ingestionHTTPListener)
		}()
		go func() {
			logger.Info("starting authenticated OTLP gRPC gateway", "address", cfg.IngestionGRPCAddr)
			serverErrors <- ingestionGRPCServer.Serve(ingestionGRPCListener)
		}()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if evaluationRepository, ok := repository.(store.AlertEvaluationRepository); ok &&
		evaluationRepository != nil && cfg.PrometheusURL != "" {
		worker := alerts.NewWorker(evaluationRepository, telemetryClient, logger, 30*time.Second, apiObservability)
		go worker.Run(ctx)
		logger.Info("workspace alert evaluator started", "interval", "30s")
	} else {
		logger.Info("workspace alert evaluator disabled; PostgreSQL alert storage and Prometheus are both required")
	}

	var notificationWorkerDone chan struct{}
	if deliveryRepository, ok := repository.(store.AlertNotificationDeliveryRepository); ok &&
		deliveryRepository != nil && webhookSender != nil {
		worker := notifications.NewWorker(deliveryRepository, webhookSender, logger, 5*time.Second, apiObservability)
		notificationWorkerDone = make(chan struct{})
		go func() {
			defer close(notificationWorkerDone)
			worker.Run(ctx)
		}()
		logger.Info("alert notification delivery worker started", "interval", "5s")
	} else {
		logger.Info("alert notification delivery worker disabled; PostgreSQL and webhook secrets are required")
	}

	var runtimeErr error
	select {
	case err := <-serverErrors:
		if err == nil {
			runtimeErr = errors.New("an ORBIT listener stopped unexpectedly")
		} else {
			runtimeErr = err
		}
	case <-ctx.Done():
	}

	stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if notificationWorkerDone != nil {
		select {
		case <-notificationWorkerDone:
		case <-shutdownCtx.Done():
			logger.Warn("alert notification worker did not stop before shutdown deadline")
		}
	}

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("API HTTP graceful shutdown failed", "error", err)
		_ = apiServer.Close()
	}
	if apiObservability != nil {
		if err := apiObservability.Shutdown(shutdownCtx); err != nil {
			logger.Warn("API self-observability flush failed during shutdown", "error", err)
		}
	}
	if ingestionHTTPServer != nil {
		if err := ingestionHTTPServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("OTLP HTTP graceful shutdown failed", "error", err)
			_ = ingestionHTTPServer.Close()
		}
	}
	if ingestionGRPCServer != nil {
		gracefulDone := make(chan struct{})
		go func() {
			ingestionGRPCServer.GracefulStop()
			close(gracefulDone)
		}()
		select {
		case <-gracefulDone:
		case <-shutdownCtx.Done():
			ingestionGRPCServer.Stop()
		}
	}
	if ingestionHTTPListener != nil {
		_ = ingestionHTTPListener.Close()
	}
	if ingestionGRPCListener != nil {
		_ = ingestionGRPCListener.Close()
	}

	if runtimeErr != nil && !errors.Is(runtimeErr, http.ErrServerClosed) {
		logger.Error("an ORBIT listener stopped unexpectedly", "error", runtimeErr)
		os.Exit(1)
	}
	logger.Info("ORBIT API stopped")
}

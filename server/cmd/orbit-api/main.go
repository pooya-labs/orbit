package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/config"
	"github.com/pooya16pg-spec/orbit/server/internal/httpapi"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
	"github.com/pooya16pg-spec/orbit/server/internal/store/postgres"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

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

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewHandler(httpapi.Config{Version: cfg.Version, Environment: cfg.Environment, AllowedOrigins: cfg.AllowedOrigins}, repository, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("starting ORBIT API", "address", cfg.HTTPAddr, "environment", cfg.Environment, "version", cfg.Version)
		serverErrors <- server.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			if closeErr := server.Close(); closeErr != nil {
				logger.Error("forced server close failed", "error", closeErr)
			}
			os.Exit(1)
		}
		logger.Info("ORBIT API stopped")
	}
}

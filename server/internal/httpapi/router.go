package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
	"github.com/pooya16pg-spec/orbit/server/internal/telemetry"
)

type Config struct {
	Version        string
	Environment    string
	AllowedOrigins []string
	Telemetry      *telemetry.Client
}

type statusResponse struct {
	Status string `json:"status"`
}

type metadataResponse struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
}

type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type servicesResponse struct {
	Data []store.Service `json:"data"`
}

type incidentsResponse struct {
	Data []store.Incident `json:"data"`
}

func NewHandler(cfg Config, repository store.Repository, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if repository == nil {
			writeError(w, r, http.StatusServiceUnavailable, "database_unavailable", "PostgreSQL is not configured")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := repository.Ping(ctx); err != nil {
			logger.Error("readiness dependency check failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "database_unavailable", "A required dependency is unavailable")
			return
		}
		writeJSON(w, http.StatusOK, statusResponse{Status: "ready"})
	})
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, metadataResponse{
			Name:        "orbit-api",
			Version:     cfg.Version,
			Environment: cfg.Environment,
		})
	})
	mux.HandleFunc("GET /api/v1/services", func(w http.ResponseWriter, r *http.Request) {
		if repository == nil {
			writeError(w, r, http.StatusServiceUnavailable, "database_unavailable", "Service inventory is not available until PostgreSQL is configured")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		items, err := repository.ListServices(ctx)
		if err != nil {
			logger.Error("list services failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "query_unavailable", "Service inventory is temporarily unavailable")
			return
		}
		writeJSON(w, http.StatusOK, servicesResponse{Data: items})
	})
	mux.HandleFunc("GET /api/v1/incidents", func(w http.ResponseWriter, r *http.Request) {
		if repository == nil {
			writeError(w, r, http.StatusServiceUnavailable, "database_unavailable", "Incident inventory is not available until PostgreSQL is configured")
			return
		}
		status := r.URL.Query().Get("status")
		if !validIncidentStatus(status) {
			writeError(w, r, http.StatusBadRequest, "invalid_status", "status must be open, investigating, monitoring, or resolved")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		items, err := repository.ListIncidents(ctx, status)
		if err != nil {
			logger.Error("list incidents failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "query_unavailable", "Incident inventory is temporarily unavailable")
			return
		}
		writeJSON(w, http.StatusOK, incidentsResponse{Data: items})
	})

	mux.HandleFunc("GET /api/v1/telemetry/metrics", func(w http.ResponseWriter, r *http.Request) {
		window, err := telemetry.ParseWindow(r.URL.Query().Get("window"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_window", err.Error())
			return
		}
		if cfg.Telemetry == nil {
			writeError(w, r, http.StatusServiceUnavailable, "telemetry_unavailable", "Prometheus is not configured")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
		defer cancel()
		result, err := cfg.Telemetry.Metrics(ctx, window)
		if err != nil {
			writeTelemetryError(w, r, logger, "Prometheus", err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("GET /api/v1/telemetry/logs", func(w http.ResponseWriter, r *http.Request) {
		window, err := telemetry.ParseWindow(r.URL.Query().Get("window"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_window", err.Error())
			return
		}
		service := r.URL.Query().Get("service")
		if service == "" {
			service = "orbit-telemetry-demo"
		}
		if err := telemetry.ValidateServiceName(service); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_service", err.Error())
			return
		}
		limit, err := telemetry.ParseLimit(r.URL.Query().Get("limit"), 50)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_limit", err.Error())
			return
		}
		if cfg.Telemetry == nil {
			writeError(w, r, http.StatusServiceUnavailable, "telemetry_unavailable", "Loki is not configured")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
		defer cancel()
		result, err := cfg.Telemetry.Logs(ctx, window, service, limit)
		if err != nil {
			writeTelemetryError(w, r, logger, "Loki", err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("GET /api/v1/telemetry/traces", func(w http.ResponseWriter, r *http.Request) {
		window, err := telemetry.ParseWindow(r.URL.Query().Get("window"))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_window", err.Error())
			return
		}
		service := r.URL.Query().Get("service")
		if service == "" {
			service = "orbit-telemetry-demo"
		}
		if err := telemetry.ValidateServiceName(service); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_service", err.Error())
			return
		}
		limit, err := telemetry.ParseLimit(r.URL.Query().Get("limit"), 20)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_limit", err.Error())
			return
		}
		if cfg.Telemetry == nil {
			writeError(w, r, http.StatusServiceUnavailable, "telemetry_unavailable", "Tempo is not configured")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
		defer cancel()
		result, err := cfg.Telemetry.Traces(ctx, window, service, limit)
		if err != nil {
			writeTelemetryError(w, r, logger, "Tempo", err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	var handler http.Handler = mux
	handler = withRequestLogging(logger, handler)
	handler = withRequestID(handler)
	handler = withRecovery(logger, handler)
	handler = withCORS(cfg.AllowedOrigins, handler)
	return handler
}


func writeTelemetryError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, backend string, err error) {
	if errors.Is(err, telemetry.ErrNotConfigured) {
		writeError(w, r, http.StatusServiceUnavailable, "telemetry_unavailable", backend+" is not configured")
		return
	}
	logger.Error("telemetry query failed", "backend", backend, "error", err, "request_id", r.Header.Get("X-Request-ID"))
	writeError(w, r, http.StatusServiceUnavailable, "telemetry_query_failed", backend+" data is temporarily unavailable")
}

func withCORS(origins []string, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if r.Method == http.MethodOptions {
			if _, ok := allowed[origin]; !ok || origin == "" {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if _, ok := allowed[origin]; ok && origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		next.ServeHTTP(w, r)
	})
}

func validIncidentStatus(status string) bool {
	switch status {
	case "", "open", "investigating", "monitoring", "resolved":
		return true
	default:
		return false
	}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: errorDetail{
		Code: code, Message: message, RequestID: r.Header.Get("X-Request-ID"),
	}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func withRequestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", time.Since(started).Milliseconds(),
			"request_id", r.Header.Get("X-Request-ID"),
		)
	})
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" || len(requestID) > 128 {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		r.Header.Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r)
	})
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(value[:])
}

func withRecovery(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("http handler panic", "panic", recovered, "path", r.URL.Path)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

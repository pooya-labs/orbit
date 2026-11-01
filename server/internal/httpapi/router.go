package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type Config struct {
	Version     string
	Environment string
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

	var handler http.Handler = mux
	handler = withRequestLogging(logger, handler)
	handler = withRequestID(handler)
	handler = withRecovery(logger, handler)
	return handler
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

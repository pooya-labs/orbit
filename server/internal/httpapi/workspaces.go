package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
	"github.com/pooya16pg-spec/orbit/server/internal/telemetry"
)

type workspaceListResponse struct {
	Data []store.Workspace `json:"data"`
}

type workspaceCreateRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type workspaceCreateResponse struct {
	Data store.Workspace `json:"data"`
}

type workspaceMembershipResponse struct {
	Data store.WorkspaceMembership `json:"data"`
}

type workspaceAuditResponse struct {
	Data []store.WorkspaceAuditEvent `json:"data"`
}

type incidentCreateRequest struct {
	Title string `json:"title"`
	Description string `json:"description"`
	ServiceID string `json:"service_id"`
	Severity string `json:"severity"`
}

type incidentTransitionRequest struct {
	Status string `json:"status"`
	Comment string `json:"comment"`
}

type incidentResponse struct {
	Data store.Incident `json:"data"`
}

type incidentTimelineResponse struct {
	Data []store.IncidentTimelineEvent `json:"data"`
}

func decodeIncidentJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}
	return nil
}

func incidentError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, operation string, err error) {
	switch {
	case errors.Is(err, store.ErrWorkspaceMembershipNotFound),
		errors.Is(err, store.ErrIncidentNotFound),
		errors.Is(err, store.ErrIncidentServiceNotFound):
		writeError(w, r, http.StatusNotFound, "incident_resource_not_found", "The workspace, incident, or service was not found")
	case errors.Is(err, store.ErrIncidentMutationForbidden):
		writeError(w, r, http.StatusForbidden, "incident_mutation_forbidden", "Your workspace role cannot change incidents")
	case errors.Is(err, store.ErrIncidentInvalidTransition),
		errors.Is(err, store.ErrIncidentNoChange):
		writeError(w, r, http.StatusConflict, "incident_transition_conflict", "The requested incident status transition is not allowed")
	case errors.Is(err, store.ErrIncidentInvalidInput),
		strings.Contains(err.Error(), "UUID"),
		strings.Contains(err.Error(), "principal"):
		writeError(w, r, http.StatusBadRequest, "invalid_incident", "Incident fields or identifiers are invalid")
	default:
		logger.Error(operation, "error", err, "request_id", r.Header.Get("X-Request-ID"))
		writeError(w, r, http.StatusServiceUnavailable, "incident_operation_unavailable", "The incident operation is temporarily unavailable")
	}
}

func registerWorkspaceRoutes(mux *http.ServeMux, repository store.Repository, telemetryClient *telemetry.Client, logger *slog.Logger) {
	workspaceRepository, ok := repository.(store.WorkspaceRepository)
	if !ok || workspaceRepository == nil {
		unavailable := func(w http.ResponseWriter, r *http.Request) {
			writeError(w, r, http.StatusServiceUnavailable, "workspace_store_unavailable", "Workspace management is not available")
		}
		mux.HandleFunc("GET /api/v1/workspaces", unavailable)
		mux.HandleFunc("POST /api/v1/workspaces", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/membership", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/audit-events", unavailable)
		return
	}

	mux.HandleFunc("GET /api/v1/workspaces", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		items, err := workspaceRepository.ListWorkspacesForPrincipal(ctx, principal.Issuer, principal.Subject)
		if err != nil {
			logger.Error("list principal workspaces failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workspace_query_failed", "Workspace list is temporarily unavailable")
			return
		}
		if items == nil {
			items = []store.Workspace{}
		}
		writeJSON(w, http.StatusOK, workspaceListResponse{Data: items})
	})

	mux.HandleFunc("POST /api/v1/workspaces", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		var input workspaceCreateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_workspace", "Workspace name and slug must be valid JSON fields")
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			writeError(w, r, http.StatusBadRequest, "invalid_workspace", "Request body must contain exactly one JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		workspace, err := workspaceRepository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
			Name: input.Name, Slug: input.Slug,
			OwnerIssuer: principal.Issuer, OwnerSubject: principal.Subject,
		})
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
				writeError(w, r, http.StatusConflict, "workspace_slug_conflict", "That workspace slug is already in use")
				return
			}
			if strings.Contains(err.Error(), "must contain") || strings.Contains(err.Error(), "slug") {
				writeError(w, r, http.StatusBadRequest, "invalid_workspace", "Workspace name or slug is invalid")
				return
			}
			logger.Error("create workspace failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workspace_create_failed", "Workspace could not be created")
			return
		}
		writeJSON(w, http.StatusCreated, workspaceCreateResponse{Data: workspace})
	})

	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/membership", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		membership, err := workspaceRepository.GetWorkspaceMembership(ctx, r.PathValue("workspaceID"), principal.Issuer, principal.Subject)
		if errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
			writeError(w, r, http.StatusNotFound, "workspace_not_found", "Workspace was not found")
			return
		}
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_workspace_id", "Workspace identifier is invalid")
			return
		}
		writeJSON(w, http.StatusOK, workspaceMembershipResponse{Data: membership})
	})

	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/audit-events", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if _, err := workspaceRepository.GetWorkspaceMembership(ctx, r.PathValue("workspaceID"), principal.Issuer, principal.Subject); err != nil {
			if errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
				writeError(w, r, http.StatusNotFound, "workspace_not_found", "Workspace was not found")
			} else {
				writeError(w, r, http.StatusBadRequest, "invalid_workspace_id", "Workspace identifier is invalid")
			}
			return
		}
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			switch raw {
			case "10":
				limit = 10
			case "25":
				limit = 25
			case "50":
				limit = 50
			case "100":
				limit = 100
			default:
				writeError(w, r, http.StatusBadRequest, "invalid_limit", "limit must be 10, 25, 50, or 100")
				return
			}
		}
		events, err := workspaceRepository.ListWorkspaceAuditEvents(ctx, r.PathValue("workspaceID"), limit)
		if err != nil {
			logger.Error("list workspace audit events failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workspace_audit_unavailable", "Workspace audit history is temporarily unavailable")
			return
		}
		if events == nil {
			events = []store.WorkspaceAuditEvent{}
		}
		writeJSON(w, http.StatusOK, workspaceAuditResponse{Data: events})
	})

	workspaceData, dataAvailable := repository.(store.WorkspaceDataRepository)
	registerScopedRead := func(pattern string, read func(http.ResponseWriter, *http.Request, context.Context, string)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			principal, ok := principalFromContext(r.Context())
			if !ok {
				unauthorized(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			workspaceID := r.PathValue("workspaceID")
			if _, err := workspaceRepository.GetWorkspaceMembership(ctx, workspaceID, principal.Issuer, principal.Subject); err != nil {
				if errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
					writeError(w, r, http.StatusNotFound, "workspace_not_found", "Workspace was not found")
				} else if strings.Contains(err.Error(), "UUID") {
					writeError(w, r, http.StatusBadRequest, "invalid_workspace_id", "Workspace identifier is invalid")
				} else {
					logger.Error("verify workspace membership failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
					writeError(w, r, http.StatusServiceUnavailable, "workspace_authorization_unavailable", "Workspace access could not be verified")
				}
				return
			}
			if !dataAvailable || workspaceData == nil {
				writeError(w, r, http.StatusServiceUnavailable, "workspace_data_unavailable", "Workspace-scoped data access is not available")
				return
			}
			read(w, r, ctx, workspaceID)
		})
	}

	registerScopedRead("GET /api/v1/workspaces/{workspaceID}/services", func(w http.ResponseWriter, r *http.Request, ctx context.Context, workspaceID string) {
		items, err := workspaceData.ListWorkspaceServices(ctx, workspaceID)
		if err != nil {
			logger.Error("list workspace services failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workspace_services_unavailable", "Workspace services are temporarily unavailable")
			return
		}
		writeJSON(w, http.StatusOK, servicesResponse{Data: items})
	})

	registerScopedRead("GET /api/v1/workspaces/{workspaceID}/incidents", func(w http.ResponseWriter, r *http.Request, ctx context.Context, workspaceID string) {
		status := r.URL.Query().Get("status")
		if !validIncidentStatus(status) {
			writeError(w, r, http.StatusBadRequest, "invalid_status", "status must be open, investigating, monitoring, or resolved")
			return
		}
		items, err := workspaceData.ListWorkspaceIncidents(ctx, workspaceID, status)
		if err != nil {
			logger.Error("list workspace incidents failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workspace_incidents_unavailable", "Workspace incidents are temporarily unavailable")
			return
		}
		writeJSON(w, http.StatusOK, incidentsResponse{Data: items})
	})

	// Keep incident lifecycle routes registered even when the repository
	// implementation does not expose the transactional mutation contract.
	incidentLifecycle, incidentLifecycleAvailable := repository.(store.IncidentLifecycleRepository)
	incidentUnavailable := func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusServiceUnavailable, "incident_lifecycle_unavailable", "Incident lifecycle storage is not available")
	}
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/incidents", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		if !incidentLifecycleAvailable || incidentLifecycle == nil {
			incidentUnavailable(w, r)
			return
		}
		var input incidentCreateRequest
		if err := decodeIncidentJSON(w, r, &input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_incident", "Incident fields must be a single valid JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		incident, err := incidentLifecycle.CreateWorkspaceIncident(ctx, store.CreateWorkspaceIncidentInput{
			WorkspaceID: r.PathValue("workspaceID"),
			Title: input.Title,
			Description: input.Description,
			ServiceID: input.ServiceID,
			Severity: input.Severity,
			ActorIssuer: principal.Issuer,
			ActorSubject: principal.Subject,
			RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil {
			incidentError(w, r, logger, "create workspace incident failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, incidentResponse{Data: incident})
	})

	mux.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/incidents/{incidentID}/status", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		if !incidentLifecycleAvailable || incidentLifecycle == nil {
			incidentUnavailable(w, r)
			return
		}
		var input incidentTransitionRequest
		if err := decodeIncidentJSON(w, r, &input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_incident_transition", "Status and optional comment must be a single valid JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		incident, err := incidentLifecycle.TransitionWorkspaceIncident(ctx, store.TransitionWorkspaceIncidentInput{
			WorkspaceID: r.PathValue("workspaceID"),
			IncidentID: r.PathValue("incidentID"),
			ToStatus: input.Status,
			Comment: input.Comment,
			ActorIssuer: principal.Issuer,
			ActorSubject: principal.Subject,
			RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil {
			incidentError(w, r, logger, "transition workspace incident failed", err)
			return
		}
		writeJSON(w, http.StatusOK, incidentResponse{Data: incident})
	})

	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/incidents/{incidentID}/timeline", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		if !incidentLifecycleAvailable || incidentLifecycle == nil {
			incidentUnavailable(w, r)
			return
		}
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			switch raw {
			case "10":
				limit = 10
			case "25":
				limit = 25
			case "50":
				limit = 50
			case "100":
				limit = 100
			default:
				writeError(w, r, http.StatusBadRequest, "invalid_limit", "limit must be 10, 25, 50, or 100")
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		events, err := incidentLifecycle.ListWorkspaceIncidentTimeline(ctx,
			r.PathValue("workspaceID"), r.PathValue("incidentID"),
			principal.Issuer, principal.Subject, limit)
		if err != nil {
			incidentError(w, r, logger, "list workspace incident timeline failed", err)
			return
		}
		if events == nil {
			events = []store.IncidentTimelineEvent{}
		}
		writeJSON(w, http.StatusOK, incidentTimelineResponse{Data: events})
	})

	registerScopedRead("GET /api/v1/workspaces/{workspaceID}/telemetry/services", func(w http.ResponseWriter, r *http.Request, ctx context.Context, workspaceID string) {
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			switch raw {
			case "10": limit = 10
			case "25": limit = 25
			case "50": limit = 50
			case "100": limit = 100
			default:
				writeError(w, r, http.StatusBadRequest, "invalid_limit", "limit must be 10, 25, 50, or 100")
				return
			}
		}
		items, err := workspaceData.ListWorkspaceTelemetryServices(ctx, workspaceID, limit)
		if err != nil {
			logger.Error("list workspace telemetry services failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workspace_telemetry_unavailable", "Workspace telemetry catalog is temporarily unavailable")
			return
		}
		writeJSON(w, http.StatusOK, struct { Data []store.TelemetryService `json:"data"` }{Data: items})
	})


	// Keep the routes registered when telemetry is unavailable so authorized
	// workspace members receive a dependency error rather than a misleading 404.
		registerScopedRead("GET /api/v1/workspaces/{workspaceID}/telemetry/metrics", func(w http.ResponseWriter, r *http.Request, ctx context.Context, workspaceID string) {
		if telemetryClient == nil {
			writeError(w, r, http.StatusServiceUnavailable, "telemetry_unavailable", "Workspace telemetry backends are not configured")
			return
		}
			window, err := telemetry.ParseWindow(r.URL.Query().Get("window"))
			if err != nil { writeError(w,r,http.StatusBadRequest,"invalid_window",err.Error()); return }
			service := r.URL.Query().Get("service")
			if err := telemetry.ValidateServiceName(service); err != nil { writeError(w,r,http.StatusBadRequest,"invalid_service",err.Error()); return }
			groupBy, err := telemetry.ParseMetricGroupBy(r.URL.Query().Get("group_by"))
			if err != nil { writeError(w,r,http.StatusBadRequest,"invalid_group_by",err.Error()); return }
			result, err := telemetryClient.WorkspaceMetrics(ctx,workspaceID,window,service,groupBy)
			if err != nil { writeTelemetryError(w,r,logger,"Workspace Prometheus",err); return }
			writeJSON(w,http.StatusOK,result)
		})
		registerScopedRead("GET /api/v1/workspaces/{workspaceID}/telemetry/logs", func(w http.ResponseWriter, r *http.Request, ctx context.Context, workspaceID string) {
		if telemetryClient == nil {
			writeError(w, r, http.StatusServiceUnavailable, "telemetry_unavailable", "Workspace telemetry backends are not configured")
			return
		}
			window, err := telemetry.ParseWindow(r.URL.Query().Get("window"))
			if err != nil { writeError(w,r,http.StatusBadRequest,"invalid_window",err.Error()); return }
			service := r.URL.Query().Get("service")
			if err := telemetry.ValidateServiceName(service); err != nil { writeError(w,r,http.StatusBadRequest,"invalid_service",err.Error()); return }
			limit, err := telemetry.ParseLimit(r.URL.Query().Get("limit"),50)
			if err != nil { writeError(w,r,http.StatusBadRequest,"invalid_limit",err.Error()); return }
			result, err := telemetryClient.WorkspaceLogs(ctx,workspaceID,window,service,limit)
			if err != nil { writeTelemetryError(w,r,logger,"Workspace Loki",err); return }
			writeJSON(w,http.StatusOK,result)
		})
		registerScopedRead("GET /api/v1/workspaces/{workspaceID}/telemetry/traces", func(w http.ResponseWriter, r *http.Request, ctx context.Context, workspaceID string) {
		if telemetryClient == nil {
			writeError(w, r, http.StatusServiceUnavailable, "telemetry_unavailable", "Workspace telemetry backends are not configured")
			return
		}
			window, err := telemetry.ParseWindow(r.URL.Query().Get("window"))
			if err != nil { writeError(w,r,http.StatusBadRequest,"invalid_window",err.Error()); return }
			service := r.URL.Query().Get("service")
			if err := telemetry.ValidateServiceName(service); err != nil { writeError(w,r,http.StatusBadRequest,"invalid_service",err.Error()); return }
			limit, err := telemetry.ParseLimit(r.URL.Query().Get("limit"),20)
			if err != nil { writeError(w,r,http.StatusBadRequest,"invalid_limit",err.Error()); return }
			result, err := telemetryClient.WorkspaceTraces(ctx,workspaceID,window,service,limit)
			if err != nil { writeTelemetryError(w,r,logger,"Workspace Tempo",err); return }
			writeJSON(w,http.StatusOK,result)
		})

}


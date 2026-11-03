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

	"github.com/pooya16pg-spec/orbit/server/internal/slo"
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

type sloCreateRequest struct {
	ServiceID string `json:"service_id"`
	Name string `json:"name"`
	Route string `json:"route"`
	Target float64 `json:"target"`
	RollingWindowDays int `json:"rolling_window_days"`
}

type sloUpdateRequest struct {
	Name string `json:"name"`
	Route string `json:"route"`
	Target float64 `json:"target"`
	RollingWindowDays int `json:"rolling_window_days"`
}

type sloEnabledRequest struct {
	Enabled *bool `json:"enabled"`
}

type sloResponse struct {
	Data store.ServiceLevelObjective `json:"data"`
}

type sloListResponse struct {
	Data []store.ServiceLevelObjective `json:"data"`
}

type workspaceSLOBudgetData struct {
	Objective store.ServiceLevelObjective `json:"objective"`
	SLI string `json:"sli"`
	ValidEventPolicy string `json:"valid_event_policy"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd time.Time `json:"window_end"`
	Budget slo.AvailabilityBudget `json:"budget"`
}

type workspaceSLOBudgetResponse struct {
	Data workspaceSLOBudgetData `json:"data"`
}

type deploymentAnnotationCreateRequest struct {
	ServiceID string `json:"service_id"`
	Version string `json:"version"`
	CommitSHA string `json:"commit_sha"`
	ChangeReference string `json:"change_reference"`
	ReleaseNotes string `json:"release_notes"`
	DeployedAt *time.Time `json:"deployed_at"`
}

type deploymentAnnotationResponse struct {
	Data store.DeploymentAnnotation `json:"data"`
}

type deploymentAnnotationListResponse struct {
	Data []store.DeploymentAnnotation `json:"data"`
}

type deploymentComparisonResponse struct {
	From store.DeploymentAnnotation `json:"from"`
	To store.DeploymentAnnotation `json:"to"`
	ElapsedSeconds int64 `json:"elapsed_seconds"`
	VersionChanged bool `json:"version_changed"`
	CommitChanged bool `json:"commit_changed"`
}

type alertRuleCreateRequest struct {
	Name string `json:"name"`
	Description string `json:"description"`
	ServiceName string `json:"service_name"`
	Metric string `json:"metric"`
	Operator string `json:"operator"`
	Threshold float64 `json:"threshold"`
	WindowSeconds int `json:"window_seconds"`
	PendingSeconds int `json:"pending_seconds"`
	Severity string `json:"severity"`
}

type alertRuleUpdateRequest = alertRuleCreateRequest

type alertRuleEnabledRequest struct {
	Enabled *bool `json:"enabled"`
}

type alertRuleResponse struct {
	Data store.AlertRule `json:"data"`
}

type alertRuleListResponse struct {
	Data []store.AlertRule `json:"data"`
}

type alertInstanceListResponse struct {
	Data []store.AlertInstance `json:"data"`
}

type alertNotificationChannelCreateRequest struct {
	Name string `json:"name"`
	SecretRef string `json:"secret_ref"`
}

type alertNotificationChannelEnabledRequest struct {
	Enabled *bool `json:"enabled"`
}

type alertNotificationChannelListResponse struct {
	Data []store.AlertNotificationChannel `json:"data"`
}

type alertNotificationDeliveryListResponse struct {
	Data []store.AlertNotificationDelivery `json:"data"`
}

type alertNotificationAttemptListResponse struct {
	Data []store.AlertNotificationAttempt `json:"data"`
}

func alertRuleError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, operation string, err error) {
	switch {
	case errors.Is(err, store.ErrWorkspaceMembershipNotFound),
		errors.Is(err, store.ErrAlertRuleNotFound):
		writeError(w, r, http.StatusNotFound, "alert_rule_not_found", "The workspace or alert rule was not found")
	case errors.Is(err, store.ErrAlertRuleForbidden):
		writeError(w, r, http.StatusForbidden, "alert_rule_forbidden", "Your workspace role cannot manage alert rules")
	case errors.Is(err, store.ErrAlertRuleNameConflict):
		writeError(w, r, http.StatusConflict, "alert_rule_name_conflict", "An alert rule with that name already exists in this workspace")
	case errors.Is(err, store.ErrAlertRuleNoChange):
		writeError(w, r, http.StatusConflict, "alert_rule_no_change", "The alert rule already has the requested state")
	case errors.Is(err, store.ErrAlertRuleInvalidInput),
		strings.Contains(err.Error(), "UUID"),
		strings.Contains(err.Error(), "principal"):
		writeError(w, r, http.StatusBadRequest, "invalid_alert_rule", "Alert rule fields or identifiers are invalid")
	default:
		logger.Error(operation, "error", err, "request_id", r.Header.Get("X-Request-ID"))
		writeError(w, r, http.StatusServiceUnavailable, "alert_rule_operation_unavailable", "The alert rule operation is temporarily unavailable")
	}
}

func sloError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, operation string, err error) {
	switch {
	case errors.Is(err, store.ErrWorkspaceMembershipNotFound),
		errors.Is(err, store.ErrSLONotFound),
		errors.Is(err, store.ErrSLOServiceNotFound):
		writeError(w, r, http.StatusNotFound, "slo_not_found", "The workspace, service, or SLO was not found")
	case errors.Is(err, store.ErrSLOForbidden):
		writeError(w, r, http.StatusForbidden, "slo_forbidden", "Your workspace role cannot manage SLO definitions")
	case errors.Is(err, store.ErrSLONameConflict):
		writeError(w, r, http.StatusConflict, "slo_name_conflict", "An SLO with that name already exists for this service")
	case errors.Is(err, store.ErrSLONoChange):
		writeError(w, r, http.StatusConflict, "slo_no_change", "The SLO already has the requested state")
	case errors.Is(err, store.ErrSLODisabled):
		writeError(w, r, http.StatusConflict, "slo_disabled", "Enable the SLO before requesting its error budget")
	case errors.Is(err, store.ErrSLOInvalidInput),
		strings.Contains(err.Error(), "UUID"),
		strings.Contains(err.Error(), "principal"):
		writeError(w, r, http.StatusBadRequest, "invalid_slo", "SLO fields or identifiers are invalid")
	default:
		logger.Error(operation, "error", err, "request_id", r.Header.Get("X-Request-ID"))
		writeError(w, r, http.StatusServiceUnavailable, "slo_unavailable", "SLO storage or telemetry is temporarily unavailable")
	}
}

func deploymentAnnotationError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, operation string, err error) {
	switch {
	case errors.Is(err, store.ErrWorkspaceMembershipNotFound),
		errors.Is(err, store.ErrDeploymentAnnotationNotFound),
		errors.Is(err, store.ErrDeploymentAnnotationServiceNotFound):
		writeError(w, r, http.StatusNotFound, "deployment_annotation_not_found", "The workspace, service, or deployment annotation was not found")
	case errors.Is(err, store.ErrDeploymentAnnotationForbidden):
		writeError(w, r, http.StatusForbidden, "deployment_annotation_forbidden", "Your workspace role cannot create deployment annotations")
	case errors.Is(err, store.ErrDeploymentAnnotationComparisonInvalid):
		writeError(w, r, http.StatusBadRequest, "invalid_deployment_comparison", "Compare two different deployments of the same service in chronological order")
	case errors.Is(err, store.ErrDeploymentAnnotationInvalidInput),
		strings.Contains(err.Error(), "UUID"),
		strings.Contains(err.Error(), "principal"):
		writeError(w, r, http.StatusBadRequest, "invalid_deployment_annotation", "Deployment annotation fields or identifiers are invalid")
	default:
		logger.Error(operation, "error", err, "request_id", r.Header.Get("X-Request-ID"))
		writeError(w, r, http.StatusServiceUnavailable, "deployment_annotation_unavailable", "Deployment history is temporarily unavailable")
	}
}

func alertNotificationError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, operation string, err error) {
	switch {
	case errors.Is(err, store.ErrWorkspaceMembershipNotFound),
		errors.Is(err, store.ErrAlertNotificationChannelNotFound):
		writeError(w, r, http.StatusNotFound, "alert_notification_resource_not_found", "The workspace or notification channel was not found")
	case errors.Is(err, store.ErrAlertRuleForbidden):
		writeError(w, r, http.StatusForbidden, "alert_notification_forbidden", "Your workspace role cannot manage notification channels")
	case errors.Is(err, store.ErrAlertNotificationChannelConflict):
		writeError(w, r, http.StatusConflict, "alert_notification_channel_conflict", "A notification channel with that name already exists in this workspace")
	case errors.Is(err, store.ErrAlertNotificationNoChange):
		writeError(w, r, http.StatusConflict, "alert_notification_no_change", "The notification channel already has the requested state")
	case errors.Is(err, store.ErrAlertNotificationInvalidInput),
		strings.Contains(err.Error(), "UUID"),
		strings.Contains(err.Error(), "principal"):
		writeError(w, r, http.StatusBadRequest, "invalid_alert_notification", "Notification channel fields or identifiers are invalid")
	default:
		logger.Error(operation, "error", err, "request_id", r.Header.Get("X-Request-ID"))
		writeError(w, r, http.StatusServiceUnavailable, "alert_notification_operation_unavailable", "The alert notification operation is temporarily unavailable")
	}
}

func configuredWebhookSecretRef(value string, configured []string) bool {
	for _, item := range configured {
		if value == item {
			return true
		}
	}
	return false
}

func parseAlertRuleLimit(r *http.Request) (int, error) {
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
			return 0, errors.New("limit must be 10, 25, 50, or 100")
		}
	}
	return limit, nil
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

func registerWorkspaceRoutes(mux *http.ServeMux, repository store.Repository, telemetryClient *telemetry.Client, webhookSecretRefs []string, logger *slog.Logger) {
	workspaceRepository, ok := repository.(store.WorkspaceRepository)
	if !ok || workspaceRepository == nil {
		unavailable := func(w http.ResponseWriter, r *http.Request) {
			writeError(w, r, http.StatusServiceUnavailable, "workspace_store_unavailable", "Workspace management is not available")
		}
		mux.HandleFunc("GET /api/v1/workspaces", unavailable)
		mux.HandleFunc("POST /api/v1/workspaces", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/membership", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/audit-events", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/alert-notification-channels", unavailable)
		mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/alert-notification-channels", unavailable)
		mux.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/alert-notification-channels/{channelID}/enabled", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/alert-notification-deliveries", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/alert-notification-deliveries/{deliveryID}/attempts", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/deployments", unavailable)
		mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/deployments", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/deployments/compare", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/deployments/{deploymentID}", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/slos", unavailable)
		mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/slos", unavailable)
		mux.HandleFunc("PUT /api/v1/workspaces/{workspaceID}/slos/{sloID}", unavailable)
		mux.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/slos/{sloID}/enabled", unavailable)
		mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/slos/{sloID}", unavailable)
		mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/slos/{sloID}/budget", unavailable)
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

	// Workspace SLOs are persisted definitions. Their live budget is computed
	// from fixed, workspace-scoped Prometheus selectors, never caller-supplied PromQL.
	sloRepository, sloRepositoryAvailable := repository.(store.WorkspaceSLORepository)
	sloUnavailable := func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusServiceUnavailable, "slo_store_unavailable", "SLO storage is not available")
	}
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/slos", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !sloRepositoryAvailable || sloRepository == nil { sloUnavailable(w, r); return }
		limit, err := parseAlertRuleLimit(r)
		if err != nil { writeError(w, r, http.StatusBadRequest, "invalid_limit", err.Error()); return }
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		items, err := sloRepository.ListWorkspaceSLOs(ctx,
			r.PathValue("workspaceID"), principal.Issuer, principal.Subject,
			r.URL.Query().Get("service_id"), limit)
		if err != nil { sloError(w, r, logger, "list workspace SLOs failed", err); return }
		if items == nil { items = []store.ServiceLevelObjective{} }
		writeJSON(w, http.StatusOK, sloListResponse{Data: items})
	})

	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/slos", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !sloRepositoryAvailable || sloRepository == nil { sloUnavailable(w, r); return }
		var input sloCreateRequest
		if err := decodeIncidentJSON(w, r, &input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_slo", "SLO fields must be a single valid JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		item, err := sloRepository.CreateWorkspaceSLO(ctx, store.CreateWorkspaceSLOInput{
			WorkspaceID: r.PathValue("workspaceID"),
			ServiceID: input.ServiceID,
			Name: input.Name,
			Route: input.Route,
			Target: input.Target,
			RollingWindowDays: input.RollingWindowDays,
			ActorIssuer: principal.Issuer,
			ActorSubject: principal.Subject,
			RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil { sloError(w, r, logger, "create workspace SLO failed", err); return }
		writeJSON(w, http.StatusCreated, sloResponse{Data: item})
	})

	mux.HandleFunc("PUT /api/v1/workspaces/{workspaceID}/slos/{sloID}", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !sloRepositoryAvailable || sloRepository == nil { sloUnavailable(w, r); return }
		var input sloUpdateRequest
		if err := decodeIncidentJSON(w, r, &input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_slo", "SLO fields must be a single valid JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		item, err := sloRepository.UpdateWorkspaceSLO(ctx, store.UpdateWorkspaceSLOInput{
			WorkspaceID: r.PathValue("workspaceID"),
			SLOID: r.PathValue("sloID"),
			Name: input.Name,
			Route: input.Route,
			Target: input.Target,
			RollingWindowDays: input.RollingWindowDays,
			ActorIssuer: principal.Issuer,
			ActorSubject: principal.Subject,
			RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil { sloError(w, r, logger, "update workspace SLO failed", err); return }
		writeJSON(w, http.StatusOK, sloResponse{Data: item})
	})

	mux.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/slos/{sloID}/enabled", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !sloRepositoryAvailable || sloRepository == nil { sloUnavailable(w, r); return }
		var input sloEnabledRequest
		if err := decodeIncidentJSON(w, r, &input); err != nil || input.Enabled == nil {
			writeError(w, r, http.StatusBadRequest, "invalid_slo_state", "enabled must be provided in one valid JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		item, err := sloRepository.SetWorkspaceSLOEnabled(ctx, store.SetWorkspaceSLOEnabledInput{
			WorkspaceID: r.PathValue("workspaceID"),
			SLOID: r.PathValue("sloID"),
			Enabled: *input.Enabled,
			ActorIssuer: principal.Issuer,
			ActorSubject: principal.Subject,
			RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil { sloError(w, r, logger, "change workspace SLO enabled state failed", err); return }
		writeJSON(w, http.StatusOK, sloResponse{Data: item})
	})

	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/slos/{sloID}", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !sloRepositoryAvailable || sloRepository == nil { sloUnavailable(w, r); return }
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		err := sloRepository.DeleteWorkspaceSLO(ctx, store.DeleteWorkspaceSLOInput{
			WorkspaceID: r.PathValue("workspaceID"),
			SLOID: r.PathValue("sloID"),
			ActorIssuer: principal.Issuer,
			ActorSubject: principal.Subject,
			RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil { sloError(w, r, logger, "delete workspace SLO failed", err); return }
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/slos/{sloID}/budget", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !sloRepositoryAvailable || sloRepository == nil { sloUnavailable(w, r); return }
		if telemetryClient == nil {
			writeError(w, r, http.StatusServiceUnavailable, "slo_telemetry_unavailable", "Workspace SLO telemetry is not configured")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 9*time.Second)
		defer cancel()
		objective, err := sloRepository.GetWorkspaceSLO(ctx,
			r.PathValue("workspaceID"), r.PathValue("sloID"), principal.Issuer, principal.Subject)
		if err != nil { sloError(w, r, logger, "get workspace SLO for budget failed", err); return }
		if !objective.Enabled {
			sloError(w, r, logger, "request budget for disabled SLO", store.ErrSLODisabled)
			return
		}
		events, err := telemetryClient.WorkspaceAvailabilityEvents(ctx,
			objective.WorkspaceID, objective.ServiceName, objective.Route, objective.RollingWindowDays)
		if err != nil { sloError(w, r, logger, "query workspace SLO availability events failed", err); return }
		budget, err := slo.CalculateAvailabilityBudget(objective.Target, events.GoodEvents, events.BadEvents)
		if err != nil {
			logger.Error("calculate workspace SLO budget failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "slo_calculation_unavailable", "The SLO budget could not be calculated safely")
			return
		}
		writeJSON(w, http.StatusOK, workspaceSLOBudgetResponse{Data: workspaceSLOBudgetData{
			Objective: objective,
			SLI: "http_request_success_rate",
			ValidEventPolicy: "Only the configured route is counted. HTTP 2xx and 3xx responses are good; all other instrumented HTTP responses are bad. Uninstrumented routes and /healthz are excluded.",
			WindowStart: events.WindowStart,
			WindowEnd: events.WindowEnd,
			Budget: budget,
		}})
	})

	// Deployment annotations are immutable release markers. Creation records
	// the authenticated actor and an audit row in the same transaction.
	deploymentRepository, deploymentRepositoryAvailable := repository.(store.DeploymentAnnotationRepository)
	deploymentUnavailable := func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusServiceUnavailable, "deployment_annotation_store_unavailable", "Deployment annotation storage is not available")
	}
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/deployments", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !deploymentRepositoryAvailable || deploymentRepository == nil { deploymentUnavailable(w, r); return }
		limit, err := parseAlertRuleLimit(r)
		if err != nil { writeError(w, r, http.StatusBadRequest, "invalid_limit", err.Error()); return }
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		items, err := deploymentRepository.ListWorkspaceDeploymentAnnotations(ctx,
			r.PathValue("workspaceID"), principal.Issuer, principal.Subject,
			r.URL.Query().Get("service_id"), limit)
		if err != nil { deploymentAnnotationError(w, r, logger, "list workspace deployment annotations failed", err); return }
		if items == nil { items = []store.DeploymentAnnotation{} }
		writeJSON(w, http.StatusOK, deploymentAnnotationListResponse{Data: items})
	})

	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/deployments", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !deploymentRepositoryAvailable || deploymentRepository == nil { deploymentUnavailable(w, r); return }
		var input deploymentAnnotationCreateRequest
		if err := decodeIncidentJSON(w, r, &input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_deployment_annotation", "Deployment annotation fields must be a single valid JSON object")
			return
		}
		deployedAt := time.Time{}
		if input.DeployedAt != nil { deployedAt = *input.DeployedAt }
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		item, err := deploymentRepository.CreateWorkspaceDeploymentAnnotation(ctx, store.CreateWorkspaceDeploymentAnnotationInput{
			WorkspaceID: r.PathValue("workspaceID"),
			ServiceID: input.ServiceID,
			Version: input.Version,
			CommitSHA: input.CommitSHA,
			ChangeReference: input.ChangeReference,
			ReleaseNotes: input.ReleaseNotes,
			DeployedAt: deployedAt,
			ActorIssuer: principal.Issuer,
			ActorSubject: principal.Subject,
			RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil { deploymentAnnotationError(w, r, logger, "create workspace deployment annotation failed", err); return }
		writeJSON(w, http.StatusCreated, deploymentAnnotationResponse{Data: item})
	})

	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/deployments/{deploymentID}", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !deploymentRepositoryAvailable || deploymentRepository == nil { deploymentUnavailable(w, r); return }
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		item, err := deploymentRepository.GetWorkspaceDeploymentAnnotation(ctx,
			r.PathValue("workspaceID"), r.PathValue("deploymentID"), principal.Issuer, principal.Subject)
		if err != nil { deploymentAnnotationError(w, r, logger, "get workspace deployment annotation failed", err); return }
		writeJSON(w, http.StatusOK, deploymentAnnotationResponse{Data: item})
	})

	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/deployments/compare", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !deploymentRepositoryAvailable || deploymentRepository == nil { deploymentUnavailable(w, r); return }
		fromID, toID := r.URL.Query().Get("from_id"), r.URL.Query().Get("to_id")
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		from, err := deploymentRepository.GetWorkspaceDeploymentAnnotation(ctx,
			r.PathValue("workspaceID"), fromID, principal.Issuer, principal.Subject)
		if err != nil { deploymentAnnotationError(w, r, logger, "read source deployment annotation for comparison failed", err); return }
		to, err := deploymentRepository.GetWorkspaceDeploymentAnnotation(ctx,
			r.PathValue("workspaceID"), toID, principal.Issuer, principal.Subject)
		if err != nil { deploymentAnnotationError(w, r, logger, "read target deployment annotation for comparison failed", err); return }
		if from.ID == to.ID || from.ServiceID != to.ServiceID ||
			from.Environment != to.Environment || !to.DeployedAt.After(from.DeployedAt) {
			deploymentAnnotationError(w, r, logger, "invalid deployment comparison", store.ErrDeploymentAnnotationComparisonInvalid)
			return
		}
		writeJSON(w, http.StatusOK, deploymentComparisonResponse{
			From: from, To: to,
			ElapsedSeconds: int64(to.DeployedAt.Sub(from.DeployedAt).Seconds()),
			VersionChanged: from.Version != to.Version,
			CommitChanged: from.CommitSHA != to.CommitSHA,
		})
	})

	// Alert rules are workspace-owned definitions. Their evaluator and notification
	// adapters are separate components; these routes only manage persisted rules.
	alertRuleRepository, alertRuleRepositoryAvailable := repository.(store.AlertRuleRepository)
	alertRuleUnavailable := func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusServiceUnavailable, "alert_rule_store_unavailable", "Alert rule storage is not available")
	}
	evaluationRepository, evaluationRepositoryAvailable := repository.(store.AlertEvaluationRepository)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/alerts", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		if !evaluationRepositoryAvailable || evaluationRepository == nil {
			writeError(w, r, http.StatusServiceUnavailable, "alert_evaluator_unavailable", "Alert evaluation storage is not available")
			return
		}
		limit, err := parseAlertRuleLimit(r)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_limit", err.Error())
			return
		}
		status := r.URL.Query().Get("status")
		switch status {
		case "", "pending", "firing", "resolved", "cancelled":
		default:
			writeError(w, r, http.StatusBadRequest, "invalid_alert_status", "status must be pending, firing, resolved, or cancelled")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		items, err := evaluationRepository.ListWorkspaceAlertInstances(ctx,
			r.PathValue("workspaceID"), principal.Issuer, principal.Subject, status, limit)
		if errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
			writeError(w, r, http.StatusNotFound, "workspace_not_found", "Workspace was not found")
			return
		}
		if errors.Is(err, store.ErrAlertRuleInvalidInput) {
			writeError(w, r, http.StatusBadRequest, "invalid_alert_filter", "Workspace alert filter is invalid")
			return
		}
		if err != nil {
			logger.Error("list workspace alert instances failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workspace_alerts_unavailable", "Workspace alerts are temporarily unavailable")
			return
		}
		if items == nil {
			items = []store.AlertInstance{}
		}
		writeJSON(w, http.StatusOK, alertInstanceListResponse{Data: items})
	})

	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/alert-rules", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		if !alertRuleRepositoryAvailable || alertRuleRepository == nil {
			alertRuleUnavailable(w, r)
			return
		}
		limit, err := parseAlertRuleLimit(r)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_limit", err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		items, err := alertRuleRepository.ListWorkspaceAlertRules(ctx,
			r.PathValue("workspaceID"), principal.Issuer, principal.Subject, limit)
		if err != nil {
			alertRuleError(w, r, logger, "list workspace alert rules failed", err)
			return
		}
		if items == nil {
			items = []store.AlertRule{}
		}
		writeJSON(w, http.StatusOK, alertRuleListResponse{Data: items})
	})

	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/alert-rules", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		if !alertRuleRepositoryAvailable || alertRuleRepository == nil {
			alertRuleUnavailable(w, r)
			return
		}
		var input alertRuleCreateRequest
		if err := decodeIncidentJSON(w, r, &input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_alert_rule", "Alert rule fields must be a single valid JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rule, err := alertRuleRepository.CreateWorkspaceAlertRule(ctx, store.CreateWorkspaceAlertRuleInput{
			WorkspaceID: r.PathValue("workspaceID"),
			Name: input.Name,
			Description: input.Description,
			ServiceName: input.ServiceName,
			Metric: input.Metric,
			Operator: input.Operator,
			Threshold: input.Threshold,
			WindowSeconds: input.WindowSeconds,
			PendingSeconds: input.PendingSeconds,
			Severity: input.Severity,
			ActorIssuer: principal.Issuer,
			ActorSubject: principal.Subject,
			RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil {
			alertRuleError(w, r, logger, "create workspace alert rule failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, alertRuleResponse{Data: rule})
	})

	mux.HandleFunc("PUT /api/v1/workspaces/{workspaceID}/alert-rules/{ruleID}", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		if !alertRuleRepositoryAvailable || alertRuleRepository == nil {
			alertRuleUnavailable(w, r)
			return
		}
		var input alertRuleUpdateRequest
		if err := decodeIncidentJSON(w, r, &input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_alert_rule", "Alert rule fields must be a single valid JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rule, err := alertRuleRepository.UpdateWorkspaceAlertRule(ctx, store.UpdateWorkspaceAlertRuleInput{
			WorkspaceID: r.PathValue("workspaceID"),
			RuleID: r.PathValue("ruleID"),
			Name: input.Name,
			Description: input.Description,
			ServiceName: input.ServiceName,
			Metric: input.Metric,
			Operator: input.Operator,
			Threshold: input.Threshold,
			WindowSeconds: input.WindowSeconds,
			PendingSeconds: input.PendingSeconds,
			Severity: input.Severity,
			ActorIssuer: principal.Issuer,
			ActorSubject: principal.Subject,
			RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil {
			alertRuleError(w, r, logger, "update workspace alert rule failed", err)
			return
		}
		writeJSON(w, http.StatusOK, alertRuleResponse{Data: rule})
	})

	mux.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/alert-rules/{ruleID}/enabled", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		if !alertRuleRepositoryAvailable || alertRuleRepository == nil {
			alertRuleUnavailable(w, r)
			return
		}
		var input alertRuleEnabledRequest
		if err := decodeIncidentJSON(w, r, &input); err != nil || input.Enabled == nil {
			writeError(w, r, http.StatusBadRequest, "invalid_alert_rule_state", "enabled must be provided in a single valid JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rule, err := alertRuleRepository.SetWorkspaceAlertRuleEnabled(ctx, store.SetWorkspaceAlertRuleEnabledInput{
			WorkspaceID: r.PathValue("workspaceID"),
			RuleID: r.PathValue("ruleID"),
			Enabled: *input.Enabled,
			ActorIssuer: principal.Issuer,
			ActorSubject: principal.Subject,
			RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil {
			alertRuleError(w, r, logger, "change workspace alert rule enabled state failed", err)
			return
		}
		writeJSON(w, http.StatusOK, alertRuleResponse{Data: rule})
	})

	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/alert-rules/{ruleID}", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		if !alertRuleRepositoryAvailable || alertRuleRepository == nil {
			alertRuleUnavailable(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		err := alertRuleRepository.DeleteWorkspaceAlertRule(ctx, store.DeleteWorkspaceAlertRuleInput{
			WorkspaceID: r.PathValue("workspaceID"),
			RuleID: r.PathValue("ruleID"),
			ActorIssuer: principal.Issuer,
			ActorSubject: principal.Subject,
			RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil {
			alertRuleError(w, r, logger, "delete workspace alert rule failed", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Notification channels store only an environment-backed secret reference.
	// The endpoint URL and token are never accepted from or returned to the browser.
	notificationRepository, notificationRepositoryAvailable := repository.(store.AlertNotificationChannelRepository)
	notificationUnavailable := func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusServiceUnavailable, "alert_notification_store_unavailable", "Alert notification storage is not available")
	}
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/alert-notification-channels", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !notificationRepositoryAvailable || notificationRepository == nil { notificationUnavailable(w, r); return }
		limit, err := parseAlertRuleLimit(r)
		if err != nil { writeError(w, r, http.StatusBadRequest, "invalid_limit", err.Error()); return }
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		items, err := notificationRepository.ListWorkspaceAlertNotificationChannels(ctx,
			r.PathValue("workspaceID"), principal.Issuer, principal.Subject, limit)
		if err != nil { alertNotificationError(w, r, logger, "list workspace alert-notification channels failed", err); return }
		if items == nil { items = []store.AlertNotificationChannel{} }
		for index := range items {
			items[index].SecretConfigured = configuredWebhookSecretRef(items[index].SecretRef, webhookSecretRefs)
		}
		writeJSON(w, http.StatusOK, alertNotificationChannelListResponse{Data: items})
	})

	mux.HandleFunc("POST /api/v1/workspaces/{workspaceID}/alert-notification-channels", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !notificationRepositoryAvailable || notificationRepository == nil { notificationUnavailable(w, r); return }
		var input alertNotificationChannelCreateRequest
		if err := decodeIncidentJSON(w, r, &input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_alert_notification_channel", "Channel name and secret_ref must be provided in one valid JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		membership, err := workspaceRepository.GetWorkspaceMembership(ctx,
			r.PathValue("workspaceID"), principal.Issuer, principal.Subject)
		if err != nil {
			alertNotificationError(w, r, logger, "verify notification-channel workspace membership failed", err)
			return
		}
		switch membership.Role {
		case "owner", "administrator", "operator":
		default:
			alertNotificationError(w, r, logger, "notification-channel mutation forbidden", store.ErrAlertRuleForbidden)
			return
		}
		if !configuredWebhookSecretRef(strings.TrimSpace(input.SecretRef), webhookSecretRefs) {
			writeError(w, r, http.StatusServiceUnavailable, "webhook_secret_not_configured", "The referenced webhook secret is not configured in this deployment")
			return
		}
		channel, err := notificationRepository.CreateWorkspaceAlertNotificationChannel(ctx, store.CreateAlertNotificationChannelInput{
			WorkspaceID: r.PathValue("workspaceID"), Name: input.Name, SecretRef: input.SecretRef,
			ActorIssuer: principal.Issuer, ActorSubject: principal.Subject, RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil { alertNotificationError(w, r, logger, "create workspace alert-notification channel failed", err); return }
		channel.SecretConfigured = true
		writeJSON(w, http.StatusCreated, struct { Data store.AlertNotificationChannel `json:"data"` }{Data: channel})
	})

	mux.HandleFunc("PATCH /api/v1/workspaces/{workspaceID}/alert-notification-channels/{channelID}/enabled", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !notificationRepositoryAvailable || notificationRepository == nil { notificationUnavailable(w, r); return }
		var input alertNotificationChannelEnabledRequest
		if err := decodeIncidentJSON(w, r, &input); err != nil || input.Enabled == nil {
			writeError(w, r, http.StatusBadRequest, "invalid_alert_notification_channel_state", "enabled must be provided in one valid JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		channel, err := notificationRepository.SetWorkspaceAlertNotificationChannelEnabled(ctx, store.SetAlertNotificationChannelEnabledInput{
			WorkspaceID: r.PathValue("workspaceID"), ChannelID: r.PathValue("channelID"), Enabled: *input.Enabled,
			ActorIssuer: principal.Issuer, ActorSubject: principal.Subject, RequestID: r.Header.Get("X-Request-ID"),
		})
		if err != nil { alertNotificationError(w, r, logger, "change workspace alert-notification channel state failed", err); return }
		channel.SecretConfigured = configuredWebhookSecretRef(channel.SecretRef, webhookSecretRefs)
		writeJSON(w, http.StatusOK, struct { Data store.AlertNotificationChannel `json:"data"` }{Data: channel})
	})

	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/alert-notification-deliveries", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !notificationRepositoryAvailable || notificationRepository == nil { notificationUnavailable(w, r); return }
		limit, err := parseAlertRuleLimit(r)
		if err != nil { writeError(w, r, http.StatusBadRequest, "invalid_limit", err.Error()); return }
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		items, err := notificationRepository.ListWorkspaceAlertNotificationDeliveries(ctx,
			r.PathValue("workspaceID"), principal.Issuer, principal.Subject, r.URL.Query().Get("status"), limit)
		if err != nil { alertNotificationError(w, r, logger, "list workspace alert-notification deliveries failed", err); return }
		if items == nil { items = []store.AlertNotificationDelivery{} }
		writeJSON(w, http.StatusOK, alertNotificationDeliveryListResponse{Data: items})
	})

	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/alert-notification-deliveries/{deliveryID}/attempts", func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok { unauthorized(w, r); return }
		if !notificationRepositoryAvailable || notificationRepository == nil { notificationUnavailable(w, r); return }
		limit, err := parseAlertRuleLimit(r)
		if err != nil { writeError(w, r, http.StatusBadRequest, "invalid_limit", err.Error()); return }
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		items, err := notificationRepository.ListWorkspaceAlertNotificationAttempts(ctx,
			r.PathValue("workspaceID"), r.PathValue("deliveryID"), principal.Issuer, principal.Subject, limit)
		if err != nil { alertNotificationError(w, r, logger, "list workspace alert-notification attempts failed", err); return }
		if items == nil { items = []store.AlertNotificationAttempt{} }
		writeJSON(w, http.StatusOK, alertNotificationAttemptListResponse{Data: items})
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
		registerScopedRead("GET /api/v1/workspaces/{workspaceID}/telemetry/metrics/catalog", func(w http.ResponseWriter, r *http.Request, ctx context.Context, workspaceID string) {
			writeJSON(w, http.StatusOK, telemetry.ListMetricCatalog())
		})
		registerScopedRead("GET /api/v1/workspaces/{workspaceID}/telemetry/metrics", func(w http.ResponseWriter, r *http.Request, ctx context.Context, workspaceID string) {
		if telemetryClient == nil {
			writeError(w, r, http.StatusServiceUnavailable, "telemetry_unavailable", "Workspace telemetry backends are not configured")
			return
		}
			window, err := telemetry.ParseWindow(r.URL.Query().Get("window"))
			if err != nil { writeError(w,r,http.StatusBadRequest,"invalid_window",err.Error()); return }
			service := r.URL.Query().Get("service")
			if err := telemetry.ValidateServiceName(service); err != nil { writeError(w,r,http.StatusBadRequest,"invalid_service",err.Error()); return }
			metricID, err := telemetry.ParseMetricID(r.URL.Query().Get("metric"))
			if err != nil { writeError(w,r,http.StatusBadRequest,"invalid_metric",err.Error()); return }
			groupBy, err := telemetry.ParseMetricGroupBy(r.URL.Query().Get("group_by"))
			if err != nil { writeError(w,r,http.StatusBadRequest,"invalid_group_by",err.Error()); return }
			result, err := telemetryClient.WorkspaceMetrics(ctx,workspaceID,window,service,metricID,groupBy)
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
		registerScopedRead("GET /api/v1/workspaces/{workspaceID}/telemetry/traces/{traceID}", func(w http.ResponseWriter, r *http.Request, ctx context.Context, workspaceID string) {
			if telemetryClient == nil {
				writeError(w, r, http.StatusServiceUnavailable, "telemetry_unavailable", "Workspace telemetry backends are not configured")
				return
			}
			result, err := telemetryClient.WorkspaceTrace(ctx, workspaceID, r.PathValue("traceID"))
			if err != nil {
				writeTelemetryError(w, r, logger, "Workspace Tempo", err)
				return
			}
			writeJSON(w, http.StatusOK, result)
		})

}


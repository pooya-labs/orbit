package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type workloadIdentityCreateRequest struct {
	Issuer string `json:"issuer"`
	Subject string `json:"subject"`
	DisplayName string `json:"display_name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type workloadIdentityStatusRequest struct {
	Status string `json:"status"`
}

type workloadIdentityAdminView struct {
	ID string `json:"id"`
	Issuer string `json:"issuer"`
	Subject string `json:"subject"`
	DisplayName string `json:"display_name"`
	Status string `json:"status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type workloadGrantAdminView struct {
	IdentityID string `json:"identity_id"`
	Issuer string `json:"issuer"`
	Subject string `json:"subject"`
	DisplayName string `json:"display_name"`
	IdentityStatus string `json:"identity_status"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	WorkspaceID string `json:"workspace_id"`
	Signal string `json:"signal"`
	GrantedAt time.Time `json:"granted_at"`
}

type workloadIdentityResponse struct {
	Data workloadIdentityAdminView `json:"data"`
}

type workloadIdentityListResponse struct {
	Data []workloadIdentityAdminView `json:"data"`
}

type workloadGrantListResponse struct {
	Data []workloadGrantAdminView `json:"data"`
}

type workloadGrantResponse struct {
	Data workloadGrantAdminView `json:"data"`
}

func mapWorkloadIdentityAdminView(item store.WorkloadIdentity) workloadIdentityAdminView {
	return workloadIdentityAdminView{
		ID: item.ID, Issuer: item.Issuer, Subject: item.Subject,
		DisplayName: item.DisplayName, Status: item.Status,
		ExpiresAt: item.ExpiresAt, CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt, RevokedAt: item.RevokedAt,
	}
}

func mapWorkloadGrantAdminView(item store.WorkloadIdentityGrant) workloadGrantAdminView {
	return workloadGrantAdminView{
		IdentityID: item.IdentityID, Issuer: item.Issuer, Subject: item.Subject,
		DisplayName: item.DisplayName, IdentityStatus: item.Status,
		ExpiresAt: item.ExpiresAt, WorkspaceID: item.WorkspaceID,
		Signal: item.Signal, GrantedAt: item.GrantedAt,
	}
}

func decodeWorkloadJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain exactly one JSON object")
	}
	return nil
}

func registerWorkloadIdentityRoutes(mux *http.ServeMux, repository store.Repository, logger *slog.Logger) {
	workloadRepository, workloadAvailable := repository.(store.WorkloadIdentityRepository)
	workspaceRepository, workspaceAvailable := repository.(store.WorkspaceRepository)
	unavailable := func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusServiceUnavailable, "workload_identity_store_unavailable", "Workload identity management is not available")
	}

	mux.HandleFunc("POST /api/v1/admin/workload-identities", func(w http.ResponseWriter, r *http.Request) {
		if !workloadAvailable || workloadRepository == nil {
			unavailable(w, r)
			return
		}
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		var input workloadIdentityCreateRequest
		if err := decodeWorkloadJSON(w, r, &input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_workload_identity", "A single valid workload identity JSON object is required")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		identity, err := workloadRepository.CreateWorkloadIdentity(ctx, store.CreateWorkloadIdentityInput{
			Issuer: input.Issuer, Subject: input.Subject, DisplayName: input.DisplayName,
			ExpiresAt: input.ExpiresAt, ActorIssuer: principal.Issuer, ActorSubject: principal.Subject,
		})
		if err != nil {
			message := strings.ToLower(err.Error())
			if strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint") {
				writeError(w, r, http.StatusConflict, "workload_identity_conflict", "That workload issuer and subject are already registered")
				return
			}
			if strings.Contains(message, "invalid") || strings.Contains(message, "must be") {
				writeError(w, r, http.StatusBadRequest, "invalid_workload_identity", "Workload identity fields are invalid")
				return
			}
			logger.Error("create workload identity failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workload_identity_create_failed", "Workload identity could not be created")
			return
		}
		writeJSON(w, http.StatusCreated, workloadIdentityResponse{Data: mapWorkloadIdentityAdminView(identity)})
	})

	mux.HandleFunc("GET /api/v1/admin/workload-identities", func(w http.ResponseWriter, r *http.Request) {
		if !workloadAvailable || workloadRepository == nil {
			unavailable(w, r)
			return
		}
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 100 {
				writeError(w, r, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
				return
			}
			limit = parsed
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		items, err := workloadRepository.ListWorkloadIdentities(ctx, limit)
		if err != nil {
			logger.Error("list workload identities failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workload_identity_list_failed", "Workload identities are temporarily unavailable")
			return
		}
		data := make([]workloadIdentityAdminView, 0, len(items))
		for _, item := range items {
			data = append(data, mapWorkloadIdentityAdminView(item))
		}
		writeJSON(w, http.StatusOK, workloadIdentityListResponse{Data: data})
	})

	mux.HandleFunc("PATCH /api/v1/admin/workload-identities/{identityID}/status", func(w http.ResponseWriter, r *http.Request) {
		if !workloadAvailable || workloadRepository == nil {
			unavailable(w, r)
			return
		}
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		var input workloadIdentityStatusRequest
		if err := decodeWorkloadJSON(w, r, &input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_workload_status", "A single JSON object with a valid status is required")
			return
		}
		if input.Status != "active" && input.Status != "disabled" && input.Status != "revoked" {
			writeError(w, r, http.StatusBadRequest, "invalid_workload_status", "status must be active, disabled, or revoked")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		err := workloadRepository.SetWorkloadIdentityStatus(ctx, r.PathValue("identityID"), principal.Issuer, principal.Subject, input.Status)
		switch {
		case errors.Is(err, store.ErrWorkloadIdentityNotFound):
			writeError(w, r, http.StatusNotFound, "workload_identity_not_found", "Workload identity was not found")
		case errors.Is(err, store.ErrWorkloadAuthorizationDenied):
			writeError(w, r, http.StatusConflict, "workload_identity_revoked", "A revoked workload identity cannot be reactivated")
		case err != nil && (strings.Contains(err.Error(), "UUID") || strings.Contains(strings.ToLower(err.Error()), "invalid")):
			writeError(w, r, http.StatusBadRequest, "invalid_workload_identity", "Workload identity identifier or status is invalid")
		case err != nil:
			logger.Error("update workload identity status failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workload_identity_update_failed", "Workload identity status could not be updated")
		default:
			writeJSON(w, http.StatusOK, map[string]string{"id": r.PathValue("identityID"), "status": input.Status})
		}
	})

	mux.HandleFunc("GET /api/v1/workspaces/{workspaceID}/workload-identities", func(w http.ResponseWriter, r *http.Request) {
		if !workloadAvailable || workloadRepository == nil || !workspaceAvailable || workspaceRepository == nil {
			unavailable(w, r)
			return
		}
		principal, ok := principalFromContext(r.Context())
		if !ok {
			unauthorized(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if !requireWorkspaceWorkloadAdmin(w, r, ctx, workspaceRepository, principal.Issuer, principal.Subject, logger) {
			return
		}
		items, err := workloadRepository.ListWorkspaceWorkloadGrants(ctx, r.PathValue("workspaceID"))
		if err != nil {
			if strings.Contains(err.Error(), "UUID") {
				writeError(w, r, http.StatusBadRequest, "invalid_workspace_id", "Workspace identifier is invalid")
				return
			}
			logger.Error("list workspace workload grants failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workload_grants_unavailable", "Workspace workload grants are temporarily unavailable")
			return
		}
		data := make([]workloadGrantAdminView, 0, len(items))
		for _, item := range items {
			data = append(data, mapWorkloadGrantAdminView(item))
		}
		writeJSON(w, http.StatusOK, workloadGrantListResponse{Data: data})
	})

	grantHandler := func(granted bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !workloadAvailable || workloadRepository == nil || !workspaceAvailable || workspaceRepository == nil {
				unavailable(w, r)
				return
			}
			principal, ok := principalFromContext(r.Context())
			if !ok {
				unauthorized(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			if !requireWorkspaceWorkloadAdmin(w, r, ctx, workspaceRepository, principal.Issuer, principal.Subject, logger) {
				return
			}
			signal := r.PathValue("signal")
			if signal != "metrics" && signal != "logs" && signal != "traces" {
				writeError(w, r, http.StatusBadRequest, "invalid_signal", "signal must be metrics, logs, or traces")
				return
			}
			err := workloadRepository.SetWorkloadWorkspaceGrant(ctx, store.WorkloadGrantInput{
				IdentityID: r.PathValue("identityID"), WorkspaceID: r.PathValue("workspaceID"),
				Signal: signal, ActorIssuer: principal.Issuer, ActorSubject: principal.Subject,
			}, granted)
			switch {
			case errors.Is(err, store.ErrWorkloadIdentityNotFound):
				writeError(w, r, http.StatusNotFound, "workload_identity_not_found", "Workload identity was not found")
			case errors.Is(err, store.ErrWorkloadAuthorizationDenied):
				writeError(w, r, http.StatusConflict, "workload_identity_inactive", "Only an active workload identity can receive grants")
			case err != nil && (strings.Contains(err.Error(), "UUID") || strings.Contains(strings.ToLower(err.Error()), "invalid")):
				writeError(w, r, http.StatusBadRequest, "invalid_workload_grant", "Workspace, identity, or signal is invalid")
			case err != nil:
				logger.Error("change workload workspace grant failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
				writeError(w, r, http.StatusServiceUnavailable, "workload_grant_update_failed", "Workspace workload grant could not be updated")
			default:
				if !granted {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				writeJSON(w, http.StatusOK, workloadGrantResponse{Data: workloadGrantAdminView{
					IdentityID: r.PathValue("identityID"), WorkspaceID: r.PathValue("workspaceID"), Signal: signal,
					GrantedAt: time.Now().UTC(),
				}})
			}
		}
	}
	mux.HandleFunc("PUT /api/v1/workspaces/{workspaceID}/workload-identities/{identityID}/grants/{signal}", grantHandler(true))
	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceID}/workload-identities/{identityID}/grants/{signal}", grantHandler(false))
}

func requireWorkspaceWorkloadAdmin(
	w http.ResponseWriter,
	r *http.Request,
	ctx context.Context,
	repository store.WorkspaceRepository,
	issuer, subject string,
	logger *slog.Logger,
) bool {
	membership, err := repository.GetWorkspaceMembership(ctx, r.PathValue("workspaceID"), issuer, subject)
	if errors.Is(err, store.ErrWorkspaceMembershipNotFound) || errors.Is(err, store.ErrWorkspaceNotFound) {
		writeError(w, r, http.StatusNotFound, "workspace_not_found", "Workspace was not found")
		return false
	}
	if err != nil {
		if strings.Contains(err.Error(), "UUID") {
			writeError(w, r, http.StatusBadRequest, "invalid_workspace_id", "Workspace identifier is invalid")
		} else {
			logger.Error("verify workload management membership failed", "error", err, "request_id", r.Header.Get("X-Request-ID"))
			writeError(w, r, http.StatusServiceUnavailable, "workspace_authorization_unavailable", "Workspace access could not be verified")
		}
		return false
	}
	if membership.Role != "owner" && membership.Role != "administrator" {
		writeError(w, r, http.StatusForbidden, "workspace_admin_required", "Workspace owner or administrator access is required")
		return false
	}
	return true
}

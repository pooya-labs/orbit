package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

var incidentSeverityValues = map[string]struct{}{
	"critical": {},
	"high":     {},
	"medium":   {},
	"low":      {},
}

var incidentStatusValues = map[string]struct{}{
	"open":          {},
	"investigating": {},
	"monitoring":    {},
	"resolved":      {},
}

func validateIncidentRequestID(requestID string) error {
	if len(requestID) > 128 {
		return fmt.Errorf("incident request ID exceeds 128 characters")
	}
	return nil
}

func requireIncidentMutationRole(ctx context.Context, tx pgx.Tx, workspaceID, issuer, subject string) error {
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
		return err
	}
	var role string
	err := tx.QueryRow(ctx, `
		SELECT role
		FROM workspace_memberships
		WHERE workspace_id = $1::uuid
		  AND user_issuer = $2
		  AND user_subject = $3
		FOR SHARE
	`, workspaceID, issuer, subject).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrWorkspaceMembershipNotFound
	}
	if err != nil {
		return fmt.Errorf("lock incident workspace membership: %w", err)
	}
	switch role {
	case "owner", "administrator", "operator":
		return nil
	default:
		return store.ErrIncidentMutationForbidden
	}
}

func marshalIncidentDetails(details map[string]any) (string, error) {
	if details == nil {
		details = map[string]any{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return "", fmt.Errorf("encode incident event details: %w", err)
	}
	if len(raw) > 16*1024 {
		return "", fmt.Errorf("incident event details exceed 16 KiB")
	}
	return string(raw), nil
}

func appendIncidentTimelineTx(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, incidentID, actorIssuer, actorSubject, eventType string,
	fromStatus, toStatus, comment, requestID string,
	details map[string]any,
) error {
	raw, err := marshalIncidentDetails(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO incident_timeline_events (
			workspace_id, incident_id, actor_issuer, actor_subject,
			event_type, from_status, to_status, comment, request_id, details
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), $8, $9, $10::jsonb)
	`, workspaceID, incidentID, actorIssuer, actorSubject, eventType,
		fromStatus, toStatus, comment, requestID, raw)
	if err != nil {
		return fmt.Errorf("append incident timeline event: %w", err)
	}
	return nil
}

func appendIncidentAuditTx(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, incidentID, actorIssuer, actorSubject, action, requestID string,
	details map[string]any,
) error {
	raw, err := marshalIncidentDetails(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO workspace_audit_events (
			workspace_id, actor_issuer, actor_subject, action,
			resource_type, resource_id, request_id, details
		) VALUES ($1::uuid, $2, $3, $4, 'incident', $5, $6, $7::jsonb)
	`, workspaceID, actorIssuer, actorSubject, action, incidentID, requestID, raw)
	if err != nil {
		return fmt.Errorf("append incident workspace audit event: %w", err)
	}
	return nil
}

func (r *Repository) CreateWorkspaceIncident(ctx context.Context, input store.CreateWorkspaceIncidentInput) (store.Incident, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Severity = strings.TrimSpace(input.Severity)
	if !workspaceIDPattern.MatchString(input.WorkspaceID) {
		return store.Incident{}, fmt.Errorf("%w: workspace ID must be a UUID", store.ErrIncidentInvalidInput)
	}
	if !workspaceIDPattern.MatchString(input.ServiceID) {
		return store.Incident{}, store.ErrIncidentServiceNotFound
	}
	if len(input.Title) < 1 || len(input.Title) > 160 {
		return store.Incident{}, fmt.Errorf("%w: title must contain between 1 and 160 characters", store.ErrIncidentInvalidInput)
	}
	if len(input.Description) > 8000 {
		return store.Incident{}, fmt.Errorf("%w: description exceeds 8000 characters", store.ErrIncidentInvalidInput)
	}
	if _, ok := incidentSeverityValues[input.Severity]; !ok {
		return store.Incident{}, fmt.Errorf("%w: severity must be critical, high, medium, or low", store.ErrIncidentInvalidInput)
	}
	if err := validateWorkspacePrincipal(input.ActorIssuer, input.ActorSubject); err != nil {
		return store.Incident{}, fmt.Errorf("%w: actor principal is invalid: %v", store.ErrIncidentInvalidInput, err)
	}
	if err := validateIncidentRequestID(input.RequestID); err != nil {
		return store.Incident{}, fmt.Errorf("%w: %v", store.ErrIncidentInvalidInput, err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.Incident{}, fmt.Errorf("begin incident creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := requireIncidentMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.Incident{}, err
	}

	var serviceName string
	err = tx.QueryRow(ctx, `
		SELECT name FROM services
		WHERE id = $1::uuid AND workspace_id = $2::uuid
	`, input.ServiceID, input.WorkspaceID).Scan(&serviceName)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Incident{}, store.ErrIncidentServiceNotFound
	}
	if err != nil {
		return store.Incident{}, fmt.Errorf("verify incident service workspace: %w", err)
	}

	var incident store.Incident
	err = tx.QueryRow(ctx, `
		INSERT INTO incidents (
			workspace_id, title, description, service_id, severity, status, source
		) VALUES ($1::uuid, $2, $3, $4::uuid, $5, 'open', 'api')
		RETURNING id::text, title, description, service_id::text, severity,
		          status, started_at, updated_at, resolved_at, source
	`, input.WorkspaceID, input.Title, input.Description, input.ServiceID, input.Severity).Scan(
		&incident.ID, &incident.Title, &incident.Description, &incident.ServiceID,
		&incident.Severity, &incident.Status, &incident.StartedAt, &incident.UpdatedAt,
		&incident.ResolvedAt, &incident.Source,
	)
	if err != nil {
		return store.Incident{}, fmt.Errorf("insert workspace incident: %w", err)
	}
	incident.ServiceName = serviceName

	details := map[string]any{
		"title": input.Title,
		"severity": input.Severity,
		"service_id": input.ServiceID,
	}
	if err := appendIncidentTimelineTx(ctx, tx, input.WorkspaceID, incident.ID,
		input.ActorIssuer, input.ActorSubject, "incident.created", "", "open", "",
		input.RequestID, details); err != nil {
		return store.Incident{}, err
	}
	if err := appendIncidentAuditTx(ctx, tx, input.WorkspaceID, incident.ID,
		input.ActorIssuer, input.ActorSubject, "incident.created", input.RequestID, details); err != nil {
		return store.Incident{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Incident{}, fmt.Errorf("commit incident creation: %w", err)
	}
	return incident, nil
}

func incidentTransitionAllowed(from, to string) bool {
	switch from {
	case "open":
		return to == "investigating" || to == "monitoring" || to == "resolved"
	case "investigating":
		return to == "monitoring" || to == "resolved"
	case "monitoring":
		return to == "investigating" || to == "resolved"
	case "resolved":
		return to == "investigating"
	default:
		return false
	}
}

func (r *Repository) TransitionWorkspaceIncident(ctx context.Context, input store.TransitionWorkspaceIncidentInput) (store.Incident, error) {
	input.ToStatus = strings.TrimSpace(input.ToStatus)
	input.Comment = strings.TrimSpace(input.Comment)
	if !workspaceIDPattern.MatchString(input.WorkspaceID) {
		return store.Incident{}, fmt.Errorf("%w: workspace ID must be a UUID", store.ErrIncidentInvalidInput)
	}
	if !workspaceIDPattern.MatchString(input.IncidentID) {
		return store.Incident{}, store.ErrIncidentNotFound
	}
	if _, ok := incidentStatusValues[input.ToStatus]; !ok {
		return store.Incident{}, fmt.Errorf("%w: status must be open, investigating, monitoring, or resolved", store.ErrIncidentInvalidInput)
	}
	if len(input.Comment) > 4000 {
		return store.Incident{}, fmt.Errorf("%w: transition comment exceeds 4000 characters", store.ErrIncidentInvalidInput)
	}
	if err := validateWorkspacePrincipal(input.ActorIssuer, input.ActorSubject); err != nil {
		return store.Incident{}, fmt.Errorf("%w: actor principal is invalid: %v", store.ErrIncidentInvalidInput, err)
	}
	if err := validateIncidentRequestID(input.RequestID); err != nil {
		return store.Incident{}, fmt.Errorf("%w: %v", store.ErrIncidentInvalidInput, err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.Incident{}, fmt.Errorf("begin incident transition: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := requireIncidentMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.Incident{}, err
	}

	var incident store.Incident
	err = tx.QueryRow(ctx, `
		SELECT i.id::text, i.title, i.description, i.service_id::text, s.name,
		       i.severity, i.status, i.started_at, i.updated_at, i.resolved_at, i.source
		FROM incidents i
		JOIN services s ON s.id = i.service_id AND s.workspace_id = i.workspace_id
		WHERE i.id = $1::uuid AND i.workspace_id = $2::uuid
		FOR UPDATE OF i
	`, input.IncidentID, input.WorkspaceID).Scan(
		&incident.ID, &incident.Title, &incident.Description, &incident.ServiceID,
		&incident.ServiceName, &incident.Severity, &incident.Status, &incident.StartedAt,
		&incident.UpdatedAt, &incident.ResolvedAt, &incident.Source,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Incident{}, store.ErrIncidentNotFound
	}
	if err != nil {
		return store.Incident{}, fmt.Errorf("lock workspace incident: %w", err)
	}
	fromStatus := incident.Status
	if fromStatus == input.ToStatus {
		return store.Incident{}, store.ErrIncidentNoChange
	}
	if !incidentTransitionAllowed(fromStatus, input.ToStatus) {
		return store.Incident{}, store.ErrIncidentInvalidTransition
	}

	eventType := "incident.status_changed"
	auditAction := "incident.status_changed"
	if fromStatus == "resolved" && input.ToStatus == "investigating" {
		eventType = "incident.reopened"
		auditAction = "incident.reopened"
	}

	err = tx.QueryRow(ctx, `
		UPDATE incidents
		SET status = $3,
		    updated_at = NOW(),
		    resolved_at = CASE
		        WHEN $3 = 'resolved' THEN COALESCE(resolved_at, NOW())
		        WHEN $2 = 'resolved' THEN NULL
		        ELSE resolved_at
		    END
		WHERE id = $1::uuid AND workspace_id = $4::uuid
		RETURNING updated_at, resolved_at
	`, input.IncidentID, fromStatus, input.ToStatus, input.WorkspaceID).Scan(
		&incident.UpdatedAt, &incident.ResolvedAt,
	)
	if err != nil {
		return store.Incident{}, fmt.Errorf("update workspace incident status: %w", err)
	}
	incident.Status = input.ToStatus
	details := map[string]any{"from_status": fromStatus, "to_status": input.ToStatus}
	if input.Comment != "" {
		details["comment"] = input.Comment
	}
	if err := appendIncidentTimelineTx(ctx, tx, input.WorkspaceID, incident.ID,
		input.ActorIssuer, input.ActorSubject, eventType, fromStatus, input.ToStatus,
		input.Comment, input.RequestID, details); err != nil {
		return store.Incident{}, err
	}
	if err := appendIncidentAuditTx(ctx, tx, input.WorkspaceID, incident.ID,
		input.ActorIssuer, input.ActorSubject, auditAction, input.RequestID, details); err != nil {
		return store.Incident{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Incident{}, fmt.Errorf("commit incident transition: %w", err)
	}
	return incident, nil
}

func (r *Repository) ListWorkspaceIncidentTimeline(
	ctx context.Context,
	workspaceID, incidentID, actorIssuer, actorSubject string,
	limit int,
) ([]store.IncidentTimelineEvent, error) {
	if !workspaceIDPattern.MatchString(workspaceID) || !workspaceIDPattern.MatchString(incidentID) {
		return nil, fmt.Errorf("workspace and incident IDs must be UUIDs")
	}
	if err := validateWorkspacePrincipal(actorIssuer, actorSubject); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var member bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM workspace_memberships
			WHERE workspace_id = $1::uuid AND user_issuer = $2 AND user_subject = $3
		)
	`, workspaceID, actorIssuer, actorSubject).Scan(&member); err != nil {
		return nil, fmt.Errorf("verify incident timeline membership: %w", err)
	}
	if !member {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM incidents
			WHERE id = $1::uuid AND workspace_id = $2::uuid
		)
	`, incidentID, workspaceID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("verify workspace incident for timeline: %w", err)
	}
	if !exists {
		return nil, store.ErrIncidentNotFound
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id::text, workspace_id::text, incident_id::text,
		       actor_issuer, actor_subject, event_type,
		       COALESCE(from_status, ''), COALESCE(to_status, ''),
		       comment, request_id, details, created_at
		FROM incident_timeline_events
		WHERE workspace_id = $1::uuid AND incident_id = $2::uuid
		ORDER BY created_at DESC, id DESC
		LIMIT $3
	`, workspaceID, incidentID, limit)
	if err != nil {
		return nil, fmt.Errorf("query incident timeline: %w", err)
	}
	defer rows.Close()

	items := make([]store.IncidentTimelineEvent, 0)
	for rows.Next() {
		var item store.IncidentTimelineEvent
		var details []byte
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.IncidentID,
			&item.ActorIssuer, &item.ActorSubject, &item.EventType,
			&item.FromStatus, &item.ToStatus, &item.Comment, &item.RequestID,
			&details, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan incident timeline event: %w", err)
		}
		if err := json.Unmarshal(details, &item.Details); err != nil {
			return nil, fmt.Errorf("decode incident timeline event details: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incident timeline events: %w", err)
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	return items, nil
}

var _ store.IncidentLifecycleRepository = (*Repository)(nil)

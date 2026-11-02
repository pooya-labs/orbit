package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

var sloRoutePattern = regexp.MustCompile(`^/[A-Za-z0-9][A-Za-z0-9/_-]{0,127}$`)

const workspaceSLOSelectColumns = `
	slo.id::text, slo.workspace_id::text, slo.service_id::text,
	services.name, services.environment, slo.name, slo.route, slo.target,
	slo.rolling_window_days, slo.enabled, slo.created_at, slo.updated_at
`

func scanWorkspaceSLO(row pgx.Row) (store.ServiceLevelObjective, error) {
	var item store.ServiceLevelObjective
	err := row.Scan(
		&item.ID, &item.WorkspaceID, &item.ServiceID,
		&item.ServiceName, &item.Environment, &item.Name, &item.Route,
		&item.Target, &item.RollingWindowDays, &item.Enabled,
		&item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func validateSLOFields(name, route string, target float64, windowDays int) error {
	name = strings.TrimSpace(name)
	route = strings.TrimSpace(route)
	if len(name) < 1 || len(name) > 120 {
		return fmt.Errorf("%w: name must contain between 1 and 120 characters", store.ErrSLOInvalidInput)
	}
	if !sloRoutePattern.MatchString(route) || strings.Contains(route, "//") {
		return fmt.Errorf("%w: route must be a normalized low-cardinality absolute path", store.ErrSLOInvalidInput)
	}
	if math.IsNaN(target) || math.IsInf(target, 0) || target < 0.90 || target > 0.9999 {
		return fmt.Errorf("%w: target must be between 0.90 and 0.9999", store.ErrSLOInvalidInput)
	}
	switch windowDays {
	case 1, 3, 7:
	default:
		return fmt.Errorf("%w: rolling window must be 1, 3, or 7 days", store.ErrSLOInvalidInput)
	}
	return nil
}

func validateSLOActor(issuer, subject, requestID string) error {
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
		return fmt.Errorf("%w: actor principal is invalid", store.ErrSLOInvalidInput)
	}
	if len(requestID) > 128 {
		return fmt.Errorf("%w: request ID exceeds 128 characters", store.ErrSLOInvalidInput)
	}
	return nil
}

func requireSLOMutationRole(ctx context.Context, tx pgx.Tx, workspaceID, issuer, subject string) error {
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
		return err
	}
	var role string
	err := tx.QueryRow(ctx, `
		SELECT role
		FROM workspace_memberships
		WHERE workspace_id = $1::uuid AND user_issuer = $2 AND user_subject = $3
		FOR SHARE
	`, workspaceID, issuer, subject).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrWorkspaceMembershipNotFound
	}
	if err != nil {
		return fmt.Errorf("lock SLO workspace membership: %w", err)
	}
	switch role {
	case "owner", "administrator", "operator":
		return nil
	default:
		return store.ErrSLOForbidden
	}
}

func workspaceSLOAuditDetails(item store.ServiceLevelObjective) map[string]any {
	return map[string]any{
		"service_id": item.ServiceID,
		"service_name": item.ServiceName,
		"environment": item.Environment,
		"name": item.Name,
		"route": item.Route,
		"target": item.Target,
		"rolling_window_days": item.RollingWindowDays,
		"enabled": item.Enabled,
	}
}

func appendSLOAuditTx(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, sloID, actorIssuer, actorSubject, action, requestID string,
	details map[string]any,
) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode SLO audit details: %w", err)
	}
	if len(raw) > 16*1024 {
		return fmt.Errorf("%w: audit details exceed 16 KiB", store.ErrSLOInvalidInput)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO workspace_audit_events (
			workspace_id, actor_issuer, actor_subject, action,
			resource_type, resource_id, request_id, details
		) VALUES ($1::uuid, $2, $3, $4, 'slo_definition', $5, $6, $7::jsonb)
	`, workspaceID, actorIssuer, actorSubject, action, sloID, requestID, string(raw))
	if err != nil {
		return fmt.Errorf("append SLO workspace audit event: %w", err)
	}
	return nil
}

func mapSLOConflict(err error) error {
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) && postgresErr.Code == "23505" &&
		postgresErr.ConstraintName == "service_level_objectives_workspace_service_name_unique" {
		return store.ErrSLONameConflict
	}
	return err
}

func (r *Repository) CreateWorkspaceSLO(
	ctx context.Context,
	input store.CreateWorkspaceSLOInput,
) (store.ServiceLevelObjective, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Route = strings.TrimSpace(input.Route)
	if !workspaceIDPattern.MatchString(input.WorkspaceID) || !workspaceIDPattern.MatchString(input.ServiceID) {
		return store.ServiceLevelObjective{}, fmt.Errorf("%w: workspace and service IDs must be UUIDs", store.ErrSLOInvalidInput)
	}
	if err := validateSLOFields(input.Name, input.Route, input.Target, input.RollingWindowDays); err != nil {
		return store.ServiceLevelObjective{}, err
	}
	if err := validateSLOActor(input.ActorIssuer, input.ActorSubject, input.RequestID); err != nil {
		return store.ServiceLevelObjective{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("begin SLO creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireSLOMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.ServiceLevelObjective{}, err
	}
	var serviceName, environment string
	err = tx.QueryRow(ctx, `
		SELECT name, environment
		FROM services
		WHERE id = $1::uuid AND workspace_id = $2::uuid
		FOR SHARE
	`, input.ServiceID, input.WorkspaceID).Scan(&serviceName, &environment)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ServiceLevelObjective{}, store.ErrDeploymentAnnotationServiceNotFound
	}
	if err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("verify SLO service workspace: %w", err)
	}

	item, err := scanWorkspaceSLO(tx.QueryRow(ctx, `
		INSERT INTO service_level_objectives (
			workspace_id, service_id, name, route, target, rolling_window_days,
			created_by_issuer, created_by_subject, updated_by_issuer, updated_by_subject
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $7, $8)
		RETURNING id::text, workspace_id::text, service_id::text,
		          $7::text, $8::text, name, route, target, rolling_window_days,
		          enabled, created_at, updated_at
	`, input.WorkspaceID, input.ServiceID, input.Name, input.Route, input.Target,
		input.RollingWindowDays, input.ActorIssuer, input.ActorSubject))
	if err != nil {
		return store.ServiceLevelObjective{}, mapSLOConflict(fmt.Errorf("insert SLO definition: %w", err))
	}
	item.ServiceName = serviceName
	item.Environment = environment
	if err := appendSLOAuditTx(ctx, tx, input.WorkspaceID, item.ID,
		input.ActorIssuer, input.ActorSubject, "slo.created", input.RequestID,
		workspaceSLOAuditDetails(item)); err != nil {
		return store.ServiceLevelObjective{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("commit SLO creation: %w", err)
	}
	return item, nil
}

func (r *Repository) ListWorkspaceSLOs(
	ctx context.Context,
	workspaceID, issuer, subject, serviceID string,
	limit int,
) ([]store.ServiceLevelObjective, error) {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return nil, fmt.Errorf("%w: workspace ID must be a UUID", store.ErrSLOInvalidInput)
	}
	if serviceID != "" && !workspaceIDPattern.MatchString(serviceID) {
		return nil, fmt.Errorf("%w: service ID must be a UUID", store.ErrSLOInvalidInput)
	}
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
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
	`, workspaceID, issuer, subject).Scan(&member); err != nil {
		return nil, fmt.Errorf("verify SLO workspace membership: %w", err)
	}
	if !member {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+workspaceSLOSelectColumns+`
		FROM service_level_objectives AS slo
		JOIN services ON services.id = slo.service_id AND services.workspace_id = slo.workspace_id
		WHERE slo.workspace_id = $1::uuid
		  AND ($2::uuid IS NULL OR slo.service_id = $2::uuid)
		ORDER BY slo.enabled DESC, slo.updated_at DESC, slo.id DESC
		LIMIT $3
	`, workspaceID, nullableUUID(serviceID), limit)
	if err != nil {
		return nil, fmt.Errorf("query workspace SLOs: %w", err)
	}
	defer rows.Close()
	items := make([]store.ServiceLevelObjective, 0)
	for rows.Next() {
		item, err := scanWorkspaceSLO(rows)
		if err != nil {
			return nil, fmt.Errorf("scan workspace SLO: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace SLOs: %w", err)
	}
	return items, nil
}

func (r *Repository) GetWorkspaceSLO(
	ctx context.Context,
	workspaceID, sloID, issuer, subject string,
) (store.ServiceLevelObjective, error) {
	if !workspaceIDPattern.MatchString(workspaceID) || !workspaceIDPattern.MatchString(sloID) {
		return store.ServiceLevelObjective{}, fmt.Errorf("%w: workspace and SLO IDs must be UUIDs", store.ErrSLOInvalidInput)
	}
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
		return store.ServiceLevelObjective{}, err
	}
	var member bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM workspace_memberships
			WHERE workspace_id = $1::uuid AND user_issuer = $2 AND user_subject = $3
		)
	`, workspaceID, issuer, subject).Scan(&member); err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("verify SLO workspace membership: %w", err)
	}
	if !member {
		return store.ServiceLevelObjective{}, store.ErrWorkspaceMembershipNotFound
	}
	item, err := scanWorkspaceSLO(r.pool.QueryRow(ctx, `
		SELECT `+workspaceSLOSelectColumns+`
		FROM service_level_objectives AS slo
		JOIN services ON services.id = slo.service_id AND services.workspace_id = slo.workspace_id
		WHERE slo.workspace_id = $1::uuid AND slo.id = $2::uuid
	`, workspaceID, sloID))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ServiceLevelObjective{}, store.ErrSLONotFound
	}
	if err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("get workspace SLO: %w", err)
	}
	return item, nil
}

func (r *Repository) UpdateWorkspaceSLO(
	ctx context.Context,
	input store.UpdateWorkspaceSLOInput,
) (store.ServiceLevelObjective, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Route = strings.TrimSpace(input.Route)
	if !workspaceIDPattern.MatchString(input.WorkspaceID) || !workspaceIDPattern.MatchString(input.SLOID) {
		return store.ServiceLevelObjective{}, fmt.Errorf("%w: workspace and SLO IDs must be UUIDs", store.ErrSLOInvalidInput)
	}
	if err := validateSLOFields(input.Name, input.Route, input.Target, input.RollingWindowDays); err != nil {
		return store.ServiceLevelObjective{}, err
	}
	if err := validateSLOActor(input.ActorIssuer, input.ActorSubject, input.RequestID); err != nil {
		return store.ServiceLevelObjective{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("begin SLO update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireSLOMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.ServiceLevelObjective{}, err
	}
	var lockedID string
	if err := tx.QueryRow(ctx, `
		SELECT id::text FROM service_level_objectives
		WHERE id = $1::uuid AND workspace_id = $2::uuid
		FOR UPDATE
	`, input.SLOID, input.WorkspaceID).Scan(&lockedID); errors.Is(err, pgx.ErrNoRows) {
		return store.ServiceLevelObjective{}, store.ErrSLONotFound
	} else if err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("lock SLO for update: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE service_level_objectives
		SET name = $3, route = $4, target = $5, rolling_window_days = $6,
		    updated_by_issuer = $7, updated_by_subject = $8, updated_at = NOW()
		WHERE id = $1::uuid AND workspace_id = $2::uuid
	`, input.SLOID, input.WorkspaceID, input.Name, input.Route, input.Target,
		input.RollingWindowDays, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.ServiceLevelObjective{}, mapSLOConflict(fmt.Errorf("update SLO definition: %w", err))
	}
	item, err := scanWorkspaceSLO(tx.QueryRow(ctx, `
		SELECT `+workspaceSLOSelectColumns+`
		FROM service_level_objectives AS slo
		JOIN services ON services.id = slo.service_id AND services.workspace_id = slo.workspace_id
		WHERE slo.id = $1::uuid AND slo.workspace_id = $2::uuid
	`, input.SLOID, input.WorkspaceID))
	if err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("read updated SLO definition: %w", err)
	}
	if err := appendSLOAuditTx(ctx, tx, input.WorkspaceID, item.ID,
		input.ActorIssuer, input.ActorSubject, "slo.updated", input.RequestID,
		workspaceSLOAuditDetails(item)); err != nil {
		return store.ServiceLevelObjective{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("commit SLO update: %w", err)
	}
	return item, nil
}

func (r *Repository) SetWorkspaceSLOEnabled(
	ctx context.Context,
	input store.SetWorkspaceSLOEnabledInput,
) (store.ServiceLevelObjective, error) {
	if !workspaceIDPattern.MatchString(input.WorkspaceID) || !workspaceIDPattern.MatchString(input.SLOID) {
		return store.ServiceLevelObjective{}, fmt.Errorf("%w: workspace and SLO IDs must be UUIDs", store.ErrSLOInvalidInput)
	}
	if err := validateSLOActor(input.ActorIssuer, input.ActorSubject, input.RequestID); err != nil {
		return store.ServiceLevelObjective{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("begin SLO enabled update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireSLOMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.ServiceLevelObjective{}, err
	}
	var current bool
	err = tx.QueryRow(ctx, `
		SELECT enabled FROM service_level_objectives
		WHERE id = $1::uuid AND workspace_id = $2::uuid
		FOR UPDATE
	`, input.SLOID, input.WorkspaceID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ServiceLevelObjective{}, store.ErrSLONotFound
	}
	if err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("lock SLO enabled state: %w", err)
	}
	if current == input.Enabled {
		return store.ServiceLevelObjective{}, store.ErrSLONoChange
	}
	if _, err := tx.Exec(ctx, `
		UPDATE service_level_objectives
		SET enabled = $3, updated_by_issuer = $4, updated_by_subject = $5, updated_at = NOW()
		WHERE id = $1::uuid AND workspace_id = $2::uuid
	`, input.SLOID, input.WorkspaceID, input.Enabled, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("update SLO enabled state: %w", err)
	}
	item, err := scanWorkspaceSLO(tx.QueryRow(ctx, `
		SELECT `+workspaceSLOSelectColumns+`
		FROM service_level_objectives AS slo
		JOIN services ON services.id = slo.service_id AND services.workspace_id = slo.workspace_id
		WHERE slo.id = $1::uuid AND slo.workspace_id = $2::uuid
	`, input.SLOID, input.WorkspaceID))
	if err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("read updated SLO state: %w", err)
	}
	action := "slo.disabled"
	if input.Enabled {
		action = "slo.enabled"
	}
	if err := appendSLOAuditTx(ctx, tx, input.WorkspaceID, item.ID,
		input.ActorIssuer, input.ActorSubject, action, input.RequestID,
		map[string]any{"name": item.Name, "enabled": item.Enabled}); err != nil {
		return store.ServiceLevelObjective{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.ServiceLevelObjective{}, fmt.Errorf("commit SLO enabled update: %w", err)
	}
	return item, nil
}

func (r *Repository) DeleteWorkspaceSLO(ctx context.Context, input store.DeleteWorkspaceSLOInput) error {
	if !workspaceIDPattern.MatchString(input.WorkspaceID) || !workspaceIDPattern.MatchString(input.SLOID) {
		return fmt.Errorf("%w: workspace and SLO IDs must be UUIDs", store.ErrSLOInvalidInput)
	}
	if err := validateSLOActor(input.ActorIssuer, input.ActorSubject, input.RequestID); err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin SLO deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireSLOMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return err
	}
	item, err := scanWorkspaceSLO(tx.QueryRow(ctx, `
		SELECT `+workspaceSLOSelectColumns+`
		FROM service_level_objectives AS slo
		JOIN services ON services.id = slo.service_id AND services.workspace_id = slo.workspace_id
		WHERE slo.id = $1::uuid AND slo.workspace_id = $2::uuid
		FOR UPDATE OF slo
	`, input.SLOID, input.WorkspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrSLONotFound
	}
	if err != nil {
		return fmt.Errorf("lock SLO for deletion: %w", err)
	}
	if err := appendSLOAuditTx(ctx, tx, input.WorkspaceID, item.ID,
		input.ActorIssuer, input.ActorSubject, "slo.deleted", input.RequestID,
		workspaceSLOAuditDetails(item)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM service_level_objectives
		WHERE id = $1::uuid AND workspace_id = $2::uuid
	`, input.SLOID, input.WorkspaceID); err != nil {
		return fmt.Errorf("delete workspace SLO: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit SLO deletion: %w", err)
	}
	return nil
}

var _ store.WorkspaceSLORepository = (*Repository)(nil)

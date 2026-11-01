package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

var (
	workspaceSlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,46}[a-z0-9])$`)
	workspaceIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	workspaceAuditActionPattern = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{0,95}$`)
	workspaceAuditResourcePattern = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{0,63}$`)
)

func validateWorkspacePrincipal(issuer, subject string) error {
	if strings.TrimSpace(issuer) == "" || strings.TrimSpace(issuer) != issuer || len(issuer) > 512 {
		return fmt.Errorf("workspace principal issuer must contain between 1 and 512 characters")
	}
	if strings.TrimSpace(subject) == "" || strings.TrimSpace(subject) != subject || len(subject) > 512 {
		return fmt.Errorf("workspace principal subject must contain between 1 and 512 characters")
	}
	return nil
}

func (r *Repository) CreateWorkspace(ctx context.Context, input store.CreateWorkspaceInput) (store.Workspace, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.TrimSpace(strings.ToLower(input.Slug))
	if len(input.Name) < 1 || len(input.Name) > 120 {
		return store.Workspace{}, fmt.Errorf("workspace name must contain between 1 and 120 characters")
	}
	if !workspaceSlugPattern.MatchString(input.Slug) {
		return store.Workspace{}, fmt.Errorf("workspace slug must be 2-48 lowercase letters, digits, or hyphens")
	}
	if err := validateWorkspacePrincipal(input.OwnerIssuer, input.OwnerSubject); err != nil {
		return store.Workspace{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.Workspace{}, fmt.Errorf("begin workspace creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var workspace store.Workspace
	err = tx.QueryRow(ctx, `
		INSERT INTO workspaces (name, slug, created_by_issuer, created_by_subject)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, name, slug, created_by_issuer, created_by_subject, created_at, updated_at
	`, input.Name, input.Slug, input.OwnerIssuer, input.OwnerSubject).Scan(
		&workspace.ID, &workspace.Name, &workspace.Slug, &workspace.CreatedByIssuer,
		&workspace.CreatedBySubject, &workspace.CreatedAt, &workspace.UpdatedAt,
	)
	if err != nil {
		return store.Workspace{}, fmt.Errorf("insert workspace: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO workspace_memberships (workspace_id, user_issuer, user_subject, role)
		VALUES ($1::uuid, $2, $3, 'owner')
	`, workspace.ID, input.OwnerIssuer, input.OwnerSubject); err != nil {
		return store.Workspace{}, fmt.Errorf("create workspace owner membership: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return store.Workspace{}, fmt.Errorf("commit workspace creation: %w", err)
	}
	return workspace, nil
}

func (r *Repository) ListWorkspacesForPrincipal(ctx context.Context, issuer, subject string) ([]store.Workspace, error) {
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT w.id::text, w.name, w.slug, w.created_by_issuer, w.created_by_subject, w.created_at, w.updated_at
		FROM workspaces AS w
		INNER JOIN workspace_memberships AS m ON m.workspace_id = w.id
		WHERE m.user_issuer = $1 AND m.user_subject = $2
		ORDER BY w.name ASC, w.id ASC
		LIMIT 100
	`, issuer, subject)
	if err != nil {
		return nil, fmt.Errorf("list principal workspaces: %w", err)
	}
	defer rows.Close()

	items := make([]store.Workspace, 0)
	for rows.Next() {
		var item store.Workspace
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.CreatedByIssuer, &item.CreatedBySubject, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan principal workspace: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate principal workspaces: %w", err)
	}
	return items, nil
}

func (r *Repository) GetWorkspaceMembership(ctx context.Context, workspaceID, issuer, subject string) (store.WorkspaceMembership, error) {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return store.WorkspaceMembership{}, fmt.Errorf("workspace ID must be a UUID")
	}
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
		return store.WorkspaceMembership{}, err
	}
	var membership store.WorkspaceMembership
	err := r.pool.QueryRow(ctx, `
		SELECT workspace_id::text, user_issuer, user_subject, role, created_at, updated_at
		FROM workspace_memberships
		WHERE workspace_id = $1::uuid AND user_issuer = $2 AND user_subject = $3
	`, workspaceID, issuer, subject).Scan(
		&membership.WorkspaceID, &membership.UserIssuer, &membership.UserSubject,
		&membership.Role, &membership.CreatedAt, &membership.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.WorkspaceMembership{}, store.ErrWorkspaceMembershipNotFound
	}
	if err != nil {
		return store.WorkspaceMembership{}, fmt.Errorf("get workspace membership: %w", err)
	}
	return membership, nil
}

func (r *Repository) AppendWorkspaceAuditEvent(ctx context.Context, event store.WorkspaceAuditEvent) (store.WorkspaceAuditEvent, error) {
	if !workspaceIDPattern.MatchString(event.WorkspaceID) {
		return store.WorkspaceAuditEvent{}, fmt.Errorf("workspace ID must be a UUID")
	}
	if err := validateWorkspacePrincipal(event.ActorIssuer, event.ActorSubject); err != nil {
		return store.WorkspaceAuditEvent{}, err
	}
	if !workspaceAuditActionPattern.MatchString(event.Action) {
		return store.WorkspaceAuditEvent{}, fmt.Errorf("audit action is invalid")
	}
	if !workspaceAuditResourcePattern.MatchString(event.ResourceType) {
		return store.WorkspaceAuditEvent{}, fmt.Errorf("audit resource type is invalid")
	}
	if len(event.ResourceID) > 256 || len(event.RequestID) > 128 {
		return store.WorkspaceAuditEvent{}, fmt.Errorf("audit resource or request ID exceeds its limit")
	}
	if event.Details == nil {
		event.Details = map[string]any{}
	}
	details, err := json.Marshal(event.Details)
	if err != nil {
		return store.WorkspaceAuditEvent{}, fmt.Errorf("encode audit event details: %w", err)
	}
	if len(details) > 16*1024 {
		return store.WorkspaceAuditEvent{}, fmt.Errorf("audit event details exceed 16 KiB")
	}

	err = r.pool.QueryRow(ctx, `
		INSERT INTO workspace_audit_events (
			workspace_id, actor_issuer, actor_subject, action,
			resource_type, resource_id, request_id, details
		)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8::jsonb)
		RETURNING id::text, created_at
	`, event.WorkspaceID, event.ActorIssuer, event.ActorSubject, event.Action,
		event.ResourceType, event.ResourceID, event.RequestID, string(details)).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return store.WorkspaceAuditEvent{}, fmt.Errorf("append workspace audit event: %w", err)
	}
	return event, nil
}

func (r *Repository) ListWorkspaceAuditEvents(ctx context.Context, workspaceID string, limit int) ([]store.WorkspaceAuditEvent, error) {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return nil, fmt.Errorf("workspace ID must be a UUID")
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, workspace_id::text, actor_issuer, actor_subject, action,
		       resource_type, resource_id, request_id, details, created_at
		FROM workspace_audit_events
		WHERE workspace_id = $1::uuid
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("list workspace audit events: %w", err)
	}
	defer rows.Close()

	items := make([]store.WorkspaceAuditEvent, 0)
	for rows.Next() {
		var item store.WorkspaceAuditEvent
		var details []byte
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.ActorIssuer, &item.ActorSubject,
			&item.Action, &item.ResourceType, &item.ResourceID, &item.RequestID,
			&details, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan workspace audit event: %w", err)
		}
		if err := json.Unmarshal(details, &item.Details); err != nil {
			return nil, fmt.Errorf("decode workspace audit event details: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace audit events: %w", err)
	}
	return items, nil
}

var _ store.WorkspaceRepository = (*Repository)(nil)

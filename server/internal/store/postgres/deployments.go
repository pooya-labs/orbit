package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

var deploymentVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_:/-]{0,127}$`)
var deploymentCommitSHA = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
var deploymentChangeReferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/#-]{0,255}$`)

const deploymentAnnotationSelectColumns = `
	id::text, workspace_id::text, service_id::text, service_name,
	environment, version, commit_sha, change_reference, release_notes,
	deployed_at, created_at
`

func scanDeploymentAnnotation(row pgx.Row) (store.DeploymentAnnotation, error) {
	var item store.DeploymentAnnotation
	err := row.Scan(
		&item.ID, &item.WorkspaceID, &item.ServiceID, &item.ServiceName,
		&item.Environment, &item.Version, &item.CommitSHA, &item.ChangeReference,
		&item.ReleaseNotes, &item.DeployedAt, &item.CreatedAt,
	)
	return item, err
}

func requireDeploymentMutationRole(ctx context.Context, tx pgx.Tx, workspaceID, issuer, subject string) error {
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
		return err
	}
	var role string
	err := tx.QueryRow(ctx, `
		SELECT role FROM workspace_memberships
		WHERE workspace_id = $1::uuid AND user_issuer = $2 AND user_subject = $3
		FOR SHARE
	`, workspaceID, issuer, subject).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrWorkspaceMembershipNotFound
	}
	if err != nil {
		return fmt.Errorf("lock deployment workspace membership: %w", err)
	}
	switch role {
	case "owner", "administrator", "operator":
		return nil
	default:
		return store.ErrDeploymentAnnotationForbidden
	}
}

func appendDeploymentAuditTx(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, annotationID, actorIssuer, actorSubject, requestID string,
	details map[string]any,
) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode deployment annotation audit details: %w", err)
	}
	if len(raw) > 16*1024 {
		return fmt.Errorf("%w: audit details exceed 16 KiB", store.ErrDeploymentAnnotationInvalidInput)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO workspace_audit_events (
			workspace_id, actor_issuer, actor_subject, action,
			resource_type, resource_id, request_id, details
		) VALUES ($1::uuid, $2, $3, 'deployment.annotation_created',
		          'deployment_annotation', $4, $5, $6::jsonb)
	`, workspaceID, actorIssuer, actorSubject, annotationID, requestID, string(raw))
	if err != nil {
		return fmt.Errorf("append deployment annotation audit event: %w", err)
	}
	return nil
}

func (r *Repository) CreateWorkspaceDeploymentAnnotation(
	ctx context.Context,
	input store.CreateWorkspaceDeploymentAnnotationInput,
) (store.DeploymentAnnotation, error) {
	input.Version = strings.TrimSpace(input.Version)
	input.CommitSHA = strings.ToLower(strings.TrimSpace(input.CommitSHA))
	input.ChangeReference = strings.TrimSpace(input.ChangeReference)
	input.ReleaseNotes = strings.TrimSpace(input.ReleaseNotes)

	if !workspaceIDPattern.MatchString(input.WorkspaceID) ||
		!workspaceIDPattern.MatchString(input.ServiceID) {
		return store.DeploymentAnnotation{}, fmt.Errorf("%w: workspace and service IDs must be UUIDs", store.ErrDeploymentAnnotationInvalidInput)
	}
	if !deploymentVersionPattern.MatchString(input.Version) {
		return store.DeploymentAnnotation{}, fmt.Errorf("%w: version must be a bounded release label", store.ErrDeploymentAnnotationInvalidInput)
	}
	if input.CommitSHA != "" && !deploymentCommitSHA.MatchString(input.CommitSHA) {
		return store.DeploymentAnnotation{}, fmt.Errorf("%w: commit SHA must contain 7 to 64 hexadecimal characters", store.ErrDeploymentAnnotationInvalidInput)
	}
	if input.ChangeReference != "" && !deploymentChangeReferencePattern.MatchString(input.ChangeReference) {
		return store.DeploymentAnnotation{}, fmt.Errorf("%w: change reference contains unsupported characters", store.ErrDeploymentAnnotationInvalidInput)
	}
	if len(input.ReleaseNotes) > 4000 {
		return store.DeploymentAnnotation{}, fmt.Errorf("%w: release notes exceed 4000 characters", store.ErrDeploymentAnnotationInvalidInput)
	}
	if err := validateWorkspacePrincipal(input.ActorIssuer, input.ActorSubject); err != nil {
		return store.DeploymentAnnotation{}, fmt.Errorf("%w: actor principal is invalid", store.ErrDeploymentAnnotationInvalidInput)
	}
	if len(input.RequestID) > 128 {
		return store.DeploymentAnnotation{}, fmt.Errorf("%w: request ID exceeds 128 characters", store.ErrDeploymentAnnotationInvalidInput)
	}
	if input.DeployedAt.IsZero() {
		input.DeployedAt = time.Now().UTC()
	} else {
		input.DeployedAt = input.DeployedAt.UTC()
	}
	if input.DeployedAt.After(time.Now().UTC().Add(5 * time.Minute)) {
		return store.DeploymentAnnotation{}, fmt.Errorf("%w: deployment time cannot be more than five minutes in the future", store.ErrDeploymentAnnotationInvalidInput)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.DeploymentAnnotation{}, fmt.Errorf("begin deployment annotation creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireDeploymentMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.DeploymentAnnotation{}, err
	}

	var serviceName, environment string
	err = tx.QueryRow(ctx, `
		SELECT name, environment
		FROM services
		WHERE id = $1::uuid AND workspace_id = $2::uuid
		FOR SHARE
	`, input.ServiceID, input.WorkspaceID).Scan(&serviceName, &environment)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.DeploymentAnnotation{}, store.ErrDeploymentAnnotationServiceNotFound
	}
	if err != nil {
		return store.DeploymentAnnotation{}, fmt.Errorf("verify deployment service workspace: %w", err)
	}

	item, err := scanDeploymentAnnotation(tx.QueryRow(ctx, `
		INSERT INTO deployment_annotations (
			workspace_id, service_id, service_name, environment, version,
			commit_sha, change_reference, release_notes, deployed_at,
			created_by_issuer, created_by_subject
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+deploymentAnnotationSelectColumns,
		input.WorkspaceID, input.ServiceID, serviceName, environment, input.Version,
		input.CommitSHA, input.ChangeReference, input.ReleaseNotes, input.DeployedAt,
		input.ActorIssuer, input.ActorSubject,
	))
	if err != nil {
		return store.DeploymentAnnotation{}, fmt.Errorf("insert workspace deployment annotation: %w", err)
	}
	if err := appendDeploymentAuditTx(ctx, tx, input.WorkspaceID, item.ID,
		input.ActorIssuer, input.ActorSubject, input.RequestID, map[string]any{
			"service_id": item.ServiceID,
			"service_name": item.ServiceName,
			"environment": item.Environment,
			"version": item.Version,
			"commit_sha": item.CommitSHA,
			"change_reference": item.ChangeReference,
			"deployed_at": item.DeployedAt,
		}); err != nil {
		return store.DeploymentAnnotation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.DeploymentAnnotation{}, fmt.Errorf("commit deployment annotation: %w", err)
	}
	return item, nil
}

func (r *Repository) ListWorkspaceDeploymentAnnotations(
	ctx context.Context,
	workspaceID, issuer, subject, serviceID string,
	limit int,
) ([]store.DeploymentAnnotation, error) {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return nil, fmt.Errorf("%w: workspace ID must be a UUID", store.ErrDeploymentAnnotationInvalidInput)
	}
	if serviceID != "" && !workspaceIDPattern.MatchString(serviceID) {
		return nil, fmt.Errorf("%w: service ID must be a UUID", store.ErrDeploymentAnnotationInvalidInput)
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
		return nil, fmt.Errorf("verify deployment annotation workspace membership: %w", err)
	}
	if !member {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+deploymentAnnotationSelectColumns+`
		FROM deployment_annotations
		WHERE workspace_id = $1::uuid AND ($2::uuid IS NULL OR service_id = $2::uuid)
		ORDER BY deployed_at DESC, id DESC
		LIMIT $3
	`, workspaceID, nullableUUID(serviceID), limit)
	if err != nil {
		return nil, fmt.Errorf("query workspace deployment annotations: %w", err)
	}
	defer rows.Close()
	items := make([]store.DeploymentAnnotation, 0)
	for rows.Next() {
		item, err := scanDeploymentAnnotation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan deployment annotation: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deployment annotations: %w", err)
	}
	return items, nil
}

func (r *Repository) GetWorkspaceDeploymentAnnotation(
	ctx context.Context,
	workspaceID, annotationID, issuer, subject string,
) (store.DeploymentAnnotation, error) {
	if !workspaceIDPattern.MatchString(workspaceID) || !workspaceIDPattern.MatchString(annotationID) {
		return store.DeploymentAnnotation{}, fmt.Errorf("%w: workspace and deployment IDs must be UUIDs", store.ErrDeploymentAnnotationInvalidInput)
	}
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
		return store.DeploymentAnnotation{}, err
	}
	var member bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM workspace_memberships
			WHERE workspace_id = $1::uuid AND user_issuer = $2 AND user_subject = $3
		)
	`, workspaceID, issuer, subject).Scan(&member); err != nil {
		return store.DeploymentAnnotation{}, fmt.Errorf("verify deployment annotation workspace membership: %w", err)
	}
	if !member {
		return store.DeploymentAnnotation{}, store.ErrWorkspaceMembershipNotFound
	}
	item, err := scanDeploymentAnnotation(r.pool.QueryRow(ctx, `
		SELECT `+deploymentAnnotationSelectColumns+`
		FROM deployment_annotations
		WHERE id = $1::uuid AND workspace_id = $2::uuid
	`, annotationID, workspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.DeploymentAnnotation{}, store.ErrDeploymentAnnotationNotFound
	}
	if err != nil {
		return store.DeploymentAnnotation{}, fmt.Errorf("get workspace deployment annotation: %w", err)
	}
	return item, nil
}

var _ store.DeploymentAnnotationRepository = (*Repository)(nil)

func nullableUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}


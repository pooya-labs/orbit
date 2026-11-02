package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

var alertRuleOperators = map[string]struct{}{
	"gt": {}, "gte": {}, "lt": {}, "lte": {},
}

var alertRuleSeverities = map[string]struct{}{
	"critical": {}, "high": {}, "medium": {}, "low": {},
}

var alertRuleWindows = map[int]struct{}{
	60: {}, 300: {}, 900: {},
}

func validateAlertRuleRequestID(value string) error {
	if len(value) > 128 {
		return fmt.Errorf("%w: request ID exceeds 128 characters", store.ErrAlertRuleInvalidInput)
	}
	return nil
}

func validateAlertRuleActor(issuer, subject string) error {
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
		return fmt.Errorf("%w: actor principal is invalid: %v", store.ErrAlertRuleInvalidInput, err)
	}
	return nil
}

func validateAlertRuleFields(name, description, serviceName, metric, operator string, threshold float64, windowSeconds, pendingSeconds int, severity string) error {
	if len(name) < 1 || len(name) > 120 {
		return fmt.Errorf("%w: name must contain between 1 and 120 characters", store.ErrAlertRuleInvalidInput)
	}
	if len(description) > 4000 {
		return fmt.Errorf("%w: description exceeds 4000 characters", store.ErrAlertRuleInvalidInput)
	}
	if serviceName != "" && !telemetryServiceNamePattern.MatchString(serviceName) {
		return fmt.Errorf("%w: service name is invalid", store.ErrAlertRuleInvalidInput)
	}
	if metric != "request_rate" {
		return fmt.Errorf("%w: metric must be request_rate", store.ErrAlertRuleInvalidInput)
	}
	if _, ok := alertRuleOperators[operator]; !ok {
		return fmt.Errorf("%w: operator must be gt, gte, lt, or lte", store.ErrAlertRuleInvalidInput)
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold <= 0 || threshold > 1_000_000_000 {
		return fmt.Errorf("%w: threshold must be finite and between 0 and 1000000000", store.ErrAlertRuleInvalidInput)
	}
	if _, ok := alertRuleWindows[windowSeconds]; !ok {
		return fmt.Errorf("%w: window_seconds must be 60, 300, or 900", store.ErrAlertRuleInvalidInput)
	}
	if pendingSeconds < 0 || pendingSeconds > 3600 {
		return fmt.Errorf("%w: pending_seconds must be between 0 and 3600", store.ErrAlertRuleInvalidInput)
	}
	if _, ok := alertRuleSeverities[severity]; !ok {
		return fmt.Errorf("%w: severity must be critical, high, medium, or low", store.ErrAlertRuleInvalidInput)
	}
	return nil
}

func requireAlertRuleMutationRole(ctx context.Context, tx pgx.Tx, workspaceID, issuer, subject string) error {
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
		return fmt.Errorf("lock alert-rule workspace membership: %w", err)
	}
	switch role {
	case "owner", "administrator", "operator":
		return nil
	default:
		return store.ErrAlertRuleForbidden
	}
}

func scanAlertRule(row pgx.Row) (store.AlertRule, error) {
	var rule store.AlertRule
	err := row.Scan(
		&rule.ID, &rule.WorkspaceID, &rule.Name, &rule.Description,
		&rule.ServiceName, &rule.Metric, &rule.Operator, &rule.Threshold,
		&rule.WindowSeconds, &rule.PendingSeconds, &rule.Severity,
		&rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
	)
	return rule, err
}

func alertRuleAuditDetails(rule store.AlertRule) map[string]any {
	return map[string]any{
		"name": rule.Name,
		"description": rule.Description,
		"service_name": rule.ServiceName,
		"metric": rule.Metric,
		"operator": rule.Operator,
		"threshold": rule.Threshold,
		"window_seconds": rule.WindowSeconds,
		"pending_seconds": rule.PendingSeconds,
		"severity": rule.Severity,
		"enabled": rule.Enabled,
	}
}

func appendAlertRuleAuditTx(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, ruleID, actorIssuer, actorSubject, action, requestID string,
	details map[string]any,
) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode alert-rule audit details: %w", err)
	}
	if len(raw) > 16*1024 {
		return fmt.Errorf("%w: audit details exceed 16 KiB", store.ErrAlertRuleInvalidInput)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO workspace_audit_events (
			workspace_id, actor_issuer, actor_subject, action,
			resource_type, resource_id, request_id, details
		) VALUES ($1::uuid, $2, $3, $4, 'alert_rule', $5, $6, $7::jsonb)
	`, workspaceID, actorIssuer, actorSubject, action, ruleID, requestID, string(raw))
	if err != nil {
		return fmt.Errorf("append alert-rule workspace audit event: %w", err)
	}
	return nil
}

func mapAlertRuleConflict(err error) error {
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) && postgresErr.Code == "23505" &&
		postgresErr.ConstraintName == "alert_rules_workspace_name_unique" {
		return store.ErrAlertRuleNameConflict
	}
	return err
}

const alertRuleSelectColumns = `
	id::text, workspace_id::text, name, description, service_name, metric,
	operator, threshold, window_seconds, pending_seconds, severity,
	enabled, created_at, updated_at
`

func (r *Repository) CreateWorkspaceAlertRule(ctx context.Context, input store.CreateWorkspaceAlertRuleInput) (store.AlertRule, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.ServiceName = strings.TrimSpace(input.ServiceName)
	input.Metric = strings.TrimSpace(input.Metric)
	input.Operator = strings.TrimSpace(input.Operator)
	input.Severity = strings.TrimSpace(input.Severity)
	if !workspaceIDPattern.MatchString(input.WorkspaceID) {
		return store.AlertRule{}, fmt.Errorf("%w: workspace ID must be a UUID", store.ErrAlertRuleInvalidInput)
	}
	if err := validateAlertRuleFields(input.Name, input.Description, input.ServiceName,
		input.Metric, input.Operator, input.Threshold, input.WindowSeconds,
		input.PendingSeconds, input.Severity); err != nil {
		return store.AlertRule{}, err
	}
	if err := validateAlertRuleActor(input.ActorIssuer, input.ActorSubject); err != nil {
		return store.AlertRule{}, err
	}
	if err := validateAlertRuleRequestID(input.RequestID); err != nil {
		return store.AlertRule{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.AlertRule{}, fmt.Errorf("begin alert-rule creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireAlertRuleMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.AlertRule{}, err
	}

	rule, err := scanAlertRule(tx.QueryRow(ctx, `
		INSERT INTO alert_rules (
			workspace_id, name, description, service_name, metric, operator,
			threshold, window_seconds, pending_seconds, severity, enabled,
			created_by_issuer, created_by_subject, updated_by_issuer, updated_by_subject
		) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, TRUE, $11, $12, $11, $12)
		RETURNING `+alertRuleSelectColumns,
		input.WorkspaceID, input.Name, input.Description, input.ServiceName, input.Metric,
		input.Operator, input.Threshold, input.WindowSeconds, input.PendingSeconds,
		input.Severity, input.ActorIssuer, input.ActorSubject,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.AlertRule{}, err
		}
		return store.AlertRule{}, mapAlertRuleConflict(fmt.Errorf("insert alert rule: %w", err))
	}
	details := alertRuleAuditDetails(rule)
	if err := appendAlertRuleAuditTx(ctx, tx, input.WorkspaceID, rule.ID,
		input.ActorIssuer, input.ActorSubject, "alert_rule.created", input.RequestID, details); err != nil {
		return store.AlertRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AlertRule{}, fmt.Errorf("commit alert-rule creation: %w", err)
	}
	return rule, nil
}

func (r *Repository) ListWorkspaceAlertRules(
	ctx context.Context,
	workspaceID, issuer, subject string,
	limit int,
) ([]store.AlertRule, error) {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return nil, fmt.Errorf("%w: workspace ID must be a UUID", store.ErrAlertRuleInvalidInput)
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
		return nil, fmt.Errorf("verify alert-rule workspace membership: %w", err)
	}
	if !member {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+alertRuleSelectColumns+`
		FROM alert_rules
		WHERE workspace_id = $1::uuid
		ORDER BY enabled DESC, updated_at DESC, id DESC
		LIMIT $2
	`, workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query workspace alert rules: %w", err)
	}
	defer rows.Close()
	items := make([]store.AlertRule, 0)
	for rows.Next() {
		rule, err := scanAlertRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan workspace alert rule: %w", err)
		}
		items = append(items, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace alert rules: %w", err)
	}
	return items, nil
}

func (r *Repository) UpdateWorkspaceAlertRule(ctx context.Context, input store.UpdateWorkspaceAlertRuleInput) (store.AlertRule, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.ServiceName = strings.TrimSpace(input.ServiceName)
	input.Metric = strings.TrimSpace(input.Metric)
	input.Operator = strings.TrimSpace(input.Operator)
	input.Severity = strings.TrimSpace(input.Severity)
	if !workspaceIDPattern.MatchString(input.WorkspaceID) || !workspaceIDPattern.MatchString(input.RuleID) {
		return store.AlertRule{}, fmt.Errorf("%w: workspace and rule IDs must be UUIDs", store.ErrAlertRuleInvalidInput)
	}
	if err := validateAlertRuleFields(input.Name, input.Description, input.ServiceName,
		input.Metric, input.Operator, input.Threshold, input.WindowSeconds,
		input.PendingSeconds, input.Severity); err != nil {
		return store.AlertRule{}, err
	}
	if err := validateAlertRuleActor(input.ActorIssuer, input.ActorSubject); err != nil {
		return store.AlertRule{}, err
	}
	if err := validateAlertRuleRequestID(input.RequestID); err != nil {
		return store.AlertRule{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.AlertRule{}, fmt.Errorf("begin alert-rule update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireAlertRuleMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.AlertRule{}, err
	}
	var lockedID string
	if err := tx.QueryRow(ctx, `
		SELECT id::text FROM alert_rules
		WHERE id = $1::uuid AND workspace_id = $2::uuid
		FOR UPDATE
	`, input.RuleID, input.WorkspaceID).Scan(&lockedID); errors.Is(err, pgx.ErrNoRows) {
		return store.AlertRule{}, store.ErrAlertRuleNotFound
	} else if err != nil {
		return store.AlertRule{}, fmt.Errorf("lock alert rule for update: %w", err)
	}
	rule, err := scanAlertRule(tx.QueryRow(ctx, `
		UPDATE alert_rules
		SET name = $3, description = $4, service_name = $5, metric = $6,
		    operator = $7, threshold = $8, window_seconds = $9,
		    pending_seconds = $10, severity = $11,
		    updated_by_issuer = $12, updated_by_subject = $13, updated_at = NOW()
		WHERE id = $1::uuid AND workspace_id = $2::uuid
		RETURNING `+alertRuleSelectColumns,
		input.RuleID, input.WorkspaceID, input.Name, input.Description, input.ServiceName,
		input.Metric, input.Operator, input.Threshold, input.WindowSeconds,
		input.PendingSeconds, input.Severity, input.ActorIssuer, input.ActorSubject,
	))
	if err != nil {
		return store.AlertRule{}, mapAlertRuleConflict(fmt.Errorf("update alert rule: %w", err))
	}
	if err := resolveAlertInstancesForRuleTx(ctx, tx, input.WorkspaceID, rule.ID, "alert.rule_updated", time.Now().UTC()); err != nil {
		return store.AlertRule{}, err
	}
	if err := appendAlertRuleAuditTx(ctx, tx, input.WorkspaceID, rule.ID,
		input.ActorIssuer, input.ActorSubject, "alert_rule.updated", input.RequestID, alertRuleAuditDetails(rule)); err != nil {
		return store.AlertRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AlertRule{}, fmt.Errorf("commit alert-rule update: %w", err)
	}
	return rule, nil
}

func (r *Repository) SetWorkspaceAlertRuleEnabled(ctx context.Context, input store.SetWorkspaceAlertRuleEnabledInput) (store.AlertRule, error) {
	if !workspaceIDPattern.MatchString(input.WorkspaceID) || !workspaceIDPattern.MatchString(input.RuleID) {
		return store.AlertRule{}, fmt.Errorf("%w: workspace and rule IDs must be UUIDs", store.ErrAlertRuleInvalidInput)
	}
	if err := validateAlertRuleActor(input.ActorIssuer, input.ActorSubject); err != nil {
		return store.AlertRule{}, err
	}
	if err := validateAlertRuleRequestID(input.RequestID); err != nil {
		return store.AlertRule{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.AlertRule{}, fmt.Errorf("begin alert-rule enable update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireAlertRuleMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.AlertRule{}, err
	}
	var current bool
	err = tx.QueryRow(ctx, `
		SELECT enabled FROM alert_rules
		WHERE id = $1::uuid AND workspace_id = $2::uuid
		FOR UPDATE
	`, input.RuleID, input.WorkspaceID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.AlertRule{}, store.ErrAlertRuleNotFound
	}
	if err != nil {
		return store.AlertRule{}, fmt.Errorf("lock alert rule enabled state: %w", err)
	}
	if current == input.Enabled {
		return store.AlertRule{}, store.ErrAlertRuleNoChange
	}
	rule, err := scanAlertRule(tx.QueryRow(ctx, `
		UPDATE alert_rules
		SET enabled = $3, updated_by_issuer = $4, updated_by_subject = $5, updated_at = NOW()
		WHERE id = $1::uuid AND workspace_id = $2::uuid
		RETURNING `+alertRuleSelectColumns,
		input.RuleID, input.WorkspaceID, input.Enabled, input.ActorIssuer, input.ActorSubject,
	))
	if err != nil {
		return store.AlertRule{}, fmt.Errorf("update alert rule enabled state: %w", err)
	}
	action := "alert_rule.disabled"
	if input.Enabled {
		action = "alert_rule.enabled"
	} else if err := resolveAlertInstancesForRuleTx(ctx, tx, input.WorkspaceID, rule.ID, "alert.rule_disabled", time.Now().UTC()); err != nil {
		return store.AlertRule{}, err
	}
	if err := appendAlertRuleAuditTx(ctx, tx, input.WorkspaceID, rule.ID,
		input.ActorIssuer, input.ActorSubject, action, input.RequestID,
		map[string]any{"name": rule.Name, "enabled": rule.Enabled}); err != nil {
		return store.AlertRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AlertRule{}, fmt.Errorf("commit alert-rule enabled update: %w", err)
	}
	return rule, nil
}

func (r *Repository) DeleteWorkspaceAlertRule(ctx context.Context, input store.DeleteWorkspaceAlertRuleInput) error {
	if !workspaceIDPattern.MatchString(input.WorkspaceID) || !workspaceIDPattern.MatchString(input.RuleID) {
		return fmt.Errorf("%w: workspace and rule IDs must be UUIDs", store.ErrAlertRuleInvalidInput)
	}
	if err := validateAlertRuleActor(input.ActorIssuer, input.ActorSubject); err != nil {
		return err
	}
	if err := validateAlertRuleRequestID(input.RequestID); err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin alert-rule deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireAlertRuleMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return err
	}
	rule, err := scanAlertRule(tx.QueryRow(ctx, `
		SELECT `+alertRuleSelectColumns+`
		FROM alert_rules
		WHERE id = $1::uuid AND workspace_id = $2::uuid
		FOR UPDATE
	`, input.RuleID, input.WorkspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrAlertRuleNotFound
	}
	if err != nil {
		return fmt.Errorf("lock alert rule for deletion: %w", err)
	}
	if err := resolveAlertInstancesForRuleTx(ctx, tx, input.WorkspaceID, rule.ID, "alert.rule_deleted", time.Now().UTC()); err != nil {
		return err
	}
	if err := appendAlertRuleAuditTx(ctx, tx, input.WorkspaceID, rule.ID,
		input.ActorIssuer, input.ActorSubject, "alert_rule.deleted", input.RequestID, alertRuleAuditDetails(rule)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM alert_rules WHERE id = $1::uuid AND workspace_id = $2::uuid
	`, input.RuleID, input.WorkspaceID); err != nil {
		return fmt.Errorf("delete workspace alert rule: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit alert-rule deletion: %w", err)
	}
	return nil
}

var _ store.AlertRuleRepository = (*Repository)(nil)

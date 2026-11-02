package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

const (
	alertEvaluationLockClass int32 = 1330795073
	alertEvaluationLockID int32 = 2
	maxEnabledAlertRules = 1000
	// Two missed 30-second evaluation intervals break a continuous pending window.
	alertPendingObservationMaxGap = 60 * time.Second
)

func (r *Repository) TryAcquireAlertEvaluationLock(ctx context.Context) (func(), bool, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil { return nil, false, fmt.Errorf("acquire alert-evaluator connection: %w", err) }
	var acquired bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1, $2)", alertEvaluationLockClass, alertEvaluationLockID).Scan(&acquired); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("acquire distributed alert-evaluator lock: %w", err)
	}
	if !acquired { conn.Release(); return func() {}, false, nil }

	var once sync.Once
	release := func() {
		once.Do(func() {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var unlocked bool
			err := conn.QueryRow(unlockCtx, "SELECT pg_advisory_unlock($1, $2)", alertEvaluationLockClass, alertEvaluationLockID).Scan(&unlocked)
			if err != nil || !unlocked { _ = conn.Conn().Close(context.Background()) }
			conn.Release()
		})
	}
	return release, true, nil
}

func (r *Repository) ListEnabledWorkspaceAlertRules(ctx context.Context, limit int) ([]store.AlertRule, error) {
	if limit < 1 || limit > maxEnabledAlertRules {
		limit = maxEnabledAlertRules
	}
	// Read one extra row in the same statement so a concurrent rule insert
	// cannot slip between a separate COUNT and SELECT and bypass the cap.
	rows, err := r.pool.Query(ctx, `SELECT `+alertRuleSelectColumns+`
		FROM alert_rules WHERE enabled = TRUE ORDER BY workspace_id, id LIMIT $1`, limit+1)
	if err != nil {
		return nil, fmt.Errorf("list enabled workspace alert rules: %w", err)
	}
	defer rows.Close()
	items := make([]store.AlertRule, 0, limit+1)
	for rows.Next() {
		rule, err := scanAlertRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan enabled workspace alert rule: %w", err)
		}
		items = append(items, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enabled workspace alert rules: %w", err)
	}
	if len(items) > limit {
		return nil, fmt.Errorf("enabled alert rule count exceeds evaluator safety limit %d", limit)
	}
	return items, nil
}

func alertRuleMatches(rule store.AlertRule, value float64) bool {
	switch rule.Operator {
	case "gt": return value > rule.Threshold
	case "gte": return value >= rule.Threshold
	case "lt": return value < rule.Threshold
	case "lte": return value <= rule.Threshold
	default: return false
	}
}

func scanAlertInstance(row pgx.Row) (store.AlertInstance, error) {
	var item store.AlertInstance
	err := row.Scan(
		&item.ID, &item.WorkspaceID, &item.AlertRuleID, &item.RuleName,
		&item.ServiceName, &item.Metric, &item.Operator, &item.Threshold,
		&item.WindowSeconds, &item.PendingSeconds, &item.Severity,
		&item.Status, &item.Value, &item.PendingSince, &item.FiredAt,
		&item.ResolvedAt, &item.LastEvaluatedAt, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

const alertInstanceSelectColumns = `
	id::text, workspace_id::text, alert_rule_id::text, rule_name,
	service_name, metric, operator, threshold, window_seconds,
	pending_seconds, severity, status, value, pending_since, fired_at,
	resolved_at, last_evaluated_at, created_at, updated_at
`

func appendAlertInstanceEventTx(ctx context.Context, tx pgx.Tx, instance store.AlertInstance, eventType string, details map[string]any) error {
	if details == nil { details = map[string]any{} }
	raw, err := json.Marshal(details)
	if err != nil { return fmt.Errorf("encode alert-instance event details: %w", err) }
	if len(raw) > 16*1024 { return fmt.Errorf("alert-instance event details exceed 16 KiB") }
	var eventID string
	err = tx.QueryRow(ctx, `INSERT INTO alert_instance_events
		(workspace_id, alert_instance_id, alert_rule_id, event_type, details)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::jsonb)
		RETURNING id::text`,
		instance.WorkspaceID, instance.ID, instance.AlertRuleID, eventType, string(raw)).Scan(&eventID)
	if err != nil {
		return fmt.Errorf("append alert-instance event: %w", err)
	}
	if eventType == "alert.firing" || eventType == "alert.resolved" {
		_, err = tx.Exec(ctx, `
			INSERT INTO alert_notification_deliveries (
				workspace_id, alert_instance_id, alert_instance_event_id, channel_id, event_type
			)
			SELECT $1::uuid, $2::uuid, $3::uuid, channels.id, $4
			FROM alert_notification_channels AS channels
			WHERE channels.workspace_id = $1::uuid AND channels.enabled = TRUE
			ON CONFLICT (alert_instance_event_id, channel_id) DO NOTHING
		`, instance.WorkspaceID, instance.ID, eventID, eventType)
		if err != nil {
			return fmt.Errorf("enqueue alert-notification deliveries: %w", err)
		}
	}
	return nil
}

func (r *Repository) ApplyWorkspaceAlertSample(ctx context.Context, workspaceID, ruleID, serviceName string, value float64, evaluatedAt time.Time) (store.AlertInstance, bool, error) {
	if !workspaceIDPattern.MatchString(workspaceID) || !workspaceIDPattern.MatchString(ruleID) {
		return store.AlertInstance{}, false, fmt.Errorf("%w: workspace and alert-rule IDs must be UUIDs", store.ErrAlertRuleInvalidInput)
	}
	if !telemetryServiceNamePattern.MatchString(serviceName) {
		return store.AlertInstance{}, false, fmt.Errorf("%w: service name is invalid", store.ErrAlertRuleInvalidInput)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return store.AlertInstance{}, false, fmt.Errorf("%w: alert sample must be finite and non-negative", store.ErrAlertRuleInvalidInput)
	}
	if evaluatedAt.IsZero() { evaluatedAt = time.Now().UTC() } else { evaluatedAt = evaluatedAt.UTC() }

	tx, err := r.pool.Begin(ctx)
	if err != nil { return store.AlertInstance{}, false, fmt.Errorf("begin alert sample evaluation: %w", err) }
	defer func() { _ = tx.Rollback(ctx) }()

	lockKey := ruleID + ":" + serviceName
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return store.AlertInstance{}, false, fmt.Errorf("serialize alert sample evaluation: %w", err)
	}

	var rule store.AlertRule
	err = tx.QueryRow(ctx, `SELECT `+alertRuleSelectColumns+`
		FROM alert_rules WHERE id = $1::uuid AND workspace_id = $2::uuid AND enabled = TRUE
		FOR SHARE`, ruleID, workspaceID).Scan(
		&rule.ID, &rule.WorkspaceID, &rule.Name, &rule.Description,
		&rule.ServiceName, &rule.Metric, &rule.Operator, &rule.Threshold,
		&rule.WindowSeconds, &rule.PendingSeconds, &rule.Severity,
		&rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil { return store.AlertInstance{}, false, fmt.Errorf("finish skipped alert evaluation: %w", err) }
		return store.AlertInstance{}, false, nil
	}
	if err != nil { return store.AlertInstance{}, false, fmt.Errorf("read enabled alert rule for evaluation: %w", err) }
	if rule.ServiceName != "" && rule.ServiceName != serviceName {
		return store.AlertInstance{}, false, fmt.Errorf("%w: sample service does not match alert rule", store.ErrAlertRuleInvalidInput)
	}

	matched := alertRuleMatches(rule, value)
	var current store.AlertInstance
	err = tx.QueryRow(ctx, `SELECT `+alertInstanceSelectColumns+`
		FROM alert_instances
		WHERE alert_rule_id = $1::uuid AND workspace_id = $2::uuid AND service_name = $3
		  AND status IN ('pending', 'firing')
		FOR UPDATE`, ruleID, workspaceID, serviceName).Scan(
		&current.ID, &current.WorkspaceID, &current.AlertRuleID, &current.RuleName,
		&current.ServiceName, &current.Metric, &current.Operator, &current.Threshold,
		&current.WindowSeconds, &current.PendingSeconds, &current.Severity,
		&current.Status, &current.Value, &current.PendingSince, &current.FiredAt,
		&current.ResolvedAt, &current.LastEvaluatedAt, &current.CreatedAt, &current.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		if !matched {
			if err := tx.Commit(ctx); err != nil { return store.AlertInstance{}, false, fmt.Errorf("finish non-firing alert evaluation: %w", err) }
			return store.AlertInstance{}, false, nil
		}
		status, eventType := "pending", "alert.pending"
		var firedAt *time.Time
		if rule.PendingSeconds == 0 {
			status, eventType, firedAt = "firing", "alert.firing", &evaluatedAt
		}
		instance, err := scanAlertInstance(tx.QueryRow(ctx, `
			INSERT INTO alert_instances (
				workspace_id, alert_rule_id, rule_name, service_name, metric,
				operator, threshold, window_seconds, pending_seconds, severity,
				status, value, pending_since, fired_at, last_evaluated_at
			) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $13)
			RETURNING `+alertInstanceSelectColumns,
			workspaceID, rule.ID, rule.Name, serviceName, rule.Metric,
			rule.Operator, rule.Threshold, rule.WindowSeconds, rule.PendingSeconds,
			rule.Severity, status, value, evaluatedAt, firedAt,
		))
		if err != nil { return store.AlertInstance{}, false, fmt.Errorf("create alert instance: %w", err) }
		if err := appendAlertInstanceEventTx(ctx, tx, instance, eventType, map[string]any{
			"value": value, "threshold": rule.Threshold, "operator": rule.Operator,
			"window_seconds": rule.WindowSeconds, "pending_seconds": rule.PendingSeconds,
		}); err != nil { return store.AlertInstance{}, false, err }
		if err := tx.Commit(ctx); err != nil { return store.AlertInstance{}, false, fmt.Errorf("commit alert instance creation: %w", err) }
		return instance, true, nil
	}
	if err != nil { return store.AlertInstance{}, false, fmt.Errorf("lock active alert instance: %w", err) }

	oldStatus, nextStatus := current.Status, current.Status
	eventType := ""
	firedAt := current.FiredAt
	resolvedAt := (*time.Time)(nil)
	pendingSince := current.PendingSince
	if matched {
		if oldStatus == "pending" {
			if evaluatedAt.Sub(current.LastEvaluatedAt) >= alertPendingObservationMaxGap {
				// Missing telemetry is not recovery, but an unobserved interval
				// cannot count toward the rule's continuous pending duration.
				pendingSince = evaluatedAt
				eventType = "alert.pending"
			} else if evaluatedAt.Sub(current.PendingSince) >= time.Duration(rule.PendingSeconds)*time.Second {
				nextStatus, firedAt, eventType = "firing", &evaluatedAt, "alert.firing"
			}
		}
	} else if oldStatus == "pending" {
		nextStatus, resolvedAt, eventType = "cancelled", &evaluatedAt, "alert.pending_cancelled"
	} else if oldStatus == "firing" {
		nextStatus, resolvedAt, eventType = "resolved", &evaluatedAt, "alert.resolved"
	}

	instance, err := scanAlertInstance(tx.QueryRow(ctx, `
		UPDATE alert_instances
		SET rule_name = $4, metric = $5, operator = $6, threshold = $7,
		    window_seconds = $8, pending_seconds = $9, severity = $10,
		    status = $11, value = $12, fired_at = $13, resolved_at = $14,
		    pending_since = $15, last_evaluated_at = $16, updated_at = NOW()
		WHERE id = $1::uuid AND workspace_id = $2::uuid AND alert_rule_id = $3::uuid
		RETURNING `+alertInstanceSelectColumns,
		current.ID, workspaceID, ruleID, rule.Name, rule.Metric, rule.Operator,
		rule.Threshold, rule.WindowSeconds, rule.PendingSeconds, rule.Severity,
		nextStatus, value, firedAt, resolvedAt, pendingSince, evaluatedAt,
	))
	if err != nil { return store.AlertInstance{}, false, fmt.Errorf("update alert instance evaluation: %w", err) }
	if eventType != "" {
		details := map[string]any{
			"from_status": oldStatus, "to_status": nextStatus, "value": value,
			"threshold": rule.Threshold, "operator": rule.Operator,
		}
		if eventType == "alert.pending" && !pendingSince.Equal(current.PendingSince) {
			details["reason"] = "telemetry_observation_gap"
			details["pending_since"] = pendingSince
		}
		if err := appendAlertInstanceEventTx(ctx, tx, instance, eventType, details); err != nil {
			return store.AlertInstance{}, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil { return store.AlertInstance{}, false, fmt.Errorf("commit alert instance evaluation: %w", err) }
	return instance, eventType != "", nil
}

func resolveAlertInstancesForRuleTx(ctx context.Context, tx pgx.Tx, workspaceID, ruleID, eventType string, at time.Time) error {
	rows, err := tx.Query(ctx, `SELECT `+alertInstanceSelectColumns+`
		FROM alert_instances
		WHERE workspace_id = $1::uuid AND alert_rule_id = $2::uuid
		  AND status IN ('pending', 'firing')
		FOR UPDATE`, workspaceID, ruleID)
	if err != nil { return fmt.Errorf("list active alert instances for rule close: %w", err) }
	items := make([]store.AlertInstance, 0)
	for rows.Next() {
		item, err := scanAlertInstance(rows)
		if err != nil { rows.Close(); return fmt.Errorf("scan active alert instance for rule close: %w", err) }
		items = append(items, item)
	}
	if err := rows.Err(); err != nil { rows.Close(); return fmt.Errorf("iterate active alert instances for rule close: %w", err) }
	rows.Close()
	for _, item := range items {
		nextStatus := "resolved"
		if item.Status == "pending" { nextStatus = "cancelled" }
		updated, err := scanAlertInstance(tx.QueryRow(ctx, `
			UPDATE alert_instances
			SET status = $3, resolved_at = $4, last_evaluated_at = $4, updated_at = NOW()
			WHERE id = $1::uuid AND workspace_id = $2::uuid
			RETURNING `+alertInstanceSelectColumns,
			item.ID, workspaceID, nextStatus, at,
		))
		if err != nil { return fmt.Errorf("close alert instance after rule change: %w", err) }
		if err := appendAlertInstanceEventTx(ctx, tx, updated, eventType, map[string]any{
			"from_status": item.Status, "to_status": nextStatus,
		}); err != nil { return err }
	}
	return nil
}

func (r *Repository) ListWorkspaceAlertInstances(ctx context.Context, workspaceID, issuer, subject, status string, limit int) ([]store.AlertInstance, error) {
	if !workspaceIDPattern.MatchString(workspaceID) { return nil, fmt.Errorf("%w: workspace ID must be a UUID", store.ErrAlertRuleInvalidInput) }
	if err := validateWorkspacePrincipal(issuer, subject); err != nil { return nil, err }
	switch status {
	case "", "pending", "firing", "resolved", "cancelled":
	default: return nil, fmt.Errorf("%w: alert status is invalid", store.ErrAlertRuleInvalidInput)
	}
	if limit < 1 || limit > 100 { limit = 50 }
	var member bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM workspace_memberships
		WHERE workspace_id = $1::uuid AND user_issuer = $2 AND user_subject = $3
	)`, workspaceID, issuer, subject).Scan(&member); err != nil {
		return nil, fmt.Errorf("verify alert-instance workspace membership: %w", err)
	}
	if !member { return nil, store.ErrWorkspaceMembershipNotFound }
	rows, err := r.pool.Query(ctx, `SELECT `+alertInstanceSelectColumns+`
		FROM alert_instances
		WHERE workspace_id = $1::uuid AND ($2 = '' OR status = $2)
		ORDER BY CASE status WHEN 'firing' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END,
		         updated_at DESC, id DESC
		LIMIT $3`, workspaceID, status, limit)
	if err != nil { return nil, fmt.Errorf("query workspace alert instances: %w", err) }
	defer rows.Close()
	items := make([]store.AlertInstance, 0)
	for rows.Next() {
		item, err := scanAlertInstance(rows)
		if err != nil { return nil, fmt.Errorf("scan workspace alert instance: %w", err) }
		items = append(items, item)
	}
	if err := rows.Err(); err != nil { return nil, fmt.Errorf("iterate workspace alert instances: %w", err) }
	return items, nil
}

var _ store.AlertEvaluationRepository = (*Repository)(nil)

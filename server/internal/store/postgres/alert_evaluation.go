package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

const (
	alertEvaluationLockClass int32 = 1330795073
	alertEvaluationLockObject int32 = 3
	maxAlertEvaluationRules = 500
)

func (r *Repository) WithAlertEvaluationLease(ctx context.Context, run func(context.Context) error) (bool, error) {
	if run == nil {
		return false, errors.New("alert evaluation callback is required")
	}
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire alert evaluator lease connection: %w", err)
	}
	var acquired bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1, $2)", alertEvaluationLockClass, alertEvaluationLockObject).Scan(&acquired); err != nil {
		conn.Release()
		return false, fmt.Errorf("acquire alert evaluator advisory lock: %w", err)
	}
	if !acquired {
		conn.Release()
		return false, nil
	}

	released := false
	release := func() error {
		if released {
			return nil
		}
		released = true
		unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var unlocked bool
		err := conn.QueryRow(unlockCtx,
			"SELECT pg_advisory_unlock($1, $2)",
			alertEvaluationLockClass, alertEvaluationLockObject,
		).Scan(&unlocked)
		if err != nil || !unlocked {
			// Never return a session with a possibly-held advisory lock to the pool.
			_ = conn.Conn().Close(context.Background())
			conn.Release()
			if err != nil {
				return fmt.Errorf("release alert evaluator advisory lock: %w", err)
			}
			return errors.New("alert evaluator advisory lock was not held at release")
		}
		conn.Release()
		return nil
	}
	defer func() {
		if !released {
			_ = release()
		}
	}()

	runErr := run(ctx)
	unlockErr := release()
	if runErr != nil && unlockErr != nil {
		return true, errors.Join(runErr, unlockErr)
	}
	if runErr != nil {
		return true, runErr
	}
	if unlockErr != nil {
		return true, unlockErr
	}
	return true, nil
}

func (r *Repository) ListEnabledAlertRulesForEvaluation(ctx context.Context, limit int) ([]store.AlertRule, error) {
	if limit < 1 || limit > maxAlertEvaluationRules {
		limit = maxAlertEvaluationRules
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+alertRuleSelectColumns+`
		FROM alert_rules
		WHERE enabled = TRUE
		ORDER BY workspace_id ASC, id ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list enabled alert rules for evaluation: %w", err)
	}
	defer rows.Close()
	items := make([]store.AlertRule, 0)
	for rows.Next() {
		rule, err := scanAlertRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan enabled alert rule: %w", err)
		}
		items = append(items, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enabled alert rules: %w", err)
	}
	return items, nil
}

func alertRuleMatches(rule store.AlertRule, value float64) bool {
	switch rule.Operator {
	case "gt":
		return value > rule.Threshold
	case "gte":
		return value >= rule.Threshold
	case "lt":
		return value < rule.Threshold
	case "lte":
		return value <= rule.Threshold
	default:
		return false
	}
}

func appendAlertEvaluationEventTx(
	ctx context.Context,
	tx pgx.Tx,
	rule store.AlertRule,
	eventType string,
	observedValue *float64,
	occurredAt time.Time,
	details map[string]any,
) error {
	if details == nil {
		details = map[string]any{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode alert event details: %w", err)
	}
	if len(raw) > 16*1024 {
		return fmt.Errorf("alert event details exceed 16 KiB")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO alert_rule_events (
			workspace_id, rule_id, rule_name, event_type, severity,
			metric, operator, threshold, observed_value, details, occurred_at
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11)
	`, rule.WorkspaceID, rule.ID, rule.Name, eventType, rule.Severity,
		rule.Metric, rule.Operator, rule.Threshold, observedValue, string(raw), occurredAt)
	if err != nil {
		return fmt.Errorf("append alert rule event: %w", err)
	}
	return nil
}

func scanAlertRuleState(row pgx.Row) (store.AlertRuleState, error) {
	var state store.AlertRuleState
	err := row.Scan(
		&state.RuleID, &state.WorkspaceID, &state.State, &state.ActiveSince,
		&state.LastValue, &state.LastEvaluatedAt, &state.LastFiredAt, &state.LastResolvedAt,
	)
	return state, err
}

func (r *Repository) ApplyAlertRuleObservation(ctx context.Context, input store.AlertRuleObservation) (store.AlertRuleState, error) {
	if !workspaceIDPattern.MatchString(input.RuleID) {
		return store.AlertRuleState{}, fmt.Errorf("%w: alert rule ID must be a UUID", store.ErrAlertRuleInvalidInput)
	}
	if input.ObservedAt.IsZero() {
		return store.AlertRuleState{}, fmt.Errorf("%w: observation timestamp is required", store.ErrAlertRuleInvalidInput)
	}
	observedAt := input.ObservedAt.UTC()
	if input.Value != nil && (math.IsNaN(*input.Value) || math.IsInf(*input.Value, 0) || *input.Value < 0) {
		return store.AlertRuleState{}, fmt.Errorf("%w: observed request rate must be finite and nonnegative", store.ErrAlertRuleInvalidInput)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.AlertRuleState{}, fmt.Errorf("begin alert rule evaluation update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rule, err := scanAlertRule(tx.QueryRow(ctx, `
		SELECT `+alertRuleSelectColumns+`
		FROM alert_rules
		WHERE id = $1::uuid
		FOR UPDATE
	`, input.RuleID))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.AlertRuleState{}, store.ErrAlertRuleNotFound
	}
	if err != nil {
		return store.AlertRuleState{}, fmt.Errorf("lock alert rule for evaluation: %w", err)
	}
	if !rule.Enabled {
		return store.AlertRuleState{}, store.ErrAlertRuleDisabled
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO alert_rule_states (rule_id, workspace_id, state)
		VALUES ($1::uuid, $2::uuid, 'normal')
		ON CONFLICT (rule_id) DO NOTHING
	`, rule.ID, rule.WorkspaceID); err != nil {
		return store.AlertRuleState{}, fmt.Errorf("initialize alert rule state: %w", err)
	}

	state, err := scanAlertRuleState(tx.QueryRow(ctx, `
		SELECT rule_id::text, workspace_id::text, state, active_since,
		       last_value, last_evaluated_at, last_fired_at, last_resolved_at
		FROM alert_rule_states
		WHERE rule_id = $1::uuid AND workspace_id = $2::uuid
		FOR UPDATE
	`, rule.ID, rule.WorkspaceID))
	if err != nil {
		return store.AlertRuleState{}, fmt.Errorf("lock alert rule evaluation state: %w", err)
	}

	if input.Value == nil {
		// Missing data is not a healthy sample. Preserve pending/firing state and
		// do not resolve an alert solely because the telemetry backend has a gap.
		if _, err := tx.Exec(ctx, `
			UPDATE alert_rule_states
			SET last_value = NULL, last_evaluated_at = $3, updated_at = NOW()
			WHERE rule_id = $1::uuid AND workspace_id = $2::uuid
		`, rule.ID, rule.WorkspaceID, observedAt); err != nil {
			return store.AlertRuleState{}, fmt.Errorf("record missing alert rule sample: %w", err)
		}
		state.LastValue = nil
		state.LastEvaluatedAt = &observedAt
		if err := tx.Commit(ctx); err != nil {
			return store.AlertRuleState{}, fmt.Errorf("commit missing alert rule sample: %w", err)
		}
		return state, nil
	}

	value := *input.Value
	nextState := state.State
	activeSince := state.ActiveSince
	lastFiredAt := state.LastFiredAt
	lastResolvedAt := state.LastResolvedAt
	eventType := ""
	details := map[string]any{
		"operator": rule.Operator,
		"threshold": rule.Threshold,
		"window_seconds": rule.WindowSeconds,
		"pending_seconds": rule.PendingSeconds,
	}

	if alertRuleMatches(rule, value) {
		switch state.State {
		case "normal":
			activeSince = &observedAt
			if rule.PendingSeconds == 0 {
				nextState = "firing"
				lastFiredAt = &observedAt
				eventType = "alert.firing"
			} else {
				nextState = "pending"
				eventType = "alert.pending"
			}
		case "pending":
			if activeSince == nil {
				// Recover conservatively from a malformed/legacy pending row.
				activeSince = &observedAt
			}
			if observedAt.Sub(*activeSince) >= time.Duration(rule.PendingSeconds)*time.Second {
				nextState = "firing"
				lastFiredAt = &observedAt
				eventType = "alert.firing"
			}
		case "firing":
			// Remain firing; event history records state transitions, not samples.
		default:
			return store.AlertRuleState{}, fmt.Errorf("unknown persisted alert rule state %q", state.State)
		}
	} else {
		switch state.State {
		case "pending":
			nextState = "normal"
			activeSince = nil
			eventType = "alert.pending_cleared"
		case "firing":
			nextState = "normal"
			activeSince = nil
			lastResolvedAt = &observedAt
			eventType = "alert.resolved"
		case "normal":
			// No transition.
		default:
			return store.AlertRuleState{}, fmt.Errorf("unknown persisted alert rule state %q", state.State)
		}
	}
	details["previous_state"] = state.State
	details["state"] = nextState

	if eventType != "" {
		if err := appendAlertEvaluationEventTx(ctx, tx, rule, eventType, input.Value, observedAt, details); err != nil {
			return store.AlertRuleState{}, err
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE alert_rule_states
		SET state = $3, active_since = $4, last_value = $5,
		    last_evaluated_at = $6, last_fired_at = $7,
		    last_resolved_at = $8, updated_at = NOW()
		WHERE rule_id = $1::uuid AND workspace_id = $2::uuid
	`, rule.ID, rule.WorkspaceID, nextState, activeSince, value, observedAt, lastFiredAt, lastResolvedAt)
	if err != nil {
		return store.AlertRuleState{}, fmt.Errorf("persist alert rule evaluation state: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AlertRuleState{}, fmt.Errorf("commit alert rule evaluation state: %w", err)
	}
	return store.AlertRuleState{
		RuleID: rule.ID,
		WorkspaceID: rule.WorkspaceID,
		State: nextState,
		ActiveSince: activeSince,
		LastValue: &value,
		LastEvaluatedAt: &observedAt,
		LastFiredAt: lastFiredAt,
		LastResolvedAt: lastResolvedAt,
	}, nil
}

func (r *Repository) clearAlertRuleStateTx(ctx context.Context, tx pgx.Tx, rule store.AlertRule, eventType string, occurredAt time.Time) error {
	var state string
	var lastValue *float64
	err := tx.QueryRow(ctx, `
		SELECT state, last_value
		FROM alert_rule_states
		WHERE rule_id = $1::uuid AND workspace_id = $2::uuid
		FOR UPDATE
	`, rule.ID, rule.WorkspaceID).Scan(&state, &lastValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock alert rule state before clearing: %w", err)
	}
	if state != "normal" {
		if err := appendAlertEvaluationEventTx(ctx, tx, rule, eventType, lastValue, occurredAt,
			map[string]any{"previous_state": state, "state": "normal"}); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE alert_rule_states
		SET state = 'normal', active_since = NULL, last_value = NULL, updated_at = NOW()
		WHERE rule_id = $1::uuid AND workspace_id = $2::uuid
	`, rule.ID, rule.WorkspaceID)
	if err != nil {
		return fmt.Errorf("clear alert rule state: %w", err)
	}
	return nil
}

func (r *Repository) GetWorkspaceAlertRuleState(
	ctx context.Context,
	workspaceID, ruleID, issuer, subject string,
) (store.AlertRuleState, error) {
	if !workspaceIDPattern.MatchString(workspaceID) || !workspaceIDPattern.MatchString(ruleID) {
		return store.AlertRuleState{}, fmt.Errorf("%w: workspace and rule IDs must be UUIDs", store.ErrAlertRuleInvalidInput)
	}
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
		return store.AlertRuleState{}, err
	}
	var member bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM workspace_memberships
			WHERE workspace_id = $1::uuid AND user_issuer = $2 AND user_subject = $3
		)
	`, workspaceID, issuer, subject).Scan(&member); err != nil {
		return store.AlertRuleState{}, fmt.Errorf("verify alert-state membership: %w", err)
	}
	if !member {
		return store.AlertRuleState{}, store.ErrWorkspaceMembershipNotFound
	}
	state, err := scanAlertRuleState(r.pool.QueryRow(ctx, `
		SELECT r.id::text, r.workspace_id::text, COALESCE(s.state, 'normal'),
		       s.active_since, s.last_value, s.last_evaluated_at, s.last_fired_at, s.last_resolved_at
		FROM alert_rules r
		LEFT JOIN alert_rule_states s
		  ON s.rule_id = r.id AND s.workspace_id = r.workspace_id
		WHERE r.id = $1::uuid AND r.workspace_id = $2::uuid
	`, ruleID, workspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.AlertRuleState{}, store.ErrAlertRuleNotFound
	}
	if err != nil {
		return store.AlertRuleState{}, fmt.Errorf("query workspace alert rule state: %w", err)
	}
	return state, nil
}

func (r *Repository) ListWorkspaceAlertRuleEvents(
	ctx context.Context,
	workspaceID, ruleID, issuer, subject string,
	limit int,
) ([]store.AlertRuleEvent, error) {
	if !workspaceIDPattern.MatchString(workspaceID) || !workspaceIDPattern.MatchString(ruleID) {
		return nil, fmt.Errorf("%w: workspace and rule IDs must be UUIDs", store.ErrAlertRuleInvalidInput)
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
		return nil, fmt.Errorf("verify alert event membership: %w", err)
	}
	if !member {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM alert_rules WHERE id = $1::uuid AND workspace_id = $2::uuid
			UNION ALL
			SELECT 1 FROM alert_rule_events WHERE rule_id = $1::uuid AND workspace_id = $2::uuid
		)
	`, ruleID, workspaceID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("verify alert rule event scope: %w", err)
	}
	if !exists {
		return nil, store.ErrAlertRuleNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, workspace_id::text, rule_id::text, rule_name,
		       event_type, severity, metric, operator, threshold,
		       observed_value, details, occurred_at
		FROM alert_rule_events
		WHERE workspace_id = $1::uuid AND rule_id = $2::uuid
		ORDER BY occurred_at DESC, id DESC
		LIMIT $3
	`, workspaceID, ruleID, limit)
	if err != nil {
		return nil, fmt.Errorf("query workspace alert rule events: %w", err)
	}
	defer rows.Close()
	items := make([]store.AlertRuleEvent, 0)
	for rows.Next() {
		var item store.AlertRuleEvent
		var details []byte
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.RuleID, &item.RuleName,
			&item.EventType, &item.Severity, &item.Metric, &item.Operator,
			&item.Threshold, &item.ObservedValue, &details, &item.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("scan alert rule event: %w", err)
		}
		if err := json.Unmarshal(details, &item.Details); err != nil {
			return nil, fmt.Errorf("decode alert rule event details: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace alert rule events: %w", err)
	}
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
	return items, nil
}

var _ store.AlertRuleEvaluationRepository = (*Repository)(nil)

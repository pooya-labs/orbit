package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

var alertNotificationSecretRefPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

const alertNotificationChannelSelectColumns = `
	id::text, workspace_id::text, name, adapter, secret_ref,
	enabled, created_at, updated_at
`

func scanAlertNotificationChannel(row pgx.Row) (store.AlertNotificationChannel, error) {
	var channel store.AlertNotificationChannel
	err := row.Scan(
		&channel.ID, &channel.WorkspaceID, &channel.Name, &channel.Adapter,
		&channel.SecretRef, &channel.Enabled, &channel.CreatedAt, &channel.UpdatedAt,
	)
	return channel, err
}

func appendAlertNotificationAuditTx(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, channelID, actorIssuer, actorSubject, action, requestID string,
	details map[string]any,
) error {
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode alert-notification audit details: %w", err)
	}
	if len(raw) > 16*1024 {
		return fmt.Errorf("%w: audit details exceed 16 KiB", store.ErrAlertNotificationInvalidInput)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO workspace_audit_events (
			workspace_id, actor_issuer, actor_subject, action,
			resource_type, resource_id, request_id, details
		) VALUES ($1::uuid, $2, $3, $4, 'alert_notification_channel', $5, $6, $7::jsonb)
	`, workspaceID, actorIssuer, actorSubject, action, channelID, requestID, string(raw))
	if err != nil {
		return fmt.Errorf("append alert-notification workspace audit event: %w", err)
	}
	return nil
}

func (r *Repository) CreateWorkspaceAlertNotificationChannel(
	ctx context.Context,
	input store.CreateAlertNotificationChannelInput,
) (store.AlertNotificationChannel, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.SecretRef = strings.TrimSpace(input.SecretRef)
	if !workspaceIDPattern.MatchString(input.WorkspaceID) {
		return store.AlertNotificationChannel{}, fmt.Errorf("%w: workspace ID must be a UUID", store.ErrAlertNotificationInvalidInput)
	}
	if len(input.Name) < 1 || len(input.Name) > 80 {
		return store.AlertNotificationChannel{}, fmt.Errorf("%w: channel name must contain between 1 and 80 characters", store.ErrAlertNotificationInvalidInput)
	}
	if !alertNotificationSecretRefPattern.MatchString(input.SecretRef) {
		return store.AlertNotificationChannel{}, fmt.Errorf("%w: secret reference is invalid", store.ErrAlertNotificationInvalidInput)
	}
	if err := validateAlertRuleActor(input.ActorIssuer, input.ActorSubject); err != nil {
		return store.AlertNotificationChannel{}, fmt.Errorf("%w: actor principal is invalid", store.ErrAlertNotificationInvalidInput)
	}
	if err := validateAlertRuleRequestID(input.RequestID); err != nil {
		return store.AlertNotificationChannel{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.AlertNotificationChannel{}, fmt.Errorf("begin alert-notification channel creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireAlertRuleMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.AlertNotificationChannel{}, err
	}

	channel, err := scanAlertNotificationChannel(tx.QueryRow(ctx, `
		INSERT INTO alert_notification_channels (
			workspace_id, name, secret_ref, created_by_issuer, created_by_subject,
			updated_by_issuer, updated_by_subject
		) VALUES ($1::uuid, $2, $3, $4, $5, $4, $5)
		RETURNING `+alertNotificationChannelSelectColumns,
		input.WorkspaceID, input.Name, input.SecretRef, input.ActorIssuer, input.ActorSubject,
	))
	if err != nil {
		var postgresErr *pgconn.PgError
		if errors.As(err, &postgresErr) && postgresErr.Code == "23505" &&
			postgresErr.ConstraintName == "alert_notification_channels_workspace_name_unique" {
			return store.AlertNotificationChannel{}, store.ErrAlertNotificationChannelConflict
		}
		return store.AlertNotificationChannel{}, fmt.Errorf("insert alert-notification channel: %w", err)
	}
	if err := appendAlertNotificationAuditTx(ctx, tx, input.WorkspaceID, channel.ID,
		input.ActorIssuer, input.ActorSubject, "alert_notification_channel.created", input.RequestID,
		map[string]any{"name": channel.Name, "adapter": channel.Adapter}); err != nil {
		return store.AlertNotificationChannel{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AlertNotificationChannel{}, fmt.Errorf("commit alert-notification channel creation: %w", err)
	}
	return channel, nil
}

func (r *Repository) ListWorkspaceAlertNotificationChannels(
	ctx context.Context,
	workspaceID, issuer, subject string,
	limit int,
) ([]store.AlertNotificationChannel, error) {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return nil, fmt.Errorf("%w: workspace ID must be a UUID", store.ErrAlertNotificationInvalidInput)
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
		return nil, fmt.Errorf("verify notification-channel workspace membership: %w", err)
	}
	if !member {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+alertNotificationChannelSelectColumns+`
		FROM alert_notification_channels
		WHERE workspace_id = $1::uuid
		ORDER BY enabled DESC, created_at DESC, id DESC
		LIMIT $2
	`, workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query workspace alert-notification channels: %w", err)
	}
	defer rows.Close()
	items := make([]store.AlertNotificationChannel, 0)
	for rows.Next() {
		item, err := scanAlertNotificationChannel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan alert-notification channel: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alert-notification channels: %w", err)
	}
	return items, nil
}

func (r *Repository) SetWorkspaceAlertNotificationChannelEnabled(
	ctx context.Context,
	input store.SetAlertNotificationChannelEnabledInput,
) (store.AlertNotificationChannel, error) {
	if !workspaceIDPattern.MatchString(input.WorkspaceID) || !workspaceIDPattern.MatchString(input.ChannelID) {
		return store.AlertNotificationChannel{}, fmt.Errorf("%w: workspace and channel IDs must be UUIDs", store.ErrAlertNotificationInvalidInput)
	}
	if err := validateAlertRuleActor(input.ActorIssuer, input.ActorSubject); err != nil {
		return store.AlertNotificationChannel{}, fmt.Errorf("%w: actor principal is invalid", store.ErrAlertNotificationInvalidInput)
	}
	if err := validateAlertRuleRequestID(input.RequestID); err != nil {
		return store.AlertNotificationChannel{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return store.AlertNotificationChannel{}, fmt.Errorf("begin alert-notification channel state update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireAlertRuleMutationRole(ctx, tx, input.WorkspaceID, input.ActorIssuer, input.ActorSubject); err != nil {
		return store.AlertNotificationChannel{}, err
	}
	current, err := scanAlertNotificationChannel(tx.QueryRow(ctx, `
		SELECT `+alertNotificationChannelSelectColumns+`
		FROM alert_notification_channels
		WHERE id = $1::uuid AND workspace_id = $2::uuid
		FOR UPDATE
	`, input.ChannelID, input.WorkspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.AlertNotificationChannel{}, store.ErrAlertNotificationChannelNotFound
	}
	if err != nil {
		return store.AlertNotificationChannel{}, fmt.Errorf("lock alert-notification channel: %w", err)
	}
	if current.Enabled == input.Enabled {
		return store.AlertNotificationChannel{}, store.ErrAlertNotificationNoChange
	}
	channel, err := scanAlertNotificationChannel(tx.QueryRow(ctx, `
		UPDATE alert_notification_channels
		SET enabled = $3, updated_by_issuer = $4, updated_by_subject = $5, updated_at = NOW()
		WHERE id = $1::uuid AND workspace_id = $2::uuid
		RETURNING `+alertNotificationChannelSelectColumns,
		input.ChannelID, input.WorkspaceID, input.Enabled, input.ActorIssuer, input.ActorSubject,
	))
	if err != nil {
		return store.AlertNotificationChannel{}, fmt.Errorf("update alert-notification channel state: %w", err)
	}
	action := "alert_notification_channel.disabled"
	if input.Enabled {
		action = "alert_notification_channel.enabled"
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE alert_notification_deliveries
			SET status = 'dead', lease_until = NULL, last_error = 'channel_disabled', updated_at = NOW()
			WHERE channel_id = $1::uuid AND workspace_id = $2::uuid AND status = 'pending'
		`, input.ChannelID, input.WorkspaceID); err != nil {
			return store.AlertNotificationChannel{}, fmt.Errorf("stop queued deliveries for disabled channel: %w", err)
		}
	}
	if err := appendAlertNotificationAuditTx(ctx, tx, input.WorkspaceID, channel.ID,
		input.ActorIssuer, input.ActorSubject, action, input.RequestID,
		map[string]any{"name": channel.Name, "enabled": channel.Enabled}); err != nil {
		return store.AlertNotificationChannel{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AlertNotificationChannel{}, fmt.Errorf("commit alert-notification channel state update: %w", err)
	}
	return channel, nil
}

func (r *Repository) ListWorkspaceAlertNotificationDeliveries(
	ctx context.Context,
	workspaceID, issuer, subject, status string,
	limit int,
) ([]store.AlertNotificationDelivery, error) {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return nil, fmt.Errorf("%w: workspace ID must be a UUID", store.ErrAlertNotificationInvalidInput)
	}
	if err := validateWorkspacePrincipal(issuer, subject); err != nil {
		return nil, err
	}
	switch status {
	case "", "pending", "processing", "delivered", "dead":
	default:
		return nil, fmt.Errorf("%w: delivery status is invalid", store.ErrAlertNotificationInvalidInput)
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
		return nil, fmt.Errorf("verify notification-delivery workspace membership: %w", err)
	}
	if !member {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT d.id::text, d.workspace_id::text, d.alert_instance_id::text,
		       d.alert_instance_event_id::text, d.channel_id::text, c.name,
		       d.event_type, d.status, d.attempts, d.next_attempt_at, d.last_attempt_at,
		       d.delivered_at, d.last_error, d.created_at, d.updated_at,
		       i.rule_name, i.service_name, i.metric, i.operator, i.threshold, i.value,
		       i.severity, i.status, e.created_at
		FROM alert_notification_deliveries d
		JOIN alert_notification_channels c ON c.id = d.channel_id AND c.workspace_id = d.workspace_id
		JOIN alert_instances i ON i.id = d.alert_instance_id AND i.workspace_id = d.workspace_id
		JOIN alert_instance_events e ON e.id = d.alert_instance_event_id AND e.workspace_id = d.workspace_id
		WHERE d.workspace_id = $1::uuid AND ($2 = '' OR d.status = $2)
		ORDER BY d.created_at DESC, d.id DESC
		LIMIT $3
	`, workspaceID, status, limit)
	if err != nil {
		return nil, fmt.Errorf("query workspace alert-notification deliveries: %w", err)
	}
	defer rows.Close()
	items := make([]store.AlertNotificationDelivery, 0)
	for rows.Next() {
		var item store.AlertNotificationDelivery
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.AlertInstanceID, &item.AlertInstanceEventID,
			&item.ChannelID, &item.ChannelName, &item.EventType, &item.Status, &item.Attempts,
			&item.NextAttemptAt, &item.LastAttemptAt, &item.DeliveredAt, &item.LastError,
			&item.CreatedAt, &item.UpdatedAt, &item.RuleName, &item.ServiceName,
			&item.Metric, &item.Operator, &item.Threshold, &item.Value, &item.Severity,
			&item.InstanceStatus, &item.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("scan alert-notification delivery: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alert-notification deliveries: %w", err)
	}
	return items, nil
}

func (r *Repository) ListWorkspaceAlertNotificationAttempts(
	ctx context.Context,
	workspaceID, deliveryID, issuer, subject string,
	limit int,
) ([]store.AlertNotificationAttempt, error) {
	if !workspaceIDPattern.MatchString(workspaceID) || !workspaceIDPattern.MatchString(deliveryID) {
		return nil, fmt.Errorf("%w: workspace and delivery IDs must be UUIDs", store.ErrAlertNotificationInvalidInput)
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
		return nil, fmt.Errorf("verify notification-attempt workspace membership: %w", err)
	}
	if !member {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT a.id::text, a.delivery_id::text, a.attempt_number, a.outcome,
		       a.http_status, a.error_code, a.attempted_at
		FROM alert_notification_attempts a
		JOIN alert_notification_deliveries d ON d.id = a.delivery_id AND d.workspace_id = a.workspace_id
		WHERE a.workspace_id = $1::uuid AND a.delivery_id = $2::uuid
		ORDER BY a.attempt_number DESC, a.id DESC
		LIMIT $3
	`, workspaceID, deliveryID, limit)
	if err != nil {
		return nil, fmt.Errorf("query workspace alert-notification attempts: %w", err)
	}
	defer rows.Close()
	items := make([]store.AlertNotificationAttempt, 0)
	for rows.Next() {
		var item store.AlertNotificationAttempt
		if err := rows.Scan(&item.ID, &item.DeliveryID, &item.AttemptNumber,
			&item.Outcome, &item.HTTPStatus, &item.ErrorCode, &item.AttemptedAt); err != nil {
			return nil, fmt.Errorf("scan alert-notification attempt: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alert-notification attempts: %w", err)
	}
	return items, nil
}

func (r *Repository) ClaimAlertNotificationDeliveries(ctx context.Context, limit, leaseSeconds int) ([]store.AlertNotificationDelivery, error) {
	if limit < 1 || limit > 20 {
		limit = 10
	}
	if leaseSeconds < 10 || leaseSeconds > 300 {
		leaseSeconds = 60
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin alert-notification delivery claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE alert_notification_deliveries
		SET status = 'dead', lease_until = NULL, last_error = 'attempt_limit_reached', updated_at = NOW()
		WHERE status = 'processing' AND lease_until <= NOW() AND attempts >= 8
	`); err != nil {
		return nil, fmt.Errorf("expire exhausted alert-notification deliveries: %w", err)
	}
	rows, err := tx.Query(ctx, `
		WITH picked AS (
			SELECT d.id
			FROM alert_notification_deliveries d
			JOIN alert_notification_channels c
			  ON c.id = d.channel_id AND c.workspace_id = d.workspace_id
			WHERE c.enabled = TRUE
			  AND d.attempts < 8
			  AND ((d.status = 'pending' AND d.next_attempt_at <= NOW())
			       OR (d.status = 'processing' AND d.lease_until <= NOW()))
			ORDER BY d.next_attempt_at ASC, d.created_at ASC, d.id ASC
			FOR UPDATE OF d SKIP LOCKED
			LIMIT $1
		),
		claimed AS (
			UPDATE alert_notification_deliveries d
			SET status = 'processing',
			    attempts = d.attempts + 1,
			    lease_until = NOW() + make_interval(secs => $2::double precision),
			    last_attempt_at = NOW(),
			    updated_at = NOW()
			FROM picked
			WHERE d.id = picked.id
			RETURNING d.*
		)
		SELECT d.id::text, d.workspace_id::text, d.alert_instance_id::text,
		       d.alert_instance_event_id::text, d.channel_id::text, c.name, c.secret_ref,
		       d.event_type, d.status, d.attempts, d.next_attempt_at, d.created_at,
		       i.rule_name, i.service_name, i.metric, i.operator, i.threshold, i.value,
		       i.severity, i.status, e.created_at
		FROM claimed d
		JOIN alert_notification_channels c ON c.id = d.channel_id AND c.workspace_id = d.workspace_id
		JOIN alert_instances i ON i.id = d.alert_instance_id AND i.workspace_id = d.workspace_id
		JOIN alert_instance_events e ON e.id = d.alert_instance_event_id AND e.workspace_id = d.workspace_id
		ORDER BY d.next_attempt_at ASC, d.created_at ASC, d.id ASC
	`, limit, leaseSeconds)
	if err != nil {
		return nil, fmt.Errorf("claim queued alert-notification deliveries: %w", err)
	}
	items := make([]store.AlertNotificationDelivery, 0)
	for rows.Next() {
		var item store.AlertNotificationDelivery
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.AlertInstanceID, &item.AlertInstanceEventID,
			&item.ChannelID, &item.ChannelName, &item.SecretRef, &item.EventType, &item.Status,
			&item.Attempts, &item.NextAttemptAt, &item.CreatedAt,
			&item.RuleName, &item.ServiceName, &item.Metric, &item.Operator,
			&item.Threshold, &item.Value, &item.Severity, &item.InstanceStatus, &item.OccurredAt,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan claimed alert-notification delivery: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate claimed alert-notification deliveries: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit alert-notification delivery claim: %w", err)
	}
	return items, nil
}

func (r *Repository) RecordAlertNotificationAttempt(
	ctx context.Context,
	input store.RecordAlertNotificationAttemptInput,
) error {
	if !workspaceIDPattern.MatchString(input.DeliveryID) ||
		input.AttemptNumber < 1 || input.AttemptNumber > 8 ||
		len(input.ErrorCode) > 80 {
		return fmt.Errorf("%w: attempt record is invalid", store.ErrAlertNotificationInvalidInput)
	}
	switch input.Outcome {
	case "delivered", "retry_scheduled", "dead":
	default:
		return fmt.Errorf("%w: attempt outcome is invalid", store.ErrAlertNotificationInvalidInput)
	}
	if input.Outcome == "retry_scheduled" && input.NextAttemptAt.IsZero() {
		return fmt.Errorf("%w: retry time is required", store.ErrAlertNotificationInvalidInput)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin alert-notification attempt record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var workspaceID string
	var status string
	var leaseUntil *time.Time
	var deliveredAt *time.Time
	var lastError string
	switch input.Outcome {
	case "delivered":
		status, leaseUntil, deliveredAt, lastError = "delivered", nil, ptrTime(time.Now().UTC()), ""
	case "retry_scheduled":
		status, leaseUntil, deliveredAt, lastError = "pending", nil, nil, input.ErrorCode
	case "dead":
		status, leaseUntil, deliveredAt, lastError = "dead", nil, nil, input.ErrorCode
	}
	err = tx.QueryRow(ctx, `
		UPDATE alert_notification_deliveries
		SET status = $3, lease_until = $4, delivered_at = $5,
		    next_attempt_at = CASE WHEN $3 = 'pending' THEN $6 ELSE next_attempt_at END,
		    last_error = $7, updated_at = NOW()
		WHERE id = $1::uuid AND status = 'processing' AND attempts = $2
		RETURNING workspace_id::text
	`, input.DeliveryID, input.AttemptNumber, status, leaseUntil, deliveredAt,
		input.NextAttemptAt, lastError).Scan(&workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("alert-notification delivery lease is no longer owned")
	}
	if err != nil {
		return fmt.Errorf("update alert-notification delivery outcome: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO alert_notification_attempts (
			workspace_id, delivery_id, attempt_number, outcome, http_status, error_code
		) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
	`, workspaceID, input.DeliveryID, input.AttemptNumber, input.Outcome, input.HTTPStatus, input.ErrorCode)
	if err != nil {
		return fmt.Errorf("append alert-notification attempt history: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit alert-notification attempt: %w", err)
	}
	return nil
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

var _ store.AlertNotificationChannelRepository = (*Repository)(nil)
var _ store.AlertNotificationDeliveryRepository = (*Repository)(nil)

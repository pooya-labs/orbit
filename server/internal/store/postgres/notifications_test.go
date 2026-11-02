package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

func createAlertNotificationChannelForTest(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	workspace store.Workspace,
	issuer, owner, name, secretRef string,
) store.AlertNotificationChannel {
	t.Helper()
	channel, err := repository.CreateWorkspaceAlertNotificationChannel(ctx, store.CreateAlertNotificationChannelInput{
		WorkspaceID: workspace.ID,
		Name: name,
		SecretRef: secretRef,
		ActorIssuer: issuer,
		ActorSubject: owner,
		RequestID: "req-notification-channel-create",
	})
	if err != nil {
		t.Fatalf("create alert-notification channel: %v", err)
	}
	return channel
}

func TestAlertNotificationDeliveryOutboxRetryAndAppendOnlyHistory(t *testing.T) {
	repository, workspace, issuer, owner := openAlertEvaluationTestRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	channel := createAlertNotificationChannelForTest(t, ctx, repository, workspace, issuer, owner, "Primary operations", "OPS_PRIMARY")
	rule := createAlertEvaluatorRule(t, ctx, repository, workspace, issuer, owner, 0)
	instance, changed, err := repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 300, time.Now().UTC())
	if err != nil || !changed || instance.Status != "firing" {
		t.Fatalf("create firing alert = %+v changed=%v err=%v", instance, changed, err)
	}

	claimed, err := repository.ClaimAlertNotificationDeliveries(ctx, 10, 60)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claimed deliveries = %d, err=%v; want one", len(claimed), err)
	}
	first := claimed[0]
	if first.ChannelID != channel.ID || first.SecretRef != "OPS_PRIMARY" ||
		first.EventType != "alert.firing" || first.Attempts != 1 || first.Status != "processing" {
		t.Fatalf("claimed delivery = %+v", first)
	}

	retryAt := time.Now().UTC().Add(-time.Second)
	if err := repository.RecordAlertNotificationAttempt(ctx, store.RecordAlertNotificationAttemptInput{
		DeliveryID: first.ID,
		AttemptNumber: first.Attempts,
		Outcome: "retry_scheduled",
		ErrorCode: "network_error",
		NextAttemptAt: retryAt,
	}); err != nil {
		t.Fatalf("record first retryable attempt: %v", err)
	}

	pending, err := repository.ListWorkspaceAlertNotificationDeliveries(ctx, workspace.ID, issuer, owner, "pending", 50)
	if err != nil || len(pending) != 1 || pending[0].LastError != "network_error" || pending[0].Attempts != 1 {
		t.Fatalf("list pending deliveries = %+v err=%v", pending, err)
	}

	reclaimed, err := repository.ClaimAlertNotificationDeliveries(ctx, 10, 60)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].ID != first.ID || reclaimed[0].Attempts != 2 {
		t.Fatalf("reclaimed deliveries = %+v err=%v", reclaimed, err)
	}
	status := 202
	if err := repository.RecordAlertNotificationAttempt(ctx, store.RecordAlertNotificationAttemptInput{
		DeliveryID: first.ID,
		AttemptNumber: reclaimed[0].Attempts,
		Outcome: "delivered",
		HTTPStatus: &status,
	}); err != nil {
		t.Fatalf("record successful attempt: %v", err)
	}

	delivered, err := repository.ListWorkspaceAlertNotificationDeliveries(ctx, workspace.ID, issuer, owner, "delivered", 50)
	if err != nil || len(delivered) != 1 || delivered[0].DeliveredAt == nil || delivered[0].Attempts != 2 {
		t.Fatalf("list delivered alerts = %+v err=%v", delivered, err)
	}
	attempts, err := repository.ListWorkspaceAlertNotificationAttempts(ctx, workspace.ID, first.ID, issuer, owner, 50)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("list attempt history = %+v err=%v", attempts, err)
	}
	if attempts[0].Outcome != "delivered" || attempts[1].Outcome != "retry_scheduled" {
		t.Fatalf("attempt history outcomes = %+v", attempts)
	}

	if _, err := repository.pool.Exec(ctx, `
		UPDATE alert_notification_attempts SET error_code = 'tampered'
		WHERE delivery_id = $1::uuid
	`, first.ID); err == nil {
		t.Fatal("notification attempt history allowed UPDATE")
	}
	if _, err := repository.pool.Exec(ctx, `
		DELETE FROM alert_notification_attempts WHERE delivery_id = $1::uuid
	`, first.ID); err == nil {
		t.Fatal("notification attempt history allowed DELETE")
	}

	if _, err := repository.ListWorkspaceAlertNotificationAttempts(ctx, workspace.ID, first.ID, issuer, "not-a-member", 50); !errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
		t.Fatalf("non-member attempt history error = %v; want membership not found", err)
	}
}

func TestDisablingAlertNotificationChannelStopsQueuedDeliveries(t *testing.T) {
	repository, workspace, issuer, owner := openAlertEvaluationTestRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	channel := createAlertNotificationChannelForTest(t, ctx, repository, workspace, issuer, owner, "Secondary operations", "OPS_SECONDARY")
	rule := createAlertEvaluatorRule(t, ctx, repository, workspace, issuer, owner, 0)
	_, changed, err := repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 300, time.Now().UTC())
	if err != nil || !changed {
		t.Fatalf("create firing alert changed=%v err=%v", changed, err)
	}
	if _, err := repository.SetWorkspaceAlertNotificationChannelEnabled(ctx, store.SetAlertNotificationChannelEnabledInput{
		WorkspaceID: workspace.ID,
		ChannelID: channel.ID,
		Enabled: false,
		ActorIssuer: issuer,
		ActorSubject: owner,
		RequestID: "req-notification-channel-disable",
	}); err != nil {
		t.Fatalf("disable notification channel: %v", err)
	}
	items, err := repository.ListWorkspaceAlertNotificationDeliveries(ctx, workspace.ID, issuer, owner, "dead", 50)
	if err != nil || len(items) != 1 || items[0].LastError != "channel_disabled" {
		t.Fatalf("disabled channel deliveries = %+v err=%v", items, err)
	}
	claimed, err := repository.ClaimAlertNotificationDeliveries(ctx, 10, 60)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("disabled channel was still claimable: %+v err=%v", claimed, err)
	}
}


func TestExpiredNotificationLeaseIsRecordedBeforeReclaim(t *testing.T) {
	repository, workspace, issuer, owner := openAlertEvaluationTestRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	channel := createAlertNotificationChannelForTest(t, ctx, repository, workspace, issuer, owner, "Crash recovery", "OPS_CRASH_RECOVERY")
	rule := createAlertEvaluatorRule(t, ctx, repository, workspace, issuer, owner, 0)
	_, changed, err := repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 300, time.Now().UTC())
	if err != nil || !changed {
		t.Fatalf("create firing alert changed=%v err=%v", changed, err)
	}
	first, err := repository.ClaimAlertNotificationDeliveries(ctx, 5, 60)
	if err != nil || len(first) != 1 || first[0].ChannelID != channel.ID || first[0].Attempts != 1 {
		t.Fatalf("first claim = %+v err=%v", first, err)
	}
	if _, err := repository.pool.Exec(ctx, `
		UPDATE alert_notification_deliveries
		SET lease_until = NOW() - INTERVAL '1 second'
		WHERE id = $1::uuid
	`, first[0].ID); err != nil {
		t.Fatalf("expire notification lease: %v", err)
	}
	reclaimed, err := repository.ClaimAlertNotificationDeliveries(ctx, 5, 60)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].ID != first[0].ID || reclaimed[0].Attempts != 2 {
		t.Fatalf("reclaimed notification = %+v err=%v", reclaimed, err)
	}
	attempts, err := repository.ListWorkspaceAlertNotificationAttempts(ctx, workspace.ID, first[0].ID, issuer, owner, 20)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("expired lease history = %+v err=%v", attempts, err)
	}
	if attempts[0].AttemptNumber != 1 || attempts[0].Outcome != "retry_scheduled" ||
		attempts[0].ErrorCode != "worker_lease_expired" {
		t.Fatalf("expired lease attempt = %+v", attempts[0])
	}
}

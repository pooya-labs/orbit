package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

func openAlertEvaluationTestRepository(t *testing.T) (*Repository, store.Workspace, string, string) {
	t.Helper()
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL repository: %v", err)
	}
	t.Cleanup(repository.Close)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	issuer := "https://alert-evaluator-tests.example/"
	owner := "alert-evaluator-owner-" + suffix
	workspace, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "Alert evaluator " + suffix,
		Slug: "alert-evaluator-" + suffix,
		OwnerIssuer: issuer,
		OwnerSubject: owner,
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	return repository, workspace, issuer, owner
}

func createAlertEvaluatorRule(t *testing.T, ctx context.Context, repository *Repository, workspace store.Workspace, issuer, owner string, pendingSeconds int) store.AlertRule {
	t.Helper()
	rule, err := repository.CreateWorkspaceAlertRule(ctx, store.CreateWorkspaceAlertRuleInput{
		WorkspaceID: workspace.ID,
		Name: "Checkout request rate " + fmt.Sprintf("%d", time.Now().UnixNano()),
		ServiceName: "checkout",
		Metric: "request_rate",
		Operator: "gt",
		Threshold: 250,
		WindowSeconds: 60,
		PendingSeconds: pendingSeconds,
		Severity: "high",
		ActorIssuer: issuer,
		ActorSubject: owner,
		RequestID: "req-alert-evaluator-create",
	})
	if err != nil {
		t.Fatalf("create alert rule: %v", err)
	}
	return rule
}

func TestWorkspaceAlertEvaluatorPendingFiringResolvedAndNoDuplicateEvents(t *testing.T) {
	repository, workspace, issuer, owner := openAlertEvaluationTestRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rule := createAlertEvaluatorRule(t, ctx, repository, workspace, issuer, owner, 60)
	base := time.Now().UTC().Truncate(time.Second)

	pending, changed, err := repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 300, base)
	if err != nil || !changed || pending.Status != "pending" || pending.FiredAt != nil {
		t.Fatalf("first sample = %+v changed=%v err=%v; want pending", pending, changed, err)
	}
	stillPending, changed, err := repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 320, base.Add(30*time.Second))
	if err != nil || changed || stillPending.Status != "pending" || stillPending.Value != 320 {
		t.Fatalf("second sample = %+v changed=%v err=%v; want same pending instance without event", stillPending, changed, err)
	}
	firing, changed, err := repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 330, base.Add(60*time.Second))
	if err != nil || !changed || firing.ID != pending.ID || firing.Status != "firing" || firing.FiredAt == nil {
		t.Fatalf("pending-to-firing = %+v changed=%v err=%v", firing, changed, err)
	}
	stillFiring, changed, err := repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 350, base.Add(90*time.Second))
	if err != nil || changed || stillFiring.ID != pending.ID || stillFiring.Status != "firing" {
		t.Fatalf("repeated firing sample = %+v changed=%v err=%v; want no duplicate transition", stillFiring, changed, err)
	}
	resolved, changed, err := repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 100, base.Add(120*time.Second))
	if err != nil || !changed || resolved.ID != pending.ID || resolved.Status != "resolved" || resolved.ResolvedAt == nil {
		t.Fatalf("firing-to-resolved = %+v changed=%v err=%v", resolved, changed, err)
	}
	if _, changed, err := repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 80, base.Add(150*time.Second)); err != nil || changed {
		t.Fatalf("below-threshold sample after resolution changed=%v err=%v; want no new instance", changed, err)
	}

	var pendingEvents, firingEvents, resolvedEvents, total int
	err = repository.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE event_type = 'alert.pending'),
		       COUNT(*) FILTER (WHERE event_type = 'alert.firing'),
		       COUNT(*) FILTER (WHERE event_type = 'alert.resolved')
		FROM alert_instance_events WHERE alert_instance_id = $1::uuid
	`, pending.ID).Scan(&total, &pendingEvents, &firingEvents, &resolvedEvents)
	if err != nil {
		t.Fatalf("count alert-instance events: %v", err)
	}
	if total != 3 || pendingEvents != 1 || firingEvents != 1 || resolvedEvents != 1 {
		t.Fatalf("event counts total=%d pending=%d firing=%d resolved=%d; want 3/1/1/1", total, pendingEvents, firingEvents, resolvedEvents)
	}

	items, err := repository.ListWorkspaceAlertInstances(ctx, workspace.ID, issuer, owner, "resolved", 50)
	if err != nil || len(items) != 1 || items[0].ID != pending.ID {
		t.Fatalf("list resolved workspace alerts = %+v err=%v", items, err)
	}
	if _, err := repository.ListWorkspaceAlertInstances(ctx, workspace.ID, issuer, "not-a-member", "", 50); !errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
		t.Fatalf("non-member alert list error = %v; want membership not found", err)
	}
}

func TestAlertEvaluatorDistributedLockHasSingleOwner(t *testing.T) {
	repository, _, _, _ := openAlertEvaluationTestRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	release, acquired, err := repository.TryAcquireAlertEvaluationLock(ctx)
	if err != nil || !acquired {
		t.Fatalf("first advisory lock acquired=%v err=%v", acquired, err)
	}
	defer release()

	otherRelease, otherAcquired, err := repository.TryAcquireAlertEvaluationLock(ctx)
	if err != nil || otherAcquired {
		t.Fatalf("second advisory lock acquired=%v err=%v; want false while first owner holds it", otherAcquired, err)
	}
	otherRelease()

	release()
	releaseAgain, acquiredAgain, err := repository.TryAcquireAlertEvaluationLock(ctx)
	if err != nil || !acquiredAgain {
		t.Fatalf("advisory lock after release acquired=%v err=%v", acquiredAgain, err)
	}
	releaseAgain()
}

func TestAlertRuleChangesCloseActiveInstancesAndPreserveHistory(t *testing.T) {
	repository, workspace, issuer, owner := openAlertEvaluationTestRepository(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rule := createAlertEvaluatorRule(t, ctx, repository, workspace, issuer, owner, 0)
	base := time.Now().UTC().Truncate(time.Second)
	firing, changed, err := repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 400, base)
	if err != nil || !changed || firing.Status != "firing" {
		t.Fatalf("create firing instance = %+v changed=%v err=%v", firing, changed, err)
	}

	updated, err := repository.UpdateWorkspaceAlertRule(ctx, store.UpdateWorkspaceAlertRuleInput{
		WorkspaceID: workspace.ID, RuleID: rule.ID,
		Name: rule.Name, ServiceName: rule.ServiceName, Metric: rule.Metric,
		Operator: rule.Operator, Threshold: 275, WindowSeconds: rule.WindowSeconds,
		PendingSeconds: rule.PendingSeconds, Severity: rule.Severity,
		ActorIssuer: issuer, ActorSubject: owner, RequestID: "req-alert-evaluator-update",
	})
	if err != nil || updated.Threshold != 275 {
		t.Fatalf("update alert rule = %+v err=%v", updated, err)
	}
	items, err := repository.ListWorkspaceAlertInstances(ctx, workspace.ID, issuer, owner, "", 50)
	if err != nil || len(items) != 1 || items[0].Status != "resolved" {
		t.Fatalf("active instance after rule update = %+v err=%v; want resolved history", items, err)
	}
	var updatedEvents int
	if err := repository.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM alert_instance_events
		WHERE alert_instance_id = $1::uuid AND event_type = 'alert.rule_updated'
	`, firing.ID).Scan(&updatedEvents); err != nil || updatedEvents != 1 {
		t.Fatalf("rule-updated event count=%d err=%v; want 1", updatedEvents, err)
	}

	_, err = repository.SetWorkspaceAlertRuleEnabled(ctx, store.SetWorkspaceAlertRuleEnabledInput{
		WorkspaceID: workspace.ID, RuleID: rule.ID, Enabled: false,
		ActorIssuer: issuer, ActorSubject: owner, RequestID: "req-alert-evaluator-disable",
	})
	if err != nil {
		t.Fatalf("disable alert rule: %v", err)
	}
	_, changed, err = repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 500, base.Add(time.Minute))
	if err != nil || changed {
		t.Fatalf("disabled rule evaluation changed=%v err=%v; want skipped", changed, err)
	}

	_, err = repository.SetWorkspaceAlertRuleEnabled(ctx, store.SetWorkspaceAlertRuleEnabledInput{
		WorkspaceID: workspace.ID, RuleID: rule.ID, Enabled: true,
		ActorIssuer: issuer, ActorSubject: owner, RequestID: "req-alert-evaluator-enable",
	})
	if err != nil {
		t.Fatalf("re-enable alert rule: %v", err)
	}
	newInstance, changed, err := repository.ApplyWorkspaceAlertSample(ctx, workspace.ID, rule.ID, "checkout", 500, base.Add(2*time.Minute))
	if err != nil || !changed || newInstance.ID == firing.ID || newInstance.Status != "firing" {
		t.Fatalf("new firing instance = %+v changed=%v err=%v", newInstance, changed, err)
	}
	if err := repository.DeleteWorkspaceAlertRule(ctx, store.DeleteWorkspaceAlertRuleInput{
		WorkspaceID: workspace.ID, RuleID: rule.ID,
		ActorIssuer: issuer, ActorSubject: owner, RequestID: "req-alert-evaluator-delete",
	}); err != nil {
		t.Fatalf("delete alert rule with active instance: %v", err)
	}
	items, err = repository.ListWorkspaceAlertInstances(ctx, workspace.ID, issuer, owner, "", 50)
	if err != nil || len(items) != 2 || items[0].Status == "firing" || items[1].Status == "firing" {
		t.Fatalf("alert history after rule deletion = %+v err=%v; want two closed instances", items, err)
	}
	var deletedEvents int
	if err := repository.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM alert_instance_events
		WHERE alert_instance_id = $1::uuid AND event_type = 'alert.rule_deleted'
	`, newInstance.ID).Scan(&deletedEvents); err != nil || deletedEvents != 1 {
		t.Fatalf("rule-deleted event count=%d err=%v; want 1", deletedEvents, err)
	}
}

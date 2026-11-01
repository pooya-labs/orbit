package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

func TestWorkspaceAlertRuleRepositoryScopeAuditAndLifecycle(t *testing.T) {
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
	issuer := "https://alert-rule-tests.example/"
	owner := "alert-owner-" + suffix
	workspace, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "Alert rules " + suffix,
		Slug: "alert-rules-" + suffix,
		OwnerIssuer: issuer,
		OwnerSubject: owner,
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	otherWorkspace, err := repository.CreateWorkspace(ctx, store.CreateWorkspaceInput{
		Name: "Alert rules other " + suffix,
		Slug: "alert-other-" + suffix,
		OwnerIssuer: issuer,
		OwnerSubject: "other-owner-" + suffix,
	})
	if err != nil {
		t.Fatalf("create other workspace: %v", err)
	}

	input := store.CreateWorkspaceAlertRuleInput{
		WorkspaceID: workspace.ID,
		Name: "High checkout request rate",
		Description: "Notify when checkout request rate remains high.",
		ServiceName: "checkout",
		Metric: "request_rate",
		Operator: "gt",
		Threshold: 250,
		WindowSeconds: 300,
		PendingSeconds: 120,
		Severity: "high",
		ActorIssuer: issuer,
		ActorSubject: owner,
		RequestID: "req-alert-create-" + suffix,
	}
	rule, err := repository.CreateWorkspaceAlertRule(ctx, input)
	if err != nil {
		t.Fatalf("create workspace alert rule: %v", err)
	}
	if rule.ID == "" || rule.WorkspaceID != workspace.ID || !rule.Enabled ||
		rule.Metric != "request_rate" || rule.Threshold != 250 || rule.Source != "" {
		t.Fatalf("unexpected created alert rule: %+v", rule)
	}

	if _, err := repository.CreateWorkspaceAlertRule(ctx, input); !errors.Is(err, store.ErrAlertRuleNameConflict) {
		t.Fatalf("duplicate name error = %v, want ErrAlertRuleNameConflict", err)
	}
	invalid := input
	invalid.Name = "Unsafe arbitrary query"
	invalid.Metric = "arbitrary_promql"
	if _, err := repository.CreateWorkspaceAlertRule(ctx, invalid); !errors.Is(err, store.ErrAlertRuleInvalidInput) {
		t.Fatalf("unsupported metric error = %v, want ErrAlertRuleInvalidInput", err)
	}
	invalid = input
	invalid.Name = "NaN threshold"
	invalid.Threshold = math.NaN()
	if _, err := repository.CreateWorkspaceAlertRule(ctx, invalid); !errors.Is(err, store.ErrAlertRuleInvalidInput) {
		t.Fatalf("NaN threshold error = %v, want ErrAlertRuleInvalidInput", err)
	}
	if _, err := repository.CreateWorkspaceAlertRule(ctx, store.CreateWorkspaceAlertRuleInput{
		WorkspaceID: workspace.ID, Name: "Non-member rule", Metric: "request_rate",
		Operator: "gt", Threshold: 1, WindowSeconds: 60, Severity: "low",
		ActorIssuer: issuer, ActorSubject: "not-a-member-" + suffix,
	}); !errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
		t.Fatalf("non-member create error = %v, want ErrWorkspaceMembershipNotFound", err)
	}

	viewer := "alert-viewer-" + suffix
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO workspace_memberships (workspace_id, user_issuer, user_subject, role)
		VALUES ($1::uuid, $2, $3, 'viewer')
	`, workspace.ID, issuer, viewer); err != nil {
		t.Fatalf("add workspace viewer: %v", err)
	}
	viewerRules, err := repository.ListWorkspaceAlertRules(ctx, workspace.ID, issuer, viewer, 50)
	if err != nil || len(viewerRules) != 1 || viewerRules[0].ID != rule.ID {
		t.Fatalf("viewer list rules = %+v, err=%v; expected one visible rule", viewerRules, err)
	}
	if _, err := repository.CreateWorkspaceAlertRule(ctx, store.CreateWorkspaceAlertRuleInput{
		WorkspaceID: workspace.ID, Name: "Viewer mutation", Metric: "request_rate",
		Operator: "gt", Threshold: 1, WindowSeconds: 60, Severity: "low",
		ActorIssuer: issuer, ActorSubject: viewer,
	}); !errors.Is(err, store.ErrAlertRuleForbidden) {
		t.Fatalf("viewer create error = %v, want ErrAlertRuleForbidden", err)
	}
	if _, err := repository.ListWorkspaceAlertRules(ctx, otherWorkspace.ID, issuer, owner, 50); !errors.Is(err, store.ErrWorkspaceMembershipNotFound) {
		t.Fatalf("cross-workspace list error = %v, want ErrWorkspaceMembershipNotFound", err)
	}

	updated, err := repository.UpdateWorkspaceAlertRule(ctx, store.UpdateWorkspaceAlertRuleInput{
		WorkspaceID: workspace.ID,
		RuleID: rule.ID,
		Name: "Very high checkout request rate",
		Description: "Updated alert threshold.",
		ServiceName: "checkout",
		Metric: "request_rate",
		Operator: "gte",
		Threshold: 300,
		WindowSeconds: 900,
		PendingSeconds: 60,
		Severity: "critical",
		ActorIssuer: issuer,
		ActorSubject: owner,
		RequestID: "req-alert-update-" + suffix,
	})
	if err != nil {
		t.Fatalf("update workspace alert rule: %v", err)
	}
	if updated.Name != "Very high checkout request rate" || updated.Operator != "gte" ||
		updated.Threshold != 300 || updated.WindowSeconds != 900 || updated.Severity != "critical" {
		t.Fatalf("updated alert rule fields mismatch: %+v", updated)
	}

	disabled, err := repository.SetWorkspaceAlertRuleEnabled(ctx, store.SetWorkspaceAlertRuleEnabledInput{
		WorkspaceID: workspace.ID, RuleID: rule.ID, Enabled: false,
		ActorIssuer: issuer, ActorSubject: owner, RequestID: "req-alert-disable-" + suffix,
	})
	if err != nil || disabled.Enabled {
		t.Fatalf("disable alert rule = %+v, err=%v", disabled, err)
	}
	if _, err := repository.SetWorkspaceAlertRuleEnabled(ctx, store.SetWorkspaceAlertRuleEnabledInput{
		WorkspaceID: workspace.ID, RuleID: rule.ID, Enabled: false,
		ActorIssuer: issuer, ActorSubject: owner,
	}); !errors.Is(err, store.ErrAlertRuleNoChange) {
		t.Fatalf("repeated disable error = %v, want ErrAlertRuleNoChange", err)
	}
	enabled, err := repository.SetWorkspaceAlertRuleEnabled(ctx, store.SetWorkspaceAlertRuleEnabledInput{
		WorkspaceID: workspace.ID, RuleID: rule.ID, Enabled: true,
		ActorIssuer: issuer, ActorSubject: owner, RequestID: "req-alert-enable-" + suffix,
	})
	if err != nil || !enabled.Enabled {
		t.Fatalf("enable alert rule = %+v, err=%v", enabled, err)
	}

	audit, err := repository.ListWorkspaceAuditEvents(ctx, workspace.ID, 100)
	if err != nil {
		t.Fatalf("list workspace audit events: %v", err)
	}
	alertAuditCount := 0
	for _, event := range audit {
		if event.ResourceType == "alert_rule" && event.ResourceID == rule.ID {
			alertAuditCount++
		}
	}
	if alertAuditCount != 4 {
		t.Fatalf("alert-rule audit event count = %d, want 4 before deletion", alertAuditCount)
	}

	if err := repository.DeleteWorkspaceAlertRule(ctx, store.DeleteWorkspaceAlertRuleInput{
		WorkspaceID: workspace.ID, RuleID: rule.ID,
		ActorIssuer: issuer, ActorSubject: owner, RequestID: "req-alert-delete-" + suffix,
	}); err != nil {
		t.Fatalf("delete workspace alert rule: %v", err)
	}
	if _, err := repository.UpdateWorkspaceAlertRule(ctx, store.UpdateWorkspaceAlertRuleInput{
		WorkspaceID: workspace.ID, RuleID: rule.ID,
	}); !errors.Is(err, store.ErrAlertRuleInvalidInput) {
		t.Fatalf("invalid update after deletion error = %v, want ErrAlertRuleInvalidInput", err)
	}
	audit, err = repository.ListWorkspaceAuditEvents(ctx, workspace.ID, 100)
	if err != nil {
		t.Fatalf("list workspace audit events after delete: %v", err)
	}
	alertAuditCount = 0
	for _, event := range audit {
		if event.ResourceType == "alert_rule" && event.ResourceID == rule.ID {
			alertAuditCount++
		}
	}
	if alertAuditCount != 5 {
		t.Fatalf("alert-rule audit event count after deletion = %d, want 5", alertAuditCount)
	}
	rules, err := repository.ListWorkspaceAlertRules(ctx, workspace.ID, issuer, owner, 50)
	if err != nil {
		t.Fatalf("list alert rules after deletion: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("remaining alert rules = %d, want 0", len(rules))
	}
}

func TestValidateAlertRuleFields(t *testing.T) {
	valid := struct {
		name, description, service, metric, operator string
		threshold float64
		window, pending int
		severity string
	}{
		name: "Request rate", description: "watch service", service: "checkout",
		metric: "request_rate", operator: "gt", threshold: 10,
		window: 300, pending: 60, severity: "high",
	}
	if err := validateAlertRuleFields(valid.name, valid.description, valid.service, valid.metric, valid.operator,
		valid.threshold, valid.window, valid.pending, valid.severity); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	cases := []struct {
		name string
		metric, operator, severity string
		threshold float64
		window, pending int
		service string
	}{
		{name: "unsupported metric", metric: "up", operator: "gt", severity: "low", threshold: 1, window: 60},
		{name: "unsupported operator", metric: "request_rate", operator: "regex", severity: "low", threshold: 1, window: 60},
		{name: "infinite threshold", metric: "request_rate", operator: "gt", severity: "low", threshold: math.Inf(1), window: 60},
		{name: "zero threshold", metric: "request_rate", operator: "gt", severity: "low", threshold: 0, window: 60},
		{name: "unsupported window", metric: "request_rate", operator: "gt", severity: "low", threshold: 1, window: 120},
		{name: "invalid pending", metric: "request_rate", operator: "gt", severity: "low", threshold: 1, window: 60, pending: 3601},
		{name: "invalid severity", metric: "request_rate", operator: "gt", severity: "urgent", threshold: 1, window: 60},
		{name: "invalid service", metric: "request_rate", operator: "gt", severity: "low", threshold: 1, window: 60, service: "{bad selector}"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := validateAlertRuleFields("test", "", test.service, test.metric, test.operator,
				test.threshold, test.window, test.pending, test.severity)
			if !errors.Is(err, store.ErrAlertRuleInvalidInput) {
				t.Fatalf("error = %v, want ErrAlertRuleInvalidInput", err)
			}
		})
	}
}

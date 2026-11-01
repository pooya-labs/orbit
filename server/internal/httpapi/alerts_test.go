package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/auth"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type alertRouteVerifier struct{}

func (alertRouteVerifier) Verify(_ context.Context, token string) (auth.Principal, error) {
	if token != "alert-test-token" {
		return auth.Principal{}, errors.New("invalid token")
	}
	return auth.Principal{
		Issuer: "https://orbit.test/issuer",
		Subject: "alert-route-operator",
		Roles: []string{"operator"},
	}, nil
}

type alertRouteRepository struct {
	*incidentRouteRepository
	rules []store.AlertRule
	createdInput store.CreateWorkspaceAlertRuleInput
	updatedInput store.UpdateWorkspaceAlertRuleInput
	enabledInput store.SetWorkspaceAlertRuleEnabledInput
	deletedInput store.DeleteWorkspaceAlertRuleInput
	createErr error
	updateErr error
	enableErr error
	deleteErr error
}

func (r *alertRouteRepository) CreateWorkspaceAlertRule(_ context.Context, input store.CreateWorkspaceAlertRuleInput) (store.AlertRule, error) {
	r.createdInput = input
	if r.createErr != nil {
		return store.AlertRule{}, r.createErr
	}
	rule := store.AlertRule{
		ID: "55555555-5555-4555-8555-555555555555",
		WorkspaceID: input.WorkspaceID,
		Name: input.Name,
		Description: input.Description,
		ServiceName: input.ServiceName,
		Metric: input.Metric,
		Operator: input.Operator,
		Threshold: input.Threshold,
		WindowSeconds: input.WindowSeconds,
		PendingSeconds: input.PendingSeconds,
		Severity: input.Severity,
		Enabled: true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	r.rules = append(r.rules, rule)
	return rule, nil
}

func (r *alertRouteRepository) ListWorkspaceAlertRules(_ context.Context, workspaceID, issuer, subject string, _ int) ([]store.AlertRule, error) {
	if workspaceID != r.workspace.ID || issuer != "https://orbit.test/issuer" || subject != "alert-route-operator" {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	return append([]store.AlertRule(nil), r.rules...), nil
}

func (r *alertRouteRepository) UpdateWorkspaceAlertRule(_ context.Context, input store.UpdateWorkspaceAlertRuleInput) (store.AlertRule, error) {
	r.updatedInput = input
	if r.updateErr != nil {
		return store.AlertRule{}, r.updateErr
	}
	for i := range r.rules {
		if r.rules[i].ID == input.RuleID && r.rules[i].WorkspaceID == input.WorkspaceID {
			r.rules[i].Name = input.Name
			r.rules[i].Description = input.Description
			r.rules[i].ServiceName = input.ServiceName
			r.rules[i].Metric = input.Metric
			r.rules[i].Operator = input.Operator
			r.rules[i].Threshold = input.Threshold
			r.rules[i].WindowSeconds = input.WindowSeconds
			r.rules[i].PendingSeconds = input.PendingSeconds
			r.rules[i].Severity = input.Severity
			r.rules[i].UpdatedAt = time.Now().UTC()
			return r.rules[i], nil
		}
	}
	return store.AlertRule{}, store.ErrAlertRuleNotFound
}

func (r *alertRouteRepository) SetWorkspaceAlertRuleEnabled(_ context.Context, input store.SetWorkspaceAlertRuleEnabledInput) (store.AlertRule, error) {
	r.enabledInput = input
	if r.enableErr != nil {
		return store.AlertRule{}, r.enableErr
	}
	for i := range r.rules {
		if r.rules[i].ID == input.RuleID && r.rules[i].WorkspaceID == input.WorkspaceID {
			r.rules[i].Enabled = input.Enabled
			r.rules[i].UpdatedAt = time.Now().UTC()
			return r.rules[i], nil
		}
	}
	return store.AlertRule{}, store.ErrAlertRuleNotFound
}

func (r *alertRouteRepository) DeleteWorkspaceAlertRule(_ context.Context, input store.DeleteWorkspaceAlertRuleInput) error {
	r.deletedInput = input
	if r.deleteErr != nil {
		return r.deleteErr
	}
	for i := range r.rules {
		if r.rules[i].ID == input.RuleID && r.rules[i].WorkspaceID == input.WorkspaceID {
			r.rules = append(r.rules[:i], r.rules[i+1:]...)
			return nil
		}
	}
	return store.ErrAlertRuleNotFound
}

func newAlertRouteRepository() *alertRouteRepository {
	incident := &incidentRouteRepository{
		fakeRepository: &fakeRepository{},
		workspace: store.Workspace{
			ID: "11111111-1111-4111-8111-111111111111",
			Name: "Alert route test",
		},
	}
	return &alertRouteRepository{incidentRouteRepository: incident}
}

func newAlertRouteHandler(repository *alertRouteRepository) http.Handler {
	return NewHandler(Config{
		Environment: "test",
		AuthVerifier: alertRouteVerifier{},
	}, repository, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func alertRouteRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer alert-test-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "req-alert-route-test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestWorkspaceAlertRuleHTTPRoutes(t *testing.T) {
	repository := newAlertRouteRepository()
	handler := newAlertRouteHandler(repository)
	base := "/api/v1/workspaces/" + repository.workspace.ID + "/alert-rules"

	created := alertRouteRequest(handler, http.MethodPost, base,
		`{"name":"High request rate","description":"Watch checkout traffic","service_name":"checkout","metric":"request_rate","operator":"gt","threshold":250,"window_seconds":300,"pending_seconds":120,"severity":"high"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s; want 201", created.Code, created.Body.String())
	}
	if repository.createdInput.ActorIssuer != "https://orbit.test/issuer" ||
		repository.createdInput.ActorSubject != "alert-route-operator" ||
		repository.createdInput.RequestID != "req-alert-route-test" {
		t.Fatalf("verified actor/request ID were not forwarded: %+v", repository.createdInput)
	}
	if repository.createdInput.Metric != "request_rate" || repository.createdInput.Threshold != 250 {
		t.Fatalf("alert rule fields were not forwarded: %+v", repository.createdInput)
	}
	ruleID := "55555555-5555-4555-8555-555555555555"

	listed := alertRouteRequest(handler, http.MethodGet, base+"?limit=25", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "High request rate") {
		t.Fatalf("list status/body = %d %s", listed.Code, listed.Body.String())
	}

	updated := alertRouteRequest(handler, http.MethodPut, base+"/"+ruleID,
		`{"name":"Very high request rate","description":"Updated description","service_name":"checkout","metric":"request_rate","operator":"gte","threshold":300,"window_seconds":900,"pending_seconds":60,"severity":"critical"}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status = %d, body=%s; want 200", updated.Code, updated.Body.String())
	}
	if repository.updatedInput.Operator != "gte" || repository.updatedInput.Severity != "critical" {
		t.Fatalf("updated alert fields were not forwarded: %+v", repository.updatedInput)
	}

	disabled := alertRouteRequest(handler, http.MethodPatch, base+"/"+ruleID+"/enabled", `{"enabled":false}`)
	if disabled.Code != http.StatusOK || !strings.Contains(disabled.Body.String(), `"enabled":false`) {
		t.Fatalf("disable status/body = %d %s", disabled.Code, disabled.Body.String())
	}
	if repository.enabledInput.Enabled {
		t.Fatal("expected disabled state to be forwarded")
	}

	deleted := alertRouteRequest(handler, http.MethodDelete, base+"/"+ruleID, "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body=%s; want 204", deleted.Code, deleted.Body.String())
	}
	if repository.deletedInput.ActorSubject != "alert-route-operator" ||
		repository.deletedInput.RequestID != "req-alert-route-test" {
		t.Fatalf("delete actor/request ID were not forwarded: %+v", repository.deletedInput)
	}
}

func TestWorkspaceAlertRuleHTTPValidationAndConflictMapping(t *testing.T) {
	repository := newAlertRouteRepository()
	handler := newAlertRouteHandler(repository)
	base := "/api/v1/workspaces/" + repository.workspace.ID + "/alert-rules"

	unknownField := alertRouteRequest(handler, http.MethodPost, base,
		`{"name":"Bad","metric":"request_rate","operator":"gt","threshold":1,"window_seconds":300,"pending_seconds":0,"severity":"low","promql":"up"}`)
	if unknownField.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", unknownField.Code)
	}

	repository.createErr = store.ErrAlertRuleNameConflict
	conflict := alertRouteRequest(handler, http.MethodPost, base,
		`{"name":"Duplicate","metric":"request_rate","operator":"gt","threshold":1,"window_seconds":300,"pending_seconds":0,"severity":"low"}`)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "alert_rule_name_conflict") {
		t.Fatalf("duplicate name status/body = %d %s; want 409 alert_rule_name_conflict", conflict.Code, conflict.Body.String())
	}

	missingEnabled := alertRouteRequest(handler, http.MethodPatch,
		"/api/v1/workspaces/"+repository.workspace.ID+"/alert-rules/55555555-5555-4555-8555-555555555555/enabled", `{}`)
	if missingEnabled.Code != http.StatusBadRequest {
		t.Fatalf("missing enabled field status = %d, want 400", missingEnabled.Code)
	}

	repository.createErr = nil
	repository.enableErr = store.ErrAlertRuleForbidden
	forbidden := alertRouteRequest(handler, http.MethodPatch, base+"/55555555-5555-4555-8555-555555555555/enabled", `{"enabled":false}`)
	if forbidden.Code != http.StatusForbidden || !strings.Contains(forbidden.Body.String(), "alert_rule_forbidden") {
		t.Fatalf("forbidden state update status/body = %d %s; want 403", forbidden.Code, forbidden.Body.String())
	}
}

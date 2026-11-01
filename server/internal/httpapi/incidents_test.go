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

type incidentRouteVerifier struct{}

func (incidentRouteVerifier) Verify(_ context.Context, token string) (auth.Principal, error) {
	if token != "incident-test-token" {
		return auth.Principal{}, errors.New("invalid token")
	}
	return auth.Principal{
		Issuer: "https://orbit.test/issuer",
		Subject: "incident-route-operator",
		Roles: []string{"operator"},
	}, nil
}

type incidentRouteRepository struct {
	*fakeRepository
	workspace store.Workspace
	created store.Incident
	transitioned store.Incident
	events []store.IncidentTimelineEvent
	createInput store.CreateWorkspaceIncidentInput
	transitionInput store.TransitionWorkspaceIncidentInput
	transitionErr error
}

func (r *incidentRouteRepository) CreateWorkspace(_ context.Context, input store.CreateWorkspaceInput) (store.Workspace, error) {
	return store.Workspace{ID: "11111111-1111-4111-8111-111111111111", Name: input.Name, Slug: input.Slug}, nil
}

func (r *incidentRouteRepository) ListWorkspacesForPrincipal(context.Context, string, string) ([]store.Workspace, error) {
	return []store.Workspace{r.workspace}, nil
}

func (r *incidentRouteRepository) GetWorkspaceMembership(_ context.Context, workspaceID, issuer, subject string) (store.WorkspaceMembership, error) {
	if workspaceID != r.workspace.ID || issuer != "https://orbit.test/issuer" || subject != "incident-route-operator" {
		return store.WorkspaceMembership{}, store.ErrWorkspaceMembershipNotFound
	}
	return store.WorkspaceMembership{WorkspaceID: workspaceID, Role: "operator"}, nil
}

func (r *incidentRouteRepository) AppendWorkspaceAuditEvent(_ context.Context, event store.WorkspaceAuditEvent) (store.WorkspaceAuditEvent, error) {
	event.ID = "audit-event"
	event.CreatedAt = time.Now().UTC()
	return event, nil
}

func (r *incidentRouteRepository) ListWorkspaceAuditEvents(context.Context, string, int) ([]store.WorkspaceAuditEvent, error) {
	return []store.WorkspaceAuditEvent{}, nil
}

func (r *incidentRouteRepository) CreateWorkspaceIncident(_ context.Context, input store.CreateWorkspaceIncidentInput) (store.Incident, error) {
	r.createInput = input
	r.created = store.Incident{
		ID: "22222222-2222-4222-8222-222222222222",
		Title: input.Title,
		Description: input.Description,
		ServiceID: input.ServiceID,
		ServiceName: "checkout",
		Severity: input.Severity,
		Status: "open",
		Source: "api",
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	return r.created, nil
}

func (r *incidentRouteRepository) TransitionWorkspaceIncident(_ context.Context, input store.TransitionWorkspaceIncidentInput) (store.Incident, error) {
	r.transitionInput = input
	if r.transitionErr != nil {
		return store.Incident{}, r.transitionErr
	}
	r.transitioned = r.created
	r.transitioned.Status = input.ToStatus
	r.transitioned.UpdatedAt = time.Now().UTC()
	return r.transitioned, nil
}

func (r *incidentRouteRepository) ListWorkspaceIncidentTimeline(_ context.Context, workspaceID, incidentID, issuer, subject string, _ int) ([]store.IncidentTimelineEvent, error) {
	if workspaceID != r.workspace.ID || incidentID != r.created.ID || issuer != "https://orbit.test/issuer" || subject != "incident-route-operator" {
		return nil, store.ErrIncidentNotFound
	}
	return r.events, nil
}

func newIncidentRouteHandler(repository *incidentRouteRepository) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(Config{
		Environment: "test",
		AuthVerifier: incidentRouteVerifier{},
	}, repository, logger)
}

func incidentRouteRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer incident-test-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "req-incident-route-test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestWorkspaceIncidentLifecycleHTTPRoutes(t *testing.T) {
	repository := &incidentRouteRepository{
		fakeRepository: &fakeRepository{},
		workspace: store.Workspace{ID: "11111111-1111-4111-8111-111111111111", Name: "Route test"},
		events: []store.IncidentTimelineEvent{{
			ID: "33333333-3333-4333-8333-333333333333",
			WorkspaceID: "11111111-1111-4111-8111-111111111111",
			IncidentID: "22222222-2222-4222-8222-222222222222",
			EventType: "incident.created",
			ToStatus: "open",
			CreatedAt: time.Now().UTC(),
		}},
	}
	handler := newIncidentRouteHandler(repository)

	created := incidentRouteRequest(handler, http.MethodPost,
		"/api/v1/workspaces/"+repository.workspace.ID+"/incidents",
		`{"title":"Checkout errors","description":"Error rate increased","service_id":"44444444-4444-4444-8444-444444444444","severity":"high"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s; want 201", created.Code, created.Body.String())
	}
	if repository.createInput.ActorIssuer != "https://orbit.test/issuer" ||
		repository.createInput.ActorSubject != "incident-route-operator" ||
		repository.createInput.RequestID != "req-incident-route-test" {
		t.Fatalf("authenticated actor/request ID were not forwarded to repository: %+v", repository.createInput)
	}

	transitioned := incidentRouteRequest(handler, http.MethodPatch,
		"/api/v1/workspaces/"+repository.workspace.ID+"/incidents/"+repository.created.ID+"/status",
		`{"status":"investigating","comment":"On-call engaged"}`)
	if transitioned.Code != http.StatusOK {
		t.Fatalf("transition status = %d, body=%s; want 200", transitioned.Code, transitioned.Body.String())
	}
	if repository.transitionInput.ToStatus != "investigating" ||
		repository.transitionInput.Comment != "On-call engaged" ||
		repository.transitionInput.ActorSubject != "incident-route-operator" {
		t.Fatalf("transition input was not forwarded correctly: %+v", repository.transitionInput)
	}

	timeline := incidentRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/"+repository.workspace.ID+"/incidents/"+repository.created.ID+"/timeline?limit=25", "")
	if timeline.Code != http.StatusOK {
		t.Fatalf("timeline status = %d, body=%s; want 200", timeline.Code, timeline.Body.String())
	}
	if !strings.Contains(timeline.Body.String(), "incident.created") {
		t.Fatalf("timeline response did not include event: %s", timeline.Body.String())
	}
}

func TestWorkspaceIncidentLifecycleHTTPRejectsUnknownFieldsAndMapsConflict(t *testing.T) {
	repository := &incidentRouteRepository{
		fakeRepository: &fakeRepository{},
		workspace: store.Workspace{ID: "11111111-1111-4111-8111-111111111111"},
		created: store.Incident{ID: "22222222-2222-4222-8222-222222222222"},
		transitionErr: store.ErrIncidentInvalidTransition,
	}
	handler := newIncidentRouteHandler(repository)

	invalid := incidentRouteRequest(handler, http.MethodPost,
		"/api/v1/workspaces/"+repository.workspace.ID+"/incidents",
		`{"title":"x","service_id":"44444444-4444-4444-8444-444444444444","severity":"low","unexpected":true}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", invalid.Code)
	}

	conflict := incidentRouteRequest(handler, http.MethodPatch,
		"/api/v1/workspaces/"+repository.workspace.ID+"/incidents/"+repository.created.ID+"/status",
		`{"status":"open"}`)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("invalid transition status = %d, body=%s; want 409", conflict.Code, conflict.Body.String())
	}
}

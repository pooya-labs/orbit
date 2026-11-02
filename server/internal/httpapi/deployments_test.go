package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type deploymentRouteRepository struct {
	*incidentRouteRepository
	items []store.DeploymentAnnotation
	createdInput store.CreateWorkspaceDeploymentAnnotationInput
}

func (r *deploymentRouteRepository) CreateWorkspaceDeploymentAnnotation(_ context.Context, input store.CreateWorkspaceDeploymentAnnotationInput) (store.DeploymentAnnotation, error) {
	r.createdInput = input
	item := store.DeploymentAnnotation{
		ID: "55555555-5555-4555-8555-555555555555",
		WorkspaceID: input.WorkspaceID,
		ServiceID: input.ServiceID,
		ServiceName: "checkout",
		Environment: "staging",
		Version: input.Version,
		CommitSHA: input.CommitSHA,
		ChangeReference: input.ChangeReference,
		ReleaseNotes: input.ReleaseNotes,
		DeployedAt: input.DeployedAt,
		CreatedAt: time.Now().UTC(),
	}
	r.items = append(r.items, item)
	return item, nil
}

func (r *deploymentRouteRepository) ListWorkspaceDeploymentAnnotations(_ context.Context, workspaceID, issuer, subject, serviceID string, _ int) ([]store.DeploymentAnnotation, error) {
	if workspaceID != r.workspace.ID || issuer != "https://orbit.test/issuer" || subject != "incident-route-operator" {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	items := make([]store.DeploymentAnnotation, 0)
	for _, item := range r.items {
		if serviceID == "" || serviceID == item.ServiceID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *deploymentRouteRepository) GetWorkspaceDeploymentAnnotation(_ context.Context, workspaceID, annotationID, issuer, subject string) (store.DeploymentAnnotation, error) {
	if workspaceID != r.workspace.ID || issuer != "https://orbit.test/issuer" || subject != "incident-route-operator" {
		return store.DeploymentAnnotation{}, store.ErrWorkspaceMembershipNotFound
	}
	for _, item := range r.items {
		if item.ID == annotationID {
			return item, nil
		}
	}
	return store.DeploymentAnnotation{}, store.ErrDeploymentAnnotationNotFound
}

func newDeploymentRouteHandler(repository *deploymentRouteRepository) http.Handler {
	return NewHandler(Config{
		Environment: "test",
		AuthVerifier: incidentRouteVerifier{},
	}, repository, testLogger())
}

func TestWorkspaceDeploymentAnnotationHTTPRoutes(t *testing.T) {
	workspaceID := "11111111-1111-4111-8111-111111111111"
	serviceID := "44444444-4444-4444-8444-444444444444"
	firstAt := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Second)
	first := store.DeploymentAnnotation{
		ID: "66666666-6666-4666-8666-666666666666",
		WorkspaceID: workspaceID,
		ServiceID: serviceID,
		ServiceName: "checkout",
		Environment: "staging",
		Version: "v1.2.0",
		CommitSHA: "abcdef1234567",
		DeployedAt: firstAt,
	}
	second := store.DeploymentAnnotation{
		ID: "77777777-7777-4777-8777-777777777777",
		WorkspaceID: workspaceID,
		ServiceID: serviceID,
		ServiceName: "checkout",
		Environment: "staging",
		Version: "v1.3.0",
		CommitSHA: "123456789abcd",
		DeployedAt: firstAt.Add(5 * time.Minute),
	}
	repository := &deploymentRouteRepository{
		incidentRouteRepository: &incidentRouteRepository{
			fakeRepository: &fakeRepository{},
			workspace: store.Workspace{ID: workspaceID, Name: "Deployment route test"},
		},
		items: []store.DeploymentAnnotation{first, second},
	}
	handler := newDeploymentRouteHandler(repository)

	created := incidentRouteRequest(handler, http.MethodPost,
		"/api/v1/workspaces/"+workspaceID+"/deployments",
		`{"service_id":"44444444-4444-4444-8444-444444444444","version":"v1.4.0","commit_sha":"fedcba9876543","change_reference":"ORBIT-123","release_notes":"Checkout deployment"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	if repository.createdInput.ActorIssuer != "https://orbit.test/issuer" ||
		repository.createdInput.ActorSubject != "incident-route-operator" ||
		repository.createdInput.RequestID != "req-incident-route-test" {
		t.Fatalf("authenticated actor/request ID were not forwarded: %+v", repository.createdInput)
	}

	listed := incidentRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/"+workspaceID+"/deployments?service_id="+serviceID+"&limit=25", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "v1.2.0") {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}

	compared := incidentRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/"+workspaceID+"/deployments/compare?from_id="+first.ID+"&to_id="+second.ID, "")
	if compared.Code != http.StatusOK || !strings.Contains(compared.Body.String(), `"version_changed":true`) ||
		!strings.Contains(compared.Body.String(), `"elapsed_seconds":300`) {
		t.Fatalf("comparison status=%d body=%s", compared.Code, compared.Body.String())
	}
}

func TestWorkspaceDeploymentComparisonRejectsNonChronologicalDeployments(t *testing.T) {
	workspaceID := "11111111-1111-4111-8111-111111111111"
	earlierID := "66666666-6666-4666-8666-666666666666"
	laterID := "77777777-7777-4777-8777-777777777777"
	now := time.Now().UTC().Truncate(time.Second)
	repository := &deploymentRouteRepository{
		incidentRouteRepository: &incidentRouteRepository{
			fakeRepository: &fakeRepository{},
			workspace: store.Workspace{ID: workspaceID},
		},
		items: []store.DeploymentAnnotation{
			{ID: earlierID, WorkspaceID: workspaceID, ServiceID: "44444444-4444-4444-8444-444444444444", DeployedAt: now},
			{ID: laterID, WorkspaceID: workspaceID, ServiceID: "44444444-4444-4444-8444-444444444444", DeployedAt: now},
		},
	}
	handler := newDeploymentRouteHandler(repository)
	response := incidentRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/"+workspaceID+"/deployments/compare?from_id="+earlierID+"&to_id="+laterID, "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("equal-time comparison status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestWorkspaceDeploymentComparisonRejectsDifferentServices(t *testing.T) {
	workspaceID := "11111111-1111-4111-8111-111111111111"
	repository := &deploymentRouteRepository{
		incidentRouteRepository: &incidentRouteRepository{
			fakeRepository: &fakeRepository{},
			workspace: store.Workspace{ID: workspaceID},
		},
		items: []store.DeploymentAnnotation{
			{ID: "66666666-6666-4666-8666-666666666666", WorkspaceID: workspaceID, ServiceID: "44444444-4444-4444-8444-444444444444", DeployedAt: time.Now().UTC()},
			{ID: "77777777-7777-4777-8777-777777777777", WorkspaceID: workspaceID, ServiceID: "88888888-8888-4888-8888-888888888888", DeployedAt: time.Now().UTC()},
		},
	}
	handler := newDeploymentRouteHandler(repository)
	response := incidentRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/"+workspaceID+"/deployments/compare?from_id="+repository.items[0].ID+"&to_id="+repository.items[1].ID, "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("cross-service comparison status=%d body=%s", response.Code, response.Body.String())
	}
}


func TestWorkspaceDeploymentComparisonRejectsDifferentEnvironments(t *testing.T) {
	workspaceID := "11111111-1111-4111-8111-111111111111"
	serviceID := "44444444-4444-4444-8444-444444444444"
	now := time.Now().UTC().Truncate(time.Second)
	repository := &deploymentRouteRepository{
		incidentRouteRepository: &incidentRouteRepository{
			fakeRepository: &fakeRepository{},
			workspace: store.Workspace{ID: workspaceID},
		},
		items: []store.DeploymentAnnotation{
			{
				ID: "66666666-6666-4666-8666-666666666666",
				WorkspaceID: workspaceID,
				ServiceID: serviceID,
				Environment: "staging",
				DeployedAt: now,
			},
			{
				ID: "77777777-7777-4777-8777-777777777777",
				WorkspaceID: workspaceID,
				ServiceID: serviceID,
				Environment: "production",
				DeployedAt: now.Add(time.Minute),
			},
		},
	}
	handler := newDeploymentRouteHandler(repository)
	response := incidentRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/"+workspaceID+"/deployments/compare?from_id="+repository.items[0].ID+"&to_id="+repository.items[1].ID, "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("cross-environment comparison status=%d body=%s", response.Code, response.Body.String())
	}
}


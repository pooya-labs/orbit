package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/auth"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type fakeWorkloadAdminRepository struct {
	*fakeRepository
	identities []store.WorkloadIdentity
	grants []store.WorkloadIdentityGrant
	grantErr error
	statusErr error
	membershipRole string
	lastActorIssuer string
	lastActorSubject string
}

func (f *fakeWorkloadAdminRepository) CreateWorkloadIdentity(_ context.Context, input store.CreateWorkloadIdentityInput) (store.WorkloadIdentity, error) {
	if f.grantErr != nil {
		return store.WorkloadIdentity{}, f.grantErr
	}
	item := store.WorkloadIdentity{
		ID: "00000000-0000-4000-8000-000000000003",
		Issuer: input.Issuer, Subject: input.Subject, DisplayName: input.DisplayName,
		Status: "active", ExpiresAt: input.ExpiresAt, CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	f.identities = append(f.identities, item)
	f.lastActorIssuer, f.lastActorSubject = input.ActorIssuer, input.ActorSubject
	return item, nil
}

func (f *fakeWorkloadAdminRepository) ListWorkloadIdentities(_ context.Context, limit int) ([]store.WorkloadIdentity, error) {
	items := append([]store.WorkloadIdentity(nil), f.identities...)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (f *fakeWorkloadAdminRepository) SetWorkloadIdentityStatus(_ context.Context, id, actorIssuer, actorSubject, status string) error {
	f.lastActorIssuer, f.lastActorSubject = actorIssuer, actorSubject
	if f.statusErr != nil {
		return f.statusErr
	}
	for i := range f.identities {
		if f.identities[i].ID != id {
			continue
		}
		if f.identities[i].Status == "revoked" && status != "revoked" {
			return store.ErrWorkloadAuthorizationDenied
		}
		f.identities[i].Status = status
		if status == "revoked" {
			now := time.Now().UTC()
			f.identities[i].RevokedAt = &now
		}
		return nil
	}
	return store.ErrWorkloadIdentityNotFound
}

func (f *fakeWorkloadAdminRepository) SetWorkloadWorkspaceGrant(_ context.Context, input store.WorkloadGrantInput, granted bool) error {
	f.lastActorIssuer, f.lastActorSubject = input.ActorIssuer, input.ActorSubject
	if f.grantErr != nil {
		return f.grantErr
	}
	if len(f.identities) == 0 || f.identities[0].ID != input.IdentityID {
		return store.ErrWorkloadIdentityNotFound
	}
	if granted {
		f.grants = append(f.grants, store.WorkloadIdentityGrant{
			IdentityID: input.IdentityID, WorkspaceID: input.WorkspaceID,
			Signal: input.Signal, GrantedAt: time.Now().UTC(),
		})
		return nil
	}
	filtered := f.grants[:0]
	for _, item := range f.grants {
		if item.IdentityID != input.IdentityID || item.WorkspaceID != input.WorkspaceID || item.Signal != input.Signal {
			filtered = append(filtered, item)
		}
	}
	f.grants = filtered
	return nil
}

func (f *fakeWorkloadAdminRepository) ListWorkspaceWorkloadGrants(_ context.Context, workspaceID string) ([]store.WorkloadIdentityGrant, error) {
	items := make([]store.WorkloadIdentityGrant, 0)
	for _, item := range f.grants {
		if item.WorkspaceID == workspaceID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (f *fakeWorkloadAdminRepository) AuthorizeWorkloadTelemetry(context.Context, string, string, string, string) (store.WorkloadIdentity, error) {
	return store.WorkloadIdentity{}, errors.New("not used by admin route tests")
}

func (f *fakeWorkloadAdminRepository) GetWorkspaceMembership(ctx context.Context, workspaceID, issuer, subject string) (store.WorkspaceMembership, error) {
	if f.membershipRole == "" {
		return f.fakeRepository.GetWorkspaceMembership(ctx, workspaceID, issuer, subject)
	}
	if workspaceID != "00000000-0000-4000-8000-000000000001" || issuer != "https://identity.example" || subject != "member-1" {
		return store.WorkspaceMembership{}, store.ErrWorkspaceMembershipNotFound
	}
	return store.WorkspaceMembership{WorkspaceID: workspaceID, UserIssuer: issuer, UserSubject: subject, Role: f.membershipRole}, nil
}

func workloadAdminHandler(repository *fakeWorkloadAdminRepository, roles ...string) http.Handler {
	verifier := fixedTokenVerifier{principal: auth.Principal{
		Issuer: "https://identity.example", Subject: "member-1", Roles: roles,
	}}
	return NewHandler(Config{
		Environment: "production", AuthVerifier: verifier,
		AllowedOrigins: []string{"https://dashboard.example"},
		RateLimitPerMinute: 100,
	}, repository, testLogger())
}

func TestWorkloadIdentityAdminRoutesRequireAdministratorRole(t *testing.T) {
	repository := &fakeWorkloadAdminRepository{fakeRepository: &fakeRepository{}}
	handler := workloadAdminHandler(repository, "viewer")
	for _, test := range []struct {
		method string
		path string
		body string
	}{
		{method: http.MethodGet, path: "/api/v1/admin/workload-identities"},
		{method: http.MethodPost, path: "/api/v1/admin/workload-identities", body: `{"issuer":"https://workloads.example","subject":"worker-1","display_name":"worker"}`},
		{method: http.MethodPatch, path: "/api/v1/admin/workload-identities/00000000-0000-4000-8000-000000000003/status", body: `{"status":"revoked"}`},
	} {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.Header.Set("Authorization", "Bearer valid-token")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d, want 403; body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func TestWorkloadIdentityAdminLifecycleAndWorkspaceGrants(t *testing.T) {
	repository := &fakeWorkloadAdminRepository{fakeRepository: &fakeRepository{}}
	adminHandler := workloadAdminHandler(repository, "administrator")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/workload-identities", strings.NewReader(`{"issuer":"https://workloads.example/","subject":"collector-1","display_name":"Collector"}`))
	request.Header.Set("Authorization", "Bearer valid-token")
	response := httptest.NewRecorder()
	adminHandler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "collector-1") || strings.Contains(strings.ToLower(response.Body.String()), "bearer") {
		t.Fatalf("unexpected workload identity response: %s", response.Body.String())
	}
	if repository.lastActorIssuer != "https://identity.example" || repository.lastActorSubject != "member-1" {
		t.Fatalf("identity creation did not use verified actor: %s/%s", repository.lastActorIssuer, repository.lastActorSubject)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/workload-identities?limit=10", nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	response = httptest.NewRecorder()
	adminHandler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "collector-1") {
		t.Fatalf("list status/body = %d %s", response.Code, response.Body.String())
	}

	operatorHandler := workloadAdminHandler(repository, "operator")
	grantPath := "/api/v1/workspaces/00000000-0000-4000-8000-000000000001/workload-identities/00000000-0000-4000-8000-000000000003/grants/metrics"
	request = httptest.NewRequest(http.MethodPut, grantPath, nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	response = httptest.NewRecorder()
	operatorHandler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(repository.grants) != 1 {
		t.Fatalf("grant status = %d, grants=%+v, body=%s", response.Code, repository.grants, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/00000000-0000-4000-8000-000000000001/workload-identities", nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	response = httptest.NewRecorder()
	operatorHandler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "metrics") {
		t.Fatalf("workspace grants list status/body = %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, grantPath, nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	response = httptest.NewRecorder()
	operatorHandler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || len(repository.grants) != 0 {
		t.Fatalf("revoke grant status = %d, remaining grants=%+v", response.Code, repository.grants)
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/workload-identities/00000000-0000-4000-8000-000000000003/status", strings.NewReader(`{"status":"revoked"}`))
	request.Header.Set("Authorization", "Bearer valid-token")
	response = httptest.NewRecorder()
	adminHandler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || repository.identities[0].Status != "revoked" {
		t.Fatalf("revoke identity status = %d, identity=%+v, body=%s", response.Code, repository.identities[0], response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/v1/admin/workload-identities/00000000-0000-4000-8000-000000000003/status", strings.NewReader(`{"status":"active"}`))
	request.Header.Set("Authorization", "Bearer valid-token")
	response = httptest.NewRecorder()
	adminHandler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("reactivate revoked identity status = %d, want 409; body=%s", response.Code, response.Body.String())
	}
}

func TestWorkloadGrantManagementRequiresWorkspaceOwnerOrAdministrator(t *testing.T) {
	repository := &fakeWorkloadAdminRepository{
		fakeRepository: &fakeRepository{},
		identities: []store.WorkloadIdentity{{ID: "00000000-0000-4000-8000-000000000003", Status: "active"}},
		membershipRole: "viewer",
	}
	handler := workloadAdminHandler(repository, "operator")
	request := httptest.NewRequest(http.MethodPut, "/api/v1/workspaces/00000000-0000-4000-8000-000000000001/workload-identities/00000000-0000-4000-8000-000000000003/grants/metrics", nil)
	request.Header.Set("Authorization", "Bearer valid-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("grant status = %d, want 403; body=%s", response.Code, response.Body.String())
	}
	if len(repository.grants) != 0 {
		t.Fatal("viewer membership was allowed to grant workload access")
	}
}

func TestWorkloadAdminCORSAllowsMutationMethods(t *testing.T) {
	repository := &fakeWorkloadAdminRepository{fakeRepository: &fakeRepository{}}
	handler := workloadAdminHandler(repository, "administrator")
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/admin/workload-identities/00000000-0000-4000-8000-000000000003/status", nil)
	request.Header.Set("Origin", "https://dashboard.example")
	request.Header.Set("Access-Control-Request-Method", "PATCH")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("CORS preflight status = %d, want 204; body=%s", response.Code, response.Body.String())
	}
	allowed := response.Header().Get("Access-Control-Allow-Methods")
	for _, method := range []string{"PUT", "PATCH", "DELETE"} {
		if !strings.Contains(allowed, method) {
			t.Fatalf("CORS allowed methods %q do not include %s", allowed, method)
		}
	}
}

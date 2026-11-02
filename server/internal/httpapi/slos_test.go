package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
	"github.com/pooya16pg-spec/orbit/server/internal/telemetry"
)

type sloRouteRepository struct {
	*incidentRouteRepository
	objective store.ServiceLevelObjective
	items []store.ServiceLevelObjective
	created store.CreateWorkspaceSLOInput
	createErr error
}

func (r *sloRouteRepository) CreateWorkspaceSLO(_ context.Context, input store.CreateWorkspaceSLOInput) (store.ServiceLevelObjective, error) {
	r.created = input
	if r.createErr != nil { return store.ServiceLevelObjective{}, r.createErr }
	item := store.ServiceLevelObjective{
		ID: "55555555-5555-4555-8555-555555555555",
		WorkspaceID: input.WorkspaceID,
		ServiceID: input.ServiceID,
		ServiceName: "checkout",
		Environment: "staging",
		Name: input.Name,
		Route: input.Route,
		Target: input.Target,
		RollingWindowDays: input.RollingWindowDays,
		Enabled: true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	r.items = append(r.items, item)
	r.objective = item
	return item, nil
}

func (r *sloRouteRepository) ListWorkspaceSLOs(_ context.Context, workspaceID, issuer, subject, serviceID string, _ int) ([]store.ServiceLevelObjective, error) {
	if workspaceID != r.workspace.ID || issuer != "https://orbit.test/issuer" || subject != "alert-route-operator" {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	items := make([]store.ServiceLevelObjective, 0)
	for _, item := range r.items {
		if serviceID == "" || item.ServiceID == serviceID { items = append(items, item) }
	}
	return items, nil
}

func (r *sloRouteRepository) GetWorkspaceSLO(_ context.Context, workspaceID, sloID, issuer, subject string) (store.ServiceLevelObjective, error) {
	if workspaceID != r.workspace.ID || issuer != "https://orbit.test/issuer" || subject != "alert-route-operator" {
		return store.ServiceLevelObjective{}, store.ErrWorkspaceMembershipNotFound
	}
	for _, item := range r.items {
		if item.ID == sloID { return item, nil }
	}
	return store.ServiceLevelObjective{}, store.ErrSLONotFound
}

func (r *sloRouteRepository) UpdateWorkspaceSLO(_ context.Context, input store.UpdateWorkspaceSLOInput) (store.ServiceLevelObjective, error) {
	for index := range r.items {
		if r.items[index].ID == input.SLOID {
			r.items[index].Name = input.Name
			r.items[index].Route = input.Route
			r.items[index].Target = input.Target
			r.items[index].RollingWindowDays = input.RollingWindowDays
			return r.items[index], nil
		}
	}
	return store.ServiceLevelObjective{}, store.ErrSLONotFound
}

func (r *sloRouteRepository) SetWorkspaceSLOEnabled(_ context.Context, input store.SetWorkspaceSLOEnabledInput) (store.ServiceLevelObjective, error) {
	for index := range r.items {
		if r.items[index].ID == input.SLOID {
			r.items[index].Enabled = input.Enabled
			r.objective = r.items[index]
			return r.items[index], nil
		}
	}
	return store.ServiceLevelObjective{}, store.ErrSLONotFound
}

func (r *sloRouteRepository) DeleteWorkspaceSLO(_ context.Context, input store.DeleteWorkspaceSLOInput) error {
	for index := range r.items {
		if r.items[index].ID == input.SLOID {
			r.items = append(r.items[:index], r.items[index+1:]...)
			return nil
		}
	}
	return store.ErrSLONotFound
}

func newSLORouteRepository() *sloRouteRepository {
	return &sloRouteRepository{
		incidentRouteRepository: &incidentRouteRepository{
			fakeRepository: &fakeRepository{},
			workspace: store.Workspace{
				ID: "11111111-1111-4111-8111-111111111111",
				Name: "SLO route test",
			},
		},
	}
}

func newSLORouteHandler(repository *sloRouteRepository, telemetryClient *telemetry.Client) http.Handler {
	return NewHandler(Config{
		Environment: "test",
		AuthVerifier: alertRouteVerifier{},
		Telemetry: telemetryClient,
	}, repository, testLogger())
}

func TestWorkspaceSLORoutesCreateAndCalculateErrorBudget(t *testing.T) {
	const (
		workspaceID = "11111111-1111-4111-8111-111111111111"
		serviceID = "44444444-4444-4444-8444-444444444444"
		sloID = "55555555-5555-4555-8555-555555555555"
	)
	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		for _, expected := range []string{
			`orbit_workspace_id="11111111-1111-4111-8111-111111111111"`,
			`service_name="checkout"`,
			`slo_route="/emit"`,
			`slo_outcome=~"good|bad"`,
			"[7d]",
		} {
			if !strings.Contains(query, expected) { t.Errorf("PromQL %q missing %q", query, expected) }
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
			{"metric":{"slo_outcome":"good"},"value":[1762000000.0,"995"]},
			{"metric":{"slo_outcome":"bad"},"value":[1762000000.0,"5"]}
		]}}`))
	}))
	defer prometheus.Close()
	telemetryClient, err := telemetry.NewClient(prometheus.URL, "", "")
	if err != nil { t.Fatal(err) }
	repository := newSLORouteRepository()
	handler := newSLORouteHandler(repository, telemetryClient)

	created := alertRouteRequest(handler, http.MethodPost,
		"/api/v1/workspaces/"+workspaceID+"/slos",
		`{"service_id":"`+serviceID+`","name":"Checkout success rate","route":"/emit","target":0.99,"rolling_window_days":7}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create SLO status=%d body=%s", created.Code, created.Body.String())
	}
	if repository.created.ActorIssuer != "https://orbit.test/issuer" ||
		repository.created.ActorSubject != "alert-route-operator" ||
		repository.created.RequestID != "req-alert-route-test" {
		t.Fatalf("authenticated actor/request ID not forwarded: %+v", repository.created)
	}

	listed := alertRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/"+workspaceID+"/slos?service_id="+serviceID+"&limit=25", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "Checkout success rate") {
		t.Fatalf("list SLO status=%d body=%s", listed.Code, listed.Body.String())
	}

	repository.objective = store.ServiceLevelObjective{
		ID: sloID, WorkspaceID: workspaceID, ServiceID: serviceID,
		ServiceName: "checkout", Environment: "staging",
		Name: "Checkout success rate", Route: "/emit", Target: 0.99,
		RollingWindowDays: 7, Enabled: true,
	}
	repository.items = []store.ServiceLevelObjective{repository.objective}
	budget := alertRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/"+workspaceID+"/slos/"+sloID+"/budget", "")
	if budget.Code != http.StatusOK ||
		!strings.Contains(budget.Body.String(), `"status":"within_budget"`) ||
		!strings.Contains(budget.Body.String(), `"good_events":995`) ||
		!strings.Contains(budget.Body.String(), `"bad_events":5`) {
		t.Fatalf("SLO budget status=%d body=%s", budget.Code, budget.Body.String())
	}
	if !strings.Contains(budget.Body.String(), "http_request_success_rate") ||
		!strings.Contains(budget.Body.String(), "2xx and 3xx") {
		t.Fatalf("budget response omitted SLI/event policy: %s", budget.Body.String())
	}
}

func TestWorkspaceSLOBudgetReportsInsufficientDataAndDisabledState(t *testing.T) {
	const (
		workspaceID = "11111111-1111-4111-8111-111111111111"
		sloID = "55555555-5555-4555-8555-555555555555"
	)
	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer prometheus.Close()
	telemetryClient, err := telemetry.NewClient(prometheus.URL, "", "")
	if err != nil { t.Fatal(err) }
	repository := newSLORouteRepository()
	repository.objective = store.ServiceLevelObjective{
		ID: sloID, WorkspaceID: workspaceID,
		ServiceID: "44444444-4444-4444-8444-444444444444",
		ServiceName: "checkout", Environment: "staging",
		Name: "Checkout availability", Route: "/emit", Target: 0.99,
		RollingWindowDays: 1, Enabled: true,
	}
	repository.items = []store.ServiceLevelObjective{repository.objective}
	handler := newSLORouteHandler(repository, telemetryClient)

	response := alertRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/"+workspaceID+"/slos/"+sloID+"/budget", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"insufficient_data"`) ||
		strings.Contains(response.Body.String(), `"availability"`) {
		t.Fatalf("insufficient-data response status=%d body=%s", response.Code, response.Body.String())
	}

	repository.items[0].Enabled = false
	repository.objective.Enabled = false
	disabled := alertRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/"+workspaceID+"/slos/"+sloID+"/budget", "")
	if disabled.Code != http.StatusConflict || !strings.Contains(disabled.Body.String(), "slo_disabled") {
		t.Fatalf("disabled SLO response status=%d body=%s", disabled.Code, disabled.Body.String())
	}
}

package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type alertNotificationRouteRepository struct {
	*alertRouteRepository
	channels []store.AlertNotificationChannel
	deliveries []store.AlertNotificationDelivery
	attempts []store.AlertNotificationAttempt
	createdInput store.CreateAlertNotificationChannelInput
	enabledInput store.SetAlertNotificationChannelEnabledInput
	createErr error
	enabledErr error
}

func (r *alertNotificationRouteRepository) CreateWorkspaceAlertNotificationChannel(_ context.Context, input store.CreateAlertNotificationChannelInput) (store.AlertNotificationChannel, error) {
	r.createdInput = input
	if r.createErr != nil {
		return store.AlertNotificationChannel{}, r.createErr
	}
	channel := store.AlertNotificationChannel{
		ID: "66666666-6666-4666-8666-666666666666",
		WorkspaceID: input.WorkspaceID,
		Name: input.Name,
		Adapter: "webhook",
		Enabled: true,
		SecretRef: input.SecretRef,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	r.channels = append(r.channels, channel)
	return channel, nil
}

func (r *alertNotificationRouteRepository) ListWorkspaceAlertNotificationChannels(_ context.Context, workspaceID, issuer, subject string, _ int) ([]store.AlertNotificationChannel, error) {
	if workspaceID != r.workspace.ID || issuer != "https://orbit.test/issuer" || subject != "alert-route-operator" {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	return append([]store.AlertNotificationChannel(nil), r.channels...), nil
}

func (r *alertNotificationRouteRepository) SetWorkspaceAlertNotificationChannelEnabled(_ context.Context, input store.SetAlertNotificationChannelEnabledInput) (store.AlertNotificationChannel, error) {
	r.enabledInput = input
	if r.enabledErr != nil {
		return store.AlertNotificationChannel{}, r.enabledErr
	}
	for index := range r.channels {
		if r.channels[index].ID == input.ChannelID && r.channels[index].WorkspaceID == input.WorkspaceID {
			r.channels[index].Enabled = input.Enabled
			return r.channels[index], nil
		}
	}
	return store.AlertNotificationChannel{}, store.ErrAlertNotificationChannelNotFound
}

func (r *alertNotificationRouteRepository) ListWorkspaceAlertNotificationDeliveries(_ context.Context, workspaceID, issuer, subject, _ string, _ int) ([]store.AlertNotificationDelivery, error) {
	if workspaceID != r.workspace.ID || issuer != "https://orbit.test/issuer" || subject != "alert-route-operator" {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	return append([]store.AlertNotificationDelivery(nil), r.deliveries...), nil
}

func (r *alertNotificationRouteRepository) ListWorkspaceAlertNotificationAttempts(_ context.Context, workspaceID, deliveryID, issuer, subject string, _ int) ([]store.AlertNotificationAttempt, error) {
	if workspaceID != r.workspace.ID || issuer != "https://orbit.test/issuer" || subject != "alert-route-operator" {
		return nil, store.ErrWorkspaceMembershipNotFound
	}
	items := make([]store.AlertNotificationAttempt, 0)
	for _, item := range r.attempts {
		if item.DeliveryID == deliveryID {
			items = append(items, item)
		}
	}
	return items, nil
}

func newAlertNotificationRouteHandler(repository *alertNotificationRouteRepository, secretRefs []string) http.Handler {
	return NewHandler(Config{
		Environment: "test",
		AuthVerifier: alertRouteVerifier{},
		AlertWebhookSecretRefs: secretRefs,
	}, repository, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestWorkspaceAlertNotificationChannelRoutesNeverExposeSecretReferences(t *testing.T) {
	repository := &alertNotificationRouteRepository{alertRouteRepository: newAlertRouteRepository()}
	handler := newAlertNotificationRouteHandler(repository, []string{"OPS_PRIMARY"})

	created := alertRouteRequest(handler, http.MethodPost,
		"/api/v1/workspaces/11111111-1111-4111-8111-111111111111/alert-notification-channels",
		`{"name":"Operations webhook","secret_ref":"OPS_PRIMARY"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "OPS_PRIMARY") {
		t.Fatalf("create response exposed the secret reference: %s", created.Body.String())
	}
	if !strings.Contains(created.Body.String(), `"secret_configured":true`) {
		t.Fatalf("create response did not confirm secret configuration: %s", created.Body.String())
	}
	if repository.createdInput.SecretRef != "OPS_PRIMARY" {
		t.Fatalf("stored secret reference = %q", repository.createdInput.SecretRef)
	}

	listed := alertRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/11111111-1111-4111-8111-111111111111/alert-notification-channels", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", listed.Code, listed.Body.String())
	}
	if strings.Contains(listed.Body.String(), "OPS_PRIMARY") {
		t.Fatalf("list response exposed the secret reference: %s", listed.Body.String())
	}
	if !strings.Contains(listed.Body.String(), `"secret_configured":true`) {
		t.Fatalf("list response did not include secret configuration state: %s", listed.Body.String())
	}

	disabled := alertRouteRequest(handler, http.MethodPatch,
		"/api/v1/workspaces/11111111-1111-4111-8111-111111111111/alert-notification-channels/66666666-6666-4666-8666-666666666666/enabled",
		`{"enabled":false}`)
	if disabled.Code != http.StatusOK || repository.enabledInput.Enabled {
		t.Fatalf("disable status=%d input=%+v body=%s", disabled.Code, repository.enabledInput, disabled.Body.String())
	}
}

func TestWorkspaceAlertNotificationChannelRejectsUnconfiguredSecretReference(t *testing.T) {
	repository := &alertNotificationRouteRepository{alertRouteRepository: newAlertRouteRepository()}
	handler := newAlertNotificationRouteHandler(repository, []string{"OPS_PRIMARY"})
	response := alertRouteRequest(handler, http.MethodPost,
		"/api/v1/workspaces/11111111-1111-4111-8111-111111111111/alert-notification-channels",
		`{"name":"Unknown destination","secret_ref":"NOT_CONFIGURED"}`)
	if response.Code != http.StatusServiceUnavailable || repository.createdInput.SecretRef != "" {
		t.Fatalf("unconfigured secret response status=%d input=%+v body=%s", response.Code, repository.createdInput, response.Body.String())
	}

	outsider := alertRouteRequest(handler, http.MethodPost,
		"/api/v1/workspaces/22222222-2222-4222-8222-222222222222/alert-notification-channels",
		`{"name":"Probe","secret_ref":"NOT_CONFIGURED"}`)
	if outsider.Code != http.StatusNotFound {
		t.Fatalf("non-member secret-reference probe status=%d, body=%s; want workspace not found before secret lookup", outsider.Code, outsider.Body.String())
	}
}

func TestWorkspaceAlertNotificationDeliveryAndAttemptHistoryRoutes(t *testing.T) {
	repository := &alertNotificationRouteRepository{
		alertRouteRepository: newAlertRouteRepository(),
		deliveries: []store.AlertNotificationDelivery{{
			ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			WorkspaceID: "11111111-1111-4111-8111-111111111111",
			EventType: "alert.firing",
			Status: "pending",
			Attempts: 2,
		}},
		attempts: []store.AlertNotificationAttempt{{
			ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			DeliveryID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			AttemptNumber: 2,
			Outcome: "retry_scheduled",
			ErrorCode: "network_error",
			AttemptedAt: time.Now().UTC(),
		}},
	}
	handler := newAlertNotificationRouteHandler(repository, []string{"OPS_PRIMARY"})
	deliveries := alertRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/11111111-1111-4111-8111-111111111111/alert-notification-deliveries?status=pending", "")
	if deliveries.Code != http.StatusOK || !strings.Contains(deliveries.Body.String(), `"status":"pending"`) {
		t.Fatalf("delivery history status=%d, body=%s", deliveries.Code, deliveries.Body.String())
	}

	attempts := alertRouteRequest(handler, http.MethodGet,
		"/api/v1/workspaces/11111111-1111-4111-8111-111111111111/alert-notification-deliveries/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/attempts", "")
	if attempts.Code != http.StatusOK || !strings.Contains(attempts.Body.String(), "network_error") {
		t.Fatalf("attempt history status=%d, body=%s", attempts.Code, attempts.Body.String())
	}
}

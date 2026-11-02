package notifications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testWebhookSender(t *testing.T, status int) (*WebhookSender, **http.Request) {
	t.Helper()
	sender, err := NewWebhookSender(
		map[string]string{"OPS_PRIMARY": "https://hooks.example.com/services/team/secret"},
		[]string{"hooks.example.com"},
	)
	if err != nil {
		t.Fatalf("create webhook sender: %v", err)
	}
	var captured *http.Request
	sender.client = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			captured = request.Clone(request.Context())
			body, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			captured.Body = io.NopCloser(strings.NewReader(string(body)))
			return &http.Response{
				StatusCode: status,
				Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader("ignored response")),
				Request: request,
			}, nil
		}),
	}
	return sender, &captured
}

func sampleDelivery() store.AlertNotificationDelivery {
	return store.AlertNotificationDelivery{
		ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		WorkspaceID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		AlertInstanceID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
		AlertInstanceEventID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
		ChannelID: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
		SecretRef: "OPS_PRIMARY",
		EventType: "alert.firing",
		RuleName: "High checkout request rate",
		ServiceName: "checkout",
		Metric: "request_rate",
		Operator: "gt",
		Threshold: 250,
		Value: 310.5,
		Severity: "high",
		InstanceStatus: "firing",
		OccurredAt: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
	}
}

func TestNewWebhookSenderRejectsUnsafeEndpoints(t *testing.T) {
	tests := []struct {
		name string
		endpoint string
		allowed []string
	}{
		{name: "plaintext", endpoint: "http://hooks.example.com/webhook", allowed: []string{"hooks.example.com"}},
		{name: "private IP literal", endpoint: "https://127.0.0.1/webhook", allowed: []string{"hooks.example.com"}},
		{name: "unlisted hostname", endpoint: "https://internal.example.com/webhook", allowed: []string{"hooks.example.com"}},
		{name: "query token", endpoint: "https://hooks.example.com/webhook?token=secret", allowed: []string{"hooks.example.com"}},
		{name: "userinfo", endpoint: "https://user:pass@hooks.example.com/webhook", allowed: []string{"hooks.example.com"}},
		{name: "alternate port", endpoint: "https://hooks.example.com:8443/webhook", allowed: []string{"hooks.example.com"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewWebhookSender(map[string]string{"OPS_PRIMARY": test.endpoint}, test.allowed); err == nil {
				t.Fatal("unsafe webhook endpoint was accepted")
			}
		})
	}
}

func TestIsPublicUnicastRejectsPrivateAndSpecialRanges(t *testing.T) {
	for _, value := range []string{
		"127.0.0.1",
		"10.1.2.3",
		"172.16.0.1",
		"192.168.1.1",
		"169.254.169.254",
		"100.64.0.1",
		"192.0.2.10",
		"198.18.0.1",
		"203.0.113.9",
		"::1",
		"fc00::1",
		"fe80::1",
		"2001:db8::1",
	} {
		address, err := netip.ParseAddr(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		if isPublicUnicast(address) {
			t.Errorf("%s was incorrectly accepted as public", value)
		}
	}
	for _, value := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		address, err := netip.ParseAddr(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		if !isPublicUnicast(address) {
			t.Errorf("%s was incorrectly rejected as public", value)
		}
	}
}

func TestWebhookSenderSendsMinimalIdempotentAlertPayload(t *testing.T) {
	sender, captured := testWebhookSender(t, http.StatusAccepted)
	result := sender.Send(context.Background(), sampleDelivery())
	if !result.Delivered || result.HTTPStatus == nil || *result.HTTPStatus != http.StatusAccepted {
		t.Fatalf("send result = %+v; want accepted delivery", result)
	}
	request := *captured
	if request == nil {
		t.Fatal("webhook request was not made")
	}
	if request.Method != http.MethodPost || request.URL.Host != "hooks.example.com" {
		t.Fatalf("unexpected webhook request: %s %s", request.Method, request.URL)
	}
	if request.Header.Get("Idempotency-Key") != "dddddddd-dddd-4ddd-8ddd-dddddddddddd" {
		t.Fatalf("idempotency key = %q", request.Header.Get("Idempotency-Key"))
	}
	if request.Header.Get("X-ORBIT-Delivery-ID") != sampleDelivery().ID {
		t.Fatalf("delivery header = %q", request.Header.Get("X-ORBIT-Delivery-ID"))
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	if strings.Contains(string(body), "secret") || strings.Contains(string(body), "OPS_PRIMARY") {
		t.Fatalf("webhook body leaked configuration material: %s", body)
	}
	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode webhook payload: %v", err)
	}
	if payload.SchemaVersion != 1 || payload.EventType != "alert.firing" ||
		payload.Alert.ServiceName != "checkout" || payload.Alert.Value != 310.5 ||
		payload.Alert.Threshold != 250 {
		t.Fatalf("unexpected webhook payload: %+v", payload)
	}
}

func TestWebhookSenderClassifiesRetryableAndPermanentResponses(t *testing.T) {
	for _, test := range []struct {
		status int
		wantDelivered bool
		wantPermanent bool
		wantError string
	}{
		{status: http.StatusTooManyRequests, wantError: "upstream_retryable"},
		{status: http.StatusServiceUnavailable, wantError: "upstream_retryable"},
		{status: http.StatusBadRequest, wantPermanent: true, wantError: "upstream_rejected"},
		{status: http.StatusTemporaryRedirect, wantPermanent: true, wantError: "redirect_rejected"},
		{status: http.StatusNoContent, wantDelivered: true},
	} {
		sender, _ := testWebhookSender(t, test.status)
		result := sender.Send(context.Background(), sampleDelivery())
		if result.Delivered != test.wantDelivered || result.Permanent != test.wantPermanent || result.ErrorCode != test.wantError {
			t.Errorf("HTTP %d result = %+v", test.status, result)
		}
	}
}

func TestWebhookSenderRequiresConfiguredSecretReference(t *testing.T) {
	sender, _ := testWebhookSender(t, http.StatusOK)
	delivery := sampleDelivery()
	delivery.SecretRef = "MISSING"
	result := sender.Send(context.Background(), delivery)
	if !result.Permanent || result.ErrorCode != "secret_not_configured" {
		t.Fatalf("missing secret result = %+v", result)
	}
}

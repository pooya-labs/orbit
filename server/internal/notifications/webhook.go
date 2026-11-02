package notifications

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

type SendResult struct {
	Delivered bool
	Permanent bool
	HTTPStatus *int
	ErrorCode string
}

type Sender interface {
	Send(context.Context, store.AlertNotificationDelivery) SendResult
}

type WebhookSender struct {
	secrets map[string]string
	allowedHosts map[string]struct{}
	client *http.Client
}

type webhookPayload struct {
	SchemaVersion int `json:"schema_version"`
	DeliveryID string `json:"delivery_id"`
	EventID string `json:"event_id"`
	EventType string `json:"event_type"`
	OccurredAt time.Time `json:"occurred_at"`
	WorkspaceID string `json:"workspace_id"`
	Alert struct {
		ID string `json:"id"`
		RuleName string `json:"rule_name"`
		ServiceName string `json:"service_name"`
		Metric string `json:"metric"`
		Operator string `json:"operator"`
		Threshold float64 `json:"threshold"`
		Value float64 `json:"value"`
		Severity string `json:"severity"`
		Status string `json:"status"`
	} `json:"alert"`
}

func NewWebhookSender(secrets map[string]string, allowedHosts []string) (*WebhookSender, error) {
	if len(secrets) > 0 && len(allowedHosts) == 0 {
		return nil, errors.New("webhook hostname allowlist is required")
	}
	hostSet := make(map[string]struct{}, len(allowedHosts))
	for _, value := range allowedHosts {
		host := strings.ToLower(strings.TrimSpace(value))
		if !validDNSHostname(host) {
			return nil, errors.New("webhook hostname allowlist contains an invalid hostname")
		}
		hostSet[host] = struct{}{}
	}
	copiedSecrets := make(map[string]string, len(secrets))
	for ref, endpoint := range secrets {
		if !validSecretRef(ref) {
			return nil, errors.New("webhook secret reference is invalid")
		}
		if _, err := validateWebhookEndpoint(endpoint, hostSet); err != nil {
			return nil, fmt.Errorf("webhook secret %s has an invalid endpoint", ref)
		}
		copiedSecrets[ref] = strings.TrimSpace(endpoint)
	}

	transport := &http.Transport{
		Proxy: nil,
		DialContext: safeDialContext(hostSet),
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 3 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: time.Second,
		MaxResponseHeaderBytes: 16 << 10,
		DisableKeepAlives: true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout: 7 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &WebhookSender{secrets: copiedSecrets, allowedHosts: hostSet, client: client}, nil
}

func (s *WebhookSender) ConfiguredSecretRefs() []string {
	if s == nil {
		return []string{}
	}
	refs := make([]string, 0, len(s.secrets))
	for ref := range s.secrets {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func (s *WebhookSender) HasSecretRef(ref string) bool {
	if s == nil {
		return false
	}
	_, ok := s.secrets[ref]
	return ok
}

func (s *WebhookSender) Send(ctx context.Context, delivery store.AlertNotificationDelivery) SendResult {
	if s == nil || s.client == nil {
		return SendResult{Permanent: true, ErrorCode: "webhook_sender_unavailable"}
	}
	endpoint, ok := s.secrets[delivery.SecretRef]
	if !ok {
		return SendResult{Permanent: true, ErrorCode: "secret_not_configured"}
	}
	if _, err := validateWebhookEndpoint(endpoint, s.allowedHosts); err != nil {
		return SendResult{Permanent: true, ErrorCode: "webhook_endpoint_invalid"}
	}
	if delivery.EventType != "alert.firing" && delivery.EventType != "alert.resolved" {
		return SendResult{Permanent: true, ErrorCode: "unsupported_event_type"}
	}

	payload := webhookPayload{
		SchemaVersion: 1,
		DeliveryID: delivery.ID,
		EventID: delivery.AlertInstanceEventID,
		EventType: delivery.EventType,
		OccurredAt: delivery.OccurredAt.UTC(),
		WorkspaceID: delivery.WorkspaceID,
	}
	payload.Alert.ID = delivery.AlertInstanceID
	payload.Alert.RuleName = delivery.RuleName
	payload.Alert.ServiceName = delivery.ServiceName
	payload.Alert.Metric = delivery.Metric
	payload.Alert.Operator = delivery.Operator
	payload.Alert.Threshold = delivery.Threshold
	payload.Alert.Value = delivery.Value
	payload.Alert.Severity = delivery.Severity
	payload.Alert.Status = delivery.InstanceStatus
	body, err := json.Marshal(payload)
	if err != nil {
		return SendResult{Permanent: true, ErrorCode: "payload_encoding_failed"}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return SendResult{Permanent: true, ErrorCode: "request_invalid"}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "ORBIT-Alert-Notifier/1.0")
	request.Header.Set("X-ORBIT-Delivery-ID", delivery.ID)
	request.Header.Set("Idempotency-Key", delivery.AlertInstanceEventID)

	response, err := s.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return SendResult{ErrorCode: "delivery_interrupted"}
		}
		return SendResult{ErrorCode: "network_error"}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	status := response.StatusCode
	if status >= 200 && status < 300 {
		return SendResult{Delivered: true, HTTPStatus: &status}
	}
	if status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500 {
		return SendResult{HTTPStatus: &status, ErrorCode: "upstream_retryable"}
	}
	if status >= 300 && status < 400 {
		return SendResult{Permanent: true, HTTPStatus: &status, ErrorCode: "redirect_rejected"}
	}
	return SendResult{Permanent: true, HTTPStatus: &status, ErrorCode: "upstream_rejected"}
}

func validateWebhookEndpoint(endpoint string, allowedHosts map[string]struct{}) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("webhook endpoint must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	host := strings.ToLower(parsed.Hostname())
	if !validDNSHostname(host) {
		return nil, errors.New("webhook endpoint hostname is invalid")
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return nil, errors.New("webhook endpoint must use port 443")
	}
	if _, ok := allowedHosts[host]; !ok {
		return nil, errors.New("webhook endpoint hostname is not allowlisted")
	}
	return parsed, nil
}

func safeDialContext(allowedHosts map[string]struct{}) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("webhook destination address is invalid")
		}
		host = strings.ToLower(host)
		if _, ok := allowedHosts[host]; !ok || port != "443" {
			return nil, errors.New("webhook destination is not allowlisted")
		}
		resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(resolved) == 0 {
			return nil, errors.New("webhook hostname could not be resolved")
		}
		ips := make([]netip.Addr, 0, len(resolved))
		for _, item := range resolved {
			parsed, ok := netip.AddrFromSlice(item.IP)
			if !ok {
				return nil, errors.New("webhook hostname resolved to an invalid IP")
			}
			parsed = parsed.Unmap()
			if !isPublicUnicast(parsed) {
				return nil, errors.New("webhook hostname resolved to a non-public IP")
			}
			ips = append(ips, parsed)
		}
		var lastErr error
		for _, ip := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), strconv.Itoa(443)))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = errors.New("webhook destination could not be reached")
		}
		return nil, lastErr
	}
}

func isPublicUnicast(address netip.Addr) bool {
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() ||
		address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() ||
		address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	reserved := []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("192.88.99.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("2001:10::/28"),
		netip.MustParsePrefix("2001::/23"),
	}
	for _, prefix := range reserved {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func validSecretRef(value string) bool {
	if len(value) < 1 || len(value) > 64 || value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for _, char := range value {
		if !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '_' {
			return false
		}
	}
	return true
}

func validDNSHostname(value string) bool {
	if len(value) == 0 || len(value) > 253 || strings.HasSuffix(value, ".") || net.ParseIP(value) != nil {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

package ingest

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type forwarderStub struct {
	calls int
	signal string
	payload []byte
	err error
}

func (f *forwarderStub) Export(_ context.Context, signal string, payload []byte) ([]byte, error) {
	f.calls++
	f.signal = signal
	f.payload = append([]byte(nil), payload...)
	if f.err != nil {
		return nil, f.err
	}
	return proto.Marshal(&metricsv1.ExportMetricsServiceResponse{})
}

func newTestHTTPGateway(t *testing.T, forwarder OTLPForwarder, requestsPerMinute int) http.Handler {
	t.Helper()
	verifier, grants := validStubs()
	authorizer, err := NewAuthorizer(verifier, grants)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHTTPGateway(authorizer, forwarder, HTTPGatewayConfig{
		RequestsPerMinute: requestsPerMinute,
		ForwardTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func metricRequest() *metricsv1.ExportMetricsServiceRequest {
	return &metricsv1.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{{
			Resource: &resourcev1.Resource{
				Attributes: []*commonv1.KeyValue{{
					Key: "service.name",
					Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "payments"}},
				}},
			},
		}},
	}
}

func TestHTTPGatewayAuthorizesAndInjectsCanonicalWorkspace(t *testing.T) {
	forwarder := &forwarderStub{}
	handler := newTestHTTPGateway(t, forwarder, 10)
	payload, err := proto.Marshal(metricRequest())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+testWorkspace+"/v1/metrics", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer signed-token")
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, body=%s", response.Code, response.Body.String())
	}
	if forwarder.calls != 1 || forwarder.signal != "metrics" {
		t.Fatalf("unexpected Collector forwarding: %+v", forwarder)
	}
	var forwarded metricsv1.ExportMetricsServiceRequest
	if err := proto.Unmarshal(forwarder.payload, &forwarded); err != nil {
		t.Fatal(err)
	}
	resource := forwarded.GetResourceMetrics()[0].GetResource()
	if got := attributeValue(resource, WorkspaceAttribute); got != testWorkspace {
		t.Fatalf("canonical workspace = %q, want %q", got, testWorkspace)
	}
	if got := attributeValue(resource, "service.name"); got != "payments" {
		t.Fatalf("service.name = %q, want payments", got)
	}
	if response.Header().Get("Content-Type") != "application/x-protobuf" {
		t.Fatalf("response Content-Type = %q", response.Header().Get("Content-Type"))
	}
}

func TestHTTPGatewaySupportsOTLPJSON(t *testing.T) {
	forwarder := &forwarderStub{}
	handler := newTestHTTPGateway(t, forwarder, 10)
	body := `{"resourceMetrics":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"payments"}}]}}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+testWorkspace+"/v1/metrics", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer signed-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, body=%s", response.Code, response.Body.String())
	}
	if forwarder.calls != 1 {
		t.Fatalf("forwarder calls = %d, want 1", forwarder.calls)
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("response Content-Type = %q", response.Header().Get("Content-Type"))
	}
	var decoded metricsv1.ExportMetricsServiceResponse
	if err := protojson.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid OTLP JSON response: %v", err)
	}
}

func TestHTTPGatewayRejectsForgedTenantAttributesWithoutForwarding(t *testing.T) {
	forwarder := &forwarderStub{}
	handler := newTestHTTPGateway(t, forwarder, 10)
	requestMessage := metricRequest()
	requestMessage.ResourceMetrics[0].Resource.Attributes = append(
		requestMessage.ResourceMetrics[0].Resource.Attributes,
		&commonv1.KeyValue{Key: WorkspaceAttribute, Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "forged"}}},
	)
	payload, _ := proto.Marshal(requestMessage)
	request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+testWorkspace+"/v1/metrics", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer signed-token")
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("HTTP status = %d, want 400", response.Code)
	}
	if forwarder.calls != 0 {
		t.Fatal("forged tenant payload reached Collector")
	}
}

func TestHTTPGatewayRejectsAmbiguousBearerAndInvalidTargetBeforeForwarding(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
		headers []string
		want int
	}{
		{name: "duplicate authorization", path: "/v1/workspaces/" + testWorkspace + "/v1/metrics", headers: []string{"Bearer one", "Bearer two"}, want: http.StatusUnauthorized},
		{name: "invalid workspace", path: "/v1/workspaces/not-a-uuid/v1/metrics", headers: []string{"Bearer one"}, want: http.StatusBadRequest},
		{name: "unknown signal", path: "/v1/workspaces/" + testWorkspace + "/v1/profiles", headers: []string{"Bearer one"}, want: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			forwarder := &forwarderStub{}
			handler := newTestHTTPGateway(t, forwarder, 10)
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader("{}"))
			for _, value := range test.headers {
				request.Header.Add("Authorization", value)
			}
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("HTTP status = %d, want %d; body=%s", response.Code, test.want, response.Body.String())
			}
			if forwarder.calls != 0 {
				t.Fatal("invalid request reached Collector")
			}
		})
	}
}

func TestHTTPGatewayEnforcesBodyAndEncodingLimits(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		contentEncoding string
		want int
	}{
		{name: "oversized body", body: strings.Repeat("x", maxOTLPRequestBytes+1), want: http.StatusRequestEntityTooLarge},
		{name: "compressed body not yet supported", body: "{}", contentEncoding: "gzip", want: http.StatusUnsupportedMediaType},
	} {
		t.Run(test.name, func(t *testing.T) {
			forwarder := &forwarderStub{}
			handler := newTestHTTPGateway(t, forwarder, 10)
			request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+testWorkspace+"/v1/metrics", strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer signed-token")
			request.Header.Set("Content-Type", "application/json")
			if test.contentEncoding != "" {
				request.Header.Set("Content-Encoding", test.contentEncoding)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("HTTP status = %d, want %d", response.Code, test.want)
			}
			if forwarder.calls != 0 {
				t.Fatal("invalid payload reached Collector")
			}
		})
	}
}


func TestHTTPGatewayRejectsDuplicateMediaHeaders(t *testing.T) {
	for _, test := range []struct {
		name string
		header string
		values []string
	}{
		{name: "duplicate content type", header: "Content-Type", values: []string{"application/json", "application/x-protobuf"}},
		{name: "duplicate content encoding", header: "Content-Encoding", values: []string{"identity", "gzip"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			forwarder := &forwarderStub{}
			handler := newTestHTTPGateway(t, forwarder, 10)
			request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+testWorkspace+"/v1/metrics", strings.NewReader("{}"))
			request.Header.Set("Authorization", "Bearer signed-token")
			request.Header.Set("Content-Type", "application/json")
			request.Header.Del(test.header)
			for _, value := range test.values {
				request.Header.Add(test.header, value)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnsupportedMediaType {
				t.Fatalf("HTTP status = %d, want 415", response.Code)
			}
			if forwarder.calls != 0 {
				t.Fatal("ambiguous media headers reached Collector")
			}
		})
	}
}

func TestHTTPGatewayMapsAuthorizationFailuresAndCollectorErrors(t *testing.T) {
	forwarder := &forwarderStub{err: errors.New("collector unavailable")}
	verifier, grants := validStubs()
	grants.err = errors.New("database unavailable")
	authorizer, _ := NewAuthorizer(verifier, grants)
	handler, err := NewHTTPGateway(authorizer, forwarder, HTTPGatewayConfig{RequestsPerMinute: 10})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := proto.Marshal(metricRequest())
	request := httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+testWorkspace+"/v1/metrics", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer signed-token")
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("authorization database failure status = %d, want 503", response.Code)
	}

	verifier, grants = validStubs()
	authorizer, _ = NewAuthorizer(verifier, grants)
	handler, _ = NewHTTPGateway(authorizer, forwarder, HTTPGatewayConfig{RequestsPerMinute: 10})
	request = httptest.NewRequest(http.MethodPost, "/v1/workspaces/"+testWorkspace+"/v1/metrics", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer signed-token")
	request.Header.Set("Content-Type", "application/x-protobuf")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("Collector failure status = %d, want 502", response.Code)
	}
}

func TestWorkspaceRateLimiterIsScopedAndBounded(t *testing.T) {
	limiter := newWorkspaceRateLimiter(1)
	now := time.Now()
	if !limiter.Allow("identity-a:workspace-a:metrics", now) {
		t.Fatal("first request was rejected")
	}
	if limiter.Allow("identity-a:workspace-a:metrics", now.Add(time.Second)) {
		t.Fatal("second request in same window was allowed")
	}
	if !limiter.Allow("identity-a:workspace-b:metrics", now.Add(time.Second)) {
		t.Fatal("independent workspace bucket was rejected")
	}
	if !limiter.Allow("identity-a:workspace-a:metrics", now.Add(time.Minute)) {
		t.Fatal("request after the rate window was rejected")
	}
}

func TestParseWorkspaceSignalPath(t *testing.T) {
	workspace, signal, ok := parseWorkspaceSignalPath("/v1/workspaces/" + testWorkspace + "/v1/traces")
	if !ok || workspace != testWorkspace || signal != "traces" {
		t.Fatalf("parsed path = %q/%q/%v", workspace, signal, ok)
	}
	for _, path := range []string{
		"/v1/workspaces/" + testWorkspace + "/v1/metrics/",
		"/v1/workspaces/" + testWorkspace + "/v1/metrics/extra",
		"/v1/workspaces//v1/metrics",
		"/v1/other/" + testWorkspace + "/v1/metrics",
	} {
		if _, _, ok := parseWorkspaceSignalPath(path); ok {
			t.Fatalf("accepted invalid path %q", path)
		}
	}
}

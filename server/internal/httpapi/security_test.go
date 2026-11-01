package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
	"testing"

	"github.com/pooya16pg-spec/orbit/server/internal/auth"
)

type fixedTokenVerifier struct {
	principal auth.Principal
	err       error
}

func (v fixedTokenVerifier) Verify(_ context.Context, token string) (auth.Principal, error) {
	if token != "valid-token" {
		return auth.Principal{}, context.Canceled
	}
	return v.principal, v.err
}

func TestAuthorizationRequiresBearerAndChecksRoles(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	viewer := fixedTokenVerifier{principal: auth.Principal{
		Issuer: "https://identity.example", Subject: "viewer-1", Roles: []string{"viewer"},
	}}
	handler := withAuthorization("production", viewer, next)

	tests := []struct {
		name       string
		method     string
		path       string
		authorization string
		wantStatus int
	}{
		{name: "valid viewer read", method: http.MethodGet, path: "/api/v1/services", authorization: "Bearer valid-token", wantStatus: http.StatusNoContent},
		{name: "missing bearer", method: http.MethodGet, path: "/api/v1/services", wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme", method: http.MethodGet, path: "/api/v1/services", authorization: "Basic valid-token", wantStatus: http.StatusUnauthorized},
		{name: "multiple authorization headers", method: http.MethodGet, path: "/api/v1/services", authorization: "Bearer valid-token", wantStatus: http.StatusUnauthorized},
		{name: "viewer cannot mutate", method: http.MethodPost, path: "/api/v1/incidents", authorization: "Bearer valid-token", wantStatus: http.StatusForbidden},
		{name: "metadata remains public", method: http.MethodGet, path: "/api/v1/meta", wantStatus: http.StatusNoContent},
		{name: "liveness remains public", method: http.MethodGet, path: "/health/live", wantStatus: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			if test.name == "multiple authorization headers" {
				request.Header.Add("Authorization", "Bearer another-token")
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if test.wantStatus == http.StatusUnauthorized && recorder.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("unauthorized response did not include WWW-Authenticate")
			}
		})
	}
}

func TestAuthorizationUsesOperatorAndAdministratorRoles(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	tests := []struct {
		name       string
		roles      []string
		path       string
		method     string
		wantStatus int
	}{
		{name: "operator can perform operational action", roles: []string{"operator"}, path: "/api/v1/incidents", method: http.MethodPost, wantStatus: http.StatusNoContent},
		{name: "operator cannot administer security", roles: []string{"operator"}, path: "/api/v1/admin/roles", method: http.MethodPost, wantStatus: http.StatusForbidden},
		{name: "administrator inherits operator", roles: []string{"administrator"}, path: "/api/v1/incidents", method: http.MethodPost, wantStatus: http.StatusNoContent},
		{name: "administrator can administer", roles: []string{"administrator"}, path: "/api/v1/admin/roles", method: http.MethodPost, wantStatus: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier := fixedTokenVerifier{principal: auth.Principal{
				Issuer: "https://identity.example", Subject: "operator-1", Roles: test.roles,
			}}
			handler := withAuthorization("production", verifier, next)
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Header.Set("Authorization", "Bearer valid-token")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
		})
	}
}

func TestAuthorizationFailsClosedOutsideDevelopment(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	for _, environment := range []string{"production", "staging"} {
		t.Run(environment, func(t *testing.T) {
			handler := withAuthorization(environment, nil, next)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/services", nil))
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
			}
		})
	}
}

func TestAuthorizationAllowsExplicitDevelopmentWithoutOIDC(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := withAuthorization("development", nil, next)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/services", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestRateLimiterEnforcesWindowAndBoundsTrackedKeys(t *testing.T) {
	limiter := newRequestRateLimiter(2)
	now := time.Now()
	keys := []string{"ip:192.0.2.1", "subject:https://identity.example|user-1"}
	if !limiter.allow(keys, now) || !limiter.allow(keys, now.Add(time.Second)) {
		t.Fatal("rate limiter rejected requests below the configured limit")
	}
	if limiter.allow(keys, now.Add(2*time.Second)) {
		t.Fatal("rate limiter accepted a request above the configured limit")
	}
	if !limiter.allow(keys, now.Add(time.Minute+time.Second)) {
		t.Fatal("rate limiter did not reset the next fixed window")
	}
}

func TestRateLimitMiddlewareReturnsRetryAfter(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := withRateLimit(1, next)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	request.RemoteAddr = "192.0.2.10:43210"

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, request)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d, want %d", first.Code, http.StatusNoContent)
	}
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, request)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("rate-limited response omitted Retry-After")
	}
}

func TestBodyLimitAndSecurityHeaders(t *testing.T) {
	bodyHandler := withBodyLimit(4, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	handler := withSecurityHeaders(bodyHandler)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader("12345"))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
	for name, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options": "DENY",
		"Referrer-Policy": "no-referrer",
	} {
		if got := recorder.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

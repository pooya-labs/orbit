package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/pooya16pg-spec/orbit/server/internal/auth"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

const (
	testWorkspace = "00000000-0000-4000-8000-000000000001"
	testIssuer    = "https://identity.example"
	testSubject   = "workload-123"
	testIdentity  = "00000000-0000-4000-8000-000000000099"
)

type verifierStub struct {
	principal auth.Principal
	err       error
	calls     int
	token     string
}

func (v *verifierStub) Verify(_ context.Context, token string) (auth.Principal, error) {
	v.calls++
	v.token = token
	return v.principal, v.err
}

type grantStub struct {
	identity store.WorkloadIdentity
	err      error
	calls    int
	issuer   string
	subject  string
	workspace string
	signal   string
}

func (g *grantStub) AuthorizeWorkloadTelemetry(_ context.Context, issuer, subject, workspace, signal string) (store.WorkloadIdentity, error) {
	g.calls++
	g.issuer, g.subject, g.workspace, g.signal = issuer, subject, workspace, signal
	return g.identity, g.err
}

func validStubs() (*verifierStub, *grantStub) {
	return &verifierStub{principal: auth.Principal{Issuer: testIssuer, Subject: testSubject}},
		&grantStub{identity: store.WorkloadIdentity{ID: testIdentity, Issuer: testIssuer, Subject: testSubject, Status: "active"}}
}

func TestBearerTokenRequiresOneUnambiguousCredential(t *testing.T) {
	for _, test := range []struct {
		name   string
		values []string
		want   string
		ok     bool
	}{
		{name: "valid", values: []string{"Bearer signed-token"}, want: "signed-token", ok: true},
		{name: "case insensitive scheme", values: []string{"bearer signed-token"}, want: "signed-token", ok: true},
		{name: "missing", values: nil},
		{name: "duplicate headers", values: []string{"Bearer first", "Bearer second"}},
		{name: "basic scheme", values: []string{"Basic token"}},
		{name: "missing token", values: []string{"Bearer"}},
		{name: "extra token", values: []string{"Bearer one two"}},
		{name: "comma ambiguity", values: []string{"Bearer one,two"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := BearerToken(test.values)
			if (err == nil) != test.ok || got != test.want {
				t.Fatalf("BearerToken() = %q, %v; want %q, ok=%v", got, err, test.want, test.ok)
			}
		})
	}
}

func TestAuthorizeUsesOnlyVerifiedPrincipalAndReturnsCanonicalDecision(t *testing.T) {
	verifier, grants := validStubs()
	authorizer, err := NewAuthorizer(verifier, grants)
	if err != nil {
		t.Fatal(err)
	}

	decision, err := authorizer.Authorize(context.Background(), []string{"Bearer signed-token"}, testWorkspace, "metrics")
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if verifier.calls != 1 || verifier.token != "signed-token" {
		t.Fatalf("unexpected verifier calls/token: %+v", verifier)
	}
	if grants.calls != 1 || grants.issuer != testIssuer || grants.subject != testSubject {
		t.Fatalf("grant lookup did not use verified principal: %+v", grants)
	}
	if grants.workspace != testWorkspace || grants.signal != "metrics" {
		t.Fatalf("grant lookup target = %s/%s", grants.workspace, grants.signal)
	}
	if decision.WorkspaceID != testWorkspace || decision.Signal != "metrics" || decision.IdentityID != testIdentity {
		t.Fatalf("unexpected authorization decision: %+v", decision)
	}
}

func TestAuthorizeRejectsInvalidTargetBeforeTokenOrDatabaseWork(t *testing.T) {
	for _, target := range []struct{ workspace, signal string }{
		{workspace: "not-a-uuid", signal: "metrics"},
		{workspace: testWorkspace, signal: "profiles"},
	} {
		verifier, grants := validStubs()
		authorizer, _ := NewAuthorizer(verifier, grants)
		if _, err := authorizer.Authorize(context.Background(), []string{"Bearer token"}, target.workspace, target.signal); !errors.Is(err, ErrInvalidTarget) {
			t.Fatalf("Authorize() error = %v, want ErrInvalidTarget", err)
		}
		if verifier.calls != 0 || grants.calls != 0 {
			t.Fatal("invalid target reached verifier or database")
		}
	}
}

func TestAuthorizeRejectsInvalidBearerBeforeVerification(t *testing.T) {
	verifier, grants := validStubs()
	authorizer, _ := NewAuthorizer(verifier, grants)
	if _, err := authorizer.Authorize(context.Background(), []string{"Bearer one", "Bearer two"}, testWorkspace, "logs"); !errors.Is(err, ErrInvalidBearer) {
		t.Fatalf("Authorize() error = %v, want ErrInvalidBearer", err)
	}
	if verifier.calls != 0 || grants.calls != 0 {
		t.Fatal("ambiguous bearer reached verifier or database")
	}
}

func TestAuthorizeDoesNotQueryGrantWhenTokenVerificationFails(t *testing.T) {
	verifier, grants := validStubs()
	verifier.err = errors.New("signature invalid")
	authorizer, _ := NewAuthorizer(verifier, grants)
	if _, err := authorizer.Authorize(context.Background(), []string{"Bearer invalid"}, testWorkspace, "traces"); !errors.Is(err, ErrInvalidBearer) {
		t.Fatalf("Authorize() error = %v, want ErrInvalidBearer", err)
	}
	if grants.calls != 0 {
		t.Fatal("invalid token reached workload grant repository")
	}
}

func TestAuthorizeMapsDeniedGrantAndFailsClosedOnRepositoryError(t *testing.T) {
	t.Run("denied", func(t *testing.T) {
		verifier, grants := validStubs()
		grants.err = store.ErrWorkloadAuthorizationDenied
		authorizer, _ := NewAuthorizer(verifier, grants)
		if _, err := authorizer.Authorize(context.Background(), []string{"Bearer token"}, testWorkspace, "metrics"); !errors.Is(err, ErrDenied) {
			t.Fatalf("Authorize() error = %v, want ErrDenied", err)
		}
	})

	t.Run("database unavailable", func(t *testing.T) {
		verifier, grants := validStubs()
		grants.err = errors.New("postgres connection refused")
		authorizer, _ := NewAuthorizer(verifier, grants)
		if _, err := authorizer.Authorize(context.Background(), []string{"Bearer token"}, testWorkspace, "metrics"); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Authorize() error = %v, want ErrUnavailable", err)
		}
	})
}

func TestAuthorizeRejectsRepositoryPrincipalMismatchOrInactiveIdentity(t *testing.T) {
	for _, identity := range []store.WorkloadIdentity{
		{ID: testIdentity, Issuer: testIssuer, Subject: "attacker", Status: "active"},
		{ID: testIdentity, Issuer: testIssuer, Subject: testSubject, Status: "disabled"},
		{ID: "", Issuer: testIssuer, Subject: testSubject, Status: "active"},
	} {
		verifier, grants := validStubs()
		grants.identity = identity
		authorizer, _ := NewAuthorizer(verifier, grants)
		if _, err := authorizer.Authorize(context.Background(), []string{"Bearer token"}, testWorkspace, "logs"); !errors.Is(err, ErrDenied) {
			t.Fatalf("Authorize() error = %v, want ErrDenied for identity %+v", err, identity)
		}
	}
}

func TestNewAuthorizerRequiresDependencies(t *testing.T) {
	verifier, grants := validStubs()
	if _, err := NewAuthorizer(nil, grants); err == nil {
		t.Fatal("NewAuthorizer accepted a nil verifier")
	}
	if _, err := NewAuthorizer(verifier, nil); err == nil {
		t.Fatal("NewAuthorizer accepted a nil grant repository")
	}
}

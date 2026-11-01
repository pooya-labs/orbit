// Package ingest contains the transport-independent authorization boundary
// for authenticated workspace telemetry ingestion.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/pooya16pg-spec/orbit/server/internal/auth"
	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

var (
	ErrInvalidBearer = errors.New("invalid workload bearer token")
	ErrInvalidTarget = errors.New("invalid telemetry ingestion target")
	ErrDenied        = errors.New("workload is not authorized for this telemetry target")
	ErrUnavailable   = errors.New("workload authorization is unavailable")
	workspaceIDRE    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

type TokenVerifier interface {
	Verify(context.Context, string) (auth.Principal, error)
}

type WorkloadAuthorizer interface {
	AuthorizeWorkloadTelemetry(context.Context, string, string, string, string) (store.WorkloadIdentity, error)
}

// Decision is produced only after token verification and a successful
// database-backed workspace+signal grant check. It is safe for transports to
// use as the source of canonical tenant attribution.
type Decision struct {
	WorkspaceID string
	Signal      string
	IdentityID  string
	Issuer      string
	Subject     string
}

type Authorizer struct {
	verifier TokenVerifier
	grants   WorkloadAuthorizer
}

func NewAuthorizer(verifier TokenVerifier, grants WorkloadAuthorizer) (*Authorizer, error) {
	if verifier == nil {
		return nil, errors.New("workload token verifier is required")
	}
	if grants == nil {
		return nil, errors.New("workload grant repository is required")
	}
	return &Authorizer{verifier: verifier, grants: grants}, nil
}

// BearerToken accepts exactly one Authorization header containing one Bearer
// credential. Multiple header values and ambiguous whitespace/token forms are
// rejected rather than selecting an arbitrary credential.
func BearerToken(values []string) (string, error) {
	if len(values) != 1 {
		return "", ErrInvalidBearer
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", ErrInvalidBearer
	}
	if strings.ContainsAny(parts[1], ",\r\n\t ") {
		return "", ErrInvalidBearer
	}
	return parts[1], nil
}

// Authorize verifies the workload access token and then checks its verified
// issuer/subject against the requested workspace and signal. workspaceID is
// an untrusted routing selector; it never grants access by itself.
func (a *Authorizer) Authorize(ctx context.Context, bearerValues []string, workspaceID, signal string) (Decision, error) {
	if !workspaceIDRE.MatchString(workspaceID) || !validSignal(signal) {
		return Decision{}, ErrInvalidTarget
	}
	token, err := BearerToken(bearerValues)
	if err != nil {
		return Decision{}, err
	}
	principal, err := a.verifier.Verify(ctx, token)
	if err != nil || strings.TrimSpace(principal.Issuer) == "" || strings.TrimSpace(principal.Subject) == "" {
		return Decision{}, ErrInvalidBearer
	}

	identity, err := a.grants.AuthorizeWorkloadTelemetry(ctx, principal.Issuer, principal.Subject, workspaceID, signal)
	if err != nil {
		if errors.Is(err, store.ErrWorkloadAuthorizationDenied) || errors.Is(err, store.ErrWorkloadIdentityNotFound) {
			return Decision{}, ErrDenied
		}
		return Decision{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	// The repository is the authorization authority. Still guard against a
	// faulty implementation returning an identity for another principal.
	if identity.Issuer != principal.Issuer || identity.Subject != principal.Subject || identity.ID == "" || identity.Status != "active" {
		return Decision{}, ErrDenied
	}
	return Decision{
		WorkspaceID: strings.ToLower(workspaceID),
		Signal:      signal,
		IdentityID:  identity.ID,
		Issuer:      principal.Issuer,
		Subject:     principal.Subject,
	}, nil
}

func validSignal(signal string) bool {
	switch signal {
	case "metrics", "logs", "traces":
		return true
	default:
		return false
	}
}

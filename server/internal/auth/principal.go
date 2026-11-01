package auth

import (
	"context"
	"slices"
)

const (
	RoleViewer        = "viewer"
	RoleOperator      = "operator"
	RoleAdministrator = "administrator"
)

// Principal is the authenticated identity established by a verified OAuth access token.
type Principal struct {
	Issuer  string
	Subject string
	Roles   []string
}

// ActorID is a stable, issuer-scoped identity; email and display names are deliberately excluded.
func (p Principal) ActorID() string {
	return p.Issuer + "|" + p.Subject
}

// HasRole applies the role hierarchy used by ORBIT: operators and administrators inherit viewer access.
func (p Principal) HasRole(role string) bool {
	if slices.Contains(p.Roles, role) {
		return true
	}
	switch role {
	case RoleViewer:
		return slices.Contains(p.Roles, RoleOperator) || slices.Contains(p.Roles, RoleAdministrator)
	case RoleOperator:
		return slices.Contains(p.Roles, RoleAdministrator)
	default:
		return false
	}
}


type principalContextKey struct{}

// TokenVerifier verifies bearer access tokens against the configured issuer and audience.
type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (Principal, error)
}

// ContextWithPrincipal attaches a verified identity to a request context.
func ContextWithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns the identity established by authentication middleware.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

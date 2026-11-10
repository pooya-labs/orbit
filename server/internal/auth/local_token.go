package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
)

// StaticTokenVerifier is reserved for the explicit, loopback-only Local Work
// runtime. It must never be used by staging or production deployments.
type StaticTokenVerifier struct {
	token     []byte
	principal Principal
}

func NewStaticTokenVerifier(token string, principal Principal) (*StaticTokenVerifier, error) {
	token = strings.TrimSpace(token)
	if len(token) < 32 || len(token) > maxBearerTokenBytes {
		return nil, errors.New("local access token must contain between 32 and 16384 bytes")
	}
	if strings.TrimSpace(principal.Issuer) == "" || strings.TrimSpace(principal.Subject) == "" {
		return nil, errors.New("local token principal issuer and subject are required")
	}
	if len(principal.Roles) == 0 {
		return nil, errors.New("local token principal must have at least one role")
	}
	roles := append([]string(nil), principal.Roles...)
	return &StaticTokenVerifier{token: []byte(token), principal: Principal{
		Issuer: principal.Issuer, Subject: principal.Subject, Roles: roles,
	}}, nil
}

func (v *StaticTokenVerifier) Verify(ctx context.Context, token string) (Principal, error) {
	if v == nil || ctx.Err() != nil || len(token) != len(v.token) ||
		subtle.ConstantTimeCompare([]byte(token), v.token) != 1 {
		return Principal{}, errors.New("invalid local access token")
	}
	principal := v.principal
	principal.Roles = append([]string(nil), v.principal.Roles...)
	return principal, nil
}

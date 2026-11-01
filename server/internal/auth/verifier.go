package auth

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

const (
	maxBearerTokenBytes = 16 << 10
	maxDiscoveryBytes   = 64 << 10
	maxJWKSBytes        = 1 << 20
	maxJWKSKeys         = 100
	jwksCacheTTL        = 5 * time.Minute
	minForcedRefresh    = 15 * time.Second
	clockSkew           = 60 * time.Second
)

var (
	ErrInvalidToken = errors.New("invalid access token")
	ErrMissingKey   = errors.New("access token signing key is not present in the trusted JWKS")
)

// Config contains the trusted resource-server settings. Issuer and JWKS URLs are never read from a token.
type Config struct {
	Issuer     string
	Audience   string
	RolesClaim string
	Algorithms []string
	Development bool
}

type discoveryDocument struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

type Verifier struct {
	issuer     string
	audience   string
	rolesClaim string
	algorithms map[string]struct{}
	jwksURL    string
	client     *http.Client

	mu                 sync.Mutex
	keys               jwk.Set
	expiresAt          time.Time
	lastUnknownRefresh time.Time
}

type tokenClaims struct {
	Issuer  string `json:"iss"`
	Subject string `json:"sub"`
}

// NewVerifier performs OIDC discovery and fetches the initial JWKS before the API starts.
// A missing or invalid issuer/JWKS document is a startup error, so non-development deployments fail closed.
func NewVerifier(ctx context.Context, cfg Config) (*Verifier, error) {
	issuer := strings.TrimSpace(cfg.Issuer)
	audience := strings.TrimSpace(cfg.Audience)
	if issuer == "" || audience == "" {
		return nil, errors.New("OIDC issuer and audience are required")
	}
	issuerURL, err := validateRemoteURL(issuer, cfg.Development)
	if err != nil {
		return nil, fmt.Errorf("invalid OIDC issuer: %w", err)
	}
	if cfg.RolesClaim == "" {
		cfg.RolesClaim = "roles"
	}
	if len(cfg.RolesClaim) > 64 || !claimNamePattern.MatchString(cfg.RolesClaim) {
		return nil, errors.New("invalid OIDC roles claim name")
	}
	algorithms := make(map[string]struct{}, len(cfg.Algorithms))
	for _, algorithm := range cfg.Algorithms {
		switch algorithm {
		case "RS256", "ES256":
			algorithms[algorithm] = struct{}{}
		default:
			return nil, fmt.Errorf("unsupported OIDC signing algorithm %q", algorithm)
		}
	}
	if len(algorithms) == 0 {
		return nil, errors.New("at least one asymmetric OIDC signing algorithm is required")
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			MaxIdleConns: 16,
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout: 30 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many OIDC discovery redirects")
			}
			if len(via) > 0 {
				previous := via[len(via)-1].URL
				if !strings.EqualFold(previous.Scheme, req.URL.Scheme) ||
					!strings.EqualFold(previous.Host, req.URL.Host) {
					return errors.New("cross-origin OIDC redirects are not allowed")
				}
			}
			return nil
		},
	}

	discoveryURL := strings.TrimRight(issuerURL.String(), "/") + "/.well-known/openid-configuration"
	discoveryBody, err := fetchBounded(ctx, client, discoveryURL, maxDiscoveryBytes)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery failed: %w", err)
	}
	var document discoveryDocument
	if err := json.Unmarshal(discoveryBody, &document); err != nil {
		return nil, fmt.Errorf("invalid OIDC discovery document: %w", err)
	}
	if document.Issuer != issuer {
		return nil, errors.New("OIDC discovery issuer does not match configured issuer")
	}
	jwksURL, err := validateRemoteURL(document.JWKSURI, cfg.Development)
	if err != nil {
		return nil, fmt.Errorf("invalid OIDC JWKS URI: %w", err)
	}

	verifier := &Verifier{
		issuer: issuer,
		audience: audience,
		rolesClaim: cfg.RolesClaim,
		algorithms: algorithms,
		jwksURL: jwksURL.String(),
		client: client,
	}
	if err := verifier.refresh(ctx); err != nil {
		return nil, fmt.Errorf("initial OIDC JWKS fetch failed: %w", err)
	}
	return verifier, nil
}

// Verify validates a compact signed OAuth access token and extracts only identity and allow-listed roles.
func (v *Verifier) Verify(ctx context.Context, rawToken string) (Principal, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" || len(rawToken) > maxBearerTokenBytes || strings.Count(rawToken, ".") != 2 {
		return Principal{}, ErrInvalidToken
	}

	message, err := jws.Parse([]byte(rawToken), jws.WithCompact())
	if err != nil || len(message.Signatures()) != 1 {
		return Principal{}, ErrInvalidToken
	}
	headers := message.Signatures()[0].ProtectedHeaders()
	if headers == nil {
		return Principal{}, ErrInvalidToken
	}
	algorithm, hasAlgorithm := headers.Algorithm()
	keyID, hasKeyID := headers.KeyID()
	if !hasAlgorithm || !hasKeyID || keyID == "" {
		return Principal{}, ErrInvalidToken
	}
	algorithmName := fmt.Sprint(algorithm)
	if _, allowed := v.algorithms[algorithmName]; !allowed {
		return Principal{}, ErrInvalidToken
	}
	if _, present := headers.JWKSetURL(); present {
		return Principal{}, ErrInvalidToken
	}
	if _, present := headers.JWK(); present {
		return Principal{}, ErrInvalidToken
	}
	if _, present := headers.X509URL(); present {
		return Principal{}, ErrInvalidToken
	}
	if critical, present := headers.Critical(); present && len(critical) > 0 {
		return Principal{}, ErrInvalidToken
	}
	if tokenType, present := headers.Type(); present &&
		!strings.EqualFold(tokenType, "JWT") &&
		!strings.EqualFold(tokenType, "at+jwt") &&
		!strings.EqualFold(tokenType, "application/at+jwt") {
		return Principal{}, ErrInvalidToken
	}

	key, err := v.lookupKey(ctx, keyID)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	if err := validateSigningKey(key, algorithmName); err != nil {
		return Principal{}, ErrInvalidToken
	}

	singleKeySet := jwk.NewSet()
	keyCopy, err := key.Clone()
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	if err := singleKeySet.AddKey(keyCopy); err != nil {
		return Principal{}, ErrInvalidToken
	}
	_, err = jwt.Parse(
		[]byte(rawToken),
		jwt.WithKeySet(singleKeySet, jws.WithRequireKid(true)),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithRequiredClaim(jwt.IssuerKey),
		jwt.WithRequiredClaim(jwt.AudienceKey),
		jwt.WithRequiredClaim(jwt.ExpirationKey),
		jwt.WithRequiredClaim(jwt.IssuedAtKey),
		jwt.WithRequiredClaim(jwt.SubjectKey),
		jwt.WithAcceptableSkew(clockSkew),
	)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}

	claims, err := decodeClaims(rawToken)
	if err != nil || claims.Issuer != v.issuer || strings.TrimSpace(claims.Subject) == "" {
		return Principal{}, ErrInvalidToken
	}
	roles, err := decodeRoles(rawToken, v.rolesClaim)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	allowedRoles := make([]string, 0, len(roles))
	seen := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		switch role {
		case RoleViewer, RoleOperator, RoleAdministrator:
			if _, ok := seen[role]; !ok {
				seen[role] = struct{}{}
				allowedRoles = append(allowedRoles, role)
			}
		}
	}
	return Principal{Issuer: claims.Issuer, Subject: claims.Subject, Roles: allowedRoles}, nil
}

// Close releases idle HTTP connections held by the verifier.
func (v *Verifier) Close() {
	if v != nil && v.client != nil {
		v.client.CloseIdleConnections()
	}
}

func (v *Verifier) lookupKey(ctx context.Context, keyID string) (jwk.Key, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	if v.keys != nil && now.Before(v.expiresAt) {
		if key, ok := v.keys.LookupKeyID(keyID); ok {
			return key, nil
		}
		if !v.lastUnknownRefresh.IsZero() && now.Sub(v.lastUnknownRefresh) < minForcedRefresh {
			return nil, ErrMissingKey
		}
		v.lastUnknownRefresh = now
	}
	if err := v.refreshLocked(ctx); err != nil {
		// Never accept a key from an expired JWKS after a failed refresh.
		return nil, err
	}
	if key, ok := v.keys.LookupKeyID(keyID); ok {
		return key, nil
	}
	return nil, ErrMissingKey
}

func (v *Verifier) refresh(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.refreshLocked(ctx)
}

func (v *Verifier) refreshLocked(ctx context.Context) error {
	body, err := fetchBounded(ctx, v.client, v.jwksURL, maxJWKSBytes)
	if err != nil {
		return err
	}
	set, err := jwk.Parse(body)
	if err != nil {
		return fmt.Errorf("parse trusted JWKS: %w", err)
	}
	if set.Len() == 0 || set.Len() > maxJWKSKeys {
		return errors.New("trusted JWKS has an invalid number of keys")
	}
	seen := make(map[string]struct{}, set.Len())
	for index := 0; index < set.Len(); index++ {
		key, ok := set.Key(index)
		if !ok {
			return errors.New("trusted JWKS contains an unreadable key")
		}
		kid, ok := key.KeyID()
		if !ok || kid == "" {
			return errors.New("trusted JWKS contains a key without kid")
		}
		if _, exists := seen[kid]; exists {
			return errors.New("trusted JWKS contains duplicate kid values")
		}
		seen[kid] = struct{}{}
	}
	v.keys = set
	v.expiresAt = time.Now().Add(jwksCacheTTL)
	return nil
}

func validateSigningKey(key jwk.Key, algorithm string) error {
	if use, ok := key.KeyUsage(); ok && use != "sig" {
		return errors.New("JWK is not designated for signatures")
	}
	if operations, ok := key.KeyOps(); ok {
		canVerify := false
		for _, operation := range operations {
			if operation == jwk.KeyOpVerify {
				canVerify = true
				break
			}
		}
		if !canVerify {
			return errors.New("JWK key_ops does not permit signature verification")
		}
	}
	if keyAlgorithm, ok := key.Algorithm(); ok && fmt.Sprint(keyAlgorithm) != algorithm {
		return errors.New("JWK algorithm does not match protected header")
	}
	switch algorithm {
	case "RS256":
		if fmt.Sprint(key.KeyType()) != "RSA" {
			return errors.New("RS256 requires an RSA JWK")
		}
		var modulus string
		if err := key.Get("n", &modulus); err != nil {
			return errors.New("RSA JWK is missing its modulus")
		}
		decoded, err := base64.RawURLEncoding.DecodeString(modulus)
		if err != nil || len(decoded) == 0 || new(big.Int).SetBytes(decoded).BitLen() < 2048 {
			return errors.New("RS256 requires an RSA modulus of at least 2048 bits")
		}
	case "ES256":
		if fmt.Sprint(key.KeyType()) != "EC" {
			return errors.New("ES256 requires an EC JWK")
		}
		var curve string
		if err := key.Get("crv", &curve); err != nil || curve != "P-256" {
			return errors.New("ES256 requires a P-256 JWK")
		}
	default:
		return errors.New("unsupported signing algorithm")
	}
	for _, privateField := range []string{"d", "p", "q", "dp", "dq", "qi", "k", "oth"} {
		if key.Has(privateField) {
			return errors.New("JWKS must not contain private key material")
		}
	}
	return nil
}

func decodeClaims(rawToken string) (tokenClaims, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return tokenClaims{}, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenClaims{}, ErrInvalidToken
	}
	var claims tokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return tokenClaims{}, ErrInvalidToken
	}
	return claims, nil
}

func decodeRoles(rawToken, claimName string) ([]string, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var claims map[string]json.RawMessage
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrInvalidToken
	}
	rawRoles, ok := claims[claimName]
	if !ok {
		return nil, nil
	}
	var roles []string
	if err := json.Unmarshal(rawRoles, &roles); err != nil {
		return nil, ErrInvalidToken
	}
	return roles, nil
}

func fetchBounded(ctx context.Context, client *http.Client, endpoint string, maxBytes int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trusted OIDC endpoint returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, errors.New("trusted OIDC response exceeds size limit")
	}
	return body, nil
}

func validateRemoteURL(value string, allowLoopbackHTTP bool) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("URL must be absolute and must not contain credentials, query, or fragment")
	}
	loopback := strings.EqualFold(parsed.Hostname(), "localhost")
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	if parsed.Scheme != "https" && !(allowLoopbackHTTP && loopback && parsed.Scheme == "http") {
		return nil, errors.New("URL must use HTTPS except for loopback development")
	}
	return parsed, nil
}

var claimNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

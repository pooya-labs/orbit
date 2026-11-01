package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	maxBearerTokenBytes = 16 << 10
	maxRemoteDocumentBytes = 1 << 20
	defaultCacheTTL = 5 * time.Minute
	forcedRefreshInterval = 30 * time.Second
)

type Config struct {
	Issuer            string
	Audience          string
	RolesClaim        string
	ClockSkew         time.Duration
	CacheTTL          time.Duration
	HTTPTimeout       time.Duration
	AllowInsecureHTTP bool
}

type Principal struct {
	Issuer  string
	Subject string
	Roles   []string
}

func (p Principal) HasRole(required string) bool {
	rank := func(role string) int {
		switch role {
		case "viewer":
			return 1
		case "operator":
			return 2
		case "administrator":
			return 3
		default:
			return 0
		}
	}
	requiredRank := rank(required)
	if requiredRank == 0 {
		return false
	}
	for _, role := range p.Roles {
		if rank(role) >= requiredRank {
			return true
		}
	}
	return false
}

type Verifier struct {
	issuer     string
	audience   string
	rolesClaim string
	skew       time.Duration
	cacheTTL   time.Duration
	client     *http.Client
	jwksURL    string

	mu                  sync.Mutex
	keys                map[string]verificationKey
	keysExpireAt        time.Time
	lastUnknownKeyFetch time.Time
}

type verificationKey struct {
	algorithm string
	publicKey crypto.PublicKey
}

type discoveryDocument struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

type jwksDocument struct {
	Keys []json.RawMessage `json:"keys"`
}

type jwkDocument struct {
	KeyType string   `json:"kty"`
	KeyID   string   `json:"kid"`
	Use     string   `json:"use"`
	Alg     string   `json:"alg"`
	KeyOps  []string `json:"key_ops"`
	N       string   `json:"n"`
	E       string   `json:"e"`
	Crv     string   `json:"crv"`
	X       string   `json:"x"`
	Y       string   `json:"y"`
}

func NewVerifier(ctx context.Context, cfg Config) (*Verifier, error) {
	cfg.Issuer = strings.TrimSpace(cfg.Issuer)
	cfg.Audience = strings.TrimSpace(cfg.Audience)
	cfg.RolesClaim = strings.TrimSpace(cfg.RolesClaim)
	if cfg.Issuer == "" || cfg.Audience == "" {
		return nil, errors.New("OIDC issuer and API audience are required")
	}
	if cfg.RolesClaim == "" {
		cfg.RolesClaim = "roles"
	}
	if cfg.ClockSkew <= 0 {
		cfg.ClockSkew = 60 * time.Second
	}
	if cfg.ClockSkew > 5*time.Minute {
		return nil, errors.New("OIDC clock skew must not exceed five minutes")
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = defaultCacheTTL
	}
	if cfg.CacheTTL < time.Minute || cfg.CacheTTL > 24*time.Hour {
		return nil, errors.New("OIDC JWKS cache TTL must be between one minute and 24 hours")
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 5 * time.Second
	}

	issuerURL, err := url.Parse(cfg.Issuer)
	if err != nil || issuerURL.Host == "" || issuerURL.User != nil || issuerURL.RawQuery != "" || issuerURL.Fragment != "" {
		return nil, errors.New("OIDC issuer must be an absolute URL without credentials, query, or fragment")
	}
	if err := validateRemoteURL(issuerURL, cfg.AllowInsecureHTTP); err != nil {
		return nil, fmt.Errorf("invalid OIDC issuer: %w", err)
	}

	client := &http.Client{
		Timeout: cfg.HTTPTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	verifier := &Verifier{
		issuer: cfg.Issuer, audience: cfg.Audience, rolesClaim: cfg.RolesClaim,
		skew: cfg.ClockSkew, cacheTTL: cfg.CacheTTL, client: client,
		keys: make(map[string]verificationKey),
	}

	discoveryURL := strings.TrimRight(cfg.Issuer, "/") + "/.well-known/openid-configuration"
	var metadata discoveryDocument
	if err := verifier.fetchJSON(ctx, discoveryURL, &metadata); err != nil {
		return nil, fmt.Errorf("load OIDC discovery document: %w", err)
	}
	if metadata.Issuer != cfg.Issuer {
		return nil, errors.New("OIDC discovery issuer does not exactly match configured issuer")
	}
	jwksURL, err := url.Parse(metadata.JWKSURI)
	if err != nil || jwksURL.Host == "" || jwksURL.User != nil || jwksURL.RawQuery != "" || jwksURL.Fragment != "" {
		return nil, errors.New("OIDC discovery returned an invalid jwks_uri")
	}
	if err := validateRemoteURL(jwksURL, cfg.AllowInsecureHTTP); err != nil {
		return nil, fmt.Errorf("invalid OIDC jwks_uri: %w", err)
	}
	verifier.jwksURL = metadata.JWKSURI

	verifier.mu.Lock()
	err = verifier.refreshKeysLocked(ctx)
	verifier.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("load OIDC signing keys: %w", err)
	}
	return verifier, nil
}

func validateRemoteURL(u *url.URL, allowInsecureHTTP bool) error {
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme != "http" || !allowInsecureHTTP {
		return errors.New("HTTPS is required outside explicit development mode")
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("development HTTP endpoints must use loopback addresses")
	}
	return nil
}

func (v *Verifier) fetchJSON(ctx context.Context, target string, destination any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := v.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("remote endpoint returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxRemoteDocumentBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxRemoteDocumentBytes {
		return errors.New("remote JSON document exceeds the one-megabyte limit")
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return fmt.Errorf("decode remote JSON document: %w", err)
	}
	return nil
}

func (v *Verifier) refreshKeysLocked(ctx context.Context) error {
	var document jwksDocument
	if err := v.fetchJSON(ctx, v.jwksURL, &document); err != nil {
		return err
	}
	keys := make(map[string]verificationKey, len(document.Keys))
	for _, raw := range document.Keys {
		var item jwkDocument
		if err := json.Unmarshal(raw, &item); err != nil {
			return fmt.Errorf("decode JWK: %w", err)
		}
		if item.KeyID == "" {
			continue
		}
		if item.Use != "" && item.Use != "sig" {
			continue
		}
		if len(item.KeyOps) > 0 && !contains(item.KeyOps, "verify") {
			continue
		}
		key, algorithm, err := parseVerificationJWK(item)
		if err != nil {
			if item.KeyType == "RSA" || item.KeyType == "EC" {
				return fmt.Errorf("invalid signing key %q: %w", item.KeyID, err)
			}
			continue
		}
		if _, exists := keys[item.KeyID]; exists {
			return fmt.Errorf("JWKS contains duplicate key ID %q", item.KeyID)
		}
		keys[item.KeyID] = verificationKey{algorithm: algorithm, publicKey: key}
	}
	if len(keys) == 0 {
		return errors.New("JWKS contains no supported signing keys")
	}
	v.keys = keys
	v.keysExpireAt = time.Now().Add(v.cacheTTL)
	return nil
}

func parseVerificationJWK(item jwkDocument) (crypto.PublicKey, string, error) {
	switch item.Alg {
	case "", "RS256", "ES256":
	default:
		return nil, "", fmt.Errorf("unsupported JWK algorithm %q", item.Alg)
	}

	switch item.KeyType {
	case "RSA":
		if item.Alg != "" && item.Alg != "RS256" {
			return nil, "", errors.New("RSA key algorithm must be RS256")
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(item.N)
		if err != nil || len(nBytes) == 0 {
			return nil, "", errors.New("invalid RSA modulus")
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(item.E)
		if err != nil || len(eBytes) == 0 || len(eBytes) > 4 {
			return nil, "", errors.New("invalid RSA exponent")
		}
		exponent := new(big.Int).SetBytes(eBytes)
		if !exponent.IsInt64() {
			return nil, "", errors.New("RSA exponent is out of range")
		}
		e := int(exponent.Int64())
		n := new(big.Int).SetBytes(nBytes)
		if n.BitLen() < 2048 || e < 3 || e%2 == 0 {
			return nil, "", errors.New("RSA key must be at least 2048 bits with a valid exponent")
		}
		return &rsa.PublicKey{N: n, E: e}, "RS256", nil

	case "EC":
		if item.Alg != "" && item.Alg != "ES256" {
			return nil, "", errors.New("EC key algorithm must be ES256")
		}
		if item.Crv != "P-256" {
			return nil, "", fmt.Errorf("unsupported EC curve %q", item.Crv)
		}
		xBytes, err := base64.RawURLEncoding.DecodeString(item.X)
		if err != nil || len(xBytes) != 32 {
			return nil, "", errors.New("invalid P-256 x coordinate")
		}
		yBytes, err := base64.RawURLEncoding.DecodeString(item.Y)
		if err != nil || len(yBytes) != 32 {
			return nil, "", errors.New("invalid P-256 y coordinate")
		}
		x, y := new(big.Int).SetBytes(xBytes), new(big.Int).SetBytes(yBytes)
		curve := elliptic.P256()
		if !curve.IsOnCurve(x, y) {
			return nil, "", errors.New("P-256 public key is not on the curve")
		}
		return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, "ES256", nil
	default:
		return nil, "", fmt.Errorf("unsupported JWK key type %q", item.KeyType)
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (v *Verifier) keyFor(ctx context.Context, keyID, algorithm string) (crypto.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	if !now.Before(v.keysExpireAt) {
		if err := v.refreshKeysLocked(ctx); err != nil {
			return nil, fmt.Errorf("refresh OIDC signing keys: %w", err)
		}
	}
	key, ok := v.keys[keyID]
	if !ok && (v.lastUnknownKeyFetch.IsZero() || now.Sub(v.lastUnknownKeyFetch) >= forcedRefreshInterval) {
		v.lastUnknownKeyFetch = now
		if err := v.refreshKeysLocked(ctx); err != nil {
			return nil, fmt.Errorf("refresh OIDC signing keys for unknown key ID: %w", err)
		}
		key, ok = v.keys[keyID]
	}
	if !ok {
		return nil, errors.New("token signing key ID is not present in the trusted JWKS")
	}
	if key.algorithm != algorithm {
		return nil, errors.New("token algorithm does not match the trusted signing key")
	}
	return key.publicKey, nil
}

func (v *Verifier) Verify(ctx context.Context, serialized string) (Principal, error) {
	if serialized == "" || len(serialized) > maxBearerTokenBytes {
		return Principal{}, errors.New("bearer token is empty or exceeds the 16 KiB limit")
	}

	parsed, err := jwt.Parse(serialized, func(token *jwt.Token) (any, error) {
		algorithm := token.Method.Alg()
		if algorithm != "RS256" && algorithm != "ES256" {
			return nil, errors.New("JWT signing algorithm is not allowed")
		}
		if tokenType, present := token.Header["typ"]; present {
			value, ok := tokenType.(string)
			if !ok || (value != "JWT" && value != "at+JWT" && value != "at+jwt") {
				return nil, errors.New("JWT type is not an access-token type")
			}
		}
		keyID, ok := token.Header["kid"].(string)
		if !ok || keyID == "" {
			return nil, errors.New("JWT is missing its key ID")
		}
		return v.keyFor(ctx, keyID, algorithm)
	},
		jwt.WithValidMethods([]string{"RS256", "ES256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(v.skew),
		jwt.WithJSONNumber(),
		jwt.WithStrictDecoding(),
	)
	if err != nil || parsed == nil || !parsed.Valid {
		return Principal{}, errors.New("access token signature or registered claims are invalid")
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Principal{}, errors.New("access token claims have an unsupported representation")
	}
	subject, err := claims.GetSubject()
	if err != nil || strings.TrimSpace(subject) == "" {
		return Principal{}, errors.New("access token subject is missing or invalid")
	}
	issuedAt, err := claims.GetIssuedAt()
	if err != nil || issuedAt == nil {
		return Principal{}, errors.New("access token issued-at claim is missing or invalid")
	}
	expiresAt, err := claims.GetExpirationTime()
	if err != nil || expiresAt == nil || !expiresAt.After(issuedAt.Time) {
		return Principal{}, errors.New("access token expiration is missing or not later than issued-at")
	}
	issuer, err := claims.GetIssuer()
	if err != nil || issuer != v.issuer {
		return Principal{}, errors.New("access token issuer does not match configuration")
	}

	roles, err := parseRoles(claims, v.rolesClaim)
	if err != nil {
		return Principal{}, errors.New("access token role claim is invalid")
	}
	return Principal{Issuer: issuer, Subject: subject, Roles: roles}, nil
}

func parseRoles(claims jwt.MapClaims, claimName string) ([]string, error) {
	raw, exists := claims[claimName]
	if !exists {
		return nil, nil
	}
	var values []string
	switch typed := raw.(type) {
	case string:
		values = []string{typed}
	case []string:
		values = typed
	case []any:
		for _, value := range typed {
			role, ok := value.(string)
			if !ok {
				return nil, errors.New("role values must be strings")
			}
			values = append(values, role)
		}
	default:
		return nil, errors.New("role claim must be a string or string array")
	}

	allowed := map[string]struct{}{
		"viewer": {}, "operator": {}, "administrator": {},
	}
	seen := make(map[string]struct{}, len(values))
	roles := make([]string, 0, len(values))
	for _, role := range values {
		if _, ok := allowed[role]; !ok {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	return roles, nil
}

package auth

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"context"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type testOIDCProvider struct {
	server *httptest.Server
	mu     sync.RWMutex
	keys   []map[string]any
	issuer string
}

func newTestOIDCProvider(t *testing.T, keys ...map[string]any) *testOIDCProvider {
	t.Helper()
	provider := &testOIDCProvider{keys: keys}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": provider.issuer,
			"jwks_uri": provider.issuer + "/keys",
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		provider.mu.RLock()
		defer provider.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": provider.keys})
	})
	provider.server = httptest.NewServer(mux)
	provider.issuer = provider.server.URL
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *testOIDCProvider) setKeys(keys ...map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys = keys
}

func rsaJWK(keyID string, publicKey *rsa.PublicKey) map[string]any {
	return map[string]any{
		"kty": "RSA", "kid": keyID, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(publicKey.E)).Bytes()),
	}
}

func ecJWK(keyID string, publicKey *ecdsa.PublicKey) map[string]any {
	x := make([]byte, 32)
	y := make([]byte, 32)
	publicKey.X.FillBytes(x)
	publicKey.Y.FillBytes(y)
	return map[string]any{
		"kty": "EC", "kid": keyID, "use": "sig", "alg": "ES256", "crv": "P-256",
		"x": base64.RawURLEncoding.EncodeToString(x),
		"y": base64.RawURLEncoding.EncodeToString(y),
	}
}

func testClaims(issuer string) map[string]any {
	now := time.Now().Unix()
	return map[string]any{
		"iss": issuer,
		"sub": "user-123",
		"aud": []string{"another-api", "orbit-api"},
		"iat": now - 30,
		"nbf": now - 15,
		"exp": now + 300,
		"roles": []string{"viewer"},
	}
}

func signRS256Token(t *testing.T, key *rsa.PrivateKey, keyID string, claims map[string]any) string {
	return signRS256TokenWithAlgorithm(t, key, keyID, "RS256", claims)
}

func signRS256TokenWithAlgorithm(t *testing.T, key *rsa.PrivateKey, keyID, algorithm string, claims map[string]any) string {
	t.Helper()
	return signToken(t, keyID, algorithm, claims, func(input []byte) ([]byte, error) {
		digest := sha256.Sum256(input)
		return rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	})
}

func signES256Token(t *testing.T, key *ecdsa.PrivateKey, keyID string, claims map[string]any) string {
	t.Helper()
	return signToken(t, keyID, "ES256", claims, func(input []byte) ([]byte, error) {
		digest := sha256.Sum256(input)
		r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
		if err != nil {
			return nil, err
		}
		signature := make([]byte, 64)
		r.FillBytes(signature[:32])
		s.FillBytes(signature[32:])
		return signature, nil
	})
}

func signToken(t *testing.T, keyID, algorithm string, claims map[string]any, signer func([]byte) ([]byte, error)) string {
	return signTokenWithType(t, "at+jwt", keyID, algorithm, claims, signer)
}

func signTokenWithType(t *testing.T, tokenType, keyID, algorithm string, claims map[string]any, signer func([]byte) ([]byte, error)) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": algorithm, "kid": keyID, "typ": tokenType})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	input := encodedHeader + "." + encodedPayload
	signature, err := signer([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func newTestVerifier(t *testing.T, provider *testOIDCProvider) *Verifier {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	verifier, err := NewVerifier(ctx, Config{
		Issuer: provider.issuer, Audience: "orbit-api",
		AllowInsecureHTTP: true, CacheTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	return verifier
}


func TestIngestionVerifierRequiresDedicatedAccessTokenProfile(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := newTestOIDCProvider(t, rsaJWK("ingestion-key", &privateKey.PublicKey))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	verifier, err := NewIngestionVerifier(ctx, Config{
		Issuer: provider.issuer, Audience: "orbit-telemetry-ingestion",
		AllowInsecureHTTP: true, CacheTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewIngestionVerifier() error = %v", err)
	}

	claims := testClaims(provider.issuer)
	claims["aud"] = []string{"orbit-telemetry-ingestion"}
	valid := signRS256Token(t, privateKey, "ingestion-key", claims)
	if _, err := verifier.Verify(ctx, valid); err != nil {
		t.Fatalf("ingestion verifier rejected valid workload token: %v", err)
	}

	missingNotBefore := testClaims(provider.issuer)
	missingNotBefore["aud"] = []string{"orbit-telemetry-ingestion"}
	delete(missingNotBefore, "nbf")
	if _, err := verifier.Verify(ctx, signRS256Token(t, privateKey, "ingestion-key", missingNotBefore)); err == nil {
		t.Fatal("ingestion verifier accepted a token without nbf")
	}

	wrongAudience := testClaims(provider.issuer)
	if _, err := verifier.Verify(ctx, signRS256Token(t, privateKey, "ingestion-key", wrongAudience)); err == nil {
		t.Fatal("ingestion verifier accepted the human API audience")
	}

	wrongType := signTokenWithType(t, "JWT", "ingestion-key", "RS256", claims, func(input []byte) ([]byte, error) {
		digest := sha256.Sum256(input)
		return rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	})
	if _, err := verifier.Verify(ctx, wrongType); err == nil {
		t.Fatal("ingestion verifier accepted a generic JWT type")
	}
}

func TestVerifyRS256AccessTokenAndRoleHierarchy(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := newTestOIDCProvider(t, rsaJWK("rsa-1", &privateKey.PublicKey))
	verifier := newTestVerifier(t, provider)

	claims := testClaims(provider.issuer)
	claims["roles"] = []string{"operator", "unknown-role", "operator"}
	token := signRS256Token(t, privateKey, "rsa-1", claims)

	principal, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if principal.Issuer != provider.issuer || principal.Subject != "user-123" {
		t.Fatalf("unexpected principal identity: %+v", principal)
	}
	if len(principal.Roles) != 1 || principal.Roles[0] != "operator" {
		t.Fatalf("unexpected normalized roles: %#v", principal.Roles)
	}
	if !principal.HasRole("viewer") || !principal.HasRole("operator") || principal.HasRole("administrator") {
		t.Fatalf("role hierarchy did not enforce operator permissions: %#v", principal.Roles)
	}
}

func TestVerifyES256AccessToken(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	provider := newTestOIDCProvider(t, ecJWK("ec-1", &privateKey.PublicKey))
	verifier := newTestVerifier(t, provider)
	token := signES256Token(t, privateKey, "ec-1", testClaims(provider.issuer))

	principal, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !principal.HasRole("viewer") {
		t.Fatalf("expected viewer role, got %#v", principal.Roles)
	}
}

func TestVerifyRejectsInvalidClaimsAndSignatures(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := newTestOIDCProvider(t, rsaJWK("rsa-1", &privateKey.PublicKey))
	verifier := newTestVerifier(t, provider)

	tests := []struct {
		name   string
		mutate func(map[string]any)
		alg    string
	}{
		{name: "wrong issuer", mutate: func(c map[string]any) { c["iss"] = "https://attacker.example" }},
		{name: "wrong audience", mutate: func(c map[string]any) { c["aud"] = []string{"other-api"} }},
		{name: "expired", mutate: func(c map[string]any) { c["exp"] = time.Now().Add(-5 * time.Minute).Unix() }},
		{name: "future not-before", mutate: func(c map[string]any) { c["nbf"] = time.Now().Add(10 * time.Minute).Unix() }},
		{name: "future issued-at", mutate: func(c map[string]any) { c["iat"] = time.Now().Add(10 * time.Minute).Unix() }},
		{name: "missing issued-at", mutate: func(c map[string]any) { delete(c, "iat") }},
		{name: "missing expiration", mutate: func(c map[string]any) { delete(c, "exp") }},
		{name: "missing subject", mutate: func(c map[string]any) { delete(c, "sub") }},
		{name: "malformed roles", mutate: func(c map[string]any) { c["roles"] = map[string]any{"admin": true} }},
		{name: "unsupported algorithm", alg: "HS256"},
		{name: "none algorithm", alg: "none"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := testClaims(provider.issuer)
			if test.mutate != nil {
				test.mutate(claims)
			}
			algorithm := test.alg
			if algorithm == "" {
				algorithm = "RS256"
			}
			token := signRS256TokenWithAlgorithm(t, privateKey, "rsa-1", algorithm, claims)
			if _, err := verifier.Verify(context.Background(), token); err == nil {
				t.Fatal("Verify() accepted an invalid access token")
			}
		})
	}

	t.Run("tampered signature", func(t *testing.T) {
		token := signRS256Token(t, privateKey, "rsa-1", testClaims(provider.issuer))
		parts := strings.Split(token, ".")
		parts[2] = base64.RawURLEncoding.EncodeToString([]byte("invalid-signature"))
		if _, err := verifier.Verify(context.Background(), strings.Join(parts, ".")); err == nil {
			t.Fatal("Verify() accepted a modified JWT payload")
		}
	})

	t.Run("oversized token", func(t *testing.T) {
		if _, err := verifier.Verify(context.Background(), strings.Repeat("x", maxBearerTokenBytes+1)); err == nil {
			t.Fatal("Verify() accepted a token larger than 16 KiB")
		}
	})
}

func TestVerifierRefreshesJWKSForRotatedKeyID(t *testing.T) {
	firstKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := newTestOIDCProvider(t, rsaJWK("old-key", &firstKey.PublicKey))
	verifier := newTestVerifier(t, provider)

	provider.setKeys(rsaJWK("new-key", &secondKey.PublicKey))
	token := signRS256Token(t, secondKey, "new-key", testClaims(provider.issuer))
	if _, err := verifier.Verify(context.Background(), token); err != nil {
		t.Fatalf("Verify() did not refresh JWKS after key rotation: %v", err)
	}
}

func TestNewVerifierRejectsIssuerMismatchAndInsecureRemoteHTTP(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := newTestOIDCProvider(t, rsaJWK("rsa-1", &privateKey.PublicKey))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = NewVerifier(ctx, Config{
		Issuer: provider.issuer, Audience: "orbit-api", AllowInsecureHTTP: false,
	})
	if err == nil {
		t.Fatal("NewVerifier() accepted HTTP outside explicit development mode")
	}

	provider.mu.Lock()
	provider.issuer = provider.issuer + "/mismatch"
	provider.mu.Unlock()
	_, err = NewVerifier(ctx, Config{
		Issuer: strings.TrimSuffix(provider.issuer, "/mismatch"), Audience: "orbit-api",
		AllowInsecureHTTP: true,
	})
	if err == nil {
		t.Fatal("NewVerifier() accepted a discovery issuer mismatch")
	}
}


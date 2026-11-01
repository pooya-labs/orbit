package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

type testOIDCServer struct {
	server *httptest.Server
	mu     sync.RWMutex
	jwks   []byte
	issuer string
}

func newTestOIDCServer(t *testing.T, privateKey *rsa.PrivateKey, kid string) *testOIDCServer {
	t.Helper()
	state := &testOIDCServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": state.issuer,
			"jwks_uri": state.issuer + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		state.mu.RLock()
		body := append([]byte(nil), state.jwks...)
		state.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	state.server = httptest.NewServer(mux)
	state.issuer = state.server.URL
	state.setKey(t, privateKey, kid)
	t.Cleanup(state.server.Close)
	return state
}

func (s *testOIDCServer) setKey(t *testing.T, privateKey *rsa.PrivateKey, kid string) {
	t.Helper()
	exponent := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
	body, err := json.Marshal(map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"kid": kid,
			"alg": "RS256",
			"use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(exponent),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.jwks = body
	s.mu.Unlock()
}

func (s *testOIDCServer) verifier(t *testing.T) *Verifier {
	t.Helper()
	verifier, err := NewVerifier(t.Context(), Config{
		Issuer: s.issuer,
		Audience: "orbit-api",
		RolesClaim: "roles",
		Algorithms: []string{"RS256"},
		Development: true,
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	t.Cleanup(verifier.Close)
	return verifier
}

func signedAccessToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any, headerExtra map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "kid": kid, "typ": "at+jwt"}
	for name, value := range headerExtra {
		header[name] = value
	}
	protected, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	input := base64.RawURLEncoding.EncodeToString(protected) + "." +
		base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(input))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func validClaims(issuer string) map[string]any {
	now := time.Now()
	return map[string]any{
		"iss": issuer,
		"sub": "user-123",
		"aud": []string{"orbit-api"},
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"roles": []string{"viewer"},
	}
}

func TestVerifierAcceptsValidAccessTokenAndExtractsPrincipal(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := newTestOIDCServer(t, key, "rotation-1")
	verifier := issuer.verifier(t)
	raw := signedAccessToken(t, key, "rotation-1", validClaims(issuer.issuer), nil)

	principal, err := verifier.Verify(t.Context(), raw)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if principal.Issuer != issuer.issuer || principal.Subject != "user-123" ||
		!reflect.DeepEqual(principal.Roles, []string{RoleViewer}) {
		t.Fatalf("unexpected principal: %+v", principal)
	}
	if principal.ActorID() != base64.RawURLEncoding.EncodeToString([]byte(issuer.issuer))+".dXNlci0xMjM=" {
		t.Fatalf("unexpected actor ID: %q", principal.ActorID())
	}
}

func TestVerifierRejectsInvalidClaimsAndUntrustedHeaders(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := newTestOIDCServer(t, key, "rotation-1")
	verifier := issuer.verifier(t)

	tests := []struct {
		name string
		token func() string
	}{
		{name: "wrong signature", token: func() string {
			return signedAccessToken(t, otherKey, "rotation-1", validClaims(issuer.issuer), nil)
		}},
		{name: "wrong issuer", token: func() string {
			claims := validClaims(issuer.issuer)
			claims["iss"] = "https://attacker.example"
			return signedAccessToken(t, key, "rotation-1", claims, nil)
		}},
		{name: "wrong audience", token: func() string {
			claims := validClaims(issuer.issuer)
			claims["aud"] = []string{"another-api"}
			return signedAccessToken(t, key, "rotation-1", claims, nil)
		}},
		{name: "expired", token: func() string {
			claims := validClaims(issuer.issuer)
			claims["exp"] = time.Now().Add(-5 * time.Minute).Unix()
			return signedAccessToken(t, key, "rotation-1", claims, nil)
		}},
		{name: "missing expiration", token: func() string {
			claims := validClaims(issuer.issuer)
			delete(claims, "exp")
			return signedAccessToken(t, key, "rotation-1", claims, nil)
		}},
		{name: "missing issued at", token: func() string {
			claims := validClaims(issuer.issuer)
			delete(claims, "iat")
			return signedAccessToken(t, key, "rotation-1", claims, nil)
		}},
		{name: "missing subject", token: func() string {
			claims := validClaims(issuer.issuer)
			delete(claims, "sub")
			return signedAccessToken(t, key, "rotation-1", claims, nil)
		}},
		{name: "malformed roles", token: func() string {
			claims := validClaims(issuer.issuer)
			claims["roles"] = "administrator"
			return signedAccessToken(t, key, "rotation-1", claims, nil)
		}},
		{name: "token supplied jku", token: func() string {
			return signedAccessToken(t, key, "rotation-1", validClaims(issuer.issuer),
				map[string]any{"jku": "https://attacker.example/keys"})
		}},
		{name: "missing key ID", token: func() string {
			return signedAccessToken(t, key, "", validClaims(issuer.issuer), nil)
		}},
		{name: "unsupported algorithm", token: func() string {
			header := map[string]any{"alg": "HS256", "kid": "rotation-1", "typ": "at+jwt"}
			protected, _ := json.Marshal(header)
			payload, _ := json.Marshal(validClaims(issuer.issuer))
			input := base64.RawURLEncoding.EncodeToString(protected) + "." + base64.RawURLEncoding.EncodeToString(payload)
			return input + "." + base64.RawURLEncoding.EncodeToString([]byte("not-a-signature"))
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := verifier.Verify(t.Context(), test.token()); err == nil {
				t.Fatal("expected token to be rejected")
			}
		})
	}
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
	issuer := newTestOIDCServer(t, firstKey, "rotation-1")
	verifier := issuer.verifier(t)

	issuer.setKey(t, secondKey, "rotation-2")
	raw := signedAccessToken(t, secondKey, "rotation-2", validClaims(issuer.issuer), nil)
	principal, err := verifier.Verify(t.Context(), raw)
	if err != nil {
		t.Fatalf("Verify after key rotation: %v", err)
	}
	if principal.Subject != "user-123" {
		t.Fatalf("unexpected principal after key rotation: %+v", principal)
	}
}

func TestNewVerifierRejectsIssuerAndJWKSConfusion(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := newTestOIDCServer(t, key, "rotation-1")

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": "https://different.example",
			"jwks_uri": issuer.issuer + "/jwks",
		})
	})
	mismatch := httptest.NewServer(mux)
	defer mismatch.Close()

	_, err = NewVerifier(t.Context(), Config{
		Issuer: mismatch.URL, Audience: "orbit-api", RolesClaim: "roles",
		Algorithms: []string{"RS256"}, Development: true,
	})
	if err == nil {
		t.Fatal("discovery issuer mismatch must be rejected")
	}

	_, err = validateRemoteURL("http://keys.example/jwks", true)
	if err == nil {
		t.Fatal("non-loopback HTTP JWKS URL must be rejected")
	}
}

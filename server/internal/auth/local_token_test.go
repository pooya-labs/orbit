package auth

import (
	"context"
	"testing"
)

func TestStaticTokenVerifierRequiresStrongTokenAndPrincipal(t *testing.T) {
	principal := Principal{Issuer: "urn:orbit:local-work", Subject: "local-owner", Roles: []string{"administrator"}}
	for _, token := range []string{"", "short-token"} {
		if _, err := NewStaticTokenVerifier(token, principal); err == nil {
			t.Fatalf("accepted weak token %q", token)
		}
	}
	if _, err := NewStaticTokenVerifier("0123456789abcdef0123456789abcdef", Principal{Roles: []string{"administrator"}}); err == nil {
		t.Fatal("accepted principal without issuer and subject")
	}
	if _, err := NewStaticTokenVerifier("0123456789abcdef0123456789abcdef", Principal{Issuer: "local", Subject: "owner"}); err == nil {
		t.Fatal("accepted principal without roles")
	}
}

func TestStaticTokenVerifierChecksExactCredentialAndReturnsCopy(t *testing.T) {
	verifier, err := NewStaticTokenVerifier(
		"0123456789abcdef0123456789abcdef",
		Principal{Issuer: "urn:orbit:local-work", Subject: "local-owner", Roles: []string{"administrator"}},
	)
	if err != nil { t.Fatal(err) }

	if _, err := verifier.Verify(context.Background(), "0123456789abcdef0123456789abcdee"); err == nil {
		t.Fatal("accepted an incorrect credential")
	}
	got, err := verifier.Verify(context.Background(), "0123456789abcdef0123456789abcdef")
	if err != nil { t.Fatalf("Verify() error = %v", err) }
	if !got.HasRole("operator") || !got.HasRole("administrator") {
		t.Fatalf("unexpected local roles: %#v", got.Roles)
	}
	got.Roles[0] = "viewer"
	again, err := verifier.Verify(context.Background(), "0123456789abcdef0123456789abcdef")
	if err != nil { t.Fatal(err) }
	if !again.HasRole("administrator") {
		t.Fatal("caller mutation escaped into verifier state")
	}
}

func TestStaticTokenVerifierHonorsContextCancellation(t *testing.T) {
	verifier, err := NewStaticTokenVerifier(
		"0123456789abcdef0123456789abcdef",
		Principal{Issuer: "local", Subject: "owner", Roles: []string{"administrator"}},
	)
	if err != nil { t.Fatal(err) }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := verifier.Verify(ctx, "0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("Verify() accepted a canceled context")
	}
}

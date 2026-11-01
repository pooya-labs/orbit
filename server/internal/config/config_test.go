package config

import (
	"reflect"
	"testing"
)

func TestCommaSeparatedOriginsTrimsAndDeduplicates(t *testing.T) {
	got := commaSeparated(" https://one.example, ,http://localhost:3000,https://one.example ")
	want := []string{"https://one.example", "http://localhost:3000"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commaSeparated() = %#v, want %#v", got, want)
	}
}

func TestLoadReadsOIDCConfigurationDefaults(t *testing.T) {
	t.Setenv("ORBIT_AUTH_ISSUER", " https://issuer.example ")
	t.Setenv("ORBIT_AUTH_AUDIENCE", " orbit-api ")
	t.Setenv("ORBIT_AUTH_ROLES_CLAIM", "")
	t.Setenv("ORBIT_AUTH_ALGORITHMS", " ")

	got := Load()
	if got.AuthIssuer != "https://issuer.example" || got.AuthAudience != "orbit-api" {
		t.Fatalf("unexpected OIDC configuration: %+v", got)
	}
	if got.AuthRolesClaim != "roles" || !reflect.DeepEqual(got.AuthAlgorithms, []string{"RS256", "ES256"}) {
		t.Fatalf("unexpected OIDC defaults: %+v", got)
	}
}

func TestValidateRequiresAuthenticationOutsideDevelopment(t *testing.T) {
	for _, environment := range []string{"staging", "production", "test"} {
		cfg := Config{Environment: environment}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected %s without OIDC configuration to fail closed", environment)
		}
	}
}

func TestValidateAllowsOnlyLoopbackHTTPInDevelopment(t *testing.T) {
	cfg := Config{
		Environment: "development",
		AuthIssuer: "http://127.0.0.1:4567",
		AuthAudience: "orbit-api",
		AuthRolesClaim: "roles",
		AuthAlgorithms: []string{"RS256"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("loopback development issuer should be allowed: %v", err)
	}
	cfg.AuthIssuer = "http://issuer.example"
	if err := cfg.Validate(); err == nil {
		t.Fatal("non-loopback HTTP issuer must be rejected")
	}
}

func TestValidateRejectsUnsafeOIDCAlgorithmsAndPartialConfiguration(t *testing.T) {
	base := Config{
		Environment: "production",
		AuthIssuer: "https://issuer.example",
		AuthAudience: "orbit-api",
		AuthRolesClaim: "roles",
		AuthAlgorithms: []string{"RS256"},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid production OIDC configuration: %v", err)
	}
	base.AuthAlgorithms = []string{"HS256"}
	if err := base.Validate(); err == nil {
		t.Fatal("symmetric signing algorithms must be rejected")
	}
	base.AuthAlgorithms = []string{"RS256"}
	base.AuthAudience = ""
	if err := base.Validate(); err == nil {
		t.Fatal("partial OIDC configuration must be rejected")
	}
}

func TestCommaSeparatedOriginsEmpty(t *testing.T) {
	if got := commaSeparated(" , "); len(got) != 0 {
		t.Fatalf("commaSeparated() = %#v, want empty list", got)
	}
}


func TestLoadDefaultsVersionToCurrentPreview(t *testing.T) {
	t.Setenv("ORBIT_VERSION", " ")

	got := Load()
	if got.Version != "0.10.0" {
		t.Fatalf("default version = %q, want 0.10.0", got.Version)
	}
}

func TestLoadReadsOptionalTelemetryEndpoints(t *testing.T) {
	t.Setenv("ORBIT_VERSION", "0.9.0")
	t.Setenv("ORBIT_PROMETHEUS_URL", " http://prometheus:9090 ")
	t.Setenv("ORBIT_LOKI_URL", "http://loki:3100")
	t.Setenv("ORBIT_TEMPO_URL", "http://tempo:3200")

	got := Load()
	if got.Version != "0.9.0" {
		t.Fatalf("version = %q, want 0.9.0", got.Version)
	}
	if got.PrometheusURL != "http://prometheus:9090" || got.LokiURL != "http://loki:3100" || got.TempoURL != "http://tempo:3200" {
		t.Fatalf("unexpected telemetry URLs: %+v", got)
	}
}

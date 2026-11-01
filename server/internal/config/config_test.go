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


func TestProductionAndStagingRequireOIDC(t *testing.T) {
	for _, environment := range []string{"production", "staging"} {
		t.Run(environment, func(t *testing.T) {
			cfg := Config{Environment: environment, RateLimitPerMinute: 120, OIDCRolesClaim: "roles"}
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted a protected environment without OIDC settings")
			}
			cfg.OIDCIssuer = "https://identity.example"
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted an incomplete OIDC configuration")
			}
			cfg.OIDCAudience = "orbit-api"
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() rejected complete OIDC configuration: %v", err)
			}
		})
	}
}

func TestDevelopmentMayOmitOIDCButCannotPartiallyConfigureIt(t *testing.T) {
	cfg := Config{
		Environment: "development", RateLimitPerMinute: 120, OIDCRolesClaim: "roles",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("development without OIDC should remain available: %v", err)
	}
	cfg.OIDCIssuer = "https://identity.example"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted only one half of OIDC configuration")
	}
}

func TestValidateRejectsUnsafeRateLimitsAndRoleClaimNames(t *testing.T) {
	base := Config{Environment: "development", RateLimitPerMinute: 120, OIDCRolesClaim: "roles"}
	for _, test := range []struct {
		name string
		edit func(*Config)
	}{
		{name: "zero rate limit", edit: func(c *Config) { c.RateLimitPerMinute = 0 }},
		{name: "excessive rate limit", edit: func(c *Config) { c.RateLimitPerMinute = 10001 }},
		{name: "invalid role claim", edit: func(c *Config) { c.OIDCRolesClaim = "roles[]" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := base
			test.edit(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted invalid security configuration")
			}
		})
	}
}

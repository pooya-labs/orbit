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
			cfg := Config{Environment: environment, RateLimitPerMinute: 120, OIDCRolesClaim: "roles", DatabaseURL: "postgres://orbit:test@localhost:5432/orbit"}
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
		{name: "nested role path is unsupported", edit: func(c *Config) { c.OIDCRolesClaim = "realm_access.roles" }},
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


func TestValidateRequiresExplicitEnvironment(t *testing.T) {
	cfg := Config{RateLimitPerMinute: 120, OIDCRolesClaim: "roles"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted an implicit runtime environment")
	}
	cfg.Environment = "development"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() rejected explicit development mode: %v", err)
	}
}


func TestProductionAndStagingRequirePostgresForSharedRateLimits(t *testing.T) {
	for _, environment := range []string{"production", "staging"} {
		t.Run(environment, func(t *testing.T) {
			cfg := Config{
				Environment: environment,
				RateLimitPerMinute: 120,
				OIDCRolesClaim: "roles",
				OIDCIssuer: "https://identity.example/",
				OIDCAudience: "orbit-api",
			}
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted a protected environment without PostgreSQL")
			}
			cfg.DatabaseURL = "postgres://orbit:test@localhost:5432/orbit"
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() rejected protected environment with shared rate-limit storage: %v", err)
			}
		})
	}
}


func TestWorkloadOTLPConfigurationMustBeCompleteAndUseDistinctAudience(t *testing.T) {
	base := Config{
		Environment: "development", RateLimitPerMinute: 120, OIDCRolesClaim: "roles",
		OIDCIssuer: "https://identity.example", OIDCAudience: "orbit-api",
	}
	tests := []struct {
		name string
		edit func(*Config)
		wantError bool
	}{
		{name: "disabled by default", edit: func(*Config) {}, wantError: false},
		{name: "audience without collector", edit: func(c *Config) { c.OIDCWorkloadAudience = "orbit-telemetry" }, wantError: true},
		{name: "collector without audience", edit: func(c *Config) { c.OTLPCollectorHTTPURL = "http://otel-collector:4318" }, wantError: true},
		{name: "same audience as human API", edit: func(c *Config) { c.OIDCWorkloadAudience = "orbit-api"; c.OTLPCollectorHTTPURL = "http://otel-collector:4318" }, wantError: true},
		{name: "complete dedicated audience", edit: func(c *Config) { c.OIDCWorkloadAudience = "orbit-telemetry"; c.OTLPCollectorHTTPURL = "http://otel-collector:4318" }, wantError: false},
		{name: "requires human OIDC issuer", edit: func(c *Config) { c.OIDCIssuer = ""; c.OIDCAudience = ""; c.OIDCWorkloadAudience = "orbit-telemetry"; c.OTLPCollectorHTTPURL = "http://otel-collector:4318" }, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := base
			test.edit(&cfg)
			err := cfg.Validate()
			if (err != nil) != test.wantError {
				t.Fatalf("Validate() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestLoadReadsWorkloadOTLPConfiguration(t *testing.T) {
	t.Setenv("ORBIT_OIDC_ISSUER", "https://identity.example")
	t.Setenv("ORBIT_OIDC_AUDIENCE", "orbit-api")
	t.Setenv("ORBIT_OIDC_WORKLOAD_AUDIENCE", "orbit-telemetry")
	t.Setenv("ORBIT_OTLP_COLLECTOR_HTTP_URL", " http://otel-collector:4318 ")
	got := Load()
	if got.OIDCWorkloadAudience != "orbit-telemetry" || got.OTLPCollectorHTTPURL != "http://otel-collector:4318" {
		t.Fatalf("unexpected workload OTLP configuration: %+v", got)
	}
}

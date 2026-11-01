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
			cfg := Config{Environment: environment, RateLimitPerMinute: 120, OIDCRolesClaim: "roles", DatabaseURL: "postgres://orbit:test@localhost:5432/orbit", IngestionHTTPAddr: "127.0.0.1:4319", IngestionGRPCAddr: "127.0.0.1:4320"}
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted a protected environment without OIDC settings")
			}
			cfg.OIDCIssuer = "https://identity.example"
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted an incomplete OIDC configuration")
			}
			cfg.OIDCAudience = "orbit-api"
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted a protected environment without workload OIDC")
			}
			cfg.WorkloadOIDCIssuer = "https://identity.example/"
			cfg.WorkloadOIDCAudience = "orbit-ingestion"
			cfg.CollectorOTLPHTTPURL = "http://otel-collector:4318"
			cfg.CollectorOTLPGRPCEndpoint = "otel-collector:4317"
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() rejected complete human/workload OIDC configuration: %v", err)
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
				WorkloadOIDCIssuer: "https://identity.example/",
				WorkloadOIDCAudience: "orbit-ingestion",
				CollectorOTLPHTTPURL: "http://otel-collector:4318",
				CollectorOTLPGRPCEndpoint: "otel-collector:4317",
				IngestionHTTPAddr: "127.0.0.1:4319",
				IngestionGRPCAddr: "127.0.0.1:4320",
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

func TestLoadReadsWorkloadIngestionConfiguration(t *testing.T) {
	t.Setenv("ORBIT_WORKLOAD_OIDC_ISSUER", " https://workloads.example/ ")
	t.Setenv("ORBIT_WORKLOAD_OIDC_AUDIENCE", " orbit-ingestion ")
	t.Setenv("ORBIT_INGESTION_HTTP_ADDR", " :5319 ")
	t.Setenv("ORBIT_INGESTION_GRPC_ADDR", " :5320 ")
	t.Setenv("ORBIT_COLLECTOR_OTLP_HTTP_URL", " http://otel-collector:4318 ")
	t.Setenv("ORBIT_COLLECTOR_OTLP_GRPC_ENDPOINT", " otel-collector:4317 ")

	got := Load()
	if got.WorkloadOIDCIssuer != "https://workloads.example/" || got.WorkloadOIDCAudience != "orbit-ingestion" {
		t.Fatalf("unexpected workload OIDC config: %+v", got)
	}
	if got.IngestionHTTPAddr != ":5319" || got.IngestionGRPCAddr != ":5320" {
		t.Fatalf("unexpected ingestion listen addresses: %+v", got)
	}
	if got.CollectorOTLPHTTPURL != "http://otel-collector:4318" || got.CollectorOTLPGRPCEndpoint != "otel-collector:4317" {
		t.Fatalf("unexpected Collector endpoints: %+v", got)
	}
}

func TestWorkloadIngestionRequiresDistinctAudiencesDatabaseAndCollectorEndpoints(t *testing.T) {
	base := Config{
		Environment: "development",
		RateLimitPerMinute: 120,
		OIDCRolesClaim: "roles",
		OIDCIssuer: "https://identity.example/",
		OIDCAudience: "orbit-api",
		WorkloadOIDCIssuer: "https://identity.example/",
		WorkloadOIDCAudience: "orbit-ingestion",
		DatabaseURL: "postgres://orbit:test@localhost:5432/orbit",
		CollectorOTLPHTTPURL: "http://otel-collector:4318",
		CollectorOTLPGRPCEndpoint: "otel-collector:4317",
		IngestionHTTPAddr: "127.0.0.1:4319",
		IngestionGRPCAddr: "127.0.0.1:4320",
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid workload ingestion config rejected: %v", err)
	}
	tests := []struct {
		name string
		edit func(*Config)
	}{
		{name: "same audience", edit: func(c *Config) { c.WorkloadOIDCAudience = c.OIDCAudience }},
		{name: "missing database", edit: func(c *Config) { c.DatabaseURL = "" }},
		{name: "missing collector HTTP endpoint", edit: func(c *Config) { c.CollectorOTLPHTTPURL = "" }},
		{name: "invalid collector HTTP endpoint", edit: func(c *Config) { c.CollectorOTLPHTTPURL = "http://user:pass@collector:4318" }},
		{name: "missing collector gRPC endpoint", edit: func(c *Config) { c.CollectorOTLPGRPCEndpoint = "" }},
		{name: "URL used for gRPC endpoint", edit: func(c *Config) { c.CollectorOTLPGRPCEndpoint = "http://collector:4317" }},
		{name: "workload without human audience", edit: func(c *Config) { c.OIDCIssuer = ""; c.OIDCAudience = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := base
			test.edit(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted an unsafe workload ingestion configuration")
			}
		})
	}
}


func TestLoadDefaultsIngestionListenersToLoopback(t *testing.T) {
	t.Setenv("ORBIT_INGESTION_HTTP_ADDR", "")
	t.Setenv("ORBIT_INGESTION_GRPC_ADDR", "")
	got := Load()
	if got.IngestionHTTPAddr != "127.0.0.1:4319" || got.IngestionGRPCAddr != "127.0.0.1:4320" {
		t.Fatalf("ingestion listener defaults are not loopback-only: %+v", got)
	}
}


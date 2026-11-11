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


func TestLoadDefaultsVersionToStableRelease(t *testing.T) {
	t.Setenv("ORBIT_VERSION", " ")

	got := Load()
	if got.Version != "1.0.0" {
		t.Fatalf("default version = %q, want 1.0.0", got.Version)
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


func TestLoadReadsAlertWebhookSecretReferencesWithoutExposingThemAsConfigValues(t *testing.T) {
	t.Setenv("ORBIT_ALERT_WEBHOOK_ALLOWED_HOSTS", " Hooks.Example.com,events.example.com ")
	t.Setenv("ORBIT_ALERT_WEBHOOK_SECRET_PAGERDUTY_PRIMARY", " https://hooks.example.com/services/team/token ")
	t.Setenv("ORBIT_ALERT_WEBHOOK_SECRET_EMPTY", " ")

	got := Load()
	if !reflect.DeepEqual(got.AlertWebhookAllowedHosts, []string{"hooks.example.com", "events.example.com"}) {
		t.Fatalf("allowed hosts = %#v", got.AlertWebhookAllowedHosts)
	}
	if got.AlertWebhookSecrets["PAGERDUTY_PRIMARY"] != "https://hooks.example.com/services/team/token" {
		t.Fatalf("webhook secret reference was not loaded")
	}
	if _, exists := got.AlertWebhookSecrets["EMPTY"]; exists {
		t.Fatal("empty webhook secret must not be loaded")
	}
}

func TestValidateWebhookSecretsRequiresHTTPSAndExactHostnameAllowlist(t *testing.T) {
	base := Config{
		Environment: "development",
		RateLimitPerMinute: 120,
		OIDCRolesClaim: "roles",
		AlertWebhookSecrets: map[string]string{
			"PAGERDUTY_PRIMARY": "https://hooks.example.com/services/team/token",
		},
		AlertWebhookAllowedHosts: []string{"hooks.example.com"},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid webhook configuration rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*Config)
	}{
		{name: "missing host allowlist", edit: func(c *Config) { c.AlertWebhookAllowedHosts = nil }},
		{name: "plaintext HTTP", edit: func(c *Config) { c.AlertWebhookSecrets["PAGERDUTY_PRIMARY"] = "http://hooks.example.com/webhook" }},
		{name: "userinfo", edit: func(c *Config) { c.AlertWebhookSecrets["PAGERDUTY_PRIMARY"] = "https://user:pass@hooks.example.com/webhook" }},
		{name: "query string", edit: func(c *Config) { c.AlertWebhookSecrets["PAGERDUTY_PRIMARY"] = "https://hooks.example.com/webhook?token=secret" }},
		{name: "unlisted host", edit: func(c *Config) { c.AlertWebhookSecrets["PAGERDUTY_PRIMARY"] = "https://other.example.com/webhook" }},
		{name: "IP literal", edit: func(c *Config) { c.AlertWebhookSecrets["PAGERDUTY_PRIMARY"] = "https://127.0.0.1/webhook"; c.AlertWebhookAllowedHosts = []string{"127.0.0.1"} }},
		{name: "non-443 port", edit: func(c *Config) { c.AlertWebhookSecrets["PAGERDUTY_PRIMARY"] = "https://hooks.example.com:8443/webhook" }},
		{name: "invalid secret reference", edit: func(c *Config) { c.AlertWebhookSecrets = map[string]string{"../BAD": "https://hooks.example.com/webhook"} }},
		{name: "invalid allowlist host", edit: func(c *Config) { c.AlertWebhookAllowedHosts = []string{"*.example.com"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Config{
				Environment: base.Environment,
				RateLimitPerMinute: base.RateLimitPerMinute,
				OIDCRolesClaim: base.OIDCRolesClaim,
				AlertWebhookSecrets: map[string]string{
					"PAGERDUTY_PRIMARY": base.AlertWebhookSecrets["PAGERDUTY_PRIMARY"],
				},
				AlertWebhookAllowedHosts: append([]string(nil), base.AlertWebhookAllowedHosts...),
			}
			test.edit(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() accepted unsafe webhook configuration")
			}
		})
	}
}

func TestLoadReadsOptionalAPISelfTelemetryEndpoint(t *testing.T) {
	t.Setenv("ORBIT_OTEL_EXPORTER_OTLP_ENDPOINT", " http://otel-collector:4318 ")
	got := Load()
	if got.SelfTelemetryOTLPHTTPURL != "http://otel-collector:4318" {
		t.Fatalf("self telemetry endpoint = %q", got.SelfTelemetryOTLPHTTPURL)
	}
}

func TestSelfTelemetryEndpointMustBeAnHTTPBaseURL(t *testing.T) {
	base := Config{Environment: "development", RateLimitPerMinute: 120, OIDCRolesClaim: "roles"}
	for _, endpoint := range []string{
		"file:///tmp/otlp",
		"http://user:secret@collector:4318",
		"http://collector:4318/v1/traces",
		"http://collector:4318?token=secret",
	} {
		cfg := base
		cfg.SelfTelemetryOTLPHTTPURL = endpoint
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate() accepted unsafe self telemetry endpoint %q", endpoint)
		}
	}
	for _, endpoint := range []string{"", "http://otel-collector:4318", "https://collector.example"} {
		cfg := base
		cfg.SelfTelemetryOTLPHTTPURL = endpoint
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() rejected optional endpoint %q: %v", endpoint, err)
		}
	}
}


func localWorkTestConfig() Config {
	return Config{
		Environment: "development",
		RateLimitPerMinute: 120,
		OIDCRolesClaim: "roles",
		DatabaseURL: "postgres://orbit:local@localhost:5432/orbit?sslmode=disable",
		PrometheusURL: "http://prometheus:9090",
		LokiURL: "http://loki:3100",
		TempoURL: "http://tempo:3200",
		CollectorOTLPHTTPURL: "http://otel-collector:4318",
		CollectorOTLPGRPCEndpoint: "otel-collector:4317",
		IngestionHTTPAddr: ":4319",
		IngestionGRPCAddr: ":4320",
		AllowedOrigins: []string{"http://localhost:3000", "http://127.0.0.1:3000"},
		LocalWorkEnabled: true,
		LocalHumanToken: "human-local-token-0123456789abcdef",
		LocalWorkloadToken: "workload-local-token-0123456789abcd",
	}
}

func TestLocalWorkRequiresCompleteLoopbackDevelopmentConfiguration(t *testing.T) {
	cfg := localWorkTestConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid Local Work configuration rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*Config)
	}{
		{name: "protected environment", edit: func(c *Config) { c.Environment = "production" }},
		{name: "missing database", edit: func(c *Config) { c.DatabaseURL = "" }},
		{name: "missing Prometheus", edit: func(c *Config) { c.PrometheusURL = "" }},
		{name: "missing Loki", edit: func(c *Config) { c.LokiURL = "" }},
		{name: "missing Tempo", edit: func(c *Config) { c.TempoURL = "" }},
		{name: "short human token", edit: func(c *Config) { c.LocalHumanToken = "short" }},
		{name: "short workload token", edit: func(c *Config) { c.LocalWorkloadToken = "short" }},
		{name: "reused token", edit: func(c *Config) { c.LocalWorkloadToken = c.LocalHumanToken }},
		{name: "OIDC overlap", edit: func(c *Config) { c.OIDCIssuer = "https://identity.example"; c.OIDCAudience = "orbit-api" }},
		{name: "remote CORS origin", edit: func(c *Config) { c.AllowedOrigins = []string{"https://pooya-labs.github.io"} }},
		{name: "CORS path", edit: func(c *Config) { c.AllowedOrigins = []string{"http://localhost:3000/orbit"} }},
		{name: "missing Collector HTTP", edit: func(c *Config) { c.CollectorOTLPHTTPURL = "" }},
		{name: "missing Collector gRPC", edit: func(c *Config) { c.CollectorOTLPGRPCEndpoint = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := localWorkTestConfig()
			test.edit(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("Validate() accepted incomplete or unsafe Local Work configuration")
			}
		})
	}
}

func TestLocalWorkTokensCannotBeConfiguredWhileModeIsDisabled(t *testing.T) {
	cfg := localWorkTestConfig()
	cfg.LocalWorkEnabled = false
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted Local Work credentials while the mode is disabled")
	}
}

func TestLocalWorkRejectsLoopbackOriginWithCredentials(t *testing.T) {
	cfg := localWorkTestConfig()
	cfg.AllowedOrigins = []string{"http://user:password@localhost:3000"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted credentials embedded in a local CORS origin")
	}
}

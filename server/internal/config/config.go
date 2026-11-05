package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr                    string
	Environment                 string
	Version                     string
	DatabaseURL                 string
	AllowedOrigins              []string
	PrometheusURL               string
	LokiURL                     string
	TempoURL                    string
	OIDCIssuer                  string
	OIDCAudience                string
	OIDCRolesClaim              string
	WorkloadOIDCIssuer          string
	WorkloadOIDCAudience        string
	IngestionHTTPAddr           string
	IngestionGRPCAddr           string
	CollectorOTLPHTTPURL        string
	CollectorOTLPGRPCEndpoint   string
	SelfTelemetryOTLPHTTPURL    string
	RateLimitPerMinute          int
	AlertWebhookSecrets         map[string]string
	AlertWebhookAllowedHosts    []string
}

func Load() Config {
	return Config{
		HTTPAddr:                  valueOrDefault("ORBIT_HTTP_ADDR", ":8080"),
		Environment:               strings.TrimSpace(os.Getenv("ORBIT_ENVIRONMENT")),
		Version:                   valueOrDefault("ORBIT_VERSION", "1.0.0"),
		DatabaseURL:               strings.TrimSpace(os.Getenv("ORBIT_DATABASE_URL")),
		AllowedOrigins:            commaSeparated(os.Getenv("ORBIT_ALLOWED_ORIGINS")),
		PrometheusURL:             strings.TrimSpace(os.Getenv("ORBIT_PROMETHEUS_URL")),
		LokiURL:                   strings.TrimSpace(os.Getenv("ORBIT_LOKI_URL")),
		TempoURL:                  strings.TrimSpace(os.Getenv("ORBIT_TEMPO_URL")),
		OIDCIssuer:                strings.TrimSpace(os.Getenv("ORBIT_OIDC_ISSUER")),
		OIDCAudience:              strings.TrimSpace(os.Getenv("ORBIT_OIDC_AUDIENCE")),
		OIDCRolesClaim:            valueOrDefault("ORBIT_OIDC_ROLES_CLAIM", "roles"),
		WorkloadOIDCIssuer:        strings.TrimSpace(os.Getenv("ORBIT_WORKLOAD_OIDC_ISSUER")),
		WorkloadOIDCAudience:      strings.TrimSpace(os.Getenv("ORBIT_WORKLOAD_OIDC_AUDIENCE")),
		IngestionHTTPAddr:         valueOrDefault("ORBIT_INGESTION_HTTP_ADDR", "127.0.0.1:4319"),
		IngestionGRPCAddr:         valueOrDefault("ORBIT_INGESTION_GRPC_ADDR", "127.0.0.1:4320"),
		CollectorOTLPHTTPURL:      strings.TrimSpace(os.Getenv("ORBIT_COLLECTOR_OTLP_HTTP_URL")),
		CollectorOTLPGRPCEndpoint: strings.TrimSpace(os.Getenv("ORBIT_COLLECTOR_OTLP_GRPC_ENDPOINT")),
		SelfTelemetryOTLPHTTPURL: strings.TrimSpace(os.Getenv("ORBIT_OTEL_EXPORTER_OTLP_ENDPOINT")),
		RateLimitPerMinute:        integerOrDefault("ORBIT_RATE_LIMIT_PER_MINUTE", 120),
		AlertWebhookSecrets:       alertWebhookSecretsFromEnv(),
		AlertWebhookAllowedHosts:  lowerCaseList(os.Getenv("ORBIT_ALERT_WEBHOOK_ALLOWED_HOSTS")),
	}
}

func (c Config) Validate() error {
	if c.SelfTelemetryOTLPHTTPURL != "" {
		if err := validateCollectorHTTPURL(c.SelfTelemetryOTLPHTTPURL); err != nil {
			return fmt.Errorf("ORBIT_OTEL_EXPORTER_OTLP_ENDPOINT: %w", err)
		}
	}
	if c.RateLimitPerMinute < 1 || c.RateLimitPerMinute > 10000 {
		return fmt.Errorf("ORBIT_RATE_LIMIT_PER_MINUTE must be between 1 and 10000")
	}
	if len(c.AlertWebhookSecrets) > 0 && len(c.AlertWebhookAllowedHosts) == 0 {
		return fmt.Errorf("ORBIT_ALERT_WEBHOOK_ALLOWED_HOSTS is required when webhook secrets are configured")
	}
	allowedHosts := make(map[string]struct{}, len(c.AlertWebhookAllowedHosts))
	for _, host := range c.AlertWebhookAllowedHosts {
		host = strings.ToLower(strings.TrimSpace(host))
		if !validWebhookHostname(host) {
			return fmt.Errorf("ORBIT_ALERT_WEBHOOK_ALLOWED_HOSTS contains an invalid hostname")
		}
		allowedHosts[host] = struct{}{}
	}
	for ref, endpoint := range c.AlertWebhookSecrets {
		if !validWebhookSecretRef(ref) {
			return fmt.Errorf("an ORBIT_ALERT_WEBHOOK_SECRET_ variable has an invalid reference name")
		}
		parsed, err := url.Parse(strings.TrimSpace(endpoint))
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
			parsed.RawQuery != "" || parsed.Fragment != "" || net.ParseIP(parsed.Hostname()) != nil ||
			(parsed.Port() != "" && parsed.Port() != "443") {
			return fmt.Errorf("webhook secret %s must contain an HTTPS URL on port 443 without userinfo, query, or fragment", ref)
		}
		if _, ok := allowedHosts[strings.ToLower(parsed.Hostname())]; !ok {
			return fmt.Errorf("webhook secret %s points to a hostname outside ORBIT_ALERT_WEBHOOK_ALLOWED_HOSTS", ref)
		}
	}
	if (c.OIDCIssuer == "") != (c.OIDCAudience == "") {
		return fmt.Errorf("ORBIT_OIDC_ISSUER and ORBIT_OIDC_AUDIENCE must be configured together")
	}
	if (c.WorkloadOIDCIssuer == "") != (c.WorkloadOIDCAudience == "") {
		return fmt.Errorf("ORBIT_WORKLOAD_OIDC_ISSUER and ORBIT_WORKLOAD_OIDC_AUDIENCE must be configured together")
	}
	if c.WorkloadOIDCIssuer != "" {
		if c.OIDCIssuer == "" || c.OIDCAudience == "" {
			return fmt.Errorf("workload ingestion requires the human OIDC audience to enforce audience separation")
		}
		if c.WorkloadOIDCAudience == c.OIDCAudience {
			return fmt.Errorf("workload OIDC audience must be distinct from the human API audience")
		}
		if c.DatabaseURL == "" {
			return fmt.Errorf("ORBIT_DATABASE_URL is required when workload ingestion is enabled")
		}
		if err := validateCollectorHTTPURL(c.CollectorOTLPHTTPURL); err != nil {
			return fmt.Errorf("ORBIT_COLLECTOR_OTLP_HTTP_URL: %w", err)
		}
		if err := validateCollectorGRPCEndpoint(c.CollectorOTLPGRPCEndpoint); err != nil {
			return fmt.Errorf("ORBIT_COLLECTOR_OTLP_GRPC_ENDPOINT: %w", err)
		}
		if strings.TrimSpace(c.IngestionHTTPAddr) == "" || strings.TrimSpace(c.IngestionGRPCAddr) == "" {
			return fmt.Errorf("OTLP ingestion HTTP and gRPC listen addresses are required")
		}
	}
	switch c.Environment {
	case "development":
	case "staging", "production":
		if c.OIDCIssuer == "" || c.OIDCAudience == "" {
			return fmt.Errorf("OIDC issuer and audience are required in %s", c.Environment)
		}
		if c.WorkloadOIDCIssuer == "" || c.WorkloadOIDCAudience == "" {
			return fmt.Errorf("workload OIDC issuer and audience are required in %s", c.Environment)
		}
		if c.DatabaseURL == "" {
			return fmt.Errorf("ORBIT_DATABASE_URL is required in %s for shared API and workload rate limiting", c.Environment)
		}
	default:
		return fmt.Errorf("ORBIT_ENVIRONMENT must be explicitly set to development, staging, or production")
	}
	if c.OIDCRolesClaim == "" || len(c.OIDCRolesClaim) > 64 {
		return fmt.Errorf("ORBIT_OIDC_ROLES_CLAIM must contain between 1 and 64 characters")
	}
	for _, char := range c.OIDCRolesClaim {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') &&
			!(char >= '0' && char <= '9') && char != '_' && char != '-' {
			return fmt.Errorf("ORBIT_OIDC_ROLES_CLAIM contains an unsupported character")
		}
	}
	return nil
}

func alertWebhookSecretsFromEnv() map[string]string {
	const prefix = "ORBIT_ALERT_WEBHOOK_SECRET_"
	secrets := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(key, prefix) {
			continue
		}
		ref := strings.TrimPrefix(key, prefix)
		if ref == "" || strings.TrimSpace(value) == "" {
			continue
		}
		secrets[ref] = strings.TrimSpace(value)
	}
	return secrets
}

func validWebhookSecretRef(value string) bool {
	if len(value) < 1 || len(value) > 64 || value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for _, char := range value {
		if !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '_' {
			return false
		}
	}
	return true
}

func validWebhookHostname(value string) bool {
	if len(value) == 0 || len(value) > 253 || strings.HasSuffix(value, ".") || net.ParseIP(value) != nil {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func lowerCaseList(value string) []string {
	items := commaSeparated(value)
	for index := range items {
		items[index] = strings.ToLower(items[index])
	}
	return items
}

func validateCollectorHTTPURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("must point to the Collector OTLP HTTP base, not a signal-specific path")
	}
	return nil
}

func validateCollectorGRPCEndpoint(value string) error {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil || host == "" {
		return fmt.Errorf("must be a host:port endpoint without a URL scheme")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("must contain a valid TCP port")
	}
	return nil
}

func integerOrDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func valueOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func commaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		result = append(result, origin)
	}
	return result
}

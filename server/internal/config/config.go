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
	RateLimitPerMinute          int
}

func Load() Config {
	return Config{
		HTTPAddr:                  valueOrDefault("ORBIT_HTTP_ADDR", ":8080"),
		Environment:               strings.TrimSpace(os.Getenv("ORBIT_ENVIRONMENT")),
		Version:                   valueOrDefault("ORBIT_VERSION", "0.10.0"),
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
		IngestionHTTPAddr:         valueOrDefault("ORBIT_INGESTION_HTTP_ADDR", ":4319"),
		IngestionGRPCAddr:         valueOrDefault("ORBIT_INGESTION_GRPC_ADDR", ":4320"),
		CollectorOTLPHTTPURL:      strings.TrimSpace(os.Getenv("ORBIT_COLLECTOR_OTLP_HTTP_URL")),
		CollectorOTLPGRPCEndpoint: strings.TrimSpace(os.Getenv("ORBIT_COLLECTOR_OTLP_GRPC_ENDPOINT")),
		RateLimitPerMinute:        integerOrDefault("ORBIT_RATE_LIMIT_PER_MINUTE", 120),
	}
}

func (c Config) Validate() error {
	if c.RateLimitPerMinute < 1 || c.RateLimitPerMinute > 10000 {
		return fmt.Errorf("ORBIT_RATE_LIMIT_PER_MINUTE must be between 1 and 10000")
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

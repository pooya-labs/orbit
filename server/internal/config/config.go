package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
)

type Config struct {
	HTTPAddr       string
	Environment    string
	Version        string
	DatabaseURL    string
	AllowedOrigins []string
	PrometheusURL string
	LokiURL string
	TempoURL string
}

func Load() Config {
	return Config{
		HTTPAddr:       valueOrDefault("ORBIT_HTTP_ADDR", ":8080"),
		Environment:    valueOrDefault("ORBIT_ENVIRONMENT", "development"),
		Version:        valueOrDefault("ORBIT_VERSION", "0.10.0"),
		DatabaseURL:    strings.TrimSpace(os.Getenv("ORBIT_DATABASE_URL")),
		AllowedOrigins: commaSeparated(os.Getenv("ORBIT_ALLOWED_ORIGINS")),
		PrometheusURL:  strings.TrimSpace(os.Getenv("ORBIT_PROMETHEUS_URL")),
		LokiURL:        strings.TrimSpace(os.Getenv("ORBIT_LOKI_URL")),
		TempoURL:       strings.TrimSpace(os.Getenv("ORBIT_TEMPO_URL")),
		AuthIssuer:     strings.TrimSpace(os.Getenv("ORBIT_AUTH_ISSUER")),
		AuthAudience:   strings.TrimSpace(os.Getenv("ORBIT_AUTH_AUDIENCE")),
		AuthRolesClaim: valueOrDefault("ORBIT_AUTH_ROLES_CLAIM", "roles"),
		AuthAlgorithms: commaSeparated(valueOrDefault("ORBIT_AUTH_ALGORITHMS", "RS256,ES256")),
	}
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

// AuthenticationEnabled reports whether the API has a complete OIDC resource-server configuration.
func (c Config) AuthenticationEnabled() bool {
	return strings.TrimSpace(c.AuthIssuer) != "" && strings.TrimSpace(c.AuthAudience) != ""
}

// Validate enforces the deployment security boundary before the API starts.
func (c Config) Validate() error {
	environment := strings.ToLower(strings.TrimSpace(c.Environment))
	if environment == "" {
		environment = "development"
	}
	issuer := strings.TrimSpace(c.AuthIssuer)
	audience := strings.TrimSpace(c.AuthAudience)
	if issuer == "" && audience == "" {
		if environment != "development" {
			return fmt.Errorf("OIDC issuer and audience are required outside explicit development mode")
		}
		return nil
	}
	if issuer == "" || audience == "" {
		return fmt.Errorf("ORBIT_AUTH_ISSUER and ORBIT_AUTH_AUDIENCE must be configured together")
	}
	if strings.ContainsAny(audience, "\r\n") {
		return fmt.Errorf("ORBIT_AUTH_AUDIENCE is invalid")
	}

	issuerURL, err := url.Parse(issuer)
	if err != nil || issuerURL.Hostname() == "" || issuerURL.User != nil ||
		issuerURL.RawQuery != "" || issuerURL.Fragment != "" {
		return fmt.Errorf("ORBIT_AUTH_ISSUER must be an absolute URL without credentials, query, or fragment")
	}
	isLoopback := isLoopbackHost(issuerURL.Hostname())
	if issuerURL.Scheme != "https" && !(environment == "development" && issuerURL.Scheme == "http" && isLoopback) {
		return fmt.Errorf("ORBIT_AUTH_ISSUER must use HTTPS outside loopback development")
	}
	if environment != "development" && issuerURL.Scheme != "https" {
		return fmt.Errorf("ORBIT_AUTH_ISSUER must use HTTPS outside development")
	}

	claim := strings.TrimSpace(c.AuthRolesClaim)
	if claim == "" || len(claim) > 64 || !authClaimPattern.MatchString(claim) {
		return fmt.Errorf("ORBIT_AUTH_ROLES_CLAIM must be a simple claim name")
	}
	if len(c.AuthAlgorithms) == 0 {
		return fmt.Errorf("ORBIT_AUTH_ALGORITHMS must contain RS256 and/or ES256")
	}
	seen := make(map[string]struct{}, len(c.AuthAlgorithms))
	for _, algorithm := range c.AuthAlgorithms {
		if algorithm != "RS256" && algorithm != "ES256" {
			return fmt.Errorf("unsupported OIDC signing algorithm %q; only RS256 and ES256 are allowed", algorithm)
		}
		if _, exists := seen[algorithm]; exists {
			return fmt.Errorf("duplicate OIDC signing algorithm %q", algorithm)
		}
		seen[algorithm] = struct{}{}
	}
	return nil
}

var authClaimPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}


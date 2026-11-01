package config

import (
	"os"
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
		Version:        valueOrDefault("ORBIT_VERSION", "0.9.0"),
		DatabaseURL:    strings.TrimSpace(os.Getenv("ORBIT_DATABASE_URL")),
		AllowedOrigins: commaSeparated(os.Getenv("ORBIT_ALLOWED_ORIGINS")),
		PrometheusURL:  strings.TrimSpace(os.Getenv("ORBIT_PROMETHEUS_URL")),
		LokiURL:        strings.TrimSpace(os.Getenv("ORBIT_LOKI_URL")),
		TempoURL:       strings.TrimSpace(os.Getenv("ORBIT_TEMPO_URL")),
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

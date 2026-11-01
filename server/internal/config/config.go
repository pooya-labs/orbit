package config

import (
	"os"
	"strings"
)

type Config struct {
	HTTPAddr    string
	Environment string
	Version     string
}

func Load() Config {
	return Config{
		HTTPAddr:    valueOrDefault("ORBIT_HTTP_ADDR", ":8080"),
		Environment: valueOrDefault("ORBIT_ENVIRONMENT", "development"),
		Version:     valueOrDefault("ORBIT_VERSION", "0.1.0"),
	}
}

func valueOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

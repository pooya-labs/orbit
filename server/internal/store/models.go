package store

import (
	"context"
	"time"
)

type Service struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Environment string     `json:"environment"`
	Region      string     `json:"region"`
	Status      string     `json:"status"`
	LatencyP95  *float64   `json:"latency_p95_ms"`
	ErrorRate   *float64   `json:"error_rate"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
	Source      string     `json:"source"`
}

type Incident struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	ServiceID   string     `json:"service_id"`
	ServiceName string     `json:"service_name"`
	Severity    string     `json:"severity"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ResolvedAt  *time.Time `json:"resolved_at"`
	Source      string     `json:"source"`
}

type TelemetryService struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Sources             []string       `json:"sources"`
	FirstSeenAt         time.Time      `json:"first_seen_at"`
	LastSeenAt          time.Time      `json:"last_seen_at"`
	LastDiscoveredAt    time.Time      `json:"last_discovered_at"`
	ResourceAttributes  map[string]any `json:"resource_attributes"`
}

type Repository interface {
	Ping(ctx context.Context) error
	UpsertTelemetryServices(ctx context.Context, services []TelemetryService) ([]TelemetryService, error)
	ListTelemetryServices(ctx context.Context, limit int) ([]TelemetryService, error)
	ListServices(ctx context.Context) ([]Service, error)
	ListIncidents(ctx context.Context, status string) ([]Incident, error)
	Close()
}

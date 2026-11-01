package postgres

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Repository struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Repository, error) {
	if databaseURL == "" {
		return nil, errors.New("database URL is required")
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	poolConfig.MaxConns = 12
	poolConfig.MinConns = 1
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}

	repository := &Repository{pool: pool}
	if err := repository.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	if err := repository.Migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate PostgreSQL schema: %w", err)
	}
	return repository, nil
}

func (r *Repository) Close() {
	r.pool.Close()
}

func (r *Repository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

func (r *Repository) Migrate(ctx context.Context) error {
	if _, err := r.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	migrations := []struct {
		version string
		file    string
	}{
		{version: "001_initial", file: "migrations/001_initial.sql"},
		{version: "002_telemetry_service_catalog", file: "migrations/002_telemetry_service_catalog.sql"},
	}

	for _, item := range migrations {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin %s migration transaction: %w", item.version, err)
		}
		var applied bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", item.version).Scan(&applied); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("check %s migration version: %w", item.version, err)
		}
		if applied {
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("finish %s migration check: %w", item.version, err)
			}
			continue
		}
		migration, err := migrationFiles.ReadFile(item.file)
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("read %s migration: %w", item.version, err)
		}
		if _, err := tx.Exec(ctx, string(migration)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("execute %s migration: %w", item.version, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", item.version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s migration: %w", item.version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s migration: %w", item.version, err)
		}
	}
	return nil
}


var telemetryServiceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

var allowedTelemetrySources = map[string]struct{}{
	"prometheus": {},
	"loki":       {},
	"tempo":      {},
}

func (r *Repository) UpsertTelemetryServices(ctx context.Context, services []store.TelemetryService) ([]store.TelemetryService, error) {
	if len(services) == 0 {
		return []store.TelemetryService{}, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin telemetry catalog upsert: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	persisted := make([]store.TelemetryService, 0, len(services))
	for _, service := range services {
		if !telemetryServiceNamePattern.MatchString(service.Name) {
			return nil, fmt.Errorf("invalid telemetry service name")
		}
		sources := make([]string, 0, len(service.Sources))
		seenSources := make(map[string]struct{}, len(service.Sources))
		for _, source := range service.Sources {
			source = strings.TrimSpace(source)
			if _, ok := allowedTelemetrySources[source]; !ok {
				return nil, fmt.Errorf("unsupported telemetry source")
			}
			if _, exists := seenSources[source]; exists {
				continue
			}
			seenSources[source] = struct{}{}
			sources = append(sources, source)
		}
		if len(sources) == 0 {
			return nil, fmt.Errorf("telemetry service must include at least one source")
		}
		sort.Strings(sources)

		attributes := service.ResourceAttributes
		if attributes == nil {
			attributes = map[string]any{}
		}
		attributesJSON, err := json.Marshal(attributes)
		if err != nil {
			return nil, fmt.Errorf("encode telemetry resource attributes: %w", err)
		}

		var item store.TelemetryService
		var persistedAttributes []byte
		err = tx.QueryRow(ctx, `
			INSERT INTO telemetry_services (name, resource_attributes)
			VALUES ($1, $2::jsonb)
			ON CONFLICT (name) DO UPDATE SET
				resource_attributes = telemetry_services.resource_attributes || EXCLUDED.resource_attributes,
				last_seen_at = NOW(),
				last_discovered_at = NOW(),
				updated_at = NOW()
			RETURNING id::text, name, first_seen_at, last_seen_at, last_discovered_at, resource_attributes
		`, service.Name, string(attributesJSON)).Scan(
			&item.ID, &item.Name, &item.FirstSeenAt, &item.LastSeenAt,
			&item.LastDiscoveredAt, &persistedAttributes,
		)
		if err != nil {
			return nil, fmt.Errorf("upsert telemetry service: %w", err)
		}
		if err := json.Unmarshal(persistedAttributes, &item.ResourceAttributes); err != nil {
			return nil, fmt.Errorf("decode telemetry resource attributes: %w", err)
		}

		for _, source := range sources {
			if _, err := tx.Exec(ctx, `
				INSERT INTO telemetry_service_sources (service_id, source)
				VALUES ($1::uuid, $2)
				ON CONFLICT (service_id, source) DO UPDATE SET
					last_seen_at = NOW(),
					last_discovered_at = NOW()
			`, item.ID, source); err != nil {
				return nil, fmt.Errorf("upsert telemetry service source: %w", err)
			}
		}
		item.Sources = sources
		persisted = append(persisted, item)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit telemetry catalog upsert: %w", err)
	}
	return persisted, nil
}


func (r *Repository) ListTelemetryServices(ctx context.Context, limit int) ([]store.TelemetryService, error) {
	if limit < 1 || limit > 250 {
		limit = 250
	}
	rows, err := r.pool.Query(ctx, `
		SELECT
			services.id::text,
			services.name,
			COALESCE(
				array_agg(sources.source ORDER BY sources.source)
					FILTER (WHERE sources.source IS NOT NULL),
				ARRAY[]::text[]
			),
			services.first_seen_at,
			services.last_seen_at,
			services.last_discovered_at,
			services.resource_attributes
		FROM telemetry_services AS services
		LEFT JOIN telemetry_service_sources AS sources
			ON sources.service_id = services.id
		WHERE services.last_seen_at >= NOW() - INTERVAL '7 days'
		GROUP BY services.id
		ORDER BY services.name ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query persisted telemetry services: %w", err)
	}
	defer rows.Close()

	items := make([]store.TelemetryService, 0)
	for rows.Next() {
		var item store.TelemetryService
		var attributes []byte
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Sources, &item.FirstSeenAt,
			&item.LastSeenAt, &item.LastDiscoveredAt, &attributes,
		); err != nil {
			return nil, fmt.Errorf("scan persisted telemetry service: %w", err)
		}
		if err := json.Unmarshal(attributes, &item.ResourceAttributes); err != nil {
			return nil, fmt.Errorf("decode persisted telemetry resource attributes: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate persisted telemetry services: %w", err)
	}
	return items, nil
}

func (r *Repository) ListServices(ctx context.Context) ([]store.Service, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, name, description, environment, region, status,
		       latency_p95_ms, error_rate, last_seen_at, source
		FROM services
		ORDER BY name ASC
		LIMIT 500
	`)
	if err != nil {
		return nil, fmt.Errorf("query services: %w", err)
	}
	defer rows.Close()

	services := make([]store.Service, 0)
	for rows.Next() {
		var item store.Service
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Description, &item.Environment,
			&item.Region, &item.Status, &item.LatencyP95, &item.ErrorRate,
			&item.LastSeenAt, &item.Source,
		); err != nil {
			return nil, fmt.Errorf("scan service: %w", err)
		}
		services = append(services, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate services: %w", err)
	}
	return services, nil
}

func (r *Repository) ListIncidents(ctx context.Context, status string) ([]store.Incident, error) {
	query := `
		SELECT i.id::text, i.title, i.description, i.service_id::text,
		       s.name, i.severity, i.status, i.started_at, i.updated_at,
		       i.resolved_at, i.source
		FROM incidents i
		JOIN services s ON s.id = i.service_id
	`
	args := []any{}
	if status != "" {
		query += " WHERE i.status = $1"
		args = append(args, status)
	}
	query += " ORDER BY i.started_at DESC LIMIT 500"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query incidents: %w", err)
	}
	defer rows.Close()

	incidents := make([]store.Incident, 0)
	for rows.Next() {
		var item store.Incident
		if err := rows.Scan(
			&item.ID, &item.Title, &item.Description, &item.ServiceID,
			&item.ServiceName, &item.Severity, &item.Status, &item.StartedAt,
			&item.UpdatedAt, &item.ResolvedAt, &item.Source,
		); err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}
		incidents = append(incidents, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incidents: %w", err)
	}
	return incidents, nil
}

var _ store.Repository = (*Repository)(nil)

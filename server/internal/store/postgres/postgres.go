package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	const version = "001_initial"
	var applied bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&applied); err != nil {
		return fmt.Errorf("check migration version: %w", err)
	}
	if applied {
		return tx.Commit(ctx)
	}

	migration, err := migrationFiles.ReadFile("migrations/001_initial.sql")
	if err != nil {
		return fmt.Errorf("read initial migration: %w", err)
	}
	if _, err := tx.Exec(ctx, string(migration)); err != nil {
		return fmt.Errorf("execute initial migration: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
		return fmt.Errorf("record initial migration: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit initial migration: %w", err)
	}
	return nil
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

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
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pooya16pg-spec/orbit/server/internal/store"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Repository struct {
	pool                 *pgxpool.Pool
	lastRateLimitCleanup atomic.Int64
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


func (r *Repository) IncrementRateLimitWindow(ctx context.Context, keyHash string) (int64, error) {
	if len(keyHash) != 64 {
		return 0, fmt.Errorf("rate-limit key hash must be a SHA-256 hex digest")
	}
	for _, char := range keyHash {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return 0, fmt.Errorf("rate-limit key hash must be lowercase hexadecimal")
		}
	}

	cleanupSlot := time.Now().UTC().Unix() / 300
	previousSlot := r.lastRateLimitCleanup.Load()
	if cleanupSlot > previousSlot && r.lastRateLimitCleanup.CompareAndSwap(previousSlot, cleanupSlot) {
		if _, err := r.pool.Exec(ctx, `DELETE FROM api_rate_limit_windows WHERE expires_at <= NOW()`); err != nil {
			r.lastRateLimitCleanup.CompareAndSwap(cleanupSlot, previousSlot)
			return 0, fmt.Errorf("prune expired API rate-limit windows: %w", err)
		}
	}

	var count int64
	err := r.pool.QueryRow(ctx, `
		WITH current_window AS (
			SELECT date_trunc('minute', NOW()) AS bucket_start
		)
		INSERT INTO api_rate_limit_windows (bucket_start, key_hash, request_count, expires_at)
		SELECT bucket_start, $1, 1, bucket_start + INTERVAL '2 minutes'
		FROM current_window
		ON CONFLICT (bucket_start, key_hash) DO UPDATE SET
			request_count = api_rate_limit_windows.request_count + 1
		RETURNING request_count
	`, keyHash).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("increment shared API rate-limit window: %w", err)
	}
	return count, nil
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
		{version: "003_telemetry_resource_attribute_sources", file: "migrations/003_telemetry_resource_attribute_sources.sql"},
		{version: "004_shared_api_rate_limits", file: "migrations/004_shared_api_rate_limits.sql"},
		{version: "005_workspaces_memberships_audit", file: "migrations/005_workspaces_memberships_audit.sql"},
		{version: "006_workspace_data_scope", file: "migrations/006_workspace_data_scope.sql"},
		{version: "007_workload_identities", file: "migrations/007_workload_identities.sql"},
		{version: "008_workload_authorization_denials", file: "migrations/008_workload_authorization_denials.sql"},
	}

	for _, item := range migrations {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin %s migration transaction: %w", item.version, err)
		}
		// Serialize schema checks and DDL across API replicas and parallel
		// test packages. The transaction-scoped lock is released on commit or rollback.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(1330795073, 1)"); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("lock schema migration %s: %w", item.version, err)
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

var resourceAttributeNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

func normalizeResourceAttributeSources(input map[string][]string) (map[string][]string, error) {
	result := make(map[string][]string, len(input))
	for key, sources := range input {
		if !resourceAttributeNamePattern.MatchString(key) {
			return nil, fmt.Errorf("invalid resource attribute name")
		}
		seen := make(map[string]struct{}, len(sources))
		for _, source := range sources {
			source = strings.TrimSpace(source)
			if _, ok := allowedTelemetrySources[source]; !ok {
				return nil, fmt.Errorf("unsupported resource attribute source")
			}
			seen[source] = struct{}{}
		}
		for source := range seen {
			result[key] = append(result[key], source)
		}
		sort.Strings(result[key])
	}
	return result, nil
}

func mergeResourceAttributeSources(existing, incoming map[string][]string) map[string][]string {
	merged := make(map[string]map[string]struct{}, len(existing)+len(incoming))
	for key, sources := range existing {
		merged[key] = make(map[string]struct{}, len(sources))
		for _, source := range sources {
			merged[key][source] = struct{}{}
		}
	}
	for key, sources := range incoming {
		if merged[key] == nil {
			merged[key] = make(map[string]struct{}, len(sources))
		}
		for _, source := range sources {
			merged[key][source] = struct{}{}
		}
	}
	result := make(map[string][]string, len(merged))
	for key, sources := range merged {
		for source := range sources {
			result[key] = append(result[key], source)
		}
		sort.Strings(result[key])
	}
	return result
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
		incomingAttributeSources, err := normalizeResourceAttributeSources(service.ResourceAttributeSources)
		if err != nil {
			return nil, err
		}

		var item store.TelemetryService
		var persistedAttributes []byte
		err = tx.QueryRow(ctx, `
			INSERT INTO telemetry_services (workspace_id, name, resource_attributes)
			VALUES ((SELECT id FROM workspaces WHERE slug = 'legacy-default'), $1, $2::jsonb)
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

		var existingSourcesJSON []byte
		if err := tx.QueryRow(ctx, `
			SELECT resource_attribute_sources
			FROM telemetry_services
			WHERE id = $1::uuid
			FOR UPDATE
		`, item.ID).Scan(&existingSourcesJSON); err != nil {
			return nil, fmt.Errorf("read resource attribute provenance: %w", err)
		}
		existingAttributeSources := make(map[string][]string)
		if err := json.Unmarshal(existingSourcesJSON, &existingAttributeSources); err != nil {
			return nil, fmt.Errorf("decode resource attribute provenance: %w", err)
		}
		item.ResourceAttributeSources = mergeResourceAttributeSources(existingAttributeSources, incomingAttributeSources)
		mergedSourcesJSON, err := json.Marshal(item.ResourceAttributeSources)
		if err != nil {
			return nil, fmt.Errorf("encode resource attribute provenance: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE telemetry_services
			SET resource_attribute_sources = $2::jsonb
			WHERE id = $1::uuid
		`, item.ID, string(mergedSourcesJSON)); err != nil {
			return nil, fmt.Errorf("persist resource attribute provenance: %w", err)
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

func (r *Repository) UpdateTelemetryServiceResourceAttributes(
	ctx context.Context,
	name string,
	attributes map[string]any,
	attributeSources map[string][]string,
) error {
	if !telemetryServiceNamePattern.MatchString(name) {
		return fmt.Errorf("invalid telemetry service name")
	}
	if attributes == nil {
		attributes = map[string]any{}
	}
	attributesJSON, err := json.Marshal(attributes)
	if err != nil {
		return fmt.Errorf("encode telemetry resource attributes: %w", err)
	}
	incomingSources, err := normalizeResourceAttributeSources(attributeSources)
	if err != nil {
		return err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin resource attribute update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var serviceID string
	var existingJSON []byte
	err = tx.QueryRow(ctx, `
		SELECT id::text, resource_attribute_sources
		FROM telemetry_services
		WHERE name = $1
		FOR UPDATE
	`, name).Scan(&serviceID, &existingJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrTelemetryServiceNotFound
	}
	if err != nil {
		return fmt.Errorf("read telemetry service for attribute update: %w", err)
	}
	existingSources := make(map[string][]string)
	if err := json.Unmarshal(existingJSON, &existingSources); err != nil {
		return fmt.Errorf("decode resource attribute provenance: %w", err)
	}
	mergedSources := mergeResourceAttributeSources(existingSources, incomingSources)
	mergedJSON, err := json.Marshal(mergedSources)
	if err != nil {
		return fmt.Errorf("encode resource attribute provenance: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE telemetry_services
		SET resource_attributes = resource_attributes || $2::jsonb,
		    resource_attribute_sources = $3::jsonb,
		    updated_at = NOW()
		WHERE id = $1::uuid
	`, serviceID, string(attributesJSON), string(mergedJSON)); err != nil {
		return fmt.Errorf("update telemetry resource attributes: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit telemetry resource attributes: %w", err)
	}
	return nil
}

var telemetryServiceCursorIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (r *Repository) ListTelemetryServicesPage(
	ctx context.Context,
	limit int,
	cursor *store.TelemetryServiceCursor,
) (store.TelemetryServicePage, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var cursorName any
	var cursorID any
	if cursor != nil {
		if !telemetryServiceNamePattern.MatchString(cursor.Name) || !telemetryServiceCursorIDPattern.MatchString(cursor.ID) {
			return store.TelemetryServicePage{}, fmt.Errorf("invalid telemetry service cursor")
		}
		cursorName = cursor.Name
		cursorID = cursor.ID
	}
	rows, err := r.pool.Query(ctx, `
		SELECT
			services.id::text,
			services.name,
			COALESCE(
				array_agg(sources.source ORDER BY sources.source)
					FILTER (WHERE sources.source IS NOT NULL AND sources.last_seen_at >= NOW() - INTERVAL '7 days'),
				ARRAY[]::text[]
			),
			services.first_seen_at,
			services.last_seen_at,
			services.last_discovered_at,
			services.resource_attributes,
			services.resource_attribute_sources
		FROM telemetry_services AS services
		LEFT JOIN telemetry_service_sources AS sources
			ON sources.service_id = services.id
		WHERE services.last_seen_at >= NOW() - INTERVAL '7 days'
		  AND ($1::text IS NULL OR (services.name, services.id) > ($1::text, $2::uuid))
		GROUP BY services.id
		ORDER BY services.name ASC, services.id ASC
		LIMIT $3
	`, cursorName, cursorID, limit+1)
	if err != nil {
		return store.TelemetryServicePage{}, fmt.Errorf("query persisted telemetry service page: %w", err)
	}
	defer rows.Close()

	items := make([]store.TelemetryService, 0, limit+1)
	for rows.Next() {
		var item store.TelemetryService
		var attributes []byte
		var attributeSources []byte
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Sources, &item.FirstSeenAt,
			&item.LastSeenAt, &item.LastDiscoveredAt, &attributes, &attributeSources,
		); err != nil {
			return store.TelemetryServicePage{}, fmt.Errorf("scan persisted telemetry service page: %w", err)
		}
		if err := json.Unmarshal(attributes, &item.ResourceAttributes); err != nil {
			return store.TelemetryServicePage{}, fmt.Errorf("decode persisted telemetry resource attributes: %w", err)
		}
		if err := json.Unmarshal(attributeSources, &item.ResourceAttributeSources); err != nil {
			return store.TelemetryServicePage{}, fmt.Errorf("decode resource attribute provenance: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return store.TelemetryServicePage{}, fmt.Errorf("iterate persisted telemetry service page: %w", err)
	}

	page := store.TelemetryServicePage{Items: items}
	if len(page.Items) > limit {
		page.HasMore = true
		page.Items = page.Items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &store.TelemetryServiceCursor{Name: last.Name, ID: last.ID}
	}
	return page, nil
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
					FILTER (WHERE sources.source IS NOT NULL AND sources.last_seen_at >= NOW() - INTERVAL '7 days'),
				ARRAY[]::text[]
			),
			services.first_seen_at,
			services.last_seen_at,
			services.last_discovered_at,
			services.resource_attributes,
			services.resource_attribute_sources
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
		var attributeSources []byte
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Sources, &item.FirstSeenAt,
			&item.LastSeenAt, &item.LastDiscoveredAt, &attributes, &attributeSources,
		); err != nil {
			return nil, fmt.Errorf("scan persisted telemetry service: %w", err)
		}
		if err := json.Unmarshal(attributes, &item.ResourceAttributes); err != nil {
			return nil, fmt.Errorf("decode persisted telemetry resource attributes: %w", err)
		}
		if err := json.Unmarshal(attributeSources, &item.ResourceAttributeSources); err != nil {
			return nil, fmt.Errorf("decode resource attribute provenance: %w", err)
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


func (r *Repository) ListWorkspaceServices(ctx context.Context, workspaceID string) ([]store.Service, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, name, description, environment, region, status,
		       latency_p95_ms, error_rate, last_seen_at, source
		FROM services
		WHERE workspace_id = $1::uuid
		ORDER BY name ASC
		LIMIT 500
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("query workspace services: %w", err)
	}
	defer rows.Close()
	items := make([]store.Service, 0)
	for rows.Next() {
		var item store.Service
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Environment,
			&item.Region, &item.Status, &item.LatencyP95, &item.ErrorRate,
			&item.LastSeenAt, &item.Source); err != nil {
			return nil, fmt.Errorf("scan workspace service: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace services: %w", err)
	}
	return items, nil
}

func (r *Repository) ListWorkspaceIncidents(ctx context.Context, workspaceID, status string) ([]store.Incident, error) {
	query := `
		SELECT i.id::text, i.title, i.description, i.service_id::text,
		       s.name, i.severity, i.status, i.started_at, i.updated_at,
		       i.resolved_at, i.source
		FROM incidents i
		JOIN services s
		  ON s.id = i.service_id AND s.workspace_id = i.workspace_id
		WHERE i.workspace_id = $1::uuid
	`
	args := []any{workspaceID}
	if status != "" {
		query += " AND i.status = $2"
		args = append(args, status)
	}
	query += " ORDER BY i.started_at DESC LIMIT 500"
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query workspace incidents: %w", err)
	}
	defer rows.Close()
	items := make([]store.Incident, 0)
	for rows.Next() {
		var item store.Incident
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.ServiceID,
			&item.ServiceName, &item.Severity, &item.Status, &item.StartedAt,
			&item.UpdatedAt, &item.ResolvedAt, &item.Source); err != nil {
			return nil, fmt.Errorf("scan workspace incident: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace incidents: %w", err)
	}
	return items, nil
}

func (r *Repository) ListWorkspaceTelemetryServices(ctx context.Context, workspaceID string, limit int) ([]store.TelemetryService, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT s.id::text, s.name,
		       COALESCE(array_agg(src.source ORDER BY src.source)
		           FILTER (WHERE src.source IS NOT NULL), ARRAY[]::text[]),
		       s.first_seen_at, s.last_seen_at, s.last_discovered_at,
		       s.resource_attributes, s.resource_attribute_sources
		FROM telemetry_services s
		LEFT JOIN telemetry_service_sources src ON src.service_id = s.id
		WHERE s.workspace_id = $1::uuid
		GROUP BY s.id
		ORDER BY s.name ASC, s.id ASC
		LIMIT $2
	`, workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("query workspace telemetry services: %w", err)
	}
	defer rows.Close()
	items := make([]store.TelemetryService, 0)
	for rows.Next() {
		var item store.TelemetryService
		var attributesJSON, sourcesJSON []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.Sources, &item.FirstSeenAt,
			&item.LastSeenAt, &item.LastDiscoveredAt, &attributesJSON, &sourcesJSON); err != nil {
			return nil, fmt.Errorf("scan workspace telemetry service: %w", err)
		}
		if err := json.Unmarshal(attributesJSON, &item.ResourceAttributes); err != nil {
			return nil, fmt.Errorf("decode workspace telemetry attributes: %w", err)
		}
		if err := json.Unmarshal(sourcesJSON, &item.ResourceAttributeSources); err != nil {
			return nil, fmt.Errorf("decode workspace telemetry attribute sources: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace telemetry services: %w", err)
	}
	return items, nil
}

var _ store.WorkspaceDataRepository = (*Repository)(nil)

CREATE TABLE services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    environment TEXT NOT NULL,
    region TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('operational', 'degraded', 'incident', 'unknown')),
    latency_p95_ms DOUBLE PRECISION,
    error_rate DOUBLE PRECISION,
    last_seen_at TIMESTAMPTZ,
    source TEXT NOT NULL DEFAULT 'live',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT services_name_environment_unique UNIQUE (name, environment)
);

CREATE INDEX services_status_idx ON services (status);
CREATE INDEX services_environment_idx ON services (environment);
CREATE INDEX services_last_seen_idx ON services (last_seen_at DESC);

CREATE TABLE incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE RESTRICT,
    severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    status TEXT NOT NULL CHECK (status IN ('open', 'investigating', 'monitoring', 'resolved')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    source TEXT NOT NULL DEFAULT 'live'
);

CREATE INDEX incidents_status_started_idx ON incidents (status, started_at DESC);
CREATE INDEX incidents_service_started_idx ON incidents (service_id, started_at DESC);

INSERT INTO services
    (id, name, description, environment, region, status, latency_p95_ms, error_rate, last_seen_at, source)
VALUES
    ('a1000000-0000-4000-8000-000000000001', 'api-gateway', 'Edge request router', 'production', 'eu-central-1', 'operational', 42, 0.0003, NOW(), 'demo_seed'),
    ('a1000000-0000-4000-8000-000000000002', 'identity-core', 'Authentication service', 'production', 'eu-central-1', 'operational', 68, 0.0008, NOW(), 'demo_seed'),
    ('a1000000-0000-4000-8000-000000000003', 'billing-engine', 'Payments and invoicing', 'production', 'eu-west-1', 'degraded', 184, 0.0142, NOW(), 'demo_seed'),
    ('a1000000-0000-4000-8000-000000000004', 'event-stream', 'Asynchronous event processing', 'production', 'eu-central-1', 'operational', 31, 0.0001, NOW(), 'demo_seed'),
    ('a1000000-0000-4000-8000-000000000005', 'search-indexer', 'Search infrastructure', 'production', 'us-east-1', 'incident', NULL, NULL, NOW() - INTERVAL '15 minutes', 'demo_seed')
ON CONFLICT (id) DO NOTHING;

INSERT INTO incidents
    (id, title, description, service_id, severity, status, started_at, updated_at, source)
VALUES
    ('b1000000-0000-4000-8000-000000000001', 'Search indexing stalled', 'The illustrative indexer workload stopped advancing its checkpoint.', 'a1000000-0000-4000-8000-000000000005', 'critical', 'investigating', NOW() - INTERVAL '14 minutes', NOW(), 'demo_seed'),
    ('b1000000-0000-4000-8000-000000000002', 'Elevated payment latency', 'The illustrative payment path is above its expected latency band.', 'a1000000-0000-4000-8000-000000000003', 'medium', 'monitoring', NOW() - INTERVAL '38 minutes', NOW(), 'demo_seed')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE telemetry_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    resource_attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT telemetry_services_name_valid
        CHECK (name ~ '^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$')
);

CREATE INDEX telemetry_services_last_seen_idx
    ON telemetry_services (last_seen_at DESC);
CREATE INDEX telemetry_services_last_discovered_idx
    ON telemetry_services (last_discovered_at DESC);

CREATE TABLE telemetry_service_sources (
    service_id UUID NOT NULL REFERENCES telemetry_services(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('prometheus', 'loki', 'tempo')),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (service_id, source)
);

CREATE INDEX telemetry_service_sources_last_discovered_idx
    ON telemetry_service_sources (last_discovered_at DESC);

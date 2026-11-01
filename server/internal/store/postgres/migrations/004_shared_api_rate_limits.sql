CREATE TABLE api_rate_limit_windows (
    bucket_start TIMESTAMPTZ NOT NULL,
    key_hash CHAR(64) NOT NULL CHECK (key_hash ~ '^[0-9a-f]{64}$'),
    request_count BIGINT NOT NULL CHECK (request_count > 0),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (bucket_start, key_hash),
    CHECK (expires_at > bucket_start)
);

CREATE INDEX api_rate_limit_windows_expiry_idx
    ON api_rate_limit_windows (expires_at);

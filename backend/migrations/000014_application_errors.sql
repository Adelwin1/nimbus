BEGIN;

CREATE TABLE application_error_tokens (
    application_id UUID PRIMARY KEY REFERENCES applications(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL CHECK (octet_length(token_hash) = 32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE application_error_groups (
    id UUID PRIMARY KEY,
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    fingerprint VARCHAR(64) NOT NULL CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
    error_type VARCHAR(80) NOT NULL,
    error_code VARCHAR(80) NOT NULL,
    title VARCHAR(160) NOT NULL,
    frames JSONB NOT NULL DEFAULT '[]'::jsonb,
    occurrences BIGINT NOT NULL DEFAULT 1 CHECK (occurrences > 0),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT application_error_group_unique
        UNIQUE(application_id, fingerprint),
    CONSTRAINT application_error_frames_array CHECK (
        jsonb_typeof(frames) = 'array'
        AND jsonb_array_length(frames) <= 20
    )
);

CREATE INDEX application_errors_recent_idx
    ON application_error_groups(application_id, last_seen_at DESC);

CREATE TABLE application_error_occurrences (
    id UUID PRIMARY KEY,
    group_id UUID NOT NULL REFERENCES application_error_groups(id) ON DELETE CASCADE,
    release_version VARCHAR(120),
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX application_error_occurrences_recent_idx
    ON application_error_occurrences(group_id, received_at DESC);

COMMIT;

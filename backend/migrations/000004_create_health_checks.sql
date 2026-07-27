-- +goose Up

CREATE TABLE health_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL
        REFERENCES applications(id)
        ON DELETE CASCADE,

    status_code INTEGER,
    latency_ms BIGINT NOT NULL,
    healthy BOOLEAN NOT NULL,
    error_message TEXT,

    checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT health_checks_status_code_valid
        CHECK (
            status_code IS NULL
            OR (
                status_code >= 100
                AND status_code <= 599
            )
        ),

    CONSTRAINT health_checks_latency_valid
        CHECK (latency_ms >= 0)
);

CREATE INDEX idx_health_checks_application_id
    ON health_checks(application_id);

CREATE INDEX idx_health_checks_application_checked_at
    ON health_checks(application_id, checked_at DESC);

CREATE INDEX idx_health_checks_checked_at
    ON health_checks(checked_at DESC);

CREATE INDEX idx_applications_last_checked_at
    ON applications(last_checked_at);

-- +goose Down

DROP INDEX IF EXISTS idx_applications_last_checked_at;
DROP TABLE IF EXISTS health_checks;

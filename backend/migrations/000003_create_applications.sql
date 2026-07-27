-- +goose Up

CREATE TABLE applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    name VARCHAR(120) NOT NULL,
    description TEXT NOT NULL DEFAULT '',

    application_url TEXT NOT NULL,
    health_url TEXT NOT NULL,
    repository_url TEXT,

    environment VARCHAR(30) NOT NULL DEFAULT 'production',
    current_version VARCHAR(100),
    status VARCHAR(30) NOT NULL DEFAULT 'unknown',

    monitoring_interval_seconds INTEGER NOT NULL DEFAULT 60,
    failure_threshold INTEGER NOT NULL DEFAULT 3,
    latency_threshold_ms INTEGER NOT NULL DEFAULT 2000,

    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    last_checked_at TIMESTAMPTZ,
    last_healthy_at TIMESTAMPTZ,

    deployment_webhook_url_encrypted TEXT,
    rollback_webhook_url_encrypted TEXT,
    webhook_token_encrypted TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT applications_name_not_blank
        CHECK (char_length(trim(name)) > 0),

    CONSTRAINT applications_environment_valid
        CHECK (
            environment IN (
                'development',
                'staging',
                'production'
            )
        ),

    CONSTRAINT applications_status_valid
        CHECK (
            status IN (
                'unknown',
                'healthy',
                'degraded',
                'down',
                'deploying'
            )
        ),

    CONSTRAINT applications_monitoring_interval_valid
        CHECK (
            monitoring_interval_seconds >= 30
            AND monitoring_interval_seconds <= 86400
        ),

    CONSTRAINT applications_failure_threshold_valid
        CHECK (
            failure_threshold >= 1
            AND failure_threshold <= 20
        ),

    CONSTRAINT applications_latency_threshold_valid
        CHECK (
            latency_threshold_ms >= 100
            AND latency_threshold_ms <= 60000
        ),

    CONSTRAINT applications_consecutive_failures_valid
        CHECK (consecutive_failures >= 0)
);

CREATE INDEX idx_applications_user_id
    ON applications(user_id);

CREATE INDEX idx_applications_user_created_at
    ON applications(user_id, created_at DESC);

CREATE INDEX idx_applications_user_status
    ON applications(user_id, status);

CREATE INDEX idx_applications_user_id_id
    ON applications(user_id, id);

-- +goose Down

DROP TABLE IF EXISTS applications;

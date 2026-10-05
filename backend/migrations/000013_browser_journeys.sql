BEGIN;

CREATE TABLE browser_journeys (
    id UUID PRIMARY KEY,
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL CHECK (char_length(trim(name)) > 0),
    base_url TEXT NOT NULL,
    steps JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT browser_journey_steps_array CHECK (
        jsonb_typeof(steps) = 'array'
        AND jsonb_array_length(steps) BETWEEN 1 AND 20
    )
);

CREATE INDEX browser_journeys_application_idx
    ON browser_journeys(application_id);

CREATE TABLE browser_journey_runs (
    id UUID PRIMARY KEY,
    journey_id UUID NOT NULL REFERENCES browser_journeys(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'passed', 'failed', 'error')),
    definition JSONB NOT NULL,
    result JSONB,
    error_message TEXT,
    queued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    lease_token UUID,
    lease_expires_at TIMESTAMPTZ,
    CONSTRAINT browser_run_definition_object CHECK (
        jsonb_typeof(definition) = 'object'
    ),
    CONSTRAINT browser_run_result_object CHECK (
        result IS NULL OR jsonb_typeof(result) = 'object'
    )
);

CREATE INDEX browser_journey_runs_history_idx
    ON browser_journey_runs(journey_id, queued_at DESC);

CREATE INDEX browser_journey_runs_queue_idx
    ON browser_journey_runs(queued_at)
    WHERE status = 'queued';

COMMIT;

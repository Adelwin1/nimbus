-- +goose Up

ALTER TABLE health_checks
ADD COLUMN incident_evaluated_at TIMESTAMPTZ;

CREATE INDEX idx_health_checks_incident_evaluation
    ON health_checks(checked_at ASC)
    WHERE incident_evaluated_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_health_checks_incident_evaluation;

ALTER TABLE health_checks
DROP COLUMN IF EXISTS incident_evaluated_at;

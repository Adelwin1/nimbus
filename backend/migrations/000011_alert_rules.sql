BEGIN;
CREATE TABLE application_alert_rules (
 application_id UUID PRIMARY KEY REFERENCES applications(id) ON DELETE CASCADE,
 rule VARCHAR(20) NOT NULL CHECK (rule IN ('off','critical','all')),
 enabled_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
COMMIT;

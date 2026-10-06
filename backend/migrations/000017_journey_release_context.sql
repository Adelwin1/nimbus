BEGIN;
ALTER TABLE browser_journey_runs ADD COLUMN release_context JSONB NOT NULL DEFAULT '{}'::jsonb
 CHECK (jsonb_typeof(release_context)='object');
COMMIT;

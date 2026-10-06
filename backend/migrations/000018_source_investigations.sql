BEGIN;
ALTER TABLE browser_journey_runs ADD COLUMN source_investigation JSONB
 CHECK(source_investigation IS NULL OR jsonb_typeof(source_investigation)='object');
COMMIT;

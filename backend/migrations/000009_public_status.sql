BEGIN;
CREATE TABLE public_status_pages (
 application_id UUID PRIMARY KEY REFERENCES applications(id) ON DELETE CASCADE,
 slug VARCHAR(80) UNIQUE
);
COMMIT;

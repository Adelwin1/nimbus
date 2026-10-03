BEGIN;
CREATE TABLE application_projects (
 application_id UUID PRIMARY KEY REFERENCES applications(id) ON DELETE CASCADE,
 project_name VARCHAR(80) NOT NULL CHECK (char_length(trim(project_name)) > 0)
);
COMMIT;

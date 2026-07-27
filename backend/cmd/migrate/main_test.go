package main

import (
	"strings"
	"testing"
)

func TestExtractUpMigrationSQLWithSections(
	t *testing.T,
) {
	t.Parallel()

	input := `
-- +goose Up
CREATE TABLE users (
	id UUID PRIMARY KEY
);

-- +goose Down
DROP TABLE users;
`

	result, err := extractUpMigrationSQL(input)
	if err != nil {
		t.Fatalf(
			"extract up migration: %v",
			err,
		)
	}

	if !strings.Contains(
		result,
		"CREATE TABLE users",
	) {
		t.Fatalf(
			"expected create statement, got %q",
			result,
		)
	}

	if strings.Contains(
		result,
		"DROP TABLE",
	) {
		t.Fatalf(
			"down migration remained: %q",
			result,
		)
	}
}

func TestExtractUpMigrationSQLRemovesOuterTransaction(
	t *testing.T,
) {
	t.Parallel()

	input := `
BEGIN;

CREATE TABLE users (
	id UUID PRIMARY KEY
);

COMMIT;
`

	result, err := extractUpMigrationSQL(input)
	if err != nil {
		t.Fatalf(
			"extract up migration: %v",
			err,
		)
	}

	if strings.Contains(
		strings.ToUpper(result),
		"BEGIN;",
	) {
		t.Fatalf(
			"outer BEGIN remained: %q",
			result,
		)
	}

	if strings.Contains(
		strings.ToUpper(result),
		"COMMIT;",
	) {
		t.Fatalf(
			"outer COMMIT remained: %q",
			result,
		)
	}
}

func TestExtractUpMigrationSQLWithoutMarkers(
	t *testing.T,
) {
	t.Parallel()

	input := `
CREATE TABLE users (
	id UUID PRIMARY KEY
);
`

	result, err := extractUpMigrationSQL(input)
	if err != nil {
		t.Fatalf(
			"extract up migration: %v",
			err,
		)
	}

	if !strings.Contains(
		result,
		"CREATE TABLE users",
	) {
		t.Fatalf(
			"unexpected result: %q",
			result,
		)
	}
}

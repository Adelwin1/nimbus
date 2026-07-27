package testutil

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	databaseTimeout     = 10 * time.Second
	databaseLockTimeout = 5 * time.Minute

	// A project-specific PostgreSQL advisory-lock namespace.
	testDatabaseLockNamespace int32 = 739297184
)

func OpenTestDatabase(
	t *testing.T,
) *pgxpool.Pool {
	t.Helper()

	testDatabaseURL := strings.TrimSpace(
		os.Getenv("TEST_DATABASE_URL"),
	)
	if testDatabaseURL == "" {
		t.Fatal(
			"TEST_DATABASE_URL is required for integration tests",
		)
	}

	developmentDatabaseURL := strings.TrimSpace(
		os.Getenv("DATABASE_URL"),
	)

	if developmentDatabaseURL != "" &&
		developmentDatabaseURL == testDatabaseURL {
		t.Fatal(
			"TEST_DATABASE_URL must not equal DATABASE_URL",
		)
	}

	config, err := pgxpool.ParseConfig(
		testDatabaseURL,
	)
	if err != nil {
		t.Fatalf(
			"parse TEST_DATABASE_URL: %v",
			err,
		)
	}

	if !IsSafeTestDatabaseName(
		config.ConnConfig.Database,
	) {
		t.Fatalf(
			"refusing to use database %q because its name does not contain test",
			config.ConnConfig.Database,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		databaseTimeout,
	)
	defer cancel()

	database, err := pgxpool.NewWithConfig(
		ctx,
		config,
	)
	if err != nil {
		t.Fatalf(
			"open test database: %v",
			err,
		)
	}

	if err := database.Ping(ctx); err != nil {
		database.Close()

		t.Fatalf(
			"ping test database: %v",
			err,
		)
	}

	// Hold a session-level advisory lock for the entire test.
	// This prevents separately running Go package tests from
	// truncating the shared test database concurrently.
	lockContext, lockCancel := context.WithTimeout(
		context.Background(),
		databaseLockTimeout,
	)
	defer lockCancel()

	lockConnection, err := database.Acquire(
		lockContext,
	)
	if err != nil {
		database.Close()

		t.Fatalf(
			"acquire test database lock connection: %v",
			err,
		)
	}

	_, err = lockConnection.Exec(
		lockContext,
		`
			SELECT pg_advisory_lock(
				hashtext(current_database()),
				$1
			)
		`,
		testDatabaseLockNamespace,
	)
	if err != nil {
		lockConnection.Release()
		database.Close()

		t.Fatalf(
			"lock test database: %v",
			err,
		)
	}

	t.Cleanup(func() {
		cleanupContext, cleanupCancel :=
			context.WithTimeout(
				context.Background(),
				databaseTimeout,
			)
		defer cleanupCancel()

		_, _ = lockConnection.Exec(
			cleanupContext,
			`
				SELECT pg_advisory_unlock(
					hashtext(current_database()),
					$1
				)
			`,
			testDatabaseLockNamespace,
		)

		lockConnection.Release()
		database.Close()
	})

	return database
}

func ResetDatabase(
	t *testing.T,
	database *pgxpool.Pool,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		databaseTimeout,
	)
	defer cancel()

	const query = `
		DO $$
		DECLARE
			table_names TEXT;
		BEGIN
			SELECT string_agg(
				format('%I.%I', schemaname, tablename),
				', '
			)
			INTO table_names
			FROM pg_tables
			WHERE schemaname = 'public'
			  AND tablename <> 'goose_db_version';

			IF table_names IS NOT NULL THEN
				EXECUTE
					'TRUNCATE TABLE ' ||
					table_names ||
					' RESTART IDENTITY CASCADE';
			END IF;
		END
		$$;
	`

	if _, err := database.Exec(ctx, query); err != nil {
		t.Fatalf(
			"reset test database: %v",
			err,
		)
	}
}

func IsSafeTestDatabaseName(
	name string,
) bool {
	normalized := strings.ToLower(
		strings.TrimSpace(name),
	)

	return normalized != "" &&
		strings.Contains(normalized, "test")
}

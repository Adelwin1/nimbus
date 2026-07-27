package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultMigrationsDirectory       = "/app/migrations"
	databaseWaitTimeout              = 60 * time.Second
	migrationLockID            int64 = 7534672891345
)

var (
	migrationFilePattern = regexp.MustCompile(
		`^(\d+)_.*\.sql$`,
	)

	outerTransactionStartPattern = regexp.MustCompile(
		`(?is)^\s*(?:BEGIN(?:\s+(?:WORK|TRANSACTION))?|START\s+TRANSACTION)\s*;\s*`,
	)

	outerTransactionCommitPattern = regexp.MustCompile(
		`(?is)\s*COMMIT(?:\s+(?:WORK|TRANSACTION))?\s*;\s*$`,
	)
)

type migration struct {
	version  int64
	name     string
	sql      string
	checksum string
}

func main() {
	logger := log.New(
		os.Stdout,
		"migrate: ",
		log.Ldate|log.Ltime|log.LUTC,
	)

	if err := run(context.Background(), logger); err != nil {
		logger.Fatal(err)
	}
}

func run(
	ctx context.Context,
	logger *log.Logger,
) error {
	databaseURL := strings.TrimSpace(
		os.Getenv("DATABASE_URL"),
	)
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	migrationsDirectory := strings.TrimSpace(
		os.Getenv("MIGRATIONS_DIR"),
	)
	if migrationsDirectory == "" {
		migrationsDirectory =
			defaultMigrationsDirectory
	}

	migrations, err := loadMigrations(
		migrationsDirectory,
	)
	if err != nil {
		return err
	}

	poolConfig, err := pgxpool.ParseConfig(
		databaseURL,
	)
	if err != nil {
		return fmt.Errorf(
			"parse database configuration: %w",
			err,
		)
	}

	// Migration files may contain multiple SQL
	// statements, so use PostgreSQL's simple protocol.
	poolConfig.ConnConfig.DefaultQueryExecMode =
		pgx.QueryExecModeSimpleProtocol
	poolConfig.MaxConns = 1

	pool, err := pgxpool.NewWithConfig(
		ctx,
		poolConfig,
	)
	if err != nil {
		return fmt.Errorf(
			"create database pool: %w",
			err,
		)
	}
	defer pool.Close()

	if err := waitForDatabase(
		ctx,
		pool,
		databaseWaitTimeout,
		logger,
	); err != nil {
		return err
	}

	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf(
			"acquire migration connection: %w",
			err,
		)
	}
	defer connection.Release()

	if _, err := connection.Exec(
		ctx,
		`SELECT pg_advisory_lock($1)`,
		migrationLockID,
	); err != nil {
		return fmt.Errorf(
			"acquire migration lock: %w",
			err,
		)
	}

	defer func() {
		unlockContext, cancel :=
			context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
		defer cancel()

		_, _ = connection.Exec(
			unlockContext,
			`SELECT pg_advisory_unlock($1)`,
			migrationLockID,
		)
	}()

	const createMigrationTable = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			checksum CHAR(64) NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`

	if _, err := connection.Exec(
		ctx,
		createMigrationTable,
	); err != nil {
		return fmt.Errorf(
			"create schema_migrations table: %w",
			err,
		)
	}

	for _, currentMigration := range migrations {
		if err := applyMigration(
			ctx,
			connection,
			currentMigration,
			logger,
		); err != nil {
			return err
		}
	}

	logger.Printf(
		"%d migration files verified",
		len(migrations),
	)

	return nil
}

func loadMigrations(
	directory string,
) ([]migration, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf(
			"read migrations directory: %w",
			err,
		)
	}

	migrations := make([]migration, 0)
	versions := make(map[int64]string)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := migrationFilePattern.FindStringSubmatch(
			entry.Name(),
		)
		if len(matches) != 2 {
			continue
		}

		version, err := strconv.ParseInt(
			matches[1],
			10,
			64,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"parse migration version %q: %w",
				entry.Name(),
				err,
			)
		}

		if existingName, exists :=
			versions[version]; exists {
			return nil, fmt.Errorf(
				"duplicate migration version %d: %s and %s",
				version,
				existingName,
				entry.Name(),
			)
		}

		path := filepath.Join(
			directory,
			entry.Name(),
		)

		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf(
				"read migration %s: %w",
				entry.Name(),
				err,
			)
		}

		upSQL, err := extractUpMigrationSQL(
			string(contents),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"parse migration %s: %w",
				entry.Name(),
				err,
			)
		}

		sum := sha256.Sum256(
			[]byte(upSQL),
		)

		versions[version] = entry.Name()

		migrations = append(
			migrations,
			migration{
				version: version,
				name:    entry.Name(),
				sql:     upSQL,
				checksum: hex.EncodeToString(
					sum[:],
				),
			},
		)
	}

	sort.Slice(
		migrations,
		func(left int, right int) bool {
			return migrations[left].version <
				migrations[right].version
		},
	)

	if len(migrations) == 0 {
		return nil, fmt.Errorf(
			"no migration files found in %s",
			directory,
		)
	}

	return migrations, nil
}

func extractUpMigrationSQL(
	contents string,
) (string, error) {
	normalized := strings.ReplaceAll(
		contents,
		"\r\n",
		"\n",
	)

	lines := strings.Split(normalized, "\n")

	upStart := -1
	downStart := -1

	for index, line := range lines {
		switch migrationSectionMarker(line) {
		case "up":
			if upStart == -1 {
				upStart = index + 1
			}

		case "down":
			if downStart == -1 {
				downStart = index
			}
		}
	}

	start := 0
	end := len(lines)

	if upStart >= 0 {
		start = upStart
	}

	if downStart >= start {
		end = downStart
	}

	sql := strings.TrimSpace(
		strings.Join(
			lines[start:end],
			"\n",
		),
	)

	// The migration runner owns the transaction.
	// Remove only an outer BEGIN/COMMIT wrapper.
	sql = outerTransactionStartPattern.ReplaceAllString(
		sql,
		"",
	)
	sql = outerTransactionCommitPattern.ReplaceAllString(
		sql,
		"",
	)
	sql = strings.TrimSpace(sql)

	if sql == "" {
		return "", fmt.Errorf(
			"up migration contains no SQL",
		)
	}

	return sql, nil
}

func migrationSectionMarker(
	line string,
) string {
	trimmed := strings.ToLower(
		strings.TrimSpace(line),
	)

	if !strings.HasPrefix(trimmed, "--") {
		return ""
	}

	marker := strings.TrimSpace(
		strings.TrimPrefix(trimmed, "--"),
	)
	marker = strings.TrimSpace(
		strings.TrimPrefix(marker, "+"),
	)
	marker = strings.ReplaceAll(
		marker,
		"_",
		" ",
	)
	marker = strings.Join(
		strings.Fields(marker),
		" ",
	)

	switch marker {
	case "up",
		"goose up",
		"migrate up",
		"migrate:up",
		"up migration",
		"migration up":
		return "up"

	case "down",
		"goose down",
		"migrate down",
		"migrate:down",
		"down migration",
		"migration down":
		return "down"

	default:
		return ""
	}
}

func applyMigration(
	ctx context.Context,
	connection *pgxpool.Conn,
	current migration,
	logger *log.Logger,
) error {
	var storedChecksum string

	err := connection.QueryRow(
		ctx,
		`
			SELECT checksum
			FROM schema_migrations
			WHERE version = $1
		`,
		current.version,
	).Scan(&storedChecksum)

	switch {
	case err == nil:
		if storedChecksum != current.checksum {
			return fmt.Errorf(
				"migration %s changed after being applied",
				current.name,
			)
		}

		logger.Printf(
			"already applied: %s",
			current.name,
		)

		return nil

	case !errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf(
			"check migration %s: %w",
			current.name,
			err,
		)
	}

	logger.Printf(
		"applying: %s",
		current.name,
	)

	transaction, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"begin migration %s: %w",
			current.name,
			err,
		)
	}

	defer func() {
		_ = transaction.Rollback(
			context.Background(),
		)
	}()

	if _, err := transaction.Exec(
		ctx,
		current.sql,
	); err != nil {
		return fmt.Errorf(
			"execute migration %s: %w",
			current.name,
			err,
		)
	}

	if _, err := transaction.Exec(
		ctx,
		`
			INSERT INTO schema_migrations (
				version,
				name,
				checksum
			)
			VALUES ($1, $2, $3)
		`,
		current.version,
		current.name,
		current.checksum,
	); err != nil {
		return fmt.Errorf(
			"record migration %s: %w",
			current.name,
			err,
		)
	}

	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf(
			"commit migration %s: %w",
			current.name,
			err,
		)
	}

	logger.Printf(
		"applied: %s",
		current.name,
	)

	return nil
}

func waitForDatabase(
	ctx context.Context,
	pool *pgxpool.Pool,
	timeout time.Duration,
	logger *log.Logger,
) error {
	deadline := time.Now().Add(timeout)
	var lastError error

	for {
		pingContext, cancel :=
			context.WithTimeout(
				ctx,
				5*time.Second,
			)

		lastError = pool.Ping(pingContext)
		cancel()

		if lastError == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf(
				"database unavailable after %s: %w",
				timeout,
				lastError,
			)
		}

		logger.Print(
			"database not ready; retrying",
		)

		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-time.After(2 * time.Second):
		}
	}
}

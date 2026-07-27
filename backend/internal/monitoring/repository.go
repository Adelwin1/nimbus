package monitoring

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrApplicationNotFound = errors.New("application not found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) FindDueApplications(
	ctx context.Context,
	limit int,
) ([]DueApplication, error) {
	if limit <= 0 {
		limit = 25
	}

	const query = `
		SELECT
			id,
			health_url,
			latency_threshold_ms,
			failure_threshold,
			consecutive_failures
		FROM applications
		WHERE
			last_checked_at IS NULL
			OR (
				last_checked_at
				+ make_interval(
					secs => monitoring_interval_seconds
				)
			) <= NOW()
		ORDER BY
			COALESCE(
				last_checked_at,
				'-infinity'::timestamptz
			) ASC
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("find due applications: %w", err)
	}
	defer rows.Close()

	applications := make([]DueApplication, 0)

	for rows.Next() {
		var application DueApplication

		if err := rows.Scan(
			&application.ID,
			&application.HealthURL,
			&application.LatencyThresholdMS,
			&application.FailureThreshold,
			&application.ConsecutiveFailures,
		); err != nil {
			return nil, fmt.Errorf(
				"scan due application: %w",
				err,
			)
		}

		applications = append(applications, application)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate due applications: %w",
			err,
		)
	}

	return applications, nil
}

func (r *Repository) StoreResult(
	ctx context.Context,
	applicationID uuid.UUID,
	result CheckResult,
) (HealthCheck, ApplicationState, error) {
	transaction, err := r.db.Begin(ctx)
	if err != nil {
		return HealthCheck{}, ApplicationState{},
			fmt.Errorf("begin health-check transaction: %w", err)
	}

	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	var failureThreshold int
	var consecutiveFailures int
	var existingLastHealthyAt *time.Time

	const lockQuery = `
		SELECT
			failure_threshold,
			consecutive_failures,
			last_healthy_at
		FROM applications
		WHERE id = $1
		FOR UPDATE
	`

	err = transaction.QueryRow(
		ctx,
		lockQuery,
		applicationID,
	).Scan(
		&failureThreshold,
		&consecutiveFailures,
		&existingLastHealthyAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return HealthCheck{}, ApplicationState{},
			ErrApplicationNotFound
	}

	if err != nil {
		return HealthCheck{}, ApplicationState{},
			fmt.Errorf("lock application for health check: %w", err)
	}

	status, newFailureCount, markHealthy :=
		evaluateApplicationState(
			result,
			consecutiveFailures,
			failureThreshold,
		)

	const insertQuery = `
		INSERT INTO health_checks (
			application_id,
			status_code,
			latency_ms,
			healthy,
			error_message,
			checked_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING
			id,
			application_id,
			status_code,
			latency_ms,
			healthy,
			error_message,
			checked_at
	`

	check, err := scanHealthCheck(
		transaction.QueryRow(
			ctx,
			insertQuery,
			applicationID,
			result.StatusCode,
			result.LatencyMS,
			result.Healthy,
			result.ErrorMessage,
			result.CheckedAt,
		),
	)
	if err != nil {
		return HealthCheck{}, ApplicationState{},
			fmt.Errorf("store health check: %w", err)
	}

	const updateQuery = `
		UPDATE applications
		SET
			status = $2,
			consecutive_failures = $3,
			last_checked_at = $4,
			last_healthy_at = CASE
				WHEN $5 THEN $4
				ELSE last_healthy_at
			END,
			updated_at = NOW()
		WHERE id = $1
		RETURNING
			status,
			consecutive_failures,
			last_checked_at,
			last_healthy_at
	`

	var state ApplicationState

	err = transaction.QueryRow(
		ctx,
		updateQuery,
		applicationID,
		status,
		newFailureCount,
		result.CheckedAt,
		markHealthy,
	).Scan(
		&state.Status,
		&state.ConsecutiveFailures,
		&state.LastCheckedAt,
		&state.LastHealthyAt,
	)
	if err != nil {
		return HealthCheck{}, ApplicationState{},
			fmt.Errorf("update application health state: %w", err)
	}

	if err := transaction.Commit(ctx); err != nil {
		return HealthCheck{}, ApplicationState{},
			fmt.Errorf("commit health-check transaction: %w", err)
	}

	return check, state, nil
}

func (r *Repository) EnsureOwned(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) error {
	const query = `
		SELECT 1
		FROM applications
		WHERE id = $1
		  AND user_id = $2
	`

	var exists int

	err := r.db.QueryRow(
		ctx,
		query,
		applicationID,
		userID,
	).Scan(&exists)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrApplicationNotFound
	}

	if err != nil {
		return fmt.Errorf("verify application ownership: %w", err)
	}

	return nil
}

func (r *Repository) GetCheckTarget(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) (DueApplication, error) {
	const query = `
		SELECT
			id,
			health_url,
			latency_threshold_ms,
			failure_threshold,
			consecutive_failures
		FROM applications
		WHERE id = $1
		  AND user_id = $2
	`

	var application DueApplication

	err := r.db.QueryRow(
		ctx,
		query,
		applicationID,
		userID,
	).Scan(
		&application.ID,
		&application.HealthURL,
		&application.LatencyThresholdMS,
		&application.FailureThreshold,
		&application.ConsecutiveFailures,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return DueApplication{}, ErrApplicationNotFound
	}

	if err != nil {
		return DueApplication{},
			fmt.Errorf("get health-check target: %w", err)
	}

	return application, nil
}

func (r *Repository) ListHistory(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
	limit int,
	offset int,
) (HistoryPage, error) {
	if err := r.EnsureOwned(
		ctx,
		applicationID,
		userID,
	); err != nil {
		return HistoryPage{}, err
	}

	const countQuery = `
		SELECT COUNT(*)
		FROM health_checks
		WHERE application_id = $1
	`

	var total int64

	if err := r.db.QueryRow(
		ctx,
		countQuery,
		applicationID,
	).Scan(&total); err != nil {
		return HistoryPage{},
			fmt.Errorf("count health checks: %w", err)
	}

	const query = `
		SELECT
			id,
			application_id,
			status_code,
			latency_ms,
			healthy,
			error_message,
			checked_at
		FROM health_checks
		WHERE application_id = $1
		ORDER BY checked_at DESC
		LIMIT $2
		OFFSET $3
	`

	rows, err := r.db.Query(
		ctx,
		query,
		applicationID,
		limit,
		offset,
	)
	if err != nil {
		return HistoryPage{},
			fmt.Errorf("list health checks: %w", err)
	}
	defer rows.Close()

	checks := make([]HealthCheck, 0)

	for rows.Next() {
		check, err := scanHealthCheck(rows)
		if err != nil {
			return HistoryPage{},
				fmt.Errorf("scan health check: %w", err)
		}

		checks = append(checks, check)
	}

	if err := rows.Err(); err != nil {
		return HistoryPage{},
			fmt.Errorf("iterate health checks: %w", err)
	}

	return HistoryPage{
		Checks: checks,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (r *Repository) GetOverview(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) (HealthOverview, error) {
	const stateQuery = `
		SELECT
			status,
			consecutive_failures,
			last_checked_at,
			last_healthy_at
		FROM applications
		WHERE id = $1
		  AND user_id = $2
	`

	var state ApplicationState

	err := r.db.QueryRow(
		ctx,
		stateQuery,
		applicationID,
		userID,
	).Scan(
		&state.Status,
		&state.ConsecutiveFailures,
		&state.LastCheckedAt,
		&state.LastHealthyAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return HealthOverview{}, ErrApplicationNotFound
	}

	if err != nil {
		return HealthOverview{},
			fmt.Errorf("get application health state: %w", err)
	}

	const statisticsQuery = `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE healthy = TRUE),
			COUNT(*) FILTER (WHERE healthy = FALSE),
			COALESCE(
				AVG(latency_ms),
				0
			),
			COALESCE(
				MIN(latency_ms),
				0
			),
			COALESCE(
				MAX(latency_ms),
				0
			)
		FROM health_checks
		WHERE application_id = $1
	`

	var statistics HealthStatistics

	err = r.db.QueryRow(
		ctx,
		statisticsQuery,
		applicationID,
	).Scan(
		&statistics.TotalChecks,
		&statistics.SuccessfulChecks,
		&statistics.FailedChecks,
		&statistics.AverageLatencyMS,
		&statistics.MinimumLatencyMS,
		&statistics.MaximumLatencyMS,
	)
	if err != nil {
		return HealthOverview{},
			fmt.Errorf("calculate health statistics: %w", err)
	}

	if statistics.TotalChecks > 0 {
		statistics.AvailabilityPercent =
			float64(statistics.SuccessfulChecks) /
				float64(statistics.TotalChecks) *
				100
	}

	const latestQuery = `
		SELECT
			id,
			application_id,
			status_code,
			latency_ms,
			healthy,
			error_message,
			checked_at
		FROM health_checks
		WHERE application_id = $1
		ORDER BY checked_at DESC
		LIMIT 1
	`

	latestCheck, err := scanHealthCheck(
		r.db.QueryRow(
			ctx,
			latestQuery,
			applicationID,
		),
	)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		statistics.LatestCheck = nil

	case err != nil:
		return HealthOverview{},
			fmt.Errorf("get latest health check: %w", err)

	default:
		statistics.LatestCheck = &latestCheck
	}

	return HealthOverview{
		State:      state,
		Statistics: statistics,
	}, nil
}

func evaluateApplicationState(
	result CheckResult,
	currentFailures int,
	failureThreshold int,
) (status string, failures int, markHealthy bool) {
	if result.Healthy {
		if result.Slow {
			return "degraded", 0, false
		}

		return "healthy", 0, true
	}

	failures = currentFailures + 1

	if failures >= failureThreshold {
		return "down", failures, false
	}

	return "degraded", failures, false
}

type rowScanner interface {
	Scan(destinations ...any) error
}

func scanHealthCheck(
	scanner rowScanner,
) (HealthCheck, error) {
	var check HealthCheck

	err := scanner.Scan(
		&check.ID,
		&check.ApplicationID,
		&check.StatusCode,
		&check.LatencyMS,
		&check.Healthy,
		&check.ErrorMessage,
		&check.CheckedAt,
	)

	return check, err
}

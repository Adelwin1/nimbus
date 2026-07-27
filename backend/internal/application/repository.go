package application

import (
	"context"
	"errors"
	"fmt"

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

func (r *Repository) Create(
	ctx context.Context,
	userID uuid.UUID,
	application Application,
) (Application, error) {
	const query = `
		INSERT INTO applications (
			user_id,
			name,
			description,
			application_url,
			health_url,
			repository_url,
			environment,
			current_version,
			status,
			monitoring_interval_seconds,
			failure_threshold,
			latency_threshold_ms,
			deployment_webhook_url_encrypted,
			rollback_webhook_url_encrypted,
			webhook_token_encrypted
		)
		VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15
		)
		RETURNING
			id,
			user_id,
			name,
			description,
			application_url,
			health_url,
			repository_url,
			environment,
			current_version,
			status,
			monitoring_interval_seconds,
			failure_threshold,
			latency_threshold_ms,
			consecutive_failures,
			last_checked_at,
			last_healthy_at,
			deployment_webhook_url_encrypted,
			rollback_webhook_url_encrypted,
			webhook_token_encrypted,
			created_at,
			updated_at
	`

	result, err := scanApplication(
		r.db.QueryRow(
			ctx,
			query,
			userID,
			application.Name,
			application.Description,
			application.ApplicationURL,
			application.HealthURL,
			application.RepositoryURL,
			application.Environment,
			application.CurrentVersion,
			application.Status,
			application.MonitoringIntervalSeconds,
			application.FailureThreshold,
			application.LatencyThresholdMS,
			application.DeploymentWebhookURLEncrypted,
			application.RollbackWebhookURLEncrypted,
			application.WebhookTokenEncrypted,
		),
	)
	if err != nil {
		return Application{}, fmt.Errorf("create application: %w", err)
	}

	result.SetSecretFlags()

	return result, nil
}

func (r *Repository) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
) ([]Application, error) {
	const query = `
		SELECT
			id,
			user_id,
			name,
			description,
			application_url,
			health_url,
			repository_url,
			environment,
			current_version,
			status,
			monitoring_interval_seconds,
			failure_threshold,
			latency_threshold_ms,
			consecutive_failures,
			last_checked_at,
			last_healthy_at,
			deployment_webhook_url_encrypted,
			rollback_webhook_url_encrypted,
			webhook_token_encrypted,
			created_at,
			updated_at
		FROM applications
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()

	applications := make([]Application, 0)

	for rows.Next() {
		application, err := scanApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}

		application.SetSecretFlags()
		applications = append(applications, application)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applications: %w", err)
	}

	return applications, nil
}

func (r *Repository) FindByID(
	ctx context.Context,
	appID uuid.UUID,
	userID uuid.UUID,
) (Application, error) {
	const query = `
		SELECT
			id,
			user_id,
			name,
			description,
			application_url,
			health_url,
			repository_url,
			environment,
			current_version,
			status,
			monitoring_interval_seconds,
			failure_threshold,
			latency_threshold_ms,
			consecutive_failures,
			last_checked_at,
			last_healthy_at,
			deployment_webhook_url_encrypted,
			rollback_webhook_url_encrypted,
			webhook_token_encrypted,
			created_at,
			updated_at
		FROM applications
		WHERE id = $1
		  AND user_id = $2
	`

	application, err := scanApplication(
		r.db.QueryRow(ctx, query, appID, userID),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrApplicationNotFound
	}

	if err != nil {
		return Application{}, fmt.Errorf("find application: %w", err)
	}

	application.SetSecretFlags()

	return application, nil
}

func (r *Repository) Update(
	ctx context.Context,
	application Application,
	userID uuid.UUID,
) (Application, error) {
	const query = `
		UPDATE applications
		SET
			name = $3,
			description = $4,
			application_url = $5,
			health_url = $6,
			repository_url = $7,
			environment = $8,
			current_version = $9,
			monitoring_interval_seconds = $10,
			failure_threshold = $11,
			latency_threshold_ms = $12,
			deployment_webhook_url_encrypted = $13,
			rollback_webhook_url_encrypted = $14,
			webhook_token_encrypted = $15,
			updated_at = NOW()
		WHERE id = $1
		  AND user_id = $2
		RETURNING
			id,
			user_id,
			name,
			description,
			application_url,
			health_url,
			repository_url,
			environment,
			current_version,
			status,
			monitoring_interval_seconds,
			failure_threshold,
			latency_threshold_ms,
			consecutive_failures,
			last_checked_at,
			last_healthy_at,
			deployment_webhook_url_encrypted,
			rollback_webhook_url_encrypted,
			webhook_token_encrypted,
			created_at,
			updated_at
	`

	updated, err := scanApplication(
		r.db.QueryRow(
			ctx,
			query,
			application.ID,
			userID,
			application.Name,
			application.Description,
			application.ApplicationURL,
			application.HealthURL,
			application.RepositoryURL,
			application.Environment,
			application.CurrentVersion,
			application.MonitoringIntervalSeconds,
			application.FailureThreshold,
			application.LatencyThresholdMS,
			application.DeploymentWebhookURLEncrypted,
			application.RollbackWebhookURLEncrypted,
			application.WebhookTokenEncrypted,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrApplicationNotFound
	}

	if err != nil {
		return Application{}, fmt.Errorf("update application: %w", err)
	}

	updated.SetSecretFlags()

	return updated, nil
}

func (r *Repository) Delete(
	ctx context.Context,
	appID uuid.UUID,
	userID uuid.UUID,
) error {
	const query = `
		DELETE FROM applications
		WHERE id = $1
		  AND user_id = $2
	`

	commandTag, err := r.db.Exec(ctx, query, appID, userID)
	if err != nil {
		return fmt.Errorf("delete application: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrApplicationNotFound
	}

	return nil
}

func (r *Repository) DashboardSummary(
	ctx context.Context,
	userID uuid.UUID,
) (DashboardSummary, error) {
	const query = `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'healthy'),
			COUNT(*) FILTER (WHERE status = 'degraded'),
			COUNT(*) FILTER (WHERE status = 'down'),
			COUNT(*) FILTER (WHERE status = 'unknown')
		FROM applications
		WHERE user_id = $1
	`

	var total int64
	var healthy int64
	var degraded int64
	var down int64
	var unknown int64

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&total,
		&healthy,
		&degraded,
		&down,
		&unknown,
	)
	if err != nil {
		return DashboardSummary{}, fmt.Errorf(
			"get dashboard summary: %w",
			err,
		)
	}

	return DashboardSummary{
		TotalApplications:    int(total),
		HealthyApplications:  int(healthy),
		DegradedApplications: int(degraded),
		DownApplications:     int(down),
		UnknownApplications:  int(unknown),
	}, nil
}

type rowScanner interface {
	Scan(destinations ...any) error
}

func scanApplication(scanner rowScanner) (Application, error) {
	var application Application

	err := scanner.Scan(
		&application.ID,
		&application.UserID,
		&application.Name,
		&application.Description,
		&application.ApplicationURL,
		&application.HealthURL,
		&application.RepositoryURL,
		&application.Environment,
		&application.CurrentVersion,
		&application.Status,
		&application.MonitoringIntervalSeconds,
		&application.FailureThreshold,
		&application.LatencyThresholdMS,
		&application.ConsecutiveFailures,
		&application.LastCheckedAt,
		&application.LastHealthyAt,
		&application.DeploymentWebhookURLEncrypted,
		&application.RollbackWebhookURLEncrypted,
		&application.WebhookTokenEncrypted,
		&application.CreatedAt,
		&application.UpdatedAt,
	)

	return application, err
}

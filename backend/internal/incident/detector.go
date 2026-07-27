package incident

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultDetectorInterval = 5 * time.Second
	defaultDetectionBatch   = 100
)

type Detector struct {
	db       *pgxpool.Pool
	service  *Service
	logger   *slog.Logger
	interval time.Duration
	batch    int
}

type storedHealthObservation struct {
	ID            uuid.UUID
	ApplicationID uuid.UUID
	StatusCode    *int
	LatencyMS     int64
	Healthy       bool
	ErrorMessage  *string

	ConsecutiveFailures int
	FailureThreshold    int
	LatencyThresholdMS  int
}

type failedDeploymentObservation struct {
	ApplicationID  uuid.UUID
	DeploymentID   uuid.UUID
	Version        string
	FailureMessage string
	Metadata       json.RawMessage
}

func NewDetector(
	db *pgxpool.Pool,
	service *Service,
	logger *slog.Logger,
) *Detector {
	return &Detector{
		db:       db,
		service:  service,
		logger:   logger,
		interval: defaultDetectorInterval,
		batch:    defaultDetectionBatch,
	}
}

func (d *Detector) Run(ctx context.Context) {
	if d.logger != nil {
		d.logger.Info(
			"incident detector started",
			"interval",
			d.interval.String(),
		)
	}

	d.runCycle(ctx)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if d.logger != nil {
				d.logger.Info(
					"incident detector stopped",
				)
			}

			return

		case <-ticker.C:
			d.runCycle(ctx)
		}
	}
}

func (d *Detector) runCycle(ctx context.Context) {
	if err := d.processHealthChecks(ctx); err != nil {
		if d.logger != nil {
			d.logger.Error(
				"incident health detection failed",
				"error",
				err,
			)
		}
	}

	if err := d.processDeploymentFailures(ctx); err != nil {
		if d.logger != nil {
			d.logger.Error(
				"incident deployment detection failed",
				"error",
				err,
			)
		}
	}
}

func (d *Detector) processHealthChecks(
	ctx context.Context,
) error {
	observations, err := d.loadUnevaluatedHealthChecks(
		ctx,
	)
	if err != nil {
		return err
	}

	for _, observation := range observations {
		applicationStatus := determineApplicationStatus(
			observation,
		)

		err := d.service.ObserveHealthResult(
			ctx,
			HealthObservation{
				ApplicationID: observation.ApplicationID,
				HealthCheckID: observation.ID,

				StatusCode:   observation.StatusCode,
				LatencyMS:    observation.LatencyMS,
				Healthy:      observation.Healthy,
				ErrorMessage: observation.ErrorMessage,

				ApplicationStatus:   applicationStatus,
				ConsecutiveFailures: observation.ConsecutiveFailures,
				FailureThreshold:    observation.FailureThreshold,
				LatencyThresholdMS:  observation.LatencyThresholdMS,
			},
		)
		if err != nil {
			if d.logger != nil {
				d.logger.Error(
					"evaluate health check for incidents",
					"health_check_id",
					observation.ID,
					"application_id",
					observation.ApplicationID,
					"error",
					err,
				)
			}

			continue
		}

		if err := d.markHealthCheckEvaluated(
			ctx,
			observation.ID,
		); err != nil {
			return err
		}
	}

	return nil
}

func (d *Detector) loadUnevaluatedHealthChecks(
	ctx context.Context,
) ([]storedHealthObservation, error) {
	const query = `
		WITH ordered_checks AS (
			SELECT
				health_checks.id,
				health_checks.application_id,
				health_checks.status_code,
				health_checks.latency_ms,
				health_checks.healthy,
				health_checks.error_message,
				health_checks.checked_at,
				health_checks.incident_evaluated_at,

				SUM(
					CASE
						WHEN health_checks.healthy THEN 1
						ELSE 0
					END
				) OVER (
					PARTITION BY
						health_checks.application_id
					ORDER BY
						health_checks.checked_at,
						health_checks.id
					ROWS BETWEEN
						UNBOUNDED PRECEDING
						AND CURRENT ROW
				) AS healthy_group
			FROM health_checks
		),

		check_streaks AS (
			SELECT
				ordered_checks.*,

				CASE
					WHEN ordered_checks.healthy THEN 0
					ELSE COUNT(*) FILTER (
						WHERE NOT ordered_checks.healthy
					) OVER (
						PARTITION BY
							ordered_checks.application_id,
							ordered_checks.healthy_group
						ORDER BY
							ordered_checks.checked_at,
							ordered_checks.id
						ROWS BETWEEN
							UNBOUNDED PRECEDING
							AND CURRENT ROW
					)
				END AS consecutive_failures
			FROM ordered_checks
		)

		SELECT
			check_streaks.id,
			check_streaks.application_id,
			check_streaks.status_code,
			check_streaks.latency_ms,
			check_streaks.healthy,
			check_streaks.error_message,
			check_streaks.consecutive_failures,
			applications.failure_threshold,
			applications.latency_threshold_ms
		FROM check_streaks
		INNER JOIN applications
			ON applications.id =
				check_streaks.application_id
		WHERE check_streaks.incident_evaluated_at
			IS NULL
		ORDER BY
			check_streaks.checked_at ASC,
			check_streaks.id ASC
		LIMIT $1
	`

	rows, err := d.db.Query(
		ctx,
		query,
		d.batch,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"load unevaluated health checks: %w",
			err,
		)
	}
	defer rows.Close()

	observations := make(
		[]storedHealthObservation,
		0,
	)

	for rows.Next() {
		var observation storedHealthObservation
		var consecutiveFailures int64

		if err := rows.Scan(
			&observation.ID,
			&observation.ApplicationID,
			&observation.StatusCode,
			&observation.LatencyMS,
			&observation.Healthy,
			&observation.ErrorMessage,
			&consecutiveFailures,
			&observation.FailureThreshold,
			&observation.LatencyThresholdMS,
		); err != nil {
			return nil, fmt.Errorf(
				"scan health incident observation: %w",
				err,
			)
		}

		observation.ConsecutiveFailures =
			int(consecutiveFailures)

		observations = append(
			observations,
			observation,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate health incident observations: %w",
			err,
		)
	}

	return observations, nil
}

func (d *Detector) markHealthCheckEvaluated(
	ctx context.Context,
	healthCheckID uuid.UUID,
) error {
	const query = `
		UPDATE health_checks
		SET incident_evaluated_at = NOW()
		WHERE id = $1
		  AND incident_evaluated_at IS NULL
	`

	if _, err := d.db.Exec(
		ctx,
		query,
		healthCheckID,
	); err != nil {
		return fmt.Errorf(
			"mark health check evaluated: %w",
			err,
		)
	}

	return nil
}

func (d *Detector) processDeploymentFailures(
	ctx context.Context,
) error {
	observations, err :=
		d.loadFailedDeploymentVerifications(ctx)
	if err != nil {
		return err
	}

	for _, observation := range observations {
		var metadata any = map[string]any{}

		if len(observation.Metadata) > 0 {
			if err := json.Unmarshal(
				observation.Metadata,
				&metadata,
			); err != nil {
				metadata = map[string]any{
					"raw_metadata": string(observation.Metadata),
				}
			}
		}

		err := d.service.OpenDeploymentFailure(
			ctx,
			DeploymentFailureObservation{
				ApplicationID:  observation.ApplicationID,
				DeploymentID:   observation.DeploymentID,
				Version:        observation.Version,
				FailureMessage: observation.FailureMessage,
				Metadata:       metadata,
			},
		)
		if err != nil && d.logger != nil {
			d.logger.Error(
				"open failed deployment incident",
				"deployment_id",
				observation.DeploymentID,
				"application_id",
				observation.ApplicationID,
				"error",
				err,
			)
		}
	}

	return nil
}

func (d *Detector) loadFailedDeploymentVerifications(
	ctx context.Context,
) ([]failedDeploymentObservation, error) {
	const query = `
		SELECT
			deployments.application_id,
			deployments.id,
			deployments.version,
			verification_event.message,
			verification_event.metadata
		FROM deployments

		INNER JOIN LATERAL (
			SELECT
				deployment_events.message,
				deployment_events.metadata
			FROM deployment_events
			WHERE deployment_events.deployment_id =
				deployments.id
			  AND deployment_events.event_type =
				'verification_failed'
			ORDER BY
				deployment_events.created_at DESC,
				deployment_events.id DESC
			LIMIT 1
		) AS verification_event
			ON TRUE

		WHERE deployments.status = 'failed'

		  AND NOT EXISTS (
				SELECT 1
				FROM incidents
				WHERE incidents.source_deployment_id =
					deployments.id
				  AND incidents.incident_type =
					'deployment_failure'
		  )

		ORDER BY deployments.completed_at ASC
		LIMIT $1
	`

	rows, err := d.db.Query(
		ctx,
		query,
		d.batch,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"load failed deployment verifications: %w",
			err,
		)
	}
	defer rows.Close()

	observations := make(
		[]failedDeploymentObservation,
		0,
	)

	for rows.Next() {
		var observation failedDeploymentObservation

		if err := rows.Scan(
			&observation.ApplicationID,
			&observation.DeploymentID,
			&observation.Version,
			&observation.FailureMessage,
			&observation.Metadata,
		); err != nil {
			return nil, fmt.Errorf(
				"scan deployment failure observation: %w",
				err,
			)
		}

		observations = append(
			observations,
			observation,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate deployment failure observations: %w",
			err,
		)
	}

	return observations, nil
}

func determineApplicationStatus(
	observation storedHealthObservation,
) string {
	if !observation.Healthy {
		threshold := observation.FailureThreshold

		if threshold <= 0 {
			threshold = defaultFailureThreshold
		}

		if observation.ConsecutiveFailures >= threshold {
			return "down"
		}

		return "degraded"
	}

	if observation.LatencyThresholdMS > 0 &&
		observation.LatencyMS >
			int64(observation.LatencyThresholdMS) {
		return "degraded"
	}

	return "healthy"
}

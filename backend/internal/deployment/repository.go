package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDeploymentNotFound     = errors.New("deployment not found")
	ErrApplicationNotFound    = errors.New("application not found")
	ErrActiveDeployment       = errors.New("application already has an active deployment")
	ErrNoSuccessfulDeployment = errors.New("no successful deployment found")
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
	request CreateRequest,
) (Deployment, error) {
	transaction, err := r.db.Begin(ctx)
	if err != nil {
		return Deployment{}, fmt.Errorf(
			"begin deployment transaction: %w",
			err,
		)
	}

	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	const query = `
		INSERT INTO deployments (
			application_id,
			version,
			commit_sha,
			release_notes,
			deployment_type,
			status,
			previous_version,
			triggered_by
		)
		SELECT
			applications.id,
			$3,
			$4,
			$5,
			'deployment',
			'pending',
			applications.current_version,
			$2
		FROM applications
		WHERE applications.id = $1
		  AND applications.user_id = $2
		RETURNING
			id,
			application_id,
			version,
			commit_sha,
			release_notes,
			deployment_type,
			status,
			previous_version,
			started_at,
			completed_at,
			triggered_by,
			created_at,
			updated_at
	`

	deployment, err := scanDeployment(
		transaction.QueryRow(
			ctx,
			query,
			applicationID,
			userID,
			request.Version,
			request.CommitSHA,
			request.ReleaseNotes,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, ErrApplicationNotFound
	}

	if err != nil {
		if isUniqueViolation(err) {
			return Deployment{}, ErrActiveDeployment
		}

		return Deployment{}, fmt.Errorf(
			"create deployment: %w",
			err,
		)
	}

	_, err = insertEvent(
		ctx,
		transaction,
		deployment.ID,
		"deployment_created",
		fmt.Sprintf(
			"Deployment %s was created.",
			deployment.Version,
		),
		map[string]any{
			"version":          deployment.Version,
			"previous_version": deployment.PreviousVersion,
		},
	)
	if err != nil {
		return Deployment{}, err
	}

	if err := transaction.Commit(ctx); err != nil {
		return Deployment{}, fmt.Errorf(
			"commit deployment transaction: %w",
			err,
		)
	}

	return deployment, nil
}

func (r *Repository) ListByApplication(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) ([]Deployment, error) {
	if err := r.EnsureApplicationOwned(
		ctx,
		applicationID,
		userID,
	); err != nil {
		return nil, err
	}

	const query = `
		SELECT
			id,
			application_id,
			version,
			commit_sha,
			release_notes,
			deployment_type,
			status,
			previous_version,
			started_at,
			completed_at,
			triggered_by,
			created_at,
			updated_at
		FROM deployments
		WHERE application_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		applicationID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list deployments: %w",
			err,
		)
	}
	defer rows.Close()

	deployments := make([]Deployment, 0)

	for rows.Next() {
		deployment, err := scanDeployment(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"scan deployment: %w",
				err,
			)
		}

		deployments = append(deployments, deployment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate deployments: %w",
			err,
		)
	}

	return deployments, nil
}

func (r *Repository) FindByID(
	ctx context.Context,
	deploymentID uuid.UUID,
	userID uuid.UUID,
) (Deployment, error) {
	const query = `
		SELECT
			deployments.id,
			deployments.application_id,
			deployments.version,
			deployments.commit_sha,
			deployments.release_notes,
			deployments.deployment_type,
			deployments.status,
			deployments.previous_version,
			deployments.started_at,
			deployments.completed_at,
			deployments.triggered_by,
			deployments.created_at,
			deployments.updated_at
		FROM deployments
		INNER JOIN applications
			ON applications.id = deployments.application_id
		WHERE deployments.id = $1
		  AND applications.user_id = $2
	`

	deployment, err := scanDeployment(
		r.db.QueryRow(
			ctx,
			query,
			deploymentID,
			userID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, ErrDeploymentNotFound
	}

	if err != nil {
		return Deployment{}, fmt.Errorf(
			"find deployment: %w",
			err,
		)
	}

	return deployment, nil
}

func (r *Repository) FindInternal(
	ctx context.Context,
	deploymentID uuid.UUID,
) (Deployment, error) {
	const query = `
		SELECT
			id,
			application_id,
			version,
			commit_sha,
			release_notes,
			deployment_type,
			status,
			previous_version,
			started_at,
			completed_at,
			triggered_by,
			created_at,
			updated_at
		FROM deployments
		WHERE id = $1
	`

	deployment, err := scanDeployment(
		r.db.QueryRow(
			ctx,
			query,
			deploymentID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, ErrDeploymentNotFound
	}

	if err != nil {
		return Deployment{}, fmt.Errorf(
			"find internal deployment: %w",
			err,
		)
	}

	return deployment, nil
}

func (r *Repository) ListEvents(
	ctx context.Context,
	deploymentID uuid.UUID,
	userID uuid.UUID,
) ([]DeploymentEvent, error) {
	const query = `
		SELECT
			deployment_events.id,
			deployment_events.deployment_id,
			deployment_events.event_type,
			deployment_events.message,
			deployment_events.metadata,
			deployment_events.created_at
		FROM deployment_events
		INNER JOIN deployments
			ON deployments.id = deployment_events.deployment_id
		INNER JOIN applications
			ON applications.id = deployments.application_id
		WHERE deployment_events.deployment_id = $1
		  AND applications.user_id = $2
		ORDER BY deployment_events.created_at ASC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		deploymentID,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list deployment events: %w",
			err,
		)
	}
	defer rows.Close()

	events := make([]DeploymentEvent, 0)

	for rows.Next() {
		event, err := scanDeploymentEvent(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"scan deployment event: %w",
				err,
			)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate deployment events: %w",
			err,
		)
	}

	if len(events) == 0 {
		if _, err := r.FindByID(
			ctx,
			deploymentID,
			userID,
		); err != nil {
			return nil, err
		}
	}

	return events, nil
}

func (r *Repository) GetApplicationConfig(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) (ApplicationDeploymentConfig, error) {
	const query = `
		SELECT
			id,
			user_id,
			current_version,
			health_url,
			latency_threshold_ms,
			deployment_webhook_url_encrypted,
			webhook_token_encrypted
		FROM applications
		WHERE id = $1
		  AND user_id = $2
	`

	var config ApplicationDeploymentConfig

	err := r.db.QueryRow(
		ctx,
		query,
		applicationID,
		userID,
	).Scan(
		&config.ID,
		&config.UserID,
		&config.CurrentVersion,
		&config.HealthURL,
		&config.LatencyThresholdMS,
		&config.DeploymentWebhookURLEncrypted,
		&config.WebhookTokenEncrypted,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return ApplicationDeploymentConfig{},
			ErrApplicationNotFound
	}

	if err != nil {
		return ApplicationDeploymentConfig{},
			fmt.Errorf(
				"get application deployment config: %w",
				err,
			)
	}

	return config, nil
}

func (r *Repository) EnsureApplicationOwned(
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
		return fmt.Errorf(
			"verify application ownership: %w",
			err,
		)
	}

	return nil
}

func (r *Repository) AddEvent(
	ctx context.Context,
	deploymentID uuid.UUID,
	eventType string,
	message string,
	metadata any,
) (DeploymentEvent, error) {
	return insertEvent(
		ctx,
		r.db,
		deploymentID,
		eventType,
		message,
		metadata,
	)
}

func (r *Repository) UpdateStatus(
	ctx context.Context,
	deploymentID uuid.UUID,
	status string,
	eventType string,
	message string,
	metadata any,
	markStarted bool,
	markCompleted bool,
) (Deployment, DeploymentEvent, error) {
	transaction, err := r.db.Begin(ctx)
	if err != nil {
		return Deployment{}, DeploymentEvent{},
			fmt.Errorf(
				"begin deployment status transaction: %w",
				err,
			)
	}

	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	const query = `
		UPDATE deployments
		SET
			status = $2,
			started_at = CASE
				WHEN $3 AND started_at IS NULL THEN NOW()
				ELSE started_at
			END,
			completed_at = CASE
				WHEN $4 THEN NOW()
				ELSE completed_at
			END,
			updated_at = NOW()
		WHERE id = $1
		RETURNING
			id,
			application_id,
			version,
			commit_sha,
			release_notes,
			deployment_type,
			status,
			previous_version,
			started_at,
			completed_at,
			triggered_by,
			created_at,
			updated_at
	`

	deployment, err := scanDeployment(
		transaction.QueryRow(
			ctx,
			query,
			deploymentID,
			status,
			markStarted,
			markCompleted,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, DeploymentEvent{},
			ErrDeploymentNotFound
	}

	if err != nil {
		return Deployment{}, DeploymentEvent{},
			fmt.Errorf(
				"update deployment status: %w",
				err,
			)
	}

	event, err := insertEvent(
		ctx,
		transaction,
		deploymentID,
		eventType,
		message,
		metadata,
	)
	if err != nil {
		return Deployment{}, DeploymentEvent{}, err
	}

	if err := transaction.Commit(ctx); err != nil {
		return Deployment{}, DeploymentEvent{},
			fmt.Errorf(
				"commit deployment status transaction: %w",
				err,
			)
	}

	return deployment, event, nil
}

func (r *Repository) UpdateApplicationVersion(
	ctx context.Context,
	applicationID uuid.UUID,
	version string,
) error {
	const query = `
		UPDATE applications
		SET
			current_version = $2,
			status = 'healthy',
			updated_at = NOW()
		WHERE id = $1
	`

	commandTag, err := r.db.Exec(
		ctx,
		query,
		applicationID,
		version,
	)
	if err != nil {
		return fmt.Errorf(
			"update application version: %w",
			err,
		)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrApplicationNotFound
	}

	return nil
}

func (r *Repository) ListRecoverable(
	ctx context.Context,
	limit int,
) ([]Deployment, error) {
	if limit <= 0 {
		limit = 25
	}

	const query = `
		SELECT
			id,
			application_id,
			version,
			commit_sha,
			release_notes,
			deployment_type,
			status,
			previous_version,
			started_at,
			completed_at,
			triggered_by,
			created_at,
			updated_at
		FROM deployments
		WHERE status IN (
			'pending',
			'triggering',
			'verifying'
		)
		  AND deployment_type = 'deployment'
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf(
			"list recoverable deployments: %w",
			err,
		)
	}
	defer rows.Close()

	deployments := make([]Deployment, 0)

	for rows.Next() {
		deployment, err := scanDeployment(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"scan recoverable deployment: %w",
				err,
			)
		}

		deployments = append(deployments, deployment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate recoverable deployments: %w",
			err,
		)
	}

	return deployments, nil
}

type rowScanner interface {
	Scan(destinations ...any) error
}

type eventQuerier interface {
	QueryRow(
		ctx context.Context,
		sql string,
		args ...any,
	) pgx.Row
}

func scanDeployment(
	scanner rowScanner,
) (Deployment, error) {
	var deployment Deployment

	err := scanner.Scan(
		&deployment.ID,
		&deployment.ApplicationID,
		&deployment.Version,
		&deployment.CommitSHA,
		&deployment.ReleaseNotes,
		&deployment.DeploymentType,
		&deployment.Status,
		&deployment.PreviousVersion,
		&deployment.StartedAt,
		&deployment.CompletedAt,
		&deployment.TriggeredBy,
		&deployment.CreatedAt,
		&deployment.UpdatedAt,
	)

	return deployment, err
}

func scanDeploymentEvent(
	scanner rowScanner,
) (DeploymentEvent, error) {
	var event DeploymentEvent

	err := scanner.Scan(
		&event.ID,
		&event.DeploymentID,
		&event.EventType,
		&event.Message,
		&event.Metadata,
		&event.CreatedAt,
	)

	return event, err
}

func insertEvent(
	ctx context.Context,
	querier eventQuerier,
	deploymentID uuid.UUID,
	eventType string,
	message string,
	metadata any,
) (DeploymentEvent, error) {
	encodedMetadata, err := json.Marshal(metadata)
	if err != nil {
		return DeploymentEvent{}, fmt.Errorf(
			"encode deployment event metadata: %w",
			err,
		)
	}

	if metadata == nil {
		encodedMetadata = []byte("{}")
	}

	const query = `
		INSERT INTO deployment_events (
			deployment_id,
			event_type,
			message,
			metadata
		)
		VALUES ($1, $2, $3, $4)
		RETURNING
			id,
			deployment_id,
			event_type,
			message,
			metadata,
			created_at
	`

	event, err := scanDeploymentEvent(
		querier.QueryRow(
			ctx,
			query,
			deploymentID,
			eventType,
			message,
			encodedMetadata,
		),
	)
	if err != nil {
		return DeploymentEvent{}, fmt.Errorf(
			"insert deployment event: %w",
			err,
		)
	}

	return event, nil
}

func isUniqueViolation(err error) bool {
	type sqlStateError interface {
		SQLState() string
	}

	var databaseError sqlStateError

	if errors.As(err, &databaseError) {
		return databaseError.SQLState() == "23505"
	}

	return false
}

func (r *Repository) HasEvent(
	ctx context.Context,
	deploymentID uuid.UUID,
	eventType string,
) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM deployment_events
			WHERE deployment_id = $1
			  AND event_type = $2
		)
	`

	var exists bool

	if err := r.db.QueryRow(
		ctx,
		query,
		deploymentID,
		eventType,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf(
			"check deployment event existence: %w",
			err,
		)
	}

	return exists, nil
}

func (r *Repository) UpdateApplicationStatus(
	ctx context.Context,
	applicationID uuid.UUID,
	status string,
) error {
	const query = `
		UPDATE applications
		SET
			status = $2,
			updated_at = NOW()
		WHERE id = $1
	`

	commandTag, err := r.db.Exec(
		ctx,
		query,
		applicationID,
		status,
	)
	if err != nil {
		return fmt.Errorf(
			"update application status: %w",
			err,
		)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrApplicationNotFound
	}

	return nil
}

func (r *Repository) FindLatestSuccessful(
	ctx context.Context,
	applicationID uuid.UUID,
) (Deployment, error) {
	const query = `
		SELECT
			id,
			application_id,
			version,
			commit_sha,
			release_notes,
			deployment_type,
			status,
			previous_version,
			started_at,
			completed_at,
			triggered_by,
			created_at,
			updated_at
		FROM deployments
		WHERE application_id = $1
		  AND status = 'successful'
		ORDER BY
			completed_at DESC NULLS LAST,
			created_at DESC
		LIMIT 1
	`

	result, err := scanDeployment(
		r.db.QueryRow(
			ctx,
			query,
			applicationID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{},
			ErrNoSuccessfulDeployment
	}

	if err != nil {
		return Deployment{}, fmt.Errorf(
			"find latest successful deployment: %w",
			err,
		)
	}

	return result, nil
}

func (r *Repository) CreateRollback(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
	target Deployment,
	incidentID uuid.UUID,
) (Deployment, error) {
	transaction, err := r.db.Begin(ctx)
	if err != nil {
		return Deployment{}, fmt.Errorf(
			"begin rollback transaction: %w",
			err,
		)
	}

	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	const query = `
		INSERT INTO deployments (
			application_id,
			version,
			commit_sha,
			release_notes,
			deployment_type,
			status,
			previous_version,
			triggered_by
		)
		SELECT
			applications.id,
			$3,
			$4,
			$5,
			'rollback',
			'pending',
			applications.current_version,
			$2
		FROM applications
		WHERE applications.id = $1
		  AND applications.user_id = $2
		RETURNING
			id,
			application_id,
			version,
			commit_sha,
			release_notes,
			deployment_type,
			status,
			previous_version,
			started_at,
			completed_at,
			triggered_by,
			created_at,
			updated_at
	`

	rollback, err := scanDeployment(
		transaction.QueryRow(
			ctx,
			query,
			applicationID,
			userID,
			target.Version,
			target.CommitSHA,
			fmt.Sprintf(
				"Rollback initiated for incident %s. Restoring verified version %s.",
				incidentID,
				target.Version,
			),
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Deployment{}, ErrApplicationNotFound
	}

	if err != nil {
		if isUniqueViolation(err) {
			return Deployment{}, ErrActiveDeployment
		}

		return Deployment{}, fmt.Errorf(
			"create rollback deployment: %w",
			err,
		)
	}

	_, err = insertEvent(
		ctx,
		transaction,
		rollback.ID,
		"rollback_created",
		fmt.Sprintf(
			"Rollback to version %s was created.",
			target.Version,
		),
		map[string]any{
			"incident_id":           incidentID,
			"target_deployment_id":  target.ID,
			"target_version":        target.Version,
			"rollback_from_version": rollback.PreviousVersion,
		},
	)
	if err != nil {
		return Deployment{}, err
	}

	if err := transaction.Commit(ctx); err != nil {
		return Deployment{}, fmt.Errorf(
			"commit rollback transaction: %w",
			err,
		)
	}

	return rollback, nil
}

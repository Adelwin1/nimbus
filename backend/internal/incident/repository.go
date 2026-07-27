package incident

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/adel/nimbus/backend/internal/activity"
"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrIncidentNotFound        = errors.New("incident not found")
	ErrIncidentAlreadyResolved = errors.New("incident is already resolved")
	ErrInvalidIncidentState    = errors.New("invalid incident state")
	ErrRollbackAlreadyLinked   = errors.New("rollback deployment is already linked")
)

type Repository struct {
	db       *pgxpool.Pool
	activity activity.Recorder
}

func NewRepository(
	db *pgxpool.Pool,
	recorders ...activity.Recorder,
) *Repository {
	var recorder activity.Recorder

	if len(recorders) > 0 {
		recorder = recorders[0]
	}

	return &Repository{
		db:       db,
		activity: recorder,
	}
}

// OpenOrGet creates a new incident or returns the existing active
// incident with the same application and deduplication key.
//
// Repeated failures update the existing incident and append another
// timeline event instead of creating duplicate incidents.
func (r *Repository) OpenOrGet(
	ctx context.Context,
	input OpenInput,
) (Incident, IncidentEvent, bool, error) {
	transaction, err := r.db.Begin(ctx)
	if err != nil {
		return Incident{}, IncidentEvent{}, false,
			fmt.Errorf(
				"begin incident transaction: %w",
				err,
			)
	}

	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	const insertQuery = `
		INSERT INTO incidents (
			application_id,
			incident_type,
			title,
			summary,
			severity,
			status,
			dedup_key,
			source_health_check_id,
			source_deployment_id
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			'open',
			$6,
			$7,
			$8
		)
		ON CONFLICT (
			application_id,
			dedup_key
		)
		WHERE status IN (
			'open',
			'acknowledged'
		)
		DO NOTHING
		RETURNING
			id,
			application_id,
			incident_type,
			title,
			summary,
			severity,
			status,
			dedup_key,
			source_health_check_id,
			source_deployment_id,
			rollback_deployment_id,
			acknowledged_at,
			acknowledged_by,
			resolved_at,
			resolved_by,
			created_at,
			updated_at
	`

	incident, err := scanIncident(
		transaction.QueryRow(
			ctx,
			insertQuery,
			input.ApplicationID,
			input.IncidentType,
			input.Title,
			input.Summary,
			input.Severity,
			input.DedupKey,
			input.SourceHealthCheckID,
			input.SourceDeploymentID,
		),
	)

	created := true

	if errors.Is(err, pgx.ErrNoRows) {
		created = false

		incident, err = r.updateExistingActive(
			ctx,
			transaction,
			input,
		)
	}

	if err != nil {
		if input.SourceDeploymentID != nil &&
			isUniqueViolation(err) {
			created = false

			incident, err = r.findBySourceDeployment(
				ctx,
				transaction,
				*input.SourceDeploymentID,
			)
		}
	}

	if err != nil {
		return Incident{}, IncidentEvent{}, false,
			fmt.Errorf("open incident: %w", err)
	}

	eventType := "incident_opened"
	eventMessage := fmt.Sprintf(
		"Incident opened: %s",
		incident.Title,
	)

	if !created {
		eventType = "incident_observed_again"
		eventMessage = fmt.Sprintf(
			"Incident condition was observed again: %s",
			incident.Title,
		)
	}

	event, err := insertEvent(
		ctx,
		transaction,
		incident.ID,
		eventType,
		eventMessage,
		nil,
		input.SourceHealthCheckID,
		input.SourceDeploymentID,
		input.Metadata,
	)
	if err != nil {
		return Incident{}, IncidentEvent{}, false, err
	}

	if err := transaction.Commit(ctx); err != nil {
		return Incident{}, IncidentEvent{}, false,
			fmt.Errorf(
				"commit incident transaction: %w",
				err,
			)
	}

	if created {
		r.recordIncidentOpened(
			incident,
			input,
		)
	}

	return incident, event, created, nil
}

func (r *Repository) recordIncidentOpened(
	incident Incident,
	input OpenInput,
) {
	if r.activity == nil {
		return
	}

	applicationID := incident.ApplicationID
	incidentID := incident.ID

	r.activity.Record(activity.RecordInput{
		ApplicationID: &applicationID,
		Action:        activity.ActionIncidentOpened,
		EntityType:    activity.EntityIncident,
		EntityID:      &incidentID,
		Summary: fmt.Sprintf(
			"Incident opened: %s.",
			incident.Title,
		),
		Metadata: map[string]any{
			"incident_type": incident.IncidentType,
			"severity":      incident.Severity,
			"dedup_key":     incident.DedupKey,
			"source_health_check_id":
				input.SourceHealthCheckID,
			"source_deployment_id":
				input.SourceDeploymentID,
		},
	})
}

func (r *Repository) ListForUser(
	ctx context.Context,
	userID uuid.UUID,
) ([]IncidentListItem, error) {
	const query = `
		SELECT
			incidents.id,
			incidents.application_id,
			incidents.incident_type,
			incidents.title,
			incidents.summary,
			incidents.severity,
			incidents.status,
			incidents.dedup_key,
			incidents.source_health_check_id,
			incidents.source_deployment_id,
			incidents.rollback_deployment_id,
			incidents.acknowledged_at,
			incidents.acknowledged_by,
			incidents.resolved_at,
			incidents.resolved_by,
			incidents.created_at,
			incidents.updated_at,
			applications.name
		FROM incidents
		INNER JOIN applications
			ON applications.id = incidents.application_id
		WHERE applications.user_id = $1
		ORDER BY
			CASE incidents.status
				WHEN 'open' THEN 1
				WHEN 'acknowledged' THEN 2
				ELSE 3
			END,
			incidents.created_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf(
			"list user incidents: %w",
			err,
		)
	}
	defer rows.Close()

	items := make([]IncidentListItem, 0)

	for rows.Next() {
		item, err := scanIncidentListItem(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"scan incident list item: %w",
				err,
			)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate user incidents: %w",
			err,
		)
	}

	return items, nil
}

func (r *Repository) ListByApplication(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) ([]IncidentListItem, error) {
	if err := r.ensureApplicationOwned(
		ctx,
		applicationID,
		userID,
	); err != nil {
		return nil, err
	}

	const query = `
		SELECT
			incidents.id,
			incidents.application_id,
			incidents.incident_type,
			incidents.title,
			incidents.summary,
			incidents.severity,
			incidents.status,
			incidents.dedup_key,
			incidents.source_health_check_id,
			incidents.source_deployment_id,
			incidents.rollback_deployment_id,
			incidents.acknowledged_at,
			incidents.acknowledged_by,
			incidents.resolved_at,
			incidents.resolved_by,
			incidents.created_at,
			incidents.updated_at,
			applications.name
		FROM incidents
		INNER JOIN applications
			ON applications.id = incidents.application_id
		WHERE incidents.application_id = $1
		  AND applications.user_id = $2
		ORDER BY incidents.created_at DESC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		applicationID,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list application incidents: %w",
			err,
		)
	}
	defer rows.Close()

	items := make([]IncidentListItem, 0)

	for rows.Next() {
		item, err := scanIncidentListItem(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"scan application incident: %w",
				err,
			)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate application incidents: %w",
			err,
		)
	}

	return items, nil
}

func (r *Repository) FindByID(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (Incident, error) {
	return r.findOwned(
		ctx,
		r.db,
		incidentID,
		userID,
	)
}

func (r *Repository) GetDetail(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (IncidentDetail, error) {
	incident, err := r.FindByID(
		ctx,
		incidentID,
		userID,
	)
	if err != nil {
		return IncidentDetail{}, err
	}

	events, err := r.ListEvents(
		ctx,
		incidentID,
		userID,
	)
	if err != nil {
		return IncidentDetail{}, err
	}

	return IncidentDetail{
		Incident: incident,
		Events:   events,
	}, nil
}

func (r *Repository) ListEvents(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) ([]IncidentEvent, error) {
	if _, err := r.FindByID(
		ctx,
		incidentID,
		userID,
	); err != nil {
		return nil, err
	}

	const query = `
		SELECT
			id,
			incident_id,
			event_type,
			message,
			actor_user_id,
			health_check_id,
			deployment_id,
			metadata,
			created_at
		FROM incident_events
		WHERE incident_id = $1
		ORDER BY created_at ASC, id ASC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		incidentID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list incident events: %w",
			err,
		)
	}
	defer rows.Close()

	events := make([]IncidentEvent, 0)

	for rows.Next() {
		event, err := scanIncidentEvent(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"scan incident event: %w",
				err,
			)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate incident events: %w",
			err,
		)
	}

	return events, nil
}

func (r *Repository) Acknowledge(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (Incident, *IncidentEvent, error) {
	transaction, err := r.db.Begin(ctx)
	if err != nil {
		return Incident{}, nil, fmt.Errorf(
			"begin acknowledgement transaction: %w",
			err,
		)
	}

	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	const query = `
		UPDATE incidents
		SET
			status = 'acknowledged',
			acknowledged_at = NOW(),
			acknowledged_by = $2,
			updated_at = NOW()
		FROM applications
		WHERE incidents.id = $1
		  AND applications.id = incidents.application_id
		  AND applications.user_id = $2
		  AND incidents.status = 'open'
		RETURNING
			incidents.id,
			incidents.application_id,
			incidents.incident_type,
			incidents.title,
			incidents.summary,
			incidents.severity,
			incidents.status,
			incidents.dedup_key,
			incidents.source_health_check_id,
			incidents.source_deployment_id,
			incidents.rollback_deployment_id,
			incidents.acknowledged_at,
			incidents.acknowledged_by,
			incidents.resolved_at,
			incidents.resolved_by,
			incidents.created_at,
			incidents.updated_at
	`

	incident, err := scanIncident(
		transaction.QueryRow(
			ctx,
			query,
			incidentID,
			userID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		existing, lookupErr := r.findOwned(
			ctx,
			transaction,
			incidentID,
			userID,
		)
		if lookupErr != nil {
			return Incident{}, nil, lookupErr
		}

		switch existing.Status {
		case StatusAcknowledged:
			return existing, nil, nil

		case StatusResolved:
			return Incident{}, nil,
				ErrIncidentAlreadyResolved

		default:
			return Incident{}, nil,
				ErrInvalidIncidentState
		}
	}

	if err != nil {
		return Incident{}, nil, fmt.Errorf(
			"acknowledge incident: %w",
			err,
		)
	}

	event, err := insertEvent(
		ctx,
		transaction,
		incident.ID,
		"incident_acknowledged",
		"Incident was acknowledged.",
		&userID,
		nil,
		nil,
		map[string]any{
			"status": StatusAcknowledged,
		},
	)
	if err != nil {
		return Incident{}, nil, err
	}

	if err := transaction.Commit(ctx); err != nil {
		return Incident{}, nil, fmt.Errorf(
			"commit incident acknowledgement: %w",
			err,
		)
	}

	return incident, &event, nil
}

func (r *Repository) Resolve(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (Incident, *IncidentEvent, error) {
	transaction, err := r.db.Begin(ctx)
	if err != nil {
		return Incident{}, nil, fmt.Errorf(
			"begin resolution transaction: %w",
			err,
		)
	}

	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	const query = `
		UPDATE incidents
		SET
			status = 'resolved',
			resolved_at = NOW(),
			resolved_by = $2,
			updated_at = NOW()
		FROM applications
		WHERE incidents.id = $1
		  AND applications.id = incidents.application_id
		  AND applications.user_id = $2
		  AND incidents.status IN (
				'open',
				'acknowledged'
		  )
		RETURNING
			incidents.id,
			incidents.application_id,
			incidents.incident_type,
			incidents.title,
			incidents.summary,
			incidents.severity,
			incidents.status,
			incidents.dedup_key,
			incidents.source_health_check_id,
			incidents.source_deployment_id,
			incidents.rollback_deployment_id,
			incidents.acknowledged_at,
			incidents.acknowledged_by,
			incidents.resolved_at,
			incidents.resolved_by,
			incidents.created_at,
			incidents.updated_at
	`

	incident, err := scanIncident(
		transaction.QueryRow(
			ctx,
			query,
			incidentID,
			userID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		existing, lookupErr := r.findOwned(
			ctx,
			transaction,
			incidentID,
			userID,
		)
		if lookupErr != nil {
			return Incident{}, nil, lookupErr
		}

		if existing.Status == StatusResolved {
			return existing, nil, nil
		}

		return Incident{}, nil, ErrInvalidIncidentState
	}

	if err != nil {
		return Incident{}, nil, fmt.Errorf(
			"resolve incident: %w",
			err,
		)
	}

	event, err := insertEvent(
		ctx,
		transaction,
		incident.ID,
		"incident_resolved",
		"Incident was resolved.",
		&userID,
		nil,
		nil,
		map[string]any{
			"status": StatusResolved,
		},
	)
	if err != nil {
		return Incident{}, nil, err
	}

	if err := transaction.Commit(ctx); err != nil {
		return Incident{}, nil, fmt.Errorf(
			"commit incident resolution: %w",
			err,
		)
	}

	return incident, &event, nil
}

func (r *Repository) LinkRollback(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
	deploymentID uuid.UUID,
) (Incident, IncidentEvent, error) {
	transaction, err := r.db.Begin(ctx)
	if err != nil {
		return Incident{}, IncidentEvent{},
			fmt.Errorf(
				"begin rollback link transaction: %w",
				err,
			)
	}

	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	const query = `
		UPDATE incidents
		SET
			rollback_deployment_id = $3,
			updated_at = NOW()
		FROM applications, deployments
		WHERE incidents.id = $1
		  AND applications.id = incidents.application_id
		  AND applications.user_id = $2
		  AND deployments.id = $3
		  AND deployments.application_id =
				incidents.application_id
		  AND incidents.status IN (
				'open',
				'acknowledged'
		  )
		  AND incidents.rollback_deployment_id IS NULL
		RETURNING
			incidents.id,
			incidents.application_id,
			incidents.incident_type,
			incidents.title,
			incidents.summary,
			incidents.severity,
			incidents.status,
			incidents.dedup_key,
			incidents.source_health_check_id,
			incidents.source_deployment_id,
			incidents.rollback_deployment_id,
			incidents.acknowledged_at,
			incidents.acknowledged_by,
			incidents.resolved_at,
			incidents.resolved_by,
			incidents.created_at,
			incidents.updated_at
	`

	incident, err := scanIncident(
		transaction.QueryRow(
			ctx,
			query,
			incidentID,
			userID,
			deploymentID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		existing, lookupErr := r.findOwned(
			ctx,
			transaction,
			incidentID,
			userID,
		)
		if lookupErr != nil {
			return Incident{}, IncidentEvent{},
				lookupErr
		}

		if existing.RollbackDeploymentID != nil {
			return Incident{}, IncidentEvent{},
				ErrRollbackAlreadyLinked
		}

		return Incident{}, IncidentEvent{},
			ErrInvalidIncidentState
	}

	if err != nil {
		if isUniqueViolation(err) {
			return Incident{}, IncidentEvent{},
				ErrRollbackAlreadyLinked
		}

		return Incident{}, IncidentEvent{},
			fmt.Errorf(
				"link rollback deployment: %w",
				err,
			)
	}

	event, err := insertEvent(
		ctx,
		transaction,
		incident.ID,
		"rollback_started",
		"Rollback deployment was started.",
		&userID,
		nil,
		&deploymentID,
		map[string]any{
			"rollback_deployment_id": deploymentID,
		},
	)
	if err != nil {
		return Incident{}, IncidentEvent{}, err
	}

	if err := transaction.Commit(ctx); err != nil {
		return Incident{}, IncidentEvent{},
			fmt.Errorf(
				"commit rollback link: %w",
				err,
			)
	}

	return incident, event, nil
}

func (r *Repository) AddEvent(
	ctx context.Context,
	incidentID uuid.UUID,
	eventType string,
	message string,
	actorUserID *uuid.UUID,
	healthCheckID *uuid.UUID,
	deploymentID *uuid.UUID,
	metadata any,
) (IncidentEvent, error) {
	return insertEvent(
		ctx,
		r.db,
		incidentID,
		eventType,
		message,
		actorUserID,
		healthCheckID,
		deploymentID,
		metadata,
	)
}

func (r *Repository) FindActiveByDedupKey(
	ctx context.Context,
	applicationID uuid.UUID,
	dedupKey string,
) (Incident, error) {
	const query = `
		SELECT
			id,
			application_id,
			incident_type,
			title,
			summary,
			severity,
			status,
			dedup_key,
			source_health_check_id,
			source_deployment_id,
			rollback_deployment_id,
			acknowledged_at,
			acknowledged_by,
			resolved_at,
			resolved_by,
			created_at,
			updated_at
		FROM incidents
		WHERE application_id = $1
		  AND dedup_key = $2
		  AND status IN (
				'open',
				'acknowledged'
		  )
		ORDER BY created_at DESC
		LIMIT 1
	`

	incident, err := scanIncident(
		r.db.QueryRow(
			ctx,
			query,
			applicationID,
			dedupKey,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrIncidentNotFound
	}

	if err != nil {
		return Incident{}, fmt.Errorf(
			"find active incident: %w",
			err,
		)
	}

	return incident, nil
}

func (r *Repository) updateExistingActive(
	ctx context.Context,
	transaction pgx.Tx,
	input OpenInput,
) (Incident, error) {
	const query = `
		UPDATE incidents
		SET
			title = $3,
			summary = $4,
			severity = CASE
				WHEN severity = 'critical' THEN severity
				WHEN $5 = 'critical' THEN 'critical'
				ELSE severity
			END,
			source_health_check_id = COALESCE(
				$6,
				source_health_check_id
			),
			source_deployment_id = COALESCE(
				$7,
				source_deployment_id
			),
			updated_at = NOW()
		WHERE application_id = $1
		  AND dedup_key = $2
		  AND status IN (
				'open',
				'acknowledged'
		  )
		RETURNING
			id,
			application_id,
			incident_type,
			title,
			summary,
			severity,
			status,
			dedup_key,
			source_health_check_id,
			source_deployment_id,
			rollback_deployment_id,
			acknowledged_at,
			acknowledged_by,
			resolved_at,
			resolved_by,
			created_at,
			updated_at
	`

	incident, err := scanIncident(
		transaction.QueryRow(
			ctx,
			query,
			input.ApplicationID,
			input.DedupKey,
			input.Title,
			input.Summary,
			input.Severity,
			input.SourceHealthCheckID,
			input.SourceDeploymentID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrIncidentNotFound
	}

	return incident, err
}

func (r *Repository) findBySourceDeployment(
	ctx context.Context,
	querier rowQuerier,
	deploymentID uuid.UUID,
) (Incident, error) {
	const query = `
		SELECT
			id,
			application_id,
			incident_type,
			title,
			summary,
			severity,
			status,
			dedup_key,
			source_health_check_id,
			source_deployment_id,
			rollback_deployment_id,
			acknowledged_at,
			acknowledged_by,
			resolved_at,
			resolved_by,
			created_at,
			updated_at
		FROM incidents
		WHERE source_deployment_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	incident, err := scanIncident(
		querier.QueryRow(
			ctx,
			query,
			deploymentID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrIncidentNotFound
	}

	return incident, err
}

func (r *Repository) findOwned(
	ctx context.Context,
	querier rowQuerier,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (Incident, error) {
	const query = `
		SELECT
			incidents.id,
			incidents.application_id,
			incidents.incident_type,
			incidents.title,
			incidents.summary,
			incidents.severity,
			incidents.status,
			incidents.dedup_key,
			incidents.source_health_check_id,
			incidents.source_deployment_id,
			incidents.rollback_deployment_id,
			incidents.acknowledged_at,
			incidents.acknowledged_by,
			incidents.resolved_at,
			incidents.resolved_by,
			incidents.created_at,
			incidents.updated_at
		FROM incidents
		INNER JOIN applications
			ON applications.id = incidents.application_id
		WHERE incidents.id = $1
		  AND applications.user_id = $2
	`

	incident, err := scanIncident(
		querier.QueryRow(
			ctx,
			query,
			incidentID,
			userID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrIncidentNotFound
	}

	if err != nil {
		return Incident{}, fmt.Errorf(
			"find incident: %w",
			err,
		)
	}

	return incident, nil
}

func (r *Repository) ensureApplicationOwned(
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
		return ErrIncidentNotFound
	}

	if err != nil {
		return fmt.Errorf(
			"verify application ownership: %w",
			err,
		)
	}

	return nil
}

type rowScanner interface {
	Scan(destinations ...any) error
}

type rowQuerier interface {
	QueryRow(
		ctx context.Context,
		sql string,
		args ...any,
	) pgx.Row
}

func scanIncident(
	scanner rowScanner,
) (Incident, error) {
	var incident Incident

	err := scanner.Scan(
		&incident.ID,
		&incident.ApplicationID,
		&incident.IncidentType,
		&incident.Title,
		&incident.Summary,
		&incident.Severity,
		&incident.Status,
		&incident.DedupKey,
		&incident.SourceHealthCheckID,
		&incident.SourceDeploymentID,
		&incident.RollbackDeploymentID,
		&incident.AcknowledgedAt,
		&incident.AcknowledgedBy,
		&incident.ResolvedAt,
		&incident.ResolvedBy,
		&incident.CreatedAt,
		&incident.UpdatedAt,
	)

	return incident, err
}

func scanIncidentListItem(
	scanner rowScanner,
) (IncidentListItem, error) {
	var item IncidentListItem

	err := scanner.Scan(
		&item.ID,
		&item.ApplicationID,
		&item.IncidentType,
		&item.Title,
		&item.Summary,
		&item.Severity,
		&item.Status,
		&item.DedupKey,
		&item.SourceHealthCheckID,
		&item.SourceDeploymentID,
		&item.RollbackDeploymentID,
		&item.AcknowledgedAt,
		&item.AcknowledgedBy,
		&item.ResolvedAt,
		&item.ResolvedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.ApplicationName,
	)

	return item, err
}

func scanIncidentEvent(
	scanner rowScanner,
) (IncidentEvent, error) {
	var event IncidentEvent

	err := scanner.Scan(
		&event.ID,
		&event.IncidentID,
		&event.EventType,
		&event.Message,
		&event.ActorUserID,
		&event.HealthCheckID,
		&event.DeploymentID,
		&event.Metadata,
		&event.CreatedAt,
	)

	return event, err
}

func insertEvent(
	ctx context.Context,
	querier rowQuerier,
	incidentID uuid.UUID,
	eventType string,
	message string,
	actorUserID *uuid.UUID,
	healthCheckID *uuid.UUID,
	deploymentID *uuid.UUID,
	metadata any,
) (IncidentEvent, error) {
	encodedMetadata, err := json.Marshal(metadata)
	if err != nil {
		return IncidentEvent{}, fmt.Errorf(
			"encode incident event metadata: %w",
			err,
		)
	}

	if metadata == nil {
		encodedMetadata = []byte("{}")
	}

	const query = `
		INSERT INTO incident_events (
			incident_id,
			event_type,
			message,
			actor_user_id,
			health_check_id,
			deployment_id,
			metadata
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7
		)
		RETURNING
			id,
			incident_id,
			event_type,
			message,
			actor_user_id,
			health_check_id,
			deployment_id,
			metadata,
			created_at
	`

	event, err := scanIncidentEvent(
		querier.QueryRow(
			ctx,
			query,
			incidentID,
			eventType,
			message,
			actorUserID,
			healthCheckID,
			deploymentID,
			encodedMetadata,
		),
	)
	if err != nil {
		return IncidentEvent{}, fmt.Errorf(
			"insert incident event: %w",
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

type RollbackConfig struct {
	ApplicationID uuid.UUID

	CurrentVersion *string

	RollbackWebhookURLEncrypted *string
	WebhookTokenEncrypted       *string
}

type RecoverableRollback struct {
	IncidentID   uuid.UUID
	DeploymentID uuid.UUID
	TriggeredBy  uuid.UUID
}

func (r *Repository) GetRollbackConfig(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) (RollbackConfig, error) {
	const query = `
		SELECT
			id,
			current_version,
			rollback_webhook_url_encrypted,
			webhook_token_encrypted
		FROM applications
		WHERE id = $1
		  AND user_id = $2
	`

	var config RollbackConfig

	err := r.db.QueryRow(
		ctx,
		query,
		applicationID,
		userID,
	).Scan(
		&config.ApplicationID,
		&config.CurrentVersion,
		&config.RollbackWebhookURLEncrypted,
		&config.WebhookTokenEncrypted,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return RollbackConfig{}, ErrIncidentNotFound
	}

	if err != nil {
		return RollbackConfig{}, fmt.Errorf(
			"get rollback configuration: %w",
			err,
		)
	}

	return config, nil
}

func (r *Repository) FindInternal(
	ctx context.Context,
	incidentID uuid.UUID,
) (Incident, error) {
	const query = `
		SELECT
			id,
			application_id,
			incident_type,
			title,
			summary,
			severity,
			status,
			dedup_key,
			source_health_check_id,
			source_deployment_id,
			rollback_deployment_id,
			acknowledged_at,
			acknowledged_by,
			resolved_at,
			resolved_by,
			created_at,
			updated_at
		FROM incidents
		WHERE id = $1
	`

	result, err := scanIncident(
		r.db.QueryRow(
			ctx,
			query,
			incidentID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrIncidentNotFound
	}

	if err != nil {
		return Incident{}, fmt.Errorf(
			"find internal incident: %w",
			err,
		)
	}

	return result, nil
}

func (r *Repository) ResolveAfterRollback(
	ctx context.Context,
	incidentID uuid.UUID,
	deploymentID uuid.UUID,
) (Incident, *IncidentEvent, error) {
	transaction, err := r.db.Begin(ctx)
	if err != nil {
		return Incident{}, nil, fmt.Errorf(
			"begin rollback resolution transaction: %w",
			err,
		)
	}

	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	const query = `
		UPDATE incidents
		SET
			status = 'resolved',
			resolved_at = NOW(),
			resolved_by = NULL,
			updated_at = NOW()
		WHERE id = $1
		  AND rollback_deployment_id = $2
		  AND status IN (
				'open',
				'acknowledged'
		  )
		RETURNING
			id,
			application_id,
			incident_type,
			title,
			summary,
			severity,
			status,
			dedup_key,
			source_health_check_id,
			source_deployment_id,
			rollback_deployment_id,
			acknowledged_at,
			acknowledged_by,
			resolved_at,
			resolved_by,
			created_at,
			updated_at
	`

	result, err := scanIncident(
		transaction.QueryRow(
			ctx,
			query,
			incidentID,
			deploymentID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		existing, lookupErr := r.FindInternal(
			ctx,
			incidentID,
		)
		if lookupErr != nil {
			return Incident{}, nil, lookupErr
		}

		if existing.Status == StatusResolved {
			return existing, nil, nil
		}

		return Incident{}, nil,
			ErrInvalidIncidentState
	}

	if err != nil {
		return Incident{}, nil, fmt.Errorf(
			"resolve incident after rollback: %w",
			err,
		)
	}

	event, err := insertEvent(
		ctx,
		transaction,
		incidentID,
		"incident_resolved_after_rollback",
		"Incident was automatically resolved after rollback health verification succeeded.",
		nil,
		nil,
		&deploymentID,
		map[string]any{
			"rollback_deployment_id": deploymentID,
			"status":                 StatusResolved,
		},
	)
	if err != nil {
		return Incident{}, nil, err
	}

	if err := transaction.Commit(ctx); err != nil {
		return Incident{}, nil, fmt.Errorf(
			"commit rollback incident resolution: %w",
			err,
		)
	}

	return result, &event, nil
}

func (r *Repository) ListRecoverableRollbacks(
	ctx context.Context,
	limit int,
) ([]RecoverableRollback, error) {
	if limit <= 0 {
		limit = 25
	}

	const query = `
		SELECT
			incidents.id,
			deployments.id,
			deployments.triggered_by
		FROM incidents
		INNER JOIN deployments
			ON deployments.id =
				incidents.rollback_deployment_id
		WHERE incidents.status IN (
			'open',
			'acknowledged'
		)
		  AND deployments.deployment_type =
				'rollback'
		  AND deployments.status IN (
				'pending',
				'triggering',
				'verifying'
		  )
		ORDER BY deployments.created_at ASC
		LIMIT $1
	`

	rows, err := r.db.Query(
		ctx,
		query,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list recoverable rollbacks: %w",
			err,
		)
	}
	defer rows.Close()

	results := make([]RecoverableRollback, 0)

	for rows.Next() {
		var result RecoverableRollback

		if err := rows.Scan(
			&result.IncidentID,
			&result.DeploymentID,
			&result.TriggeredBy,
		); err != nil {
			return nil, fmt.Errorf(
				"scan recoverable rollback: %w",
				err,
			)
		}

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate recoverable rollbacks: %w",
			err,
		)
	}

	return results, nil
}

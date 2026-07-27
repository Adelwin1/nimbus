package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const recordTimeout = 5 * time.Second

type Repository struct {
	db     *pgxpool.Pool
	logger *slog.Logger
}

func NewRepository(
	db *pgxpool.Pool,
	logger *slog.Logger,
) *Repository {
	return &Repository{
		db:     db,
		logger: logger,
	}
}

// Record intentionally does not return an error.
//
// Activity recording must not cause the user's main action to
// fail. Database failures are logged with sanitized values.
func (r *Repository) Record(input RecordInput) {
	if err := validateInput(input); err != nil {
		r.logFailure(
			"invalid activity input",
			input,
			err,
		)

		return
	}

	metadata, err := json.Marshal(
		SanitizeMetadata(input.Metadata),
	)
	if err != nil {
		r.logFailure(
			"encode activity metadata",
			input,
			err,
		)

		return
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		recordTimeout,
	)
	defer cancel()

	const query = `
		INSERT INTO activity_logs (
			actor_user_id,
			application_id,
			action,
			entity_type,
			entity_id,
			summary,
			metadata,
			request_id,
			ip_address,
			user_agent
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			NULLIF($8, ''),
			NULLIF($9, '')::inet,
			NULLIF($10, '')
		)
	`

	_, err = r.db.Exec(
		ctx,
		query,
		input.ActorUserID,
		input.ApplicationID,
		strings.TrimSpace(input.Action),
		strings.TrimSpace(input.EntityType),
		input.EntityID,
		RedactText(strings.TrimSpace(input.Summary)),
		metadata,
		strings.TrimSpace(input.RequestID),
		strings.TrimSpace(input.IPAddress),
		RedactText(strings.TrimSpace(input.UserAgent)),
	)
	if err != nil {
		r.logFailure(
			"record activity",
			input,
			err,
		)
	}
}

func validateInput(input RecordInput) error {
	action := strings.TrimSpace(input.Action)
	entityType := strings.TrimSpace(
		input.EntityType,
	)
	summary := strings.TrimSpace(input.Summary)

	if action == "" {
		return fmt.Errorf("activity action is required")
	}

	if len(action) > 100 {
		return fmt.Errorf(
			"activity action exceeds 100 characters",
		)
	}

	if entityType == "" {
		return fmt.Errorf(
			"activity entity type is required",
		)
	}

	if len(entityType) > 60 {
		return fmt.Errorf(
			"activity entity type exceeds 60 characters",
		)
	}

	if summary == "" {
		return fmt.Errorf(
			"activity summary is required",
		)
	}

	return nil
}

func (r *Repository) logFailure(
	message string,
	input RecordInput,
	err error,
) {
	if r.logger == nil {
		return
	}

	r.logger.Error(
		message,
		"action",
		RedactText(input.Action),
		"entity_type",
		RedactText(input.EntityType),
		"entity_id",
		safeUUID(input.EntityID),
		"error",
		RedactText(err.Error()),
	)
}

func safeUUID(value *uuid.UUID) string {
	if value == nil {
		return ""
	}

	return value.String()
}

package deployment

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	TypeDeployment = "deployment"
	TypeRollback   = "rollback"

	StatusPending    = "pending"
	StatusTriggering = "triggering"
	StatusVerifying  = "verifying"
	StatusSuccessful = "successful"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
)

type Deployment struct {
	ID            uuid.UUID `json:"id"`
	ApplicationID uuid.UUID `json:"application_id"`

	Version      string  `json:"version"`
	CommitSHA    *string `json:"commit_sha"`
	ReleaseNotes string  `json:"release_notes"`

	DeploymentType string `json:"deployment_type"`
	Status         string `json:"status"`

	PreviousVersion *string `json:"previous_version"`

	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`

	TriggeredBy uuid.UUID `json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DeploymentEvent struct {
	ID           uuid.UUID       `json:"id"`
	DeploymentID uuid.UUID       `json:"deployment_id"`
	EventType    string          `json:"event_type"`
	Message      string          `json:"message"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
}

type CreateRequest struct {
	Version      string  `json:"version"`
	CommitSHA    *string `json:"commit_sha"`
	ReleaseNotes string  `json:"release_notes"`
}

type ApplicationDeploymentConfig struct {
	ID     uuid.UUID
	UserID uuid.UUID

	CurrentVersion *string

	HealthURL          string
	LatencyThresholdMS int

	DeploymentWebhookURLEncrypted *string
	WebhookTokenEncrypted         *string
}

type DeploymentDetail struct {
	Deployment Deployment        `json:"deployment"`
	Events     []DeploymentEvent `json:"events"`
}

type DeploymentListResponse struct {
	Deployments []Deployment `json:"deployments"`
}

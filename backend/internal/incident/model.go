package incident

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	TypeHealthFailure     = "health_failure"
	TypeExcessiveLatency  = "excessive_latency"
	TypeDeploymentFailure = "deployment_failure"

	SeverityWarning  = "warning"
	SeverityCritical = "critical"

	StatusOpen         = "open"
	StatusAcknowledged = "acknowledged"
	StatusResolved     = "resolved"
)

type Incident struct {
	ID            uuid.UUID `json:"id"`
	ApplicationID uuid.UUID `json:"application_id"`

	IncidentType string `json:"incident_type"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Severity     string `json:"severity"`
	Status       string `json:"status"`
	DedupKey     string `json:"dedup_key"`

	SourceHealthCheckID  *uuid.UUID `json:"source_health_check_id"`
	SourceDeploymentID   *uuid.UUID `json:"source_deployment_id"`
	RollbackDeploymentID *uuid.UUID `json:"rollback_deployment_id"`

	AcknowledgedAt *time.Time `json:"acknowledged_at"`
	AcknowledgedBy *uuid.UUID `json:"acknowledged_by"`

	ResolvedAt *time.Time `json:"resolved_at"`
	ResolvedBy *uuid.UUID `json:"resolved_by"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type IncidentListItem struct {
	Incident

	ApplicationName string `json:"application_name"`
}

type IncidentEvent struct {
	ID         uuid.UUID `json:"id"`
	IncidentID uuid.UUID `json:"incident_id"`

	EventType string `json:"event_type"`
	Message   string `json:"message"`

	ActorUserID   *uuid.UUID `json:"actor_user_id"`
	HealthCheckID *uuid.UUID `json:"health_check_id"`
	DeploymentID  *uuid.UUID `json:"deployment_id"`

	Metadata  json.RawMessage `json:"metadata"`
	CreatedAt time.Time       `json:"created_at"`
}

type OpenInput struct {
	ApplicationID uuid.UUID

	IncidentType string
	Title        string
	Summary      string
	Severity     string
	DedupKey     string

	SourceHealthCheckID *uuid.UUID
	SourceDeploymentID  *uuid.UUID

	Metadata any
}

type IncidentDetail struct {
	Incident Incident        `json:"incident"`
	Events   []IncidentEvent `json:"events"`
}

type IncidentListResponse struct {
	Incidents []IncidentListItem `json:"incidents"`
}

type ActionResponse struct {
	Incident Incident       `json:"incident"`
	Event    *IncidentEvent `json:"event,omitempty"`
}

package activity

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Log struct {
	ID uuid.UUID `json:"id"`

	ActorUserID   *uuid.UUID `json:"actor_user_id"`
	ApplicationID *uuid.UUID `json:"application_id"`

	Action     string     `json:"action"`
	EntityType string     `json:"entity_type"`
	EntityID   *uuid.UUID `json:"entity_id"`

	Summary  string          `json:"summary"`
	Metadata json.RawMessage `json:"metadata"`

	RequestID *string `json:"request_id"`
	IPAddress *string `json:"ip_address"`
	UserAgent *string `json:"user_agent"`

	CreatedAt time.Time `json:"created_at"`
}

type RecordInput struct {
	ActorUserID   *uuid.UUID
	ApplicationID *uuid.UUID

	Action     string
	EntityType string
	EntityID   *uuid.UUID

	Summary  string
	Metadata any

	RequestID string
	IPAddress string
	UserAgent string
}

type Recorder interface {
	Record(input RecordInput)
}

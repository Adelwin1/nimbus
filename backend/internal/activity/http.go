package activity

import (
	"net"
	"net/http"
	"strings"

	appmiddleware "github.com/adel/nimbus/backend/internal/middleware"
	"github.com/google/uuid"
)

type RequestRecordInput struct {
	ActorUserID   *uuid.UUID
	ApplicationID *uuid.UUID

	Action     string
	EntityType string
	EntityID   *uuid.UUID

	Summary  string
	Metadata any
}

func RecordRequest(
	recorder Recorder,
	request *http.Request,
	input RequestRecordInput,
) {
	if recorder == nil || request == nil {
		return
	}

	recorder.Record(
		RecordInput{
			ActorUserID:   input.ActorUserID,
			ApplicationID: input.ApplicationID,
			Action:        input.Action,
			EntityType:    input.EntityType,
			EntityID:      input.EntityID,
			Summary:       input.Summary,
			Metadata:      input.Metadata,
			RequestID: appmiddleware.GetRequestID(
				request.Context(),
			),
			IPAddress: clientIPAddress(request),
			UserAgent: request.UserAgent(),
		},
	)
}

func clientIPAddress(
	request *http.Request,
) string {
	address := strings.TrimSpace(
		request.RemoteAddr,
	)
	if address == "" {
		return ""
	}

	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}

	parsed := net.ParseIP(address)
	if parsed != nil {
		return parsed.String()
	}

	return ""
}

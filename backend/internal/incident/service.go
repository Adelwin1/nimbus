package incident

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const defaultFailureThreshold = 3

type HealthObservation struct {
	ApplicationID uuid.UUID
	HealthCheckID uuid.UUID

	StatusCode   *int
	LatencyMS    int64
	Healthy      bool
	ErrorMessage *string

	ApplicationStatus   string
	ConsecutiveFailures int
	FailureThreshold    int
	LatencyThresholdMS  int
}

type DeploymentFailureObservation struct {
	ApplicationID  uuid.UUID
	DeploymentID   uuid.UUID
	Version        string
	FailureMessage string
	Metadata       any
}

type IncidentStore interface {
	OpenOrGet(
		ctx context.Context,
		input OpenInput,
	) (Incident, IncidentEvent, bool, error)

	ListForUser(
		ctx context.Context,
		userID uuid.UUID,
	) ([]IncidentListItem, error)

	ListByApplication(
		ctx context.Context,
		applicationID uuid.UUID,
		userID uuid.UUID,
	) ([]IncidentListItem, error)

	GetDetail(
		ctx context.Context,
		incidentID uuid.UUID,
		userID uuid.UUID,
	) (IncidentDetail, error)

	Acknowledge(
		ctx context.Context,
		incidentID uuid.UUID,
		userID uuid.UUID,
	) (Incident, *IncidentEvent, error)

	Resolve(
		ctx context.Context,
		incidentID uuid.UUID,
		userID uuid.UUID,
	) (Incident, *IncidentEvent, error)
}

type Service struct {
	repository IncidentStore
}

func NewService(
	repository IncidentStore,
) *Service {
	return &Service{
		repository: repository,
	}
}

func (s *Service) ObserveHealthResult(
	ctx context.Context,
	observation HealthObservation,
) error {
	if observation.ApplicationID == uuid.Nil ||
		observation.HealthCheckID == uuid.Nil {
		return errorsInvalidObservation(
			"application ID and health-check ID are required",
		)
	}

	if !observation.Healthy {
		return s.observeHealthFailure(
			ctx,
			observation,
		)
	}

	if observation.LatencyThresholdMS > 0 &&
		observation.LatencyMS >
			int64(observation.LatencyThresholdMS) {
		return s.observeExcessiveLatency(
			ctx,
			observation,
		)
	}

	return nil
}

func (s *Service) OpenDeploymentFailure(
	ctx context.Context,
	observation DeploymentFailureObservation,
) error {
	if observation.ApplicationID == uuid.Nil ||
		observation.DeploymentID == uuid.Nil {
		return errorsInvalidObservation(
			"application ID and deployment ID are required",
		)
	}

	version := strings.TrimSpace(observation.Version)
	if version == "" {
		version = "unknown version"
	}

	summary := strings.TrimSpace(
		observation.FailureMessage,
	)
	if summary == "" {
		summary = "Post-deployment health verification failed."
	}

	deploymentID := observation.DeploymentID

	_, _, _, err := s.repository.OpenOrGet(
		ctx,
		OpenInput{
			ApplicationID: observation.ApplicationID,
			IncidentType:  TypeDeploymentFailure,
			Title: fmt.Sprintf(
				"Deployment %s failed verification",
				version,
			),
			Summary:  summary,
			Severity: SeverityCritical,
			DedupKey: fmt.Sprintf(
				"deployment_failure:%s",
				observation.DeploymentID,
			),
			SourceDeploymentID: &deploymentID,
			Metadata: map[string]any{
				"version": observation.Version,
				"failure": observation.FailureMessage,
				"details": observation.Metadata,
			},
		},
	)

	return err
}

func (s *Service) ListForUser(
	ctx context.Context,
	userID uuid.UUID,
) ([]IncidentListItem, error) {
	return s.repository.ListForUser(
		ctx,
		userID,
	)
}

func (s *Service) ListByApplication(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) ([]IncidentListItem, error) {
	return s.repository.ListByApplication(
		ctx,
		applicationID,
		userID,
	)
}

func (s *Service) GetDetail(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (IncidentDetail, error) {
	return s.repository.GetDetail(
		ctx,
		incidentID,
		userID,
	)
}

func (s *Service) Acknowledge(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (ActionResponse, error) {
	incident, event, err :=
		s.repository.Acknowledge(
			ctx,
			incidentID,
			userID,
		)
	if err != nil {
		return ActionResponse{}, err
	}

	return ActionResponse{
		Incident: incident,
		Event:    event,
	}, nil
}

func (s *Service) Resolve(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (ActionResponse, error) {
	incident, event, err :=
		s.repository.Resolve(
			ctx,
			incidentID,
			userID,
		)
	if err != nil {
		return ActionResponse{}, err
	}

	return ActionResponse{
		Incident: incident,
		Event:    event,
	}, nil
}

func (s *Service) observeHealthFailure(
	ctx context.Context,
	observation HealthObservation,
) error {
	failureThreshold :=
		observation.FailureThreshold

	if failureThreshold <= 0 {
		failureThreshold =
			defaultFailureThreshold
	}

	if observation.ConsecutiveFailures <
		failureThreshold {
		return nil
	}

	summary := fmt.Sprintf(
		"Nimbus recorded %d consecutive failed health checks.",
		observation.ConsecutiveFailures,
	)

	if observation.ErrorMessage != nil &&
		strings.TrimSpace(
			*observation.ErrorMessage,
		) != "" {
		summary = fmt.Sprintf(
			"%s Latest error: %s",
			summary,
			strings.TrimSpace(
				*observation.ErrorMessage,
			),
		)
	}

	healthCheckID := observation.HealthCheckID

	_, _, _, err := s.repository.OpenOrGet(
		ctx,
		OpenInput{
			ApplicationID:       observation.ApplicationID,
			IncidentType:        TypeHealthFailure,
			Title:               "Application health checks are failing",
			Summary:             summary,
			Severity:            SeverityCritical,
			DedupKey:            TypeHealthFailure,
			SourceHealthCheckID: &healthCheckID,
			Metadata: map[string]any{
				"status_code":          observation.StatusCode,
				"latency_ms":           observation.LatencyMS,
				"application_status":   observation.ApplicationStatus,
				"consecutive_failures": observation.ConsecutiveFailures,
				"failure_threshold":    failureThreshold,
				"error_message":        observation.ErrorMessage,
			},
		},
	)

	return err
}

func (s *Service) observeExcessiveLatency(
	ctx context.Context,
	observation HealthObservation,
) error {
	severity := latencySeverity(
		observation.LatencyMS,
		observation.LatencyThresholdMS,
	)

	healthCheckID := observation.HealthCheckID

	_, _, _, err := s.repository.OpenOrGet(
		ctx,
		OpenInput{
			ApplicationID: observation.ApplicationID,
			IncidentType:  TypeExcessiveLatency,
			Title:         "Application latency is excessive",
			Summary: fmt.Sprintf(
				"Health-check latency was %d ms, above the configured %d ms threshold.",
				observation.LatencyMS,
				observation.LatencyThresholdMS,
			),
			Severity:            severity,
			DedupKey:            TypeExcessiveLatency,
			SourceHealthCheckID: &healthCheckID,
			Metadata: map[string]any{
				"status_code":          observation.StatusCode,
				"latency_ms":           observation.LatencyMS,
				"latency_threshold_ms": observation.LatencyThresholdMS,
				"application_status":   observation.ApplicationStatus,
			},
		},
	)

	return err
}

func latencySeverity(
	latencyMS int64,
	thresholdMS int,
) string {
	if thresholdMS > 0 &&
		latencyMS >= int64(thresholdMS*2) {
		return SeverityCritical
	}

	return SeverityWarning
}

func errorsInvalidObservation(
	message string,
) error {
	return fmt.Errorf(
		"invalid incident observation: %s",
		message,
	)
}

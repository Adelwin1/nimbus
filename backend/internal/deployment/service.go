package deployment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/adel/nimbus/backend/internal/activity"
	"github.com/adel/nimbus/backend/internal/monitoring"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput         = errors.New("invalid deployment input")
	ErrDeploymentInProgress = errors.New("deployment is already processing")
)

const (
	defaultVerificationAttempts  = 6
	defaultRequiredHealthyChecks = 3
	defaultVerificationInterval  = 10 * time.Second
)

type SecretCipher interface {
	Decrypt(ciphertext string) (string, error)
}

type WebhookRunner interface {
	Execute(
		ctx context.Context,
		encryptedURL *string,
		encryptedToken *string,
		payload WebhookPayload,
	) (WebhookResult, error)
}

type HealthCheckRunner interface {
	CheckNow(
		ctx context.Context,
		applicationID uuid.UUID,
		userID uuid.UUID,
	) (
		monitoring.HealthCheck,
		monitoring.ApplicationState,
		error,
	)
}

type Service struct {
	rootContext context.Context
	repository  *Repository
	webhook     WebhookRunner
	monitoring  HealthCheckRunner
	activity    activity.Recorder

	verificationAttempts  int
	requiredHealthyChecks int
	verificationInterval  time.Duration

	inFlight sync.Map
}

func NewService(
	rootContext context.Context,
	repository *Repository,
	webhook WebhookRunner,
	monitoringService HealthCheckRunner,
	recorders ...activity.Recorder,
) *Service {
	var recorder activity.Recorder

	if len(recorders) > 0 {
		recorder = recorders[0]
	}

	return &Service{
		rootContext: rootContext,
		repository:  repository,
		webhook:     webhook,
		monitoring:  monitoringService,
		activity:    recorder,

		verificationAttempts:  defaultVerificationAttempts,
		requiredHealthyChecks: defaultRequiredHealthyChecks,
		verificationInterval:  defaultVerificationInterval,
	}
}

func (s *Service) Create(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
	request CreateRequest,
) (Deployment, error) {
	request.Version = strings.TrimSpace(request.Version)
	request.ReleaseNotes = strings.TrimSpace(
		request.ReleaseNotes,
	)
	request.CommitSHA = normalizeOptionalString(
		request.CommitSHA,
	)

	if err := validateCreateRequest(request); err != nil {
		return Deployment{}, err
	}

	config, err := s.repository.GetApplicationConfig(
		ctx,
		applicationID,
		userID,
	)
	if err != nil {
		return Deployment{}, err
	}

	if config.DeploymentWebhookURLEncrypted == nil ||
		strings.TrimSpace(
			*config.DeploymentWebhookURLEncrypted,
		) == "" {
		return Deployment{}, ErrWebhookNotConfigured
	}

	deployment, err := s.repository.Create(
		ctx,
		applicationID,
		userID,
		request,
	)
	if err != nil {
		return Deployment{}, err
	}

	go s.process(
		s.rootContext,
		deployment.ID,
		userID,
	)

	return deployment, nil
}

func (s *Service) List(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) ([]Deployment, error) {
	return s.repository.ListByApplication(
		ctx,
		applicationID,
		userID,
	)
}

func (s *Service) GetDetail(
	ctx context.Context,
	deploymentID uuid.UUID,
	userID uuid.UUID,
) (DeploymentDetail, error) {
	deployment, err := s.repository.FindByID(
		ctx,
		deploymentID,
		userID,
	)
	if err != nil {
		return DeploymentDetail{}, err
	}

	events, err := s.repository.ListEvents(
		ctx,
		deploymentID,
		userID,
	)
	if err != nil {
		return DeploymentDetail{}, err
	}

	return DeploymentDetail{
		Deployment: deployment,
		Events:     events,
	}, nil
}

func (s *Service) ResumeUnfinished(
	ctx context.Context,
) error {
	deployments, err := s.repository.ListRecoverable(
		ctx,
		100,
	)
	if err != nil {
		return err
	}

	for _, deployment := range deployments {
		deployment := deployment

		go s.process(
			s.rootContext,
			deployment.ID,
			deployment.TriggeredBy,
		)
	}

	return nil
}

func (s *Service) process(
	ctx context.Context,
	deploymentID uuid.UUID,
	userID uuid.UUID,
) {
	if !s.acquire(deploymentID) {
		return
	}
	defer s.release(deploymentID)

	deployment, err := s.repository.FindInternal(
		ctx,
		deploymentID,
	)
	if err != nil {
		return
	}

	if isTerminalStatus(deployment.Status) {
		return
	}

	config, err := s.repository.GetApplicationConfig(
		ctx,
		deployment.ApplicationID,
		userID,
	)
	if err != nil {
		s.failDeployment(
			ctx,
			deployment,
			"configuration_error",
			"Deployment configuration could not be loaded.",
			map[string]any{
				"reason": "application configuration unavailable",
			},
		)
		return
	}

	webhookSucceeded, err := s.repository.HasEvent(
		ctx,
		deployment.ID,
		"webhook_succeeded",
	)
	if err != nil {
		s.failDeployment(
			ctx,
			deployment,
			"event_lookup_failed",
			"Deployment progress could not be recovered.",
			nil,
		)
		return
	}

	if !webhookSucceeded {
		if err := s.triggerWebhook(
			ctx,
			deployment,
			config,
		); err != nil {
			return
		}
	}

	s.verifyDeployment(
		ctx,
		deployment,
		userID,
	)
}

func (s *Service) triggerWebhook(
	ctx context.Context,
	deployment Deployment,
	config ApplicationDeploymentConfig,
) error {
	_ = s.repository.UpdateApplicationStatus(
		ctx,
		deployment.ApplicationID,
		"deploying",
	)

	_, _, err := s.repository.UpdateStatus(
		ctx,
		deployment.ID,
		StatusTriggering,
		"webhook_triggering",
		"Deployment webhook is being triggered.",
		map[string]any{
			"version": deployment.Version,
		},
		true,
		false,
	)
	if err != nil {
		return err
	}

	result, err := s.webhook.Execute(
		ctx,
		config.DeploymentWebhookURLEncrypted,
		config.WebhookTokenEncrypted,
		WebhookPayload{
			DeploymentID:   deployment.ID,
			ApplicationID:  deployment.ApplicationID,
			Version:        deployment.Version,
			CommitSHA:      deployment.CommitSHA,
			ReleaseNotes:   deployment.ReleaseNotes,
			DeploymentType: deployment.DeploymentType,
			TriggeredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		metadata := map[string]any{
			"reason": err.Error(),
		}

		if result.StatusCode != 0 {
			metadata["status_code"] = result.StatusCode
			metadata["latency_ms"] = result.LatencyMS
		}

		s.failDeployment(
			ctx,
			deployment,
			"webhook_failed",
			"Deployment webhook failed.",
			metadata,
		)

		return err
	}

	_, _, err = s.repository.UpdateStatus(
		ctx,
		deployment.ID,
		StatusVerifying,
		"webhook_succeeded",
		"Deployment webhook completed successfully.",
		map[string]any{
			"status_code": result.StatusCode,
			"latency_ms":  result.LatencyMS,
		},
		false,
		false,
	)

	return err
}

func (s *Service) verifyDeployment(
	ctx context.Context,
	deployment Deployment,
	userID uuid.UUID,
) {
	consecutiveHealthy := 0

	for attempt := 1; attempt <= s.verificationAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-time.After(s.verificationInterval):
			case <-ctx.Done():
				return
			}
		}

		check, state, err := s.monitoring.CheckNow(
			ctx,
			deployment.ApplicationID,
			userID,
		)

		if errors.Is(err, monitoring.ErrCheckInProgress) {
			attempt--
			continue
		}

		if err != nil {
			consecutiveHealthy = 0

			_, _ = s.repository.AddEvent(
				ctx,
				deployment.ID,
				"verification_check_error",
				"Post-deployment health check could not be completed.",
				map[string]any{
					"attempt": attempt,
					"reason":  err.Error(),
				},
			)

			continue
		}

		passed :=
			check.Healthy &&
				state.Status == "healthy"

		if passed {
			consecutiveHealthy++
		} else {
			consecutiveHealthy = 0
		}

		_, _ = s.repository.AddEvent(
			ctx,
			deployment.ID,
			"verification_check_completed",
			fmt.Sprintf(
				"Verification check %d completed with status %s.",
				attempt,
				state.Status,
			),
			map[string]any{
				"attempt":             attempt,
				"healthy":             check.Healthy,
				"application_status":  state.Status,
				"status_code":         check.StatusCode,
				"latency_ms":          check.LatencyMS,
				"error_message":       check.ErrorMessage,
				"consecutive_healthy": consecutiveHealthy,
				"required_healthy":    s.requiredHealthyChecks,
			},
		)

		if consecutiveHealthy >= s.requiredHealthyChecks {
			if err := s.repository.UpdateApplicationVersion(
				ctx,
				deployment.ApplicationID,
				deployment.Version,
			); err != nil {
				s.failDeployment(
					ctx,
					deployment,
					"version_update_failed",
					"Deployment verification passed, but the application version could not be updated.",
					nil,
				)
				return
			}

			_, _, err := s.repository.UpdateStatus(
				ctx,
				deployment.ID,
				StatusSuccessful,
				"deployment_succeeded",
				fmt.Sprintf(
					"Deployment %s was verified successfully.",
					deployment.Version,
				),
				map[string]any{
					"required_healthy_checks": s.requiredHealthyChecks,
				},
				false,
				true,
			)
			if err == nil {
				s.recordDeploymentActivity(
					deployment,
					activity.ActionDeploymentSucceeded,
					fmt.Sprintf(
						"Deployment %s succeeded.",
						deployment.Version,
					),
					map[string]any{
						"version":                 deployment.Version,
						"required_healthy_checks": s.requiredHealthyChecks,
					},
				)
			}

			return
		}
	}

	s.failDeployment(
		ctx,
		deployment,
		"verification_failed",
		"Deployment failed post-deployment health verification.",
		map[string]any{
			"attempts":                     s.verificationAttempts,
			"required_consecutive_healthy": s.requiredHealthyChecks,
		},
	)
}

func (s *Service) failDeployment(
	ctx context.Context,
	deployment Deployment,
	eventType string,
	message string,
	metadata any,
) {
	_ = s.repository.UpdateApplicationStatus(
		ctx,
		deployment.ApplicationID,
		"degraded",
	)

	_, _, err := s.repository.UpdateStatus(
		ctx,
		deployment.ID,
		StatusFailed,
		eventType,
		message,
		metadata,
		false,
		true,
	)
	if err == nil {
		s.recordDeploymentActivity(
			deployment,
			activity.ActionDeploymentFailed,
			fmt.Sprintf(
				"Deployment %s failed.",
				deployment.Version,
			),
			map[string]any{
				"version":    deployment.Version,
				"event_type": eventType,
				"details":    metadata,
			},
		)
	}
}

func (s *Service) recordDeploymentActivity(
	deployment Deployment,
	action string,
	summary string,
	metadata any,
) {
	if s.activity == nil {
		return
	}

	applicationID := deployment.ApplicationID
	deploymentID := deployment.ID

	s.activity.Record(activity.RecordInput{
		ApplicationID: &applicationID,
		Action:        action,
		EntityType:    activity.EntityDeployment,
		EntityID:      &deploymentID,
		Summary:       summary,
		Metadata:      metadata,
	})
}

func (s *Service) acquire(
	deploymentID uuid.UUID,
) bool {
	_, alreadyRunning := s.inFlight.LoadOrStore(
		deploymentID,
		struct{}{},
	)

	return !alreadyRunning
}

func (s *Service) release(
	deploymentID uuid.UUID,
) {
	s.inFlight.Delete(deploymentID)
}

func validateCreateRequest(
	request CreateRequest,
) error {
	if len(request.Version) < 1 ||
		len(request.Version) > 100 {
		return fmt.Errorf(
			"%w: version must be between 1 and 100 characters",
			ErrInvalidInput,
		)
	}

	if request.CommitSHA != nil {
		if len(*request.CommitSHA) > 100 {
			return fmt.Errorf(
				"%w: commit SHA must not exceed 100 characters",
				ErrInvalidInput,
			)
		}

		for _, character := range *request.CommitSHA {
			if !isCommitCharacter(character) {
				return fmt.Errorf(
					"%w: commit SHA contains invalid characters",
					ErrInvalidInput,
				)
			}
		}
	}

	if len(request.ReleaseNotes) > 10000 {
		return fmt.Errorf(
			"%w: release notes must not exceed 10000 characters",
			ErrInvalidInput,
		)
	}

	return nil
}

func normalizeOptionalString(
	value *string,
) *string {
	if value == nil {
		return nil
	}

	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}

	return &normalized
}

func isCommitCharacter(
	character rune,
) bool {
	return character >= '0' && character <= '9' ||
		character >= 'a' && character <= 'f' ||
		character >= 'A' && character <= 'F'
}

func isTerminalStatus(status string) bool {
	switch status {
	case StatusSuccessful,
		StatusFailed,
		StatusCancelled:
		return true

	default:
		return false
	}
}

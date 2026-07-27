package incident

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/adel/nimbus/backend/internal/activity"
	"github.com/adel/nimbus/backend/internal/deployment"
	"github.com/adel/nimbus/backend/internal/monitoring"
	"github.com/google/uuid"
)

var (
	ErrRollbackWebhookNotConfigured = errors.New(
		"rollback webhook is not configured",
	)
	ErrNoPreviousHealthyDeployment = errors.New(
		"no previous healthy deployment exists",
	)
	ErrRollbackInProgress = errors.New(
		"another deployment or rollback is already active",
	)
)

const (
	rollbackVerificationAttempts  = 6
	rollbackRequiredHealthyChecks = 3
	rollbackVerificationInterval  = 10 * time.Second
)

type RollbackService struct {
	rootContext context.Context

	incidents   *Repository
	deployments *deployment.Repository
	webhook     deployment.WebhookRunner
	monitoring  deployment.HealthCheckRunner
	activity    activity.Recorder

	verificationAttempts  int
	requiredHealthyChecks int
	verificationInterval  time.Duration

	inFlight sync.Map
}

func NewRollbackService(
	rootContext context.Context,
	incidentRepository *Repository,
	deploymentRepository *deployment.Repository,
	webhook deployment.WebhookRunner,
	monitoringService deployment.HealthCheckRunner,
	recorders ...activity.Recorder,
) *RollbackService {
	var recorder activity.Recorder

	if len(recorders) > 0 {
		recorder = recorders[0]
	}
	return &RollbackService{
		rootContext: rootContext,
		incidents:   incidentRepository,
		deployments: deploymentRepository,
		webhook:     webhook,
		monitoring:  monitoringService,
		activity:    recorder,

		verificationAttempts:  rollbackVerificationAttempts,
		requiredHealthyChecks: rollbackRequiredHealthyChecks,
		verificationInterval:  rollbackVerificationInterval,
	}
}

func (s *RollbackService) Start(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (deployment.Deployment, error) {
	currentIncident, err := s.incidents.FindByID(
		ctx,
		incidentID,
		userID,
	)
	if err != nil {
		return deployment.Deployment{}, err
	}

	if currentIncident.Status == StatusResolved {
		return deployment.Deployment{},
			ErrIncidentAlreadyResolved
	}

	if currentIncident.RollbackDeploymentID != nil {
		return s.deployments.FindByID(
			ctx,
			*currentIncident.RollbackDeploymentID,
			userID,
		)
	}

	config, err := s.incidents.GetRollbackConfig(
		ctx,
		currentIncident.ApplicationID,
		userID,
	)
	if err != nil {
		return deployment.Deployment{}, err
	}

	if config.RollbackWebhookURLEncrypted == nil ||
		strings.TrimSpace(
			*config.RollbackWebhookURLEncrypted,
		) == "" {
		return deployment.Deployment{},
			ErrRollbackWebhookNotConfigured
	}

	target, err := s.deployments.FindLatestSuccessful(
		ctx,
		currentIncident.ApplicationID,
	)
	if errors.Is(
		err,
		deployment.ErrNoSuccessfulDeployment,
	) {
		return deployment.Deployment{},
			ErrNoPreviousHealthyDeployment
	}

	if err != nil {
		return deployment.Deployment{}, err
	}

	rollback, err := s.deployments.CreateRollback(
		ctx,
		currentIncident.ApplicationID,
		userID,
		target,
		incidentID,
	)
	if errors.Is(err, deployment.ErrActiveDeployment) {
		return deployment.Deployment{},
			ErrRollbackInProgress
	}

	if err != nil {
		return deployment.Deployment{}, err
	}

	_, _, err = s.incidents.LinkRollback(
		ctx,
		incidentID,
		userID,
		rollback.ID,
	)
	if err != nil {
		_, _, _ = s.deployments.UpdateStatus(
			ctx,
			rollback.ID,
			deployment.StatusFailed,
			"rollback_link_failed",
			"Rollback could not be linked to its incident.",
			map[string]any{
				"incident_id": incidentID,
				"reason":      err.Error(),
			},
			false,
			true,
		)

		return deployment.Deployment{}, err
	}

	_, _ = s.incidents.AddEvent(
		ctx,
		incidentID,
		"rollback_target_selected",
		fmt.Sprintf(
			"Version %s was selected as the latest verified rollback target.",
			target.Version,
		),
		&userID,
		nil,
		&rollback.ID,
		map[string]any{
			"target_deployment_id": target.ID,
			"target_version":       target.Version,
			"current_version":      config.CurrentVersion,
		},
	)

	go s.process(
		s.rootContext,
		incidentID,
		rollback.ID,
		userID,
	)

	return rollback, nil
}

func (s *RollbackService) ResumeUnfinished(
	ctx context.Context,
) error {
	results, err :=
		s.incidents.ListRecoverableRollbacks(
			ctx,
			100,
		)
	if err != nil {
		return err
	}

	for _, result := range results {
		result := result

		go s.process(
			s.rootContext,
			result.IncidentID,
			result.DeploymentID,
			result.TriggeredBy,
		)
	}

	return nil
}

func (s *RollbackService) process(
	ctx context.Context,
	incidentID uuid.UUID,
	deploymentID uuid.UUID,
	userID uuid.UUID,
) {
	if !s.acquire(deploymentID) {
		return
	}
	defer s.release(deploymentID)

	rollback, err := s.deployments.FindInternal(
		ctx,
		deploymentID,
	)
	if err != nil {
		return
	}

	if rollback.Status == deployment.StatusSuccessful {
		_, _, _ = s.incidents.ResolveAfterRollback(
			ctx,
			incidentID,
			deploymentID,
		)
		return
	}

	if rollback.Status == deployment.StatusFailed ||
		rollback.Status == deployment.StatusCancelled {
		return
	}

	config, err := s.incidents.GetRollbackConfig(
		ctx,
		rollback.ApplicationID,
		userID,
	)
	if err != nil {
		s.failRollback(
			ctx,
			incidentID,
			rollback,
			"rollback_configuration_failed",
			"Rollback configuration could not be loaded.",
			map[string]any{
				"reason": err.Error(),
			},
		)
		return
	}

	webhookSucceeded, err := s.deployments.HasEvent(
		ctx,
		deploymentID,
		"rollback_webhook_succeeded",
	)
	if err != nil {
		s.failRollback(
			ctx,
			incidentID,
			rollback,
			"rollback_progress_recovery_failed",
			"Rollback progress could not be recovered.",
			nil,
		)
		return
	}

	if !webhookSucceeded {
		if err := s.triggerWebhook(
			ctx,
			incidentID,
			rollback,
			config,
		); err != nil {
			return
		}
	}

	s.verifyHealth(
		ctx,
		incidentID,
		rollback,
		userID,
	)
}

func (s *RollbackService) triggerWebhook(
	ctx context.Context,
	incidentID uuid.UUID,
	rollback deployment.Deployment,
	config RollbackConfig,
) error {
	_ = s.deployments.UpdateApplicationStatus(
		ctx,
		rollback.ApplicationID,
		"deploying",
	)

	_, _, err := s.deployments.UpdateStatus(
		ctx,
		rollback.ID,
		deployment.StatusTriggering,
		"rollback_webhook_triggering",
		"Rollback webhook is being triggered.",
		map[string]any{
			"incident_id": incidentID,
			"version":     rollback.Version,
		},
		true,
		false,
	)
	if err != nil {
		return err
	}

	_, _ = s.incidents.AddEvent(
		ctx,
		incidentID,
		"rollback_webhook_triggering",
		"Encrypted rollback webhook is being triggered.",
		nil,
		nil,
		&rollback.ID,
		map[string]any{
			"target_version": rollback.Version,
		},
	)

	result, err := s.webhook.Execute(
		ctx,
		config.RollbackWebhookURLEncrypted,
		config.WebhookTokenEncrypted,
		deployment.WebhookPayload{
			DeploymentID:   rollback.ID,
			ApplicationID:  rollback.ApplicationID,
			Version:        rollback.Version,
			CommitSHA:      rollback.CommitSHA,
			ReleaseNotes:   rollback.ReleaseNotes,
			DeploymentType: deployment.TypeRollback,
			TriggeredAt:    time.Now().UTC(),

			IncidentID:          &incidentID,
			RollbackFromVersion: rollback.PreviousVersion,
		},
	)
	if err != nil {
		metadata := map[string]any{
			"reason": err.Error(),
		}

		if result.StatusCode != 0 {
			metadata["status_code"] =
				result.StatusCode
			metadata["latency_ms"] =
				result.LatencyMS
		}

		s.failRollback(
			ctx,
			incidentID,
			rollback,
			"rollback_webhook_failed",
			"Rollback webhook failed.",
			metadata,
		)

		return err
	}

	_, _, err = s.deployments.UpdateStatus(
		ctx,
		rollback.ID,
		deployment.StatusVerifying,
		"rollback_webhook_succeeded",
		"Rollback webhook completed successfully.",
		map[string]any{
			"status_code": result.StatusCode,
			"latency_ms":  result.LatencyMS,
		},
		false,
		false,
	)
	if err != nil {
		return err
	}

	_, _ = s.incidents.AddEvent(
		ctx,
		incidentID,
		"rollback_webhook_succeeded",
		"Rollback webhook completed successfully. Health verification started.",
		nil,
		nil,
		&rollback.ID,
		map[string]any{
			"status_code": result.StatusCode,
			"latency_ms":  result.LatencyMS,
		},
	)

	return nil
}

func (s *RollbackService) verifyHealth(
	ctx context.Context,
	incidentID uuid.UUID,
	rollback deployment.Deployment,
	userID uuid.UUID,
) {
	consecutiveHealthy := 0

	for attempt := 1; attempt <= s.verificationAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-time.After(
				s.verificationInterval,
			):
			case <-ctx.Done():
				return
			}
		}

		check, state, err := s.monitoring.CheckNow(
			ctx,
			rollback.ApplicationID,
			userID,
		)

		if errors.Is(
			err,
			monitoring.ErrCheckInProgress,
		) {
			attempt--

			select {
			case <-time.After(s.verificationInterval):
			case <-ctx.Done():
				return
			}

			continue
		}

		if err != nil {
			consecutiveHealthy = 0

			_, _ = s.deployments.AddEvent(
				ctx,
				rollback.ID,
				"rollback_verification_error",
				"Rollback health verification could not be completed.",
				map[string]any{
					"attempt": attempt,
					"reason":  err.Error(),
				},
			)

			_, _ = s.incidents.AddEvent(
				ctx,
				incidentID,
				"rollback_verification_error",
				"Rollback health verification could not be completed.",
				nil,
				nil,
				&rollback.ID,
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

		metadata := map[string]any{
			"attempt":             attempt,
			"healthy":             check.Healthy,
			"application_status":  state.Status,
			"status_code":         check.StatusCode,
			"latency_ms":          check.LatencyMS,
			"consecutive_healthy": consecutiveHealthy,
			"required_healthy":    s.requiredHealthyChecks,
		}

		_, _ = s.deployments.AddEvent(
			ctx,
			rollback.ID,
			"rollback_verification_check_completed",
			fmt.Sprintf(
				"Rollback verification check %d completed with status %s.",
				attempt,
				state.Status,
			),
			metadata,
		)

		_, _ = s.incidents.AddEvent(
			ctx,
			incidentID,
			"rollback_verification_check_completed",
			fmt.Sprintf(
				"Rollback verification check %d completed with status %s.",
				attempt,
				state.Status,
			),
			nil,
			&check.ID,
			&rollback.ID,
			metadata,
		)

		if consecutiveHealthy >=
			s.requiredHealthyChecks {
			s.completeRollback(
				ctx,
				incidentID,
				rollback,
			)
			return
		}
	}

	s.failRollback(
		ctx,
		incidentID,
		rollback,
		"rollback_verification_failed",
		"Rollback failed to restore healthy application responses.",
		map[string]any{
			"attempts":                     s.verificationAttempts,
			"required_consecutive_healthy": s.requiredHealthyChecks,
		},
	)
}

func (s *RollbackService) completeRollback(
	ctx context.Context,
	incidentID uuid.UUID,
	rollback deployment.Deployment,
) {
	if err := s.deployments.UpdateApplicationVersion(
		ctx,
		rollback.ApplicationID,
		rollback.Version,
	); err != nil {
		s.failRollback(
			ctx,
			incidentID,
			rollback,
			"rollback_version_update_failed",
			"Health was restored, but the application version could not be updated.",
			map[string]any{
				"reason": err.Error(),
			},
		)
		return
	}

	_, _, err := s.deployments.UpdateStatus(
		ctx,
		rollback.ID,
		deployment.StatusSuccessful,
		"rollback_succeeded",
		fmt.Sprintf(
			"Rollback to version %s was verified successfully.",
			rollback.Version,
		),
		map[string]any{
			"restored_version":        rollback.Version,
			"required_healthy_checks": s.requiredHealthyChecks,
		},
		false,
		true,
	)
	if err != nil {
		return
	}

	_, _ = s.incidents.AddEvent(
		ctx,
		incidentID,
		"rollback_succeeded",
		fmt.Sprintf(
			"Rollback restored verified version %s.",
			rollback.Version,
		),
		nil,
		nil,
		&rollback.ID,
		map[string]any{
			"restored_version": rollback.Version,
		},
	)

	_, _, _ = s.incidents.ResolveAfterRollback(
		ctx,
		incidentID,
		rollback.ID,
	)

	s.recordRollbackActivity(
		rollback,
		activity.ActionRollbackSucceeded,
		fmt.Sprintf(
			"Rollback to version %s succeeded.",
			rollback.Version,
		),
		map[string]any{
			"incident_id":      incidentID,
			"restored_version": rollback.Version,
		},
	)
}

func (s *RollbackService) failRollback(
	ctx context.Context,
	incidentID uuid.UUID,
	rollback deployment.Deployment,
	eventType string,
	message string,
	metadata any,
) {
	_ = s.deployments.UpdateApplicationStatus(
		ctx,
		rollback.ApplicationID,
		"degraded",
	)

	_, _, _ = s.deployments.UpdateStatus(
		ctx,
		rollback.ID,
		deployment.StatusFailed,
		eventType,
		message,
		metadata,
		false,
		true,
	)

	_, _ = s.incidents.AddEvent(
		ctx,
		incidentID,
		eventType,
		message,
		nil,
		nil,
		&rollback.ID,
		metadata,
	)

	s.recordRollbackActivity(
		rollback,
		activity.ActionRollbackFailed,
		fmt.Sprintf(
			"Rollback to version %s failed.",
			rollback.Version,
		),
		map[string]any{
			"incident_id": incidentID,
			"event_type":  eventType,
			"details":     metadata,
		},
	)
}

func (s *RollbackService) recordRollbackActivity(
	rollback deployment.Deployment,
	action string,
	summary string,
	metadata any,
) {
	if s.activity == nil {
		return
	}

	applicationID := rollback.ApplicationID
	rollbackID := rollback.ID

	s.activity.Record(activity.RecordInput{
		ApplicationID: &applicationID,
		Action:        action,
		EntityType:    activity.EntityRollback,
		EntityID:      &rollbackID,
		Summary:       summary,
		Metadata:      metadata,
	})
}

func (s *RollbackService) acquire(
	deploymentID uuid.UUID,
) bool {
	_, loaded := s.inFlight.LoadOrStore(
		deploymentID,
		struct{}{},
	)

	return !loaded
}

func (s *RollbackService) release(
	deploymentID uuid.UUID,
) {
	s.inFlight.Delete(deploymentID)
}

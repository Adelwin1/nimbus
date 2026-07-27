package application

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalidInput = errors.New("invalid application input")

type SecretEncryptor interface {
	Encrypt(plaintext string) (string, error)
}

type Service struct {
	repository *Repository
	encryptor  SecretEncryptor
}

func NewService(
	repository *Repository,
	encryptor SecretEncryptor,
) *Service {
	return &Service{
		repository: repository,
		encryptor:  encryptor,
	}
}

func (s *Service) Create(
	ctx context.Context,
	userID uuid.UUID,
	request CreateRequest,
) (Application, error) {
	application := Application{
		UserID:         userID,
		Name:           strings.TrimSpace(request.Name),
		Description:    strings.TrimSpace(request.Description),
		ApplicationURL: strings.TrimSpace(request.ApplicationURL),
		HealthURL:      strings.TrimSpace(request.HealthURL),
		RepositoryURL:  normalizeOptionalString(request.RepositoryURL),
		Environment: strings.ToLower(
			strings.TrimSpace(request.Environment),
		),
		CurrentVersion: normalizeOptionalString(request.CurrentVersion),
		Status:         StatusUnknown,

		MonitoringIntervalSeconds: request.MonitoringIntervalSeconds,
		FailureThreshold:          request.FailureThreshold,
		LatencyThresholdMS:        request.LatencyThresholdMS,
	}

	applyCreateDefaults(&application)

	if err := validateApplication(application); err != nil {
		return Application{}, err
	}

	deploymentWebhook, err := s.encryptWebhookURL(
		request.DeploymentWebhookURL,
		"deployment webhook URL",
	)
	if err != nil {
		return Application{}, err
	}

	rollbackWebhook, err := s.encryptWebhookURL(
		request.RollbackWebhookURL,
		"rollback webhook URL",
	)
	if err != nil {
		return Application{}, err
	}

	webhookToken, err := s.encryptWebhookToken(request.WebhookToken)
	if err != nil {
		return Application{}, err
	}

	application.DeploymentWebhookURLEncrypted = deploymentWebhook
	application.RollbackWebhookURLEncrypted = rollbackWebhook
	application.WebhookTokenEncrypted = webhookToken

	return s.repository.Create(ctx, userID, application)
}

func (s *Service) List(
	ctx context.Context,
	userID uuid.UUID,
) ([]Application, error) {
	return s.repository.ListByUser(ctx, userID)
}

func (s *Service) Get(
	ctx context.Context,
	appID uuid.UUID,
	userID uuid.UUID,
) (Application, error) {
	return s.repository.FindByID(ctx, appID, userID)
}

func (s *Service) Update(
	ctx context.Context,
	appID uuid.UUID,
	userID uuid.UUID,
	request UpdateRequest,
) (Application, error) {
	if !hasUpdates(request) {
		return Application{}, fmt.Errorf(
			"%w: at least one field must be provided",
			ErrInvalidInput,
		)
	}

	application, err := s.repository.FindByID(
		ctx,
		appID,
		userID,
	)
	if err != nil {
		return Application{}, err
	}

	if request.Name != nil {
		application.Name = strings.TrimSpace(*request.Name)
	}

	if request.Description != nil {
		application.Description = strings.TrimSpace(
			*request.Description,
		)
	}

	if request.ApplicationURL != nil {
		application.ApplicationURL = strings.TrimSpace(
			*request.ApplicationURL,
		)
	}

	if request.HealthURL != nil {
		application.HealthURL = strings.TrimSpace(
			*request.HealthURL,
		)
	}

	if request.RepositoryURL != nil {
		application.RepositoryURL = normalizeOptionalString(
			request.RepositoryURL,
		)
	}

	if request.Environment != nil {
		application.Environment = strings.ToLower(
			strings.TrimSpace(*request.Environment),
		)
	}

	if request.CurrentVersion != nil {
		application.CurrentVersion = normalizeOptionalString(
			request.CurrentVersion,
		)
	}

	if request.MonitoringIntervalSeconds != nil {
		application.MonitoringIntervalSeconds =
			*request.MonitoringIntervalSeconds
	}

	if request.FailureThreshold != nil {
		application.FailureThreshold = *request.FailureThreshold
	}

	if request.LatencyThresholdMS != nil {
		application.LatencyThresholdMS = *request.LatencyThresholdMS
	}

	if request.DeploymentWebhookURL != nil {
		encryptedValue, err := s.encryptWebhookURL(
			request.DeploymentWebhookURL,
			"deployment webhook URL",
		)
		if err != nil {
			return Application{}, err
		}

		application.DeploymentWebhookURLEncrypted = encryptedValue
	}

	if request.RollbackWebhookURL != nil {
		encryptedValue, err := s.encryptWebhookURL(
			request.RollbackWebhookURL,
			"rollback webhook URL",
		)
		if err != nil {
			return Application{}, err
		}

		application.RollbackWebhookURLEncrypted = encryptedValue
	}

	if request.WebhookToken != nil {
		encryptedValue, err := s.encryptWebhookToken(
			request.WebhookToken,
		)
		if err != nil {
			return Application{}, err
		}

		application.WebhookTokenEncrypted = encryptedValue
	}

	if err := validateApplication(application); err != nil {
		return Application{}, err
	}

	return s.repository.Update(ctx, application, userID)
}

func (s *Service) Delete(
	ctx context.Context,
	appID uuid.UUID,
	userID uuid.UUID,
) error {
	return s.repository.Delete(ctx, appID, userID)
}

func (s *Service) Dashboard(
	ctx context.Context,
	userID uuid.UUID,
) (DashboardSummary, error) {
	return s.repository.DashboardSummary(ctx, userID)
}

func (s *Service) encryptWebhookURL(
	value *string,
	fieldName string,
) (*string, error) {
	if value == nil {
		return nil, nil
	}

	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil, nil
	}

	if err := validateHTTPURL(fieldName, normalized); err != nil {
		return nil, err
	}

	encrypted, err := s.encryptor.Encrypt(normalized)
	if err != nil {
		return nil, fmt.Errorf("encrypt %s: %w", fieldName, err)
	}

	return &encrypted, nil
}

func (s *Service) encryptWebhookToken(
	value *string,
) (*string, error) {
	if value == nil {
		return nil, nil
	}

	if strings.TrimSpace(*value) == "" {
		return nil, nil
	}

	if len(*value) > 4096 {
		return nil, fmt.Errorf(
			"%w: webhook token must not exceed 4096 characters",
			ErrInvalidInput,
		)
	}

	encrypted, err := s.encryptor.Encrypt(*value)
	if err != nil {
		return nil, fmt.Errorf("encrypt webhook token: %w", err)
	}

	return &encrypted, nil
}

func applyCreateDefaults(application *Application) {
	if application.Environment == "" {
		application.Environment = EnvironmentProduction
	}

	if application.MonitoringIntervalSeconds == 0 {
		application.MonitoringIntervalSeconds = 60
	}

	if application.FailureThreshold == 0 {
		application.FailureThreshold = 3
	}

	if application.LatencyThresholdMS == 0 {
		application.LatencyThresholdMS = 2000
	}
}

func validateApplication(application Application) error {
	if len(application.Name) < 2 || len(application.Name) > 120 {
		return fmt.Errorf(
			"%w: name must be between 2 and 120 characters",
			ErrInvalidInput,
		)
	}

	if len(application.Description) > 5000 {
		return fmt.Errorf(
			"%w: description must not exceed 5000 characters",
			ErrInvalidInput,
		)
	}

	if err := validateHTTPURL(
		"application URL",
		application.ApplicationURL,
	); err != nil {
		return err
	}

	if err := validateHTTPURL(
		"health URL",
		application.HealthURL,
	); err != nil {
		return err
	}

	if application.RepositoryURL != nil {
		if err := validateHTTPURL(
			"repository URL",
			*application.RepositoryURL,
		); err != nil {
			return err
		}
	}

	switch application.Environment {
	case EnvironmentDevelopment,
		EnvironmentStaging,
		EnvironmentProduction:
	default:
		return fmt.Errorf(
			"%w: environment must be development, staging, or production",
			ErrInvalidInput,
		)
	}

	if application.CurrentVersion != nil &&
		len(*application.CurrentVersion) > 100 {
		return fmt.Errorf(
			"%w: current version must not exceed 100 characters",
			ErrInvalidInput,
		)
	}

	if application.MonitoringIntervalSeconds < 30 ||
		application.MonitoringIntervalSeconds > 86400 {
		return fmt.Errorf(
			"%w: monitoring interval must be between 30 and 86400 seconds",
			ErrInvalidInput,
		)
	}

	if application.FailureThreshold < 1 ||
		application.FailureThreshold > 20 {
		return fmt.Errorf(
			"%w: failure threshold must be between 1 and 20",
			ErrInvalidInput,
		)
	}

	if application.LatencyThresholdMS < 100 ||
		application.LatencyThresholdMS > 60000 {
		return fmt.Errorf(
			"%w: latency threshold must be between 100 and 60000 milliseconds",
			ErrInvalidInput,
		)
	}

	return nil
}

func validateHTTPURL(fieldName string, value string) error {
	if value == "" {
		return fmt.Errorf(
			"%w: %s is required",
			ErrInvalidInput,
			fieldName,
		)
	}

	if len(value) > 2048 {
		return fmt.Errorf(
			"%w: %s must not exceed 2048 characters",
			ErrInvalidInput,
			fieldName,
		)
	}

	if strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf(
			"%w: %s contains invalid whitespace",
			ErrInvalidInput,
			fieldName,
		)
	}

	parsed, err := url.ParseRequestURI(value)
	if err != nil ||
		parsed.Host == "" ||
		parsed.Hostname() == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf(
			"%w: %s must be a valid HTTP or HTTPS URL",
			ErrInvalidInput,
			fieldName,
		)
	}

	if parsed.User != nil {
		return fmt.Errorf(
			"%w: %s must not contain embedded credentials",
			ErrInvalidInput,
			fieldName,
		)
	}

	if parsed.Fragment != "" {
		return fmt.Errorf(
			"%w: %s must not contain a URL fragment",
			ErrInvalidInput,
			fieldName,
		)
	}

	return nil
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}

	return &normalized
}

func hasUpdates(request UpdateRequest) bool {
	return request.Name != nil ||
		request.Description != nil ||
		request.ApplicationURL != nil ||
		request.HealthURL != nil ||
		request.RepositoryURL != nil ||
		request.Environment != nil ||
		request.CurrentVersion != nil ||
		request.MonitoringIntervalSeconds != nil ||
		request.FailureThreshold != nil ||
		request.LatencyThresholdMS != nil ||
		request.DeploymentWebhookURL != nil ||
		request.RollbackWebhookURL != nil ||
		request.WebhookToken != nil
}

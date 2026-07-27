package application

import (
	"time"

	"github.com/google/uuid"
)

const (
	EnvironmentDevelopment = "development"
	EnvironmentStaging     = "staging"
	EnvironmentProduction  = "production"

	StatusUnknown   = "unknown"
	StatusHealthy   = "healthy"
	StatusDegraded  = "degraded"
	StatusDown      = "down"
	StatusDeploying = "deploying"
)

type Application struct {
	ID     uuid.UUID `json:"id"`
	UserID uuid.UUID `json:"-"`

	Name           string  `json:"name"`
	Description    string  `json:"description"`
	ApplicationURL string  `json:"application_url"`
	HealthURL      string  `json:"health_url"`
	RepositoryURL  *string `json:"repository_url"`

	Environment    string  `json:"environment"`
	CurrentVersion *string `json:"current_version"`
	Status         string  `json:"status"`

	MonitoringIntervalSeconds int `json:"monitoring_interval_seconds"`
	FailureThreshold          int `json:"failure_threshold"`
	LatencyThresholdMS        int `json:"latency_threshold_ms"`

	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastCheckedAt       *time.Time `json:"last_checked_at"`
	LastHealthyAt       *time.Time `json:"last_healthy_at"`

	DeploymentWebhookURLEncrypted *string `json:"-"`
	RollbackWebhookURLEncrypted   *string `json:"-"`
	WebhookTokenEncrypted         *string `json:"-"`

	DeploymentWebhookConfigured bool `json:"deployment_webhook_configured"`
	RollbackWebhookConfigured   bool `json:"rollback_webhook_configured"`
	WebhookTokenConfigured      bool `json:"webhook_token_configured"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateRequest struct {
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	ApplicationURL string  `json:"application_url"`
	HealthURL      string  `json:"health_url"`
	RepositoryURL  *string `json:"repository_url"`

	Environment    string  `json:"environment"`
	CurrentVersion *string `json:"current_version"`

	MonitoringIntervalSeconds int `json:"monitoring_interval_seconds"`
	FailureThreshold          int `json:"failure_threshold"`
	LatencyThresholdMS        int `json:"latency_threshold_ms"`

	DeploymentWebhookURL *string `json:"deployment_webhook_url"`
	RollbackWebhookURL   *string `json:"rollback_webhook_url"`
	WebhookToken         *string `json:"webhook_token"`
}

type UpdateRequest struct {
	Name           *string `json:"name"`
	Description    *string `json:"description"`
	ApplicationURL *string `json:"application_url"`
	HealthURL      *string `json:"health_url"`
	RepositoryURL  *string `json:"repository_url"`

	Environment    *string `json:"environment"`
	CurrentVersion *string `json:"current_version"`

	MonitoringIntervalSeconds *int `json:"monitoring_interval_seconds"`
	FailureThreshold          *int `json:"failure_threshold"`
	LatencyThresholdMS        *int `json:"latency_threshold_ms"`

	DeploymentWebhookURL *string `json:"deployment_webhook_url"`
	RollbackWebhookURL   *string `json:"rollback_webhook_url"`
	WebhookToken         *string `json:"webhook_token"`
}

type DashboardSummary struct {
	TotalApplications    int `json:"total_applications"`
	HealthyApplications  int `json:"healthy_applications"`
	DegradedApplications int `json:"degraded_applications"`
	DownApplications     int `json:"down_applications"`
	UnknownApplications  int `json:"unknown_applications"`
}

func (a *Application) SetSecretFlags() {
	a.DeploymentWebhookConfigured =
		a.DeploymentWebhookURLEncrypted != nil &&
			*a.DeploymentWebhookURLEncrypted != ""

	a.RollbackWebhookConfigured =
		a.RollbackWebhookURLEncrypted != nil &&
			*a.RollbackWebhookURLEncrypted != ""

	a.WebhookTokenConfigured =
		a.WebhookTokenEncrypted != nil &&
			*a.WebhookTokenEncrypted != ""
}

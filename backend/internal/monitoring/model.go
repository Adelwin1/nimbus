package monitoring

import (
	"time"

	"github.com/google/uuid"
)

type DueApplication struct {
	ID                  uuid.UUID
	HealthURL           string
	LatencyThresholdMS  int
	FailureThreshold    int
	ConsecutiveFailures int
}

type HealthCheck struct {
	ID            uuid.UUID `json:"id"`
	ApplicationID uuid.UUID `json:"application_id"`
	StatusCode    *int      `json:"status_code"`
	LatencyMS     int64     `json:"latency_ms"`
	Healthy       bool      `json:"healthy"`
	ErrorMessage  *string   `json:"error_message"`
	CheckedAt     time.Time `json:"checked_at"`
}

type ApplicationState struct {
	Status              string     `json:"status"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastCheckedAt       *time.Time `json:"last_checked_at"`
	LastHealthyAt       *time.Time `json:"last_healthy_at"`
}

type HealthStatistics struct {
	TotalChecks         int64        `json:"total_checks"`
	SuccessfulChecks    int64        `json:"successful_checks"`
	FailedChecks        int64        `json:"failed_checks"`
	AvailabilityPercent float64      `json:"availability_percent"`
	AverageLatencyMS    float64      `json:"average_latency_ms"`
	MinimumLatencyMS    int64        `json:"minimum_latency_ms"`
	MaximumLatencyMS    int64        `json:"maximum_latency_ms"`
	LatestCheck         *HealthCheck `json:"latest_check"`
}

type HealthOverview struct {
	State      ApplicationState `json:"state"`
	Statistics HealthStatistics `json:"statistics"`
}

type HistoryPage struct {
	Checks []HealthCheck `json:"checks"`
	Total  int64         `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

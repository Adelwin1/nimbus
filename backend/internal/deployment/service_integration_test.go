package deployment

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adel/nimbus/backend/internal/application"
	"github.com/adel/nimbus/backend/internal/auth"
	appcrypto "github.com/adel/nimbus/backend/internal/crypto"
	"github.com/adel/nimbus/backend/internal/monitoring"
	"github.com/adel/nimbus/backend/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	originalVersion = "v1.0.0"
	targetVersion   = "v2.0.0"
)

type webhookStub struct {
	mu sync.Mutex

	result WebhookResult
	err    error
	calls  int

	started chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (stub *webhookStub) Execute(
	ctx context.Context,
	encryptedURL *string,
	encryptedToken *string,
	payload WebhookPayload,
) (WebhookResult, error) {
	stub.mu.Lock()
	stub.calls++
	stub.mu.Unlock()

	if stub.started != nil {
		stub.once.Do(func() {
			close(stub.started)
		})
	}

	if stub.release != nil {
		select {
		case <-stub.release:
		case <-ctx.Done():
			return WebhookResult{}, ctx.Err()
		}
	}

	return stub.result, stub.err
}

func (stub *webhookStub) CallCount() int {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	return stub.calls
}

type monitoringResult struct {
	healthy bool
	status  string
	err     error
}

type monitoringStub struct {
	mu      sync.Mutex
	results []monitoringResult
	calls   int
}

func (stub *monitoringStub) CheckNow(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) (
	monitoring.HealthCheck,
	monitoring.ApplicationState,
	error,
) {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	index := stub.calls
	stub.calls++

	var result monitoringResult

	switch {
	case len(stub.results) == 0:
		result = monitoringResult{
			healthy: true,
			status:  application.StatusHealthy,
		}

	case index >= len(stub.results):
		result = stub.results[len(stub.results)-1]

	default:
		result = stub.results[index]
	}

	check := monitoring.HealthCheck{}
	check.Healthy = result.healthy

	state := monitoring.ApplicationState{}
	state.Status = result.status

	return check, state, result.err
}

func (stub *monitoringStub) CallCount() int {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	return stub.calls
}

type deploymentTestEnvironment struct {
	database *pgxpool.Pool

	repository *Repository
	service    *Service

	userID      uuid.UUID
	otherUserID uuid.UUID
	appID       uuid.UUID
}

func TestDeploymentSuccessfulVerification(
	t *testing.T,
) {
	webhook := &webhookStub{
		result: WebhookResult{
			StatusCode:  202,
			LatencyMS:   12,
			CompletedAt: time.Now().UTC(),
		},
	}

	health := &monitoringStub{
		results: []monitoringResult{
			{
				healthy: true,
				status:  application.StatusHealthy,
			},
			{
				healthy: true,
				status:  application.StatusHealthy,
			},
		},
	}

	environment := newDeploymentTestEnvironment(
		t,
		webhook,
		health,
	)

	created, err := environment.service.Create(
		context.Background(),
		environment.appID,
		environment.userID,
		validDeploymentRequest(),
	)
	if err != nil {
		t.Fatalf(
			"create deployment: %v",
			err,
		)
	}

	if created.Status != StatusPending {
		t.Fatalf(
			"initial status = %q, expected %q",
			created.Status,
			StatusPending,
		)
	}

	completed := waitForDeploymentStatus(
		t,
		environment.repository,
		created.ID,
		StatusSuccessful,
	)

	if completed.StartedAt == nil {
		t.Fatal(
			"successful deployment has no start time",
		)
	}

	if completed.CompletedAt == nil {
		t.Fatal(
			"successful deployment has no completion time",
		)
	}

	assertApplicationState(
		t,
		environment.database,
		environment.appID,
		targetVersion,
		application.StatusHealthy,
	)

	assertDeploymentEvent(
		t,
		environment.repository,
		created.ID,
		"webhook_succeeded",
	)

	assertDeploymentEvent(
		t,
		environment.repository,
		created.ID,
		"deployment_succeeded",
	)

	if webhook.CallCount() != 1 {
		t.Fatalf(
			"webhook calls = %d, expected 1",
			webhook.CallCount(),
		)
	}

	if health.CallCount() != 2 {
		t.Fatalf(
			"health-check calls = %d, expected 2",
			health.CallCount(),
		)
	}
}

func TestDeploymentWebhookFailure(
	t *testing.T,
) {
	webhook := &webhookStub{
		result: WebhookResult{
			StatusCode: 500,
			LatencyMS:  8,
		},
		err: fmt.Errorf(
			"%w: HTTP 500",
			ErrWebhookRejected,
		),
	}

	health := &monitoringStub{}

	environment := newDeploymentTestEnvironment(
		t,
		webhook,
		health,
	)

	created, err := environment.service.Create(
		context.Background(),
		environment.appID,
		environment.userID,
		validDeploymentRequest(),
	)
	if err != nil {
		t.Fatalf(
			"create deployment: %v",
			err,
		)
	}

	waitForDeploymentStatus(
		t,
		environment.repository,
		created.ID,
		StatusFailed,
	)

	assertApplicationState(
		t,
		environment.database,
		environment.appID,
		originalVersion,
		application.StatusDegraded,
	)

	assertDeploymentEvent(
		t,
		environment.repository,
		created.ID,
		"webhook_failed",
	)

	if health.CallCount() != 0 {
		t.Fatalf(
			"health-check calls = %d, expected 0",
			health.CallCount(),
		)
	}
}

func TestDeploymentVerificationFailure(
	t *testing.T,
) {
	webhook := &webhookStub{
		result: WebhookResult{
			StatusCode:  200,
			CompletedAt: time.Now().UTC(),
		},
	}

	health := &monitoringStub{
		results: []monitoringResult{
			{
				healthy: false,
				status:  application.StatusDown,
			},
			{
				healthy: false,
				status:  application.StatusDown,
			},
			{
				healthy: false,
				status:  application.StatusDown,
			},
		},
	}

	environment := newDeploymentTestEnvironment(
		t,
		webhook,
		health,
	)

	created, err := environment.service.Create(
		context.Background(),
		environment.appID,
		environment.userID,
		validDeploymentRequest(),
	)
	if err != nil {
		t.Fatalf(
			"create deployment: %v",
			err,
		)
	}

	waitForDeploymentStatus(
		t,
		environment.repository,
		created.ID,
		StatusFailed,
	)

	assertApplicationState(
		t,
		environment.database,
		environment.appID,
		originalVersion,
		application.StatusDegraded,
	)

	assertDeploymentEvent(
		t,
		environment.repository,
		created.ID,
		"verification_failed",
	)

	if health.CallCount() != 3 {
		t.Fatalf(
			"health-check calls = %d, expected 3",
			health.CallCount(),
		)
	}
}

func TestDeploymentRejectsConcurrentDeployment(
	t *testing.T,
) {
	started := make(chan struct{})
	release := make(chan struct{})

	webhook := &webhookStub{
		result: WebhookResult{
			StatusCode: 200,
		},
		started: started,
		release: release,
	}

	health := &monitoringStub{
		results: []monitoringResult{
			{
				healthy: true,
				status:  application.StatusHealthy,
			},
		},
	}

	environment := newDeploymentTestEnvironment(
		t,
		webhook,
		health,
	)

	environment.service.requiredHealthyChecks = 1

	first, err := environment.service.Create(
		context.Background(),
		environment.appID,
		environment.userID,
		validDeploymentRequest(),
	)
	if err != nil {
		t.Fatalf(
			"create first deployment: %v",
			err,
		)
	}

	waitForSignal(t, started)

	secondRequest := validDeploymentRequest()
	secondRequest.Version = "v3.0.0"

	_, err = environment.service.Create(
		context.Background(),
		environment.appID,
		environment.userID,
		secondRequest,
	)

	if !errors.Is(err, ErrActiveDeployment) {
		t.Fatalf(
			"second deployment error = %v, expected active deployment",
			err,
		)
	}

	close(release)

	waitForDeploymentStatus(
		t,
		environment.repository,
		first.ID,
		StatusSuccessful,
	)
}

func TestDeploymentOwnershipIsolation(
	t *testing.T,
) {
	webhook := &webhookStub{
		result: WebhookResult{
			StatusCode: 200,
		},
	}

	health := &monitoringStub{
		results: []monitoringResult{
			{
				healthy: true,
				status:  application.StatusHealthy,
			},
			{
				healthy: true,
				status:  application.StatusHealthy,
			},
		},
	}

	environment := newDeploymentTestEnvironment(
		t,
		webhook,
		health,
	)

	created, err := environment.service.Create(
		context.Background(),
		environment.appID,
		environment.userID,
		validDeploymentRequest(),
	)
	if err != nil {
		t.Fatalf(
			"create deployment: %v",
			err,
		)
	}

	waitForDeploymentStatus(
		t,
		environment.repository,
		created.ID,
		StatusSuccessful,
	)

	_, err = environment.service.GetDetail(
		context.Background(),
		created.ID,
		environment.otherUserID,
	)

	if !errors.Is(
		err,
		ErrDeploymentNotFound,
	) {
		t.Fatalf(
			"other user detail error = %v, expected deployment not found",
			err,
		)
	}

	_, err = environment.service.List(
		context.Background(),
		environment.appID,
		environment.otherUserID,
	)

	if !errors.Is(
		err,
		ErrApplicationNotFound,
	) {
		t.Fatalf(
			"other user list error = %v, expected application not found",
			err,
		)
	}

	_, err = environment.service.Create(
		context.Background(),
		environment.appID,
		environment.otherUserID,
		validDeploymentRequest(),
	)

	if !errors.Is(
		err,
		ErrApplicationNotFound,
	) {
		t.Fatalf(
			"other user create error = %v, expected application not found",
			err,
		)
	}
}

func TestDeploymentResumeUnfinished(
	t *testing.T,
) {
	webhook := &webhookStub{
		result: WebhookResult{
			StatusCode: 200,
		},
	}

	health := &monitoringStub{
		results: []monitoringResult{
			{
				healthy: true,
				status:  application.StatusHealthy,
			},
			{
				healthy: true,
				status:  application.StatusHealthy,
			},
		},
	}

	environment := newDeploymentTestEnvironment(
		t,
		webhook,
		health,
	)

	pending, err :=
		environment.repository.Create(
			context.Background(),
			environment.appID,
			environment.userID,
			validDeploymentRequest(),
		)
	if err != nil {
		t.Fatalf(
			"create recoverable deployment: %v",
			err,
		)
	}

	if pending.Status != StatusPending {
		t.Fatalf(
			"recoverable status = %q, expected pending",
			pending.Status,
		)
	}

	if err := environment.service.ResumeUnfinished(
		context.Background(),
	); err != nil {
		t.Fatalf(
			"resume unfinished deployments: %v",
			err,
		)
	}

	waitForDeploymentStatus(
		t,
		environment.repository,
		pending.ID,
		StatusSuccessful,
	)

	assertApplicationState(
		t,
		environment.database,
		environment.appID,
		targetVersion,
		application.StatusHealthy,
	)

	if webhook.CallCount() != 1 {
		t.Fatalf(
			"resume webhook calls = %d, expected 1",
			webhook.CallCount(),
		)
	}
}

func TestDeploymentRejectsInvalidInput(
	t *testing.T,
) {
	webhook := &webhookStub{
		result: WebhookResult{
			StatusCode: 200,
		},
	}

	health := &monitoringStub{}

	environment := newDeploymentTestEnvironment(
		t,
		webhook,
		health,
	)

	invalidCommit := "not-a-commit"

	tests := []struct {
		name    string
		request CreateRequest
	}{
		{
			name: "missing version",
			request: CreateRequest{
				Version: "   ",
			},
		},
		{
			name: "invalid commit SHA",
			request: CreateRequest{
				Version:   targetVersion,
				CommitSHA: &invalidCommit,
			},
		},
		{
			name: "release notes too long",
			request: CreateRequest{
				Version:      targetVersion,
				ReleaseNotes: strings.Repeat("a", 10001),
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err :=
					environment.service.Create(
						context.Background(),
						environment.appID,
						environment.userID,
						test.request,
					)

				if !errors.Is(
					err,
					ErrInvalidInput,
				) {
					t.Fatalf(
						"error = %v, expected invalid input",
						err,
					)
				}
			},
		)
	}
}

func newDeploymentTestEnvironment(
	t *testing.T,
	webhook WebhookRunner,
	health HealthCheckRunner,
) deploymentTestEnvironment {
	t.Helper()

	database :=
		testutil.OpenTestDatabase(t)

	testutil.ResetDatabase(t, database)

	ctx, cancel :=
		context.WithCancel(
			context.Background(),
		)
	t.Cleanup(cancel)

	userRepository :=
		auth.NewRepository(database)

	user, err := userRepository.CreateUser(
		ctx,
		"Deployment Owner",
		"deployment-owner@example.com",
		"$2a$12$deployment-owner-hash",
	)
	if err != nil {
		t.Fatalf(
			"create deployment owner: %v",
			err,
		)
	}

	otherUser, err :=
		userRepository.CreateUser(
			ctx,
			"Other Deployment User",
			"other-deployment-user@example.com",
			"$2a$12$other-user-hash",
		)
	if err != nil {
		t.Fatalf(
			"create other user: %v",
			err,
		)
	}

	key := base64.StdEncoding.EncodeToString(
		[]byte(
			"0123456789abcdef0123456789abcdef",
		),
	)

	encryptor, err :=
		appcrypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf(
			"create encryptor: %v",
			err,
		)
	}

	applicationRepository :=
		application.NewRepository(database)

	applicationService :=
		application.NewService(
			applicationRepository,
			encryptor,
		)

	deploymentWebhook :=
		"https://deploy.example.com/webhook"
	webhookToken :=
		"deployment-test-token"
	currentVersion := originalVersion

	createdApplication, err :=
		applicationService.Create(
			ctx,
			user.ID,
			application.CreateRequest{
				Name:                      "Deployment Test App",
				Description:               "Deployment integration test application.",
				ApplicationURL:            "https://app.example.com",
				HealthURL:                 "https://health.example.com",
				Environment:               application.EnvironmentProduction,
				CurrentVersion:            &currentVersion,
				MonitoringIntervalSeconds: 60,
				FailureThreshold:          3,
				LatencyThresholdMS:        2000,
				DeploymentWebhookURL:      &deploymentWebhook,
				WebhookToken:              &webhookToken,
			},
		)
	if err != nil {
		t.Fatalf(
			"create deployment application: %v",
			err,
		)
	}

	repository := NewRepository(database)

	service := NewService(
		ctx,
		repository,
		webhook,
		health,
	)

	service.verificationAttempts = 3
	service.requiredHealthyChecks = 2
	service.verificationInterval =
		time.Millisecond

	return deploymentTestEnvironment{
		database: database,

		repository: repository,
		service:    service,

		userID:      user.ID,
		otherUserID: otherUser.ID,
		appID:       createdApplication.ID,
	}
}

func validDeploymentRequest() CreateRequest {
	commitSHA := "abc1234"

	return CreateRequest{
		Version:      targetVersion,
		CommitSHA:    &commitSHA,
		ReleaseNotes: "Production release.",
	}
}

func waitForDeploymentStatus(
	t *testing.T,
	repository *Repository,
	deploymentID uuid.UUID,
	expectedStatus string,
) Deployment {
	t.Helper()

	deadline := time.Now().Add(
		5 * time.Second,
	)

	var latest Deployment
	var latestError error

	for time.Now().Before(deadline) {
		latest, latestError =
			repository.FindInternal(
				context.Background(),
				deploymentID,
			)

		if latestError == nil &&
			latest.Status == expectedStatus {
			return latest
		}

		if latestError == nil &&
			isTerminalStatus(latest.Status) &&
			latest.Status != expectedStatus {
			t.Fatalf(
				"deployment reached terminal status %q, expected %q",
				latest.Status,
				expectedStatus,
			)
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf(
		"deployment did not reach %q; latest status=%q error=%v",
		expectedStatus,
		latest.Status,
		latestError,
	)

	return Deployment{}
}

func waitForSignal(
	t *testing.T,
	signal <-chan struct{},
) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal(
			"timed out waiting for webhook execution",
		)
	}
}

func assertDeploymentEvent(
	t *testing.T,
	repository *Repository,
	deploymentID uuid.UUID,
	eventType string,
) {
	t.Helper()

	found, err := repository.HasEvent(
		context.Background(),
		deploymentID,
		eventType,
	)
	if err != nil {
		t.Fatalf(
			"lookup event %q: %v",
			eventType,
			err,
		)
	}

	if !found {
		t.Fatalf(
			"deployment event %q was not recorded",
			eventType,
		)
	}
}

func assertApplicationState(
	t *testing.T,
	database *pgxpool.Pool,
	applicationID uuid.UUID,
	expectedVersion string,
	expectedStatus string,
) {
	t.Helper()

	const query = `
		SELECT
			current_version,
			status
		FROM applications
		WHERE id = $1
	`

	var currentVersion sql.NullString
	var currentStatus string

	err := database.QueryRow(
		context.Background(),
		query,
		applicationID,
	).Scan(
		&currentVersion,
		&currentStatus,
	)
	if err != nil {
		t.Fatalf(
			"read application state: %v",
			err,
		)
	}

	if !currentVersion.Valid {
		t.Fatal(
			"application current version was NULL",
		)
	}

	if currentVersion.String != expectedVersion {
		t.Fatalf(
			"application version = %q, expected %q",
			currentVersion.String,
			expectedVersion,
		)
	}

	if currentStatus != expectedStatus {
		t.Fatalf(
			"application status = %q, expected %q",
			currentStatus,
			expectedStatus,
		)
	}
}

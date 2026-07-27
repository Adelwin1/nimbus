package incident

import (
	"context"
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
	"github.com/adel/nimbus/backend/internal/deployment"
	"github.com/adel/nimbus/backend/internal/monitoring"
	"github.com/adel/nimbus/backend/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	rollbackCurrentVersion = "v2.0.0"
	rollbackTargetVersion  = "v1.0.0"
)

type fakeIncidentStore struct {
	openCalls int
	lastInput OpenInput
}

func (store *fakeIncidentStore) OpenOrGet(
	ctx context.Context,
	input OpenInput,
) (Incident, IncidentEvent, bool, error) {
	store.openCalls++
	store.lastInput = input

	return Incident{
		ID:            uuid.New(),
		ApplicationID: input.ApplicationID,
		IncidentType:  input.IncidentType,
		Status:        StatusOpen,
	}, IncidentEvent{}, true, nil
}

func (store *fakeIncidentStore) ListForUser(
	ctx context.Context,
	userID uuid.UUID,
) ([]IncidentListItem, error) {
	return nil, nil
}

func (store *fakeIncidentStore) ListByApplication(
	ctx context.Context,
	applicationID uuid.UUID,
	userID uuid.UUID,
) ([]IncidentListItem, error) {
	return nil, nil
}

func (store *fakeIncidentStore) GetDetail(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (IncidentDetail, error) {
	return IncidentDetail{}, nil
}

func (store *fakeIncidentStore) Acknowledge(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (Incident, *IncidentEvent, error) {
	return Incident{}, nil, nil
}

func (store *fakeIncidentStore) Resolve(
	ctx context.Context,
	incidentID uuid.UUID,
	userID uuid.UUID,
) (Incident, *IncidentEvent, error) {
	return Incident{}, nil, nil
}

type rollbackWebhookStub struct {
	mu sync.Mutex

	result deployment.WebhookResult
	err    error
	calls  int
}

func (stub *rollbackWebhookStub) Execute(
	ctx context.Context,
	encryptedURL *string,
	encryptedToken *string,
	payload deployment.WebhookPayload,
) (deployment.WebhookResult, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	stub.calls++

	return stub.result, stub.err
}

func (stub *rollbackWebhookStub) CallCount() int {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	return stub.calls
}

type rollbackHealthResult struct {
	healthy bool
	status  string
	err     error
}

type rollbackHealthStub struct {
	mu sync.Mutex

	results []rollbackHealthResult
	calls   int
}

func (stub *rollbackHealthStub) CheckNow(
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

	result := rollbackHealthResult{
		healthy: true,
		status:  application.StatusHealthy,
	}

	if len(stub.results) > 0 {
		if index >= len(stub.results) {
			index = len(stub.results) - 1
		}

		result = stub.results[index]
	}

	check := monitoring.HealthCheck{
		ID:      uuid.New(),
		Healthy: result.healthy,
	}

	state := monitoring.ApplicationState{
		Status: result.status,
	}

	return check, state, result.err
}

func (stub *rollbackHealthStub) CallCount() int {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	return stub.calls
}

type incidentTestEnvironment struct {
	database *pgxpool.Pool

	incidents   *Repository
	deployments *deployment.Repository
	service     *Service
	rollback    *RollbackService

	userID      uuid.UUID
	otherUserID uuid.UUID
	appID       uuid.UUID

	rootContext context.Context
}

func TestIncidentHealthThreshold(
	t *testing.T,
) {
	store := &fakeIncidentStore{}
	service := NewService(store)

	applicationID := uuid.New()
	healthCheckID := uuid.New()

	err := service.ObserveHealthResult(
		context.Background(),
		HealthObservation{
			ApplicationID: applicationID,
			HealthCheckID: healthCheckID,
			Healthy:       false,

			ApplicationStatus:   application.StatusDegraded,
			ConsecutiveFailures: 2,
			FailureThreshold:    3,
		},
	)
	if err != nil {
		t.Fatalf(
			"observe below threshold: %v",
			err,
		)
	}

	if store.openCalls != 0 {
		t.Fatalf(
			"incident opened below threshold; calls=%d",
			store.openCalls,
		)
	}

	message := "connection refused"

	err = service.ObserveHealthResult(
		context.Background(),
		HealthObservation{
			ApplicationID: applicationID,
			HealthCheckID: healthCheckID,
			Healthy:       false,
			ErrorMessage:  &message,

			ApplicationStatus:   application.StatusDown,
			ConsecutiveFailures: 3,
			FailureThreshold:    3,
		},
	)
	if err != nil {
		t.Fatalf(
			"observe threshold failure: %v",
			err,
		)
	}

	if store.openCalls != 1 {
		t.Fatalf(
			"incident open calls = %d, expected 1",
			store.openCalls,
		)
	}

	if store.lastInput.IncidentType !=
		TypeHealthFailure {
		t.Fatalf(
			"incident type = %q, expected %q",
			store.lastInput.IncidentType,
			TypeHealthFailure,
		)
	}

	if store.lastInput.Severity !=
		SeverityCritical {
		t.Fatalf(
			"severity = %q, expected %q",
			store.lastInput.Severity,
			SeverityCritical,
		)
	}

	if store.lastInput.DedupKey !=
		TypeHealthFailure {
		t.Fatalf(
			"dedup key = %q, expected %q",
			store.lastInput.DedupKey,
			TypeHealthFailure,
		)
	}

	if !strings.Contains(
		store.lastInput.Summary,
		message,
	) {
		t.Fatalf(
			"summary did not contain latest error: %q",
			store.lastInput.Summary,
		)
	}
}

func TestIncidentDeduplicationAndLifecycle(
	t *testing.T,
) {
	environment := newIncidentTestEnvironment(
		t,
		&rollbackWebhookStub{},
		&rollbackHealthStub{},
	)

	input := OpenInput{
		ApplicationID: environment.appID,
		IncidentType:  TypeHealthFailure,
		Title:         "Application health checks are failing",
		Summary:       "Three consecutive checks failed.",
		Severity:      SeverityCritical,
		DedupKey:      TypeHealthFailure,
	}

	first, firstEvent, created, err :=
		environment.incidents.OpenOrGet(
			context.Background(),
			input,
		)
	if err != nil {
		t.Fatalf(
			"open first incident: %v",
			err,
		)
	}

	if !created {
		t.Fatal(
			"first incident was not marked created",
		)
	}

	if firstEvent.EventType !=
		"incident_opened" {
		t.Fatalf(
			"first event type = %q",
			firstEvent.EventType,
		)
	}

	second, secondEvent, created, err :=
		environment.incidents.OpenOrGet(
			context.Background(),
			input,
		)
	if err != nil {
		t.Fatalf(
			"observe duplicate incident: %v",
			err,
		)
	}

	if created {
		t.Fatal(
			"duplicate incident was marked created",
		)
	}

	if second.ID != first.ID {
		t.Fatalf(
			"duplicate incident ID = %s, expected %s",
			second.ID,
			first.ID,
		)
	}

	if secondEvent.EventType !=
		"incident_observed_again" {
		t.Fatalf(
			"duplicate event type = %q",
			secondEvent.EventType,
		)
	}

	acknowledged, err :=
		environment.service.Acknowledge(
			context.Background(),
			first.ID,
			environment.userID,
		)
	if err != nil {
		t.Fatalf(
			"acknowledge incident: %v",
			err,
		)
	}

	if acknowledged.Incident.Status !=
		StatusAcknowledged {
		t.Fatalf(
			"acknowledged status = %q",
			acknowledged.Incident.Status,
		)
	}

	if acknowledged.Event == nil ||
		acknowledged.Event.EventType !=
			"incident_acknowledged" {
		t.Fatal(
			"acknowledgement event was missing",
		)
	}

	repeatedAck, err :=
		environment.service.Acknowledge(
			context.Background(),
			first.ID,
			environment.userID,
		)
	if err != nil {
		t.Fatalf(
			"repeat acknowledgement: %v",
			err,
		)
	}

	if repeatedAck.Event != nil {
		t.Fatal(
			"repeat acknowledgement created an event",
		)
	}

	resolved, err :=
		environment.service.Resolve(
			context.Background(),
			first.ID,
			environment.userID,
		)
	if err != nil {
		t.Fatalf(
			"resolve incident: %v",
			err,
		)
	}

	if resolved.Incident.Status !=
		StatusResolved {
		t.Fatalf(
			"resolved status = %q",
			resolved.Incident.Status,
		)
	}

	repeatedResolution, err :=
		environment.service.Resolve(
			context.Background(),
			first.ID,
			environment.userID,
		)
	if err != nil {
		t.Fatalf(
			"repeat resolution: %v",
			err,
		)
	}

	if repeatedResolution.Event != nil {
		t.Fatal(
			"repeat resolution created an event",
		)
	}

	_, err = environment.service.Acknowledge(
		context.Background(),
		first.ID,
		environment.userID,
	)

	if !errors.Is(
		err,
		ErrIncidentAlreadyResolved,
	) {
		t.Fatalf(
			"acknowledge resolved error = %v",
			err,
		)
	}

	detail, err :=
		environment.service.GetDetail(
			context.Background(),
			first.ID,
			environment.userID,
		)
	if err != nil {
		t.Fatalf(
			"get incident detail: %v",
			err,
		)
	}

	if len(detail.Events) < 4 {
		t.Fatalf(
			"timeline event count = %d, expected at least 4",
			len(detail.Events),
		)
	}

	_, err = environment.service.GetDetail(
		context.Background(),
		first.ID,
		environment.otherUserID,
	)

	if !errors.Is(
		err,
		ErrIncidentNotFound,
	) {
		t.Fatalf(
			"other user detail error = %v",
			err,
		)
	}
}

func TestRollbackSuccessResolvesIncident(
	t *testing.T,
) {
	webhook := &rollbackWebhookStub{
		result: deployment.WebhookResult{
			StatusCode:  200,
			CompletedAt: time.Now().UTC(),
		},
	}

	health := &rollbackHealthStub{
		results: []rollbackHealthResult{
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

	environment := newIncidentTestEnvironment(
		t,
		webhook,
		health,
	)

	incident := openTestIncident(
		t,
		environment,
		"rollback-success",
	)

	rollback, err := environment.rollback.Start(
		context.Background(),
		incident.ID,
		environment.userID,
	)
	if err != nil {
		t.Fatalf(
			"start rollback: %v",
			err,
		)
	}

	waitForRollbackStatus(
		t,
		environment.deployments,
		rollback.ID,
		deployment.StatusSuccessful,
	)

	resolved := waitForIncidentStatus(
		t,
		environment.incidents,
		incident.ID,
		StatusResolved,
	)

	if resolved.RollbackDeploymentID == nil ||
		*resolved.RollbackDeploymentID !=
			rollback.ID {
		t.Fatal(
			"incident was not linked to rollback",
		)
	}

	assertIncidentApplicationState(
		t,
		environment.database,
		environment.appID,
		rollbackTargetVersion,
		application.StatusHealthy,
	)

	assertIncidentEvent(
		t,
		environment,
		incident.ID,
		"rollback_succeeded",
	)

	assertIncidentEvent(
		t,
		environment,
		incident.ID,
		"incident_resolved_after_rollback",
	)

	if webhook.CallCount() != 1 {
		t.Fatalf(
			"webhook calls = %d, expected 1",
			webhook.CallCount(),
		)
	}

	if health.CallCount() != 2 {
		t.Fatalf(
			"health calls = %d, expected 2",
			health.CallCount(),
		)
	}
}

func TestRollbackWebhookFailure(
	t *testing.T,
) {
	webhook := &rollbackWebhookStub{
		result: deployment.WebhookResult{
			StatusCode: 500,
		},
		err: fmt.Errorf(
			"%w: HTTP 500",
			deployment.ErrWebhookRejected,
		),
	}

	environment := newIncidentTestEnvironment(
		t,
		webhook,
		&rollbackHealthStub{},
	)

	incident := openTestIncident(
		t,
		environment,
		"rollback-webhook-failure",
	)

	rollback, err := environment.rollback.Start(
		context.Background(),
		incident.ID,
		environment.userID,
	)
	if err != nil {
		t.Fatalf(
			"start rollback: %v",
			err,
		)
	}

	waitForRollbackStatus(
		t,
		environment.deployments,
		rollback.ID,
		deployment.StatusFailed,
	)

	current, err :=
		environment.incidents.FindInternal(
			context.Background(),
			incident.ID,
		)
	if err != nil {
		t.Fatalf(
			"find incident: %v",
			err,
		)
	}

	if current.Status == StatusResolved {
		t.Fatal(
			"failed rollback resolved the incident",
		)
	}

	assertIncidentApplicationState(
		t,
		environment.database,
		environment.appID,
		rollbackCurrentVersion,
		application.StatusDegraded,
	)

	assertIncidentEvent(
		t,
		environment,
		incident.ID,
		"rollback_webhook_failed",
	)
}

func TestRollbackVerificationFailure(
	t *testing.T,
) {
	webhook := &rollbackWebhookStub{
		result: deployment.WebhookResult{
			StatusCode: 200,
		},
	}

	health := &rollbackHealthStub{
		results: []rollbackHealthResult{
			{
				healthy: false,
				status:  application.StatusDown,
			},
		},
	}

	environment := newIncidentTestEnvironment(
		t,
		webhook,
		health,
	)

	incident := openTestIncident(
		t,
		environment,
		"rollback-verification-failure",
	)

	rollback, err := environment.rollback.Start(
		context.Background(),
		incident.ID,
		environment.userID,
	)
	if err != nil {
		t.Fatalf(
			"start rollback: %v",
			err,
		)
	}

	waitForRollbackStatus(
		t,
		environment.deployments,
		rollback.ID,
		deployment.StatusFailed,
	)

	assertIncidentApplicationState(
		t,
		environment.database,
		environment.appID,
		rollbackCurrentVersion,
		application.StatusDegraded,
	)

	assertIncidentEvent(
		t,
		environment,
		incident.ID,
		"rollback_verification_failed",
	)
}

func TestRollbackOwnershipIsolation(
	t *testing.T,
) {
	environment := newIncidentTestEnvironment(
		t,
		&rollbackWebhookStub{},
		&rollbackHealthStub{},
	)

	incident := openTestIncident(
		t,
		environment,
		"rollback-ownership",
	)

	_, err := environment.rollback.Start(
		context.Background(),
		incident.ID,
		environment.otherUserID,
	)

	if !errors.Is(
		err,
		ErrIncidentNotFound,
	) {
		t.Fatalf(
			"other user rollback error = %v",
			err,
		)
	}
}

func TestRollbackResumeUnfinished(
	t *testing.T,
) {
	webhook := &rollbackWebhookStub{
		result: deployment.WebhookResult{
			StatusCode: 200,
		},
	}

	health := &rollbackHealthStub{
		results: []rollbackHealthResult{
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

	environment := newIncidentTestEnvironment(
		t,
		webhook,
		health,
	)

	incident := openTestIncident(
		t,
		environment,
		"rollback-resume",
	)

	target, err :=
		environment.deployments.FindLatestSuccessful(
			context.Background(),
			environment.appID,
		)
	if err != nil {
		t.Fatalf(
			"find rollback target: %v",
			err,
		)
	}

	rollback, err :=
		environment.deployments.CreateRollback(
			context.Background(),
			environment.appID,
			environment.userID,
			target,
			incident.ID,
		)
	if err != nil {
		t.Fatalf(
			"create unfinished rollback: %v",
			err,
		)
	}

	_, _, err =
		environment.incidents.LinkRollback(
			context.Background(),
			incident.ID,
			environment.userID,
			rollback.ID,
		)
	if err != nil {
		t.Fatalf(
			"link unfinished rollback: %v",
			err,
		)
	}

	if err := environment.rollback.ResumeUnfinished(
		context.Background(),
	); err != nil {
		t.Fatalf(
			"resume rollbacks: %v",
			err,
		)
	}

	waitForRollbackStatus(
		t,
		environment.deployments,
		rollback.ID,
		deployment.StatusSuccessful,
	)

	waitForIncidentStatus(
		t,
		environment.incidents,
		incident.ID,
		StatusResolved,
	)
}

func newIncidentTestEnvironment(
	t *testing.T,
	webhook deployment.WebhookRunner,
	health deployment.HealthCheckRunner,
) incidentTestEnvironment {
	t.Helper()

	database :=
		testutil.OpenTestDatabase(t)

	testutil.ResetDatabase(t, database)

	rootContext, cancel :=
		context.WithCancel(
			context.Background(),
		)
	t.Cleanup(cancel)

	userRepository := auth.NewRepository(database)

	user, err := userRepository.CreateUser(
		rootContext,
		"Incident Owner",
		"incident-owner@example.com",
		"$2a$12$incident-owner-hash",
	)
	if err != nil {
		t.Fatalf(
			"create incident owner: %v",
			err,
		)
	}

	otherUser, err :=
		userRepository.CreateUser(
			rootContext,
			"Other Incident User",
			"other-incident-user@example.com",
			"$2a$12$other-incident-user-hash",
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

	currentVersion := rollbackCurrentVersion
	rollbackURL :=
		"https://rollback.example.com/webhook"
	webhookToken := "rollback-test-token"

	createdApplication, err :=
		applicationService.Create(
			rootContext,
			user.ID,
			application.CreateRequest{
				Name:                      "Incident Test App",
				Description:               "Incident and rollback integration tests.",
				ApplicationURL:            "https://app.example.com",
				HealthURL:                 "https://health.example.com",
				Environment:               application.EnvironmentProduction,
				CurrentVersion:            &currentVersion,
				MonitoringIntervalSeconds: 60,
				FailureThreshold:          3,
				LatencyThresholdMS:        2000,
				RollbackWebhookURL:        &rollbackURL,
				WebhookToken:              &webhookToken,
			},
		)
	if err != nil {
		t.Fatalf(
			"create incident application: %v",
			err,
		)
	}

	deploymentRepository :=
		deployment.NewRepository(database)

	targetCommit := "abc1234"

	target, err := deploymentRepository.Create(
		rootContext,
		createdApplication.ID,
		user.ID,
		deployment.CreateRequest{
			Version:      rollbackTargetVersion,
			CommitSHA:    &targetCommit,
			ReleaseNotes: "Previous verified release.",
		},
	)
	if err != nil {
		t.Fatalf(
			"create rollback target: %v",
			err,
		)
	}

	_, _, err = deploymentRepository.UpdateStatus(
		rootContext,
		target.ID,
		deployment.StatusSuccessful,
		"deployment_succeeded",
		"Previous deployment was verified.",
		nil,
		true,
		true,
	)
	if err != nil {
		t.Fatalf(
			"mark rollback target successful: %v",
			err,
		)
	}

	incidentRepository :=
		NewRepository(database)

	incidentService :=
		NewService(incidentRepository)

	rollbackService := NewRollbackService(
		rootContext,
		incidentRepository,
		deploymentRepository,
		webhook,
		health,
	)

	rollbackService.verificationAttempts = 3
	rollbackService.requiredHealthyChecks = 2
	rollbackService.verificationInterval =
		time.Millisecond

	return incidentTestEnvironment{
		database: database,

		incidents:   incidentRepository,
		deployments: deploymentRepository,
		service:     incidentService,
		rollback:    rollbackService,

		userID:      user.ID,
		otherUserID: otherUser.ID,
		appID:       createdApplication.ID,

		rootContext: rootContext,
	}
}

func openTestIncident(
	t *testing.T,
	environment incidentTestEnvironment,
	dedupSuffix string,
) Incident {
	t.Helper()

	result, _, created, err :=
		environment.incidents.OpenOrGet(
			context.Background(),
			OpenInput{
				ApplicationID: environment.appID,
				IncidentType:  TypeDeploymentFailure,
				Title:         "Deployment verification failed",
				Summary:       "Application remained unhealthy.",
				Severity:      SeverityCritical,
				DedupKey: "deployment_failure:" +
					dedupSuffix,
			},
		)
	if err != nil {
		t.Fatalf(
			"open test incident: %v",
			err,
		)
	}

	if !created {
		t.Fatal(
			"test incident was not created",
		)
	}

	return result
}

func waitForRollbackStatus(
	t *testing.T,
	repository *deployment.Repository,
	deploymentID uuid.UUID,
	expected string,
) deployment.Deployment {
	t.Helper()

	deadline := time.Now().Add(
		5 * time.Second,
	)

	var latest deployment.Deployment
	var latestError error

	for time.Now().Before(deadline) {
		latest, latestError =
			repository.FindInternal(
				context.Background(),
				deploymentID,
			)

		if latestError == nil &&
			latest.Status == expected {
			return latest
		}

		if latestError == nil &&
			isRollbackTerminal(latest.Status) &&
			latest.Status != expected {
			t.Fatalf(
				"rollback reached %q, expected %q",
				latest.Status,
				expected,
			)
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf(
		"rollback did not reach %q; latest=%q error=%v",
		expected,
		latest.Status,
		latestError,
	)

	return deployment.Deployment{}
}

func waitForIncidentStatus(
	t *testing.T,
	repository *Repository,
	incidentID uuid.UUID,
	expected string,
) Incident {
	t.Helper()

	deadline := time.Now().Add(
		5 * time.Second,
	)

	var latest Incident
	var latestError error

	for time.Now().Before(deadline) {
		latest, latestError =
			repository.FindInternal(
				context.Background(),
				incidentID,
			)

		if latestError == nil &&
			latest.Status == expected {
			return latest
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf(
		"incident did not reach %q; latest=%q error=%v",
		expected,
		latest.Status,
		latestError,
	)

	return Incident{}
}

func isRollbackTerminal(
	status string,
) bool {
	switch status {
	case deployment.StatusSuccessful,
		deployment.StatusFailed,
		deployment.StatusCancelled:
		return true

	default:
		return false
	}
}

func assertIncidentEvent(
	t *testing.T,
	environment incidentTestEnvironment,
	incidentID uuid.UUID,
	eventType string,
) {
	t.Helper()

	detail, err :=
		environment.incidents.GetDetail(
			context.Background(),
			incidentID,
			environment.userID,
		)
	if err != nil {
		t.Fatalf(
			"get incident timeline: %v",
			err,
		)
	}

	for _, event := range detail.Events {
		if event.EventType == eventType {
			return
		}
	}

	t.Fatalf(
		"incident event %q was not recorded",
		eventType,
	)
}

func assertIncidentApplicationState(
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

	var version string
	var status string

	err := database.QueryRow(
		context.Background(),
		query,
		applicationID,
	).Scan(
		&version,
		&status,
	)
	if err != nil {
		t.Fatalf(
			"read application state: %v",
			err,
		)
	}

	if version != expectedVersion {
		t.Fatalf(
			"version = %q, expected %q",
			version,
			expectedVersion,
		)
	}

	if status != expectedStatus {
		t.Fatalf(
			"status = %q, expected %q",
			status,
			expectedStatus,
		)
	}
}

package application_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/adel/nimbus/backend/internal/application"
	"github.com/adel/nimbus/backend/internal/auth"
	appcrypto "github.com/adel/nimbus/backend/internal/crypto"
	"github.com/adel/nimbus/backend/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	deploymentWebhook = "https://hooks.example.com/deploy"
	rollbackWebhook   = "https://hooks.example.com/rollback"
	webhookToken      = "nimbus-webhook-secret"
)

type applicationTestEnvironment struct {
	database *pgxpool.Pool
	service  *application.Service
}

func TestApplicationCRUDAndSecretEncryption(
	t *testing.T,
) {
	environment := newApplicationTestEnvironment(t)

	ownerID := createTestUser(
		t,
		environment.database,
		"Owner",
		"owner@example.com",
	)

	created, err := environment.service.Create(
		context.Background(),
		ownerID,
		validCreateRequest(),
	)
	if err != nil {
		t.Fatalf(
			"create application: %v",
			err,
		)
	}

	if created.ID == uuid.Nil {
		t.Fatal("created application ID was empty")
	}

	if created.Name != "Nimbus API" {
		t.Fatalf(
			"name = %q, expected %q",
			created.Name,
			"Nimbus API",
		)
	}

	if created.Status != application.StatusUnknown {
		t.Fatalf(
			"status = %q, expected %q",
			created.Status,
			application.StatusUnknown,
		)
	}

	if !created.DeploymentWebhookConfigured {
		t.Fatal(
			"deployment webhook was not marked configured",
		)
	}

	if !created.RollbackWebhookConfigured {
		t.Fatal(
			"rollback webhook was not marked configured",
		)
	}

	if !created.WebhookTokenConfigured {
		t.Fatal(
			"webhook token was not marked configured",
		)
	}

	assertSecretsEncrypted(
		t,
		environment.database,
		created.ID,
	)

	applications, err :=
		environment.service.List(
			context.Background(),
			ownerID,
		)
	if err != nil {
		t.Fatalf(
			"list applications: %v",
			err,
		)
	}

	if len(applications) != 1 {
		t.Fatalf(
			"application count = %d, expected 1",
			len(applications),
		)
	}

	found, err := environment.service.Get(
		context.Background(),
		created.ID,
		ownerID,
	)
	if err != nil {
		t.Fatalf(
			"get application: %v",
			err,
		)
	}

	if found.ID != created.ID {
		t.Fatalf(
			"found application ID = %s, expected %s",
			found.ID,
			created.ID,
		)
	}

	updatedName := "Nimbus Production API"
	updatedDescription :=
		"Production reliability service."
	updatedEnvironment :=
		application.EnvironmentProduction
	updatedVersion := "v2.0.0"
	updatedInterval := 120
	updatedFailureThreshold := 5
	updatedLatencyThreshold := 3000

	updated, err := environment.service.Update(
		context.Background(),
		created.ID,
		ownerID,
		application.UpdateRequest{
			Name:                      &updatedName,
			Description:               &updatedDescription,
			Environment:               &updatedEnvironment,
			CurrentVersion:            &updatedVersion,
			MonitoringIntervalSeconds: &updatedInterval,
			FailureThreshold:          &updatedFailureThreshold,
			LatencyThresholdMS:        &updatedLatencyThreshold,
		},
	)
	if err != nil {
		t.Fatalf(
			"update application: %v",
			err,
		)
	}

	if updated.Name != updatedName {
		t.Fatalf(
			"updated name = %q, expected %q",
			updated.Name,
			updatedName,
		)
	}

	if updated.CurrentVersion == nil ||
		*updated.CurrentVersion != updatedVersion {
		t.Fatalf(
			"updated version was not saved",
		)
	}

	if !updated.DeploymentWebhookConfigured ||
		!updated.RollbackWebhookConfigured ||
		!updated.WebhookTokenConfigured {
		t.Fatal(
			"updating unrelated fields removed secret configuration",
		)
	}

	if err := environment.service.Delete(
		context.Background(),
		created.ID,
		ownerID,
	); err != nil {
		t.Fatalf(
			"delete application: %v",
			err,
		)
	}

	_, err = environment.service.Get(
		context.Background(),
		created.ID,
		ownerID,
	)

	if !errors.Is(
		err,
		application.ErrApplicationNotFound,
	) {
		t.Fatalf(
			"get deleted application error = %v, expected application not found",
			err,
		)
	}

	applications, err =
		environment.service.List(
			context.Background(),
			ownerID,
		)
	if err != nil {
		t.Fatalf(
			"list after deletion: %v",
			err,
		)
	}

	if len(applications) != 0 {
		t.Fatalf(
			"application count after deletion = %d, expected 0",
			len(applications),
		)
	}
}

func TestApplicationOwnershipIsolation(
	t *testing.T,
) {
	environment := newApplicationTestEnvironment(t)

	ownerID := createTestUser(
		t,
		environment.database,
		"Owner",
		"owner-isolation@example.com",
	)

	otherUserID := createTestUser(
		t,
		environment.database,
		"Other User",
		"other-user@example.com",
	)

	created, err := environment.service.Create(
		context.Background(),
		ownerID,
		validCreateRequest(),
	)
	if err != nil {
		t.Fatalf(
			"create owner application: %v",
			err,
		)
	}

	_, err = environment.service.Get(
		context.Background(),
		created.ID,
		otherUserID,
	)

	if !errors.Is(
		err,
		application.ErrApplicationNotFound,
	) {
		t.Fatalf(
			"other user get error = %v, expected application not found",
			err,
		)
	}

	newName := "Stolen Application"

	_, err = environment.service.Update(
		context.Background(),
		created.ID,
		otherUserID,
		application.UpdateRequest{
			Name: &newName,
		},
	)

	if !errors.Is(
		err,
		application.ErrApplicationNotFound,
	) {
		t.Fatalf(
			"other user update error = %v, expected application not found",
			err,
		)
	}

	err = environment.service.Delete(
		context.Background(),
		created.ID,
		otherUserID,
	)

	if !errors.Is(
		err,
		application.ErrApplicationNotFound,
	) {
		t.Fatalf(
			"other user delete error = %v, expected application not found",
			err,
		)
	}

	otherApplications, err :=
		environment.service.List(
			context.Background(),
			otherUserID,
		)
	if err != nil {
		t.Fatalf(
			"list other user's applications: %v",
			err,
		)
	}

	if len(otherApplications) != 0 {
		t.Fatalf(
			"other user saw %d application(s), expected 0",
			len(otherApplications),
		)
	}

	ownerApplication, err :=
		environment.service.Get(
			context.Background(),
			created.ID,
			ownerID,
		)
	if err != nil {
		t.Fatalf(
			"owner could not retrieve application after unauthorized attempts: %v",
			err,
		)
	}

	if ownerApplication.Name != "Nimbus API" {
		t.Fatalf(
			"owner application name changed to %q",
			ownerApplication.Name,
		)
	}
}

func TestApplicationRejectsInvalidInput(
	t *testing.T,
) {
	environment := newApplicationTestEnvironment(t)

	ownerID := createTestUser(
		t,
		environment.database,
		"Validation User",
		"validation@example.com",
	)

	tests := []struct {
		name   string
		mutate func(
			request *application.CreateRequest,
		)
	}{
		{
			name: "short name",
			mutate: func(
				request *application.CreateRequest,
			) {
				request.Name = "A"
			},
		},
		{
			name: "invalid application URL",
			mutate: func(
				request *application.CreateRequest,
			) {
				request.ApplicationURL =
					"not-a-url"
			},
		},
		{
			name: "invalid health URL",
			mutate: func(
				request *application.CreateRequest,
			) {
				request.HealthURL =
					"ftp://example.com/health"
			},
		},
		{
			name: "monitoring interval too low",
			mutate: func(
				request *application.CreateRequest,
			) {
				request.MonitoringIntervalSeconds =
					29
			},
		},
		{
			name: "failure threshold too high",
			mutate: func(
				request *application.CreateRequest,
			) {
				request.FailureThreshold = 21
			},
		},
		{
			name: "latency threshold too low",
			mutate: func(
				request *application.CreateRequest,
			) {
				request.LatencyThresholdMS = 99
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				request := validCreateRequest()
				test.mutate(&request)

				_, err :=
					environment.service.Create(
						context.Background(),
						ownerID,
						request,
					)

				if !errors.Is(
					err,
					application.ErrInvalidInput,
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

func TestApplicationSecretsAreNotJSONSerialized(
	t *testing.T,
) {
	environment := newApplicationTestEnvironment(t)

	ownerID := createTestUser(
		t,
		environment.database,
		"Serialization User",
		"serialization@example.com",
	)

	created, err := environment.service.Create(
		context.Background(),
		ownerID,
		validCreateRequest(),
	)
	if err != nil {
		t.Fatalf(
			"create application: %v",
			err,
		)
	}

	serialized := marshalJSON(t, created)

	for _, secret := range []string{
		deploymentWebhook,
		rollbackWebhook,
		webhookToken,
		"deployment_webhook_url_encrypted",
		"rollback_webhook_url_encrypted",
		"webhook_token_encrypted",
	} {
		if strings.Contains(
			serialized,
			secret,
		) {
			t.Fatalf(
				"serialized application exposed %q\nJSON: %s",
				secret,
				serialized,
			)
		}
	}

	for _, configuredField := range []string{
		`"deployment_webhook_configured":true`,
		`"rollback_webhook_configured":true`,
		`"webhook_token_configured":true`,
	} {
		if !strings.Contains(
			serialized,
			configuredField,
		) {
			t.Fatalf(
				"serialized application missing %s\nJSON: %s",
				configuredField,
				serialized,
			)
		}
	}
}

func newApplicationTestEnvironment(
	t *testing.T,
) applicationTestEnvironment {
	t.Helper()

	database :=
		testutil.OpenTestDatabase(t)

	testutil.ResetDatabase(t, database)

	key := base64.StdEncoding.EncodeToString(
		[]byte(
			"0123456789abcdef0123456789abcdef",
		),
	)

	encryptor, err :=
		appcrypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf(
			"create test encryptor: %v",
			err,
		)
	}

	repository :=
		application.NewRepository(database)

	service := application.NewService(
		repository,
		encryptor,
	)

	return applicationTestEnvironment{
		database: database,
		service:  service,
	}
}

func createTestUser(
	t *testing.T,
	database *pgxpool.Pool,
	name string,
	email string,
) uuid.UUID {
	t.Helper()

	repository := auth.NewRepository(database)

	user, err := repository.CreateUser(
		context.Background(),
		name,
		email,
		"$2a$12$test-password-hash",
	)
	if err != nil {
		t.Fatalf(
			"create test user: %v",
			err,
		)
	}

	return user.ID
}

func validCreateRequest() application.CreateRequest {
	repositoryURL :=
		"https://github.com/example/nimbus"
	currentVersion := "v1.0.0"

	deploymentURL := deploymentWebhook
	rollbackURL := rollbackWebhook
	token := webhookToken

	return application.CreateRequest{
		Name:                      "Nimbus API",
		Description:               "Application reliability API.",
		ApplicationURL:            "https://api.example.com",
		HealthURL:                 "https://api.example.com/health",
		RepositoryURL:             &repositoryURL,
		Environment:               application.EnvironmentProduction,
		CurrentVersion:            &currentVersion,
		MonitoringIntervalSeconds: 60,
		FailureThreshold:          3,
		LatencyThresholdMS:        2000,
		DeploymentWebhookURL:      &deploymentURL,
		RollbackWebhookURL:        &rollbackURL,
		WebhookToken:              &token,
	}
}

func assertSecretsEncrypted(
	t *testing.T,
	database *pgxpool.Pool,
	applicationID uuid.UUID,
) {
	t.Helper()

	const query = `
		SELECT
			deployment_webhook_url_encrypted,
			rollback_webhook_url_encrypted,
			webhook_token_encrypted
		FROM applications
		WHERE id = $1
	`

	var encryptedDeployment string
	var encryptedRollback string
	var encryptedToken string

	err := database.QueryRow(
		context.Background(),
		query,
		applicationID,
	).Scan(
		&encryptedDeployment,
		&encryptedRollback,
		&encryptedToken,
	)
	if err != nil {
		t.Fatalf(
			"read encrypted secrets: %v",
			err,
		)
	}

	tests := []struct {
		name      string
		plaintext string
		encrypted string
	}{
		{
			name:      "deployment webhook",
			plaintext: deploymentWebhook,
			encrypted: encryptedDeployment,
		},
		{
			name:      "rollback webhook",
			plaintext: rollbackWebhook,
			encrypted: encryptedRollback,
		},
		{
			name:      "webhook token",
			plaintext: webhookToken,
			encrypted: encryptedToken,
		},
	}

	for _, test := range tests {
		if test.encrypted == "" {
			t.Fatalf(
				"%s ciphertext was empty",
				test.name,
			)
		}

		if test.encrypted == test.plaintext {
			t.Fatalf(
				"%s was stored in plaintext",
				test.name,
			)
		}

		if strings.Contains(
			test.encrypted,
			test.plaintext,
		) {
			t.Fatalf(
				"%s ciphertext contains plaintext",
				test.name,
			)
		}
	}
}

func marshalJSON(
	t *testing.T,
	value any,
) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf(
			"marshal JSON: %v",
			err,
		)
	}

	return string(encoded)
}

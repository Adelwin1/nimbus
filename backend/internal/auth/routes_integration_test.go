package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adel/nimbus/backend/internal/auth"
	"github.com/adel/nimbus/backend/internal/testutil"
)

const (
	testName     = "Nimbus Test User"
	testEmail    = "nimbus-test@example.com"
	testPassword = "StrongPassword123!"
)

type authResponse struct {
	User struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"user"`

	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func TestAuthRegistrationLoginAndMe(
	t *testing.T,
) {
	router := newAuthRouter(
		t,
		15*time.Minute,
	)

	registerResponse := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/register",
		map[string]any{
			"name":     testName,
			"email":    testEmail,
			"password": testPassword,
		},
		"",
	)

	testutil.AssertStatus(
		t,
		registerResponse,
		http.StatusCreated,
	)

	assertNoPasswordExposure(
		t,
		registerResponse.Body.String(),
	)

	registered := testutil.DecodeJSON[authResponse](t, registerResponse)

	if registered.User.Email != testEmail {
		t.Fatalf(
			"registered email = %q, expected %q",
			registered.User.Email,
			testEmail,
		)
	}

	if registered.User.Name != testName {
		t.Fatalf(
			"registered name = %q, expected %q",
			registered.User.Name,
			testName,
		)
	}

	if registered.User.ID == "" {
		t.Fatal("registered user ID was empty")
	}

	assertTokensPresent(t, registered)

	loginResponse := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/login",
		map[string]any{
			"email":    testEmail,
			"password": testPassword,
		},
		"",
	)

	testutil.AssertStatus(
		t,
		loginResponse,
		http.StatusOK,
	)

	assertNoPasswordExposure(
		t,
		loginResponse.Body.String(),
	)

	loggedIn := testutil.DecodeJSON[authResponse](t, loginResponse)

	assertTokensPresent(t, loggedIn)

	meResponse := performJSONRequest(
		t,
		router,
		http.MethodGet,
		"/me",
		nil,
		loggedIn.AccessToken,
	)

	testutil.AssertStatus(
		t,
		meResponse,
		http.StatusOK,
	)

	meBody := meResponse.Body.String()

	if !strings.Contains(meBody, testEmail) {
		t.Fatalf(
			"/me response did not contain %q\nbody: %s",
			testEmail,
			meBody,
		)
	}

	assertNoPasswordExposure(t, meBody)
}

func TestAuthRejectsDuplicateEmail(
	t *testing.T,
) {
	router := newAuthRouter(
		t,
		15*time.Minute,
	)

	first := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/register",
		map[string]any{
			"name":     testName,
			"email":    testEmail,
			"password": testPassword,
		},
		"",
	)

	testutil.AssertStatus(
		t,
		first,
		http.StatusCreated,
	)

	second := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/register",
		map[string]any{
			"name":     "Duplicate User",
			"email":    testEmail,
			"password": testPassword,
		},
		"",
	)

	testutil.AssertStructuredError(
		t,
		second,
		http.StatusConflict,
		"email_taken",
	)
}

func TestAuthRejectsInvalidCredentials(
	t *testing.T,
) {
	router := newAuthRouter(
		t,
		15*time.Minute,
	)

	registerUser(t, router)

	response := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/login",
		map[string]any{
			"email":    testEmail,
			"password": "IncorrectPassword123!",
		},
		"",
	)

	errorResponse :=
		testutil.AssertStructuredError(
			t,
			response,
			http.StatusUnauthorized,
			"invalid_credentials",
		)

	if strings.Contains(
		errorResponse.Error.Message,
		"IncorrectPassword123!",
	) {
		t.Fatal(
			"invalid credentials response exposed the password",
		)
	}
}

func TestAuthRefreshAndLogout(
	t *testing.T,
) {
	router := newAuthRouter(
		t,
		15*time.Minute,
	)

	registered := registerUser(t, router)

	refreshResponse := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/refresh",
		map[string]any{
			"refresh_token": registered.RefreshToken,
		},
		"",
	)

	testutil.AssertStatus(
		t,
		refreshResponse,
		http.StatusOK,
	)

	refreshed := testutil.DecodeJSON[authResponse](t, refreshResponse)

	assertTokensPresent(t, refreshed)

	logoutResponse := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/logout",
		map[string]any{
			"refresh_token": refreshed.RefreshToken,
		},
		"",
	)

	testutil.AssertStatus(
		t,
		logoutResponse,
		http.StatusNoContent,
	)

	reuseResponse := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/refresh",
		map[string]any{
			"refresh_token": refreshed.RefreshToken,
		},
		"",
	)

	testutil.AssertStructuredError(
		t,
		reuseResponse,
		http.StatusUnauthorized,
		"invalid_token",
	)
}

func TestAuthRejectsInvalidRefreshToken(
	t *testing.T,
) {
	router := newAuthRouter(
		t,
		15*time.Minute,
	)

	response := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/refresh",
		map[string]any{
			"refresh_token": "not-a-valid-token",
		},
		"",
	)

	testutil.AssertStructuredError(
		t,
		response,
		http.StatusUnauthorized,
		"invalid_token",
	)
}

func TestAuthMeRejectsUnauthorizedRequests(
	t *testing.T,
) {
	router := newAuthRouter(
		t,
		15*time.Minute,
	)

	tests := []struct {
		name          string
		authorization string
	}{
		{
			name: "missing authorization",
		},
		{
			name:          "wrong authentication scheme",
			authorization: "Basic abc123",
		},
		{
			name:          "empty bearer token",
			authorization: "Bearer ",
		},
		{
			name:          "malformed bearer token",
			authorization: "Bearer not-a-real-access-token",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				request := testutil.JSONRequest(
					t,
					http.MethodGet,
					"/me",
					nil,
				)

				if test.authorization != "" {
					request.Header.Set(
						"Authorization",
						test.authorization,
					)
				}

				response :=
					httptest.NewRecorder()

				router.ServeHTTP(
					response,
					request,
				)

				testutil.AssertStructuredError(
					t,
					response,
					http.StatusUnauthorized,
					"unauthorized",
				)
			},
		)
	}
}

func TestAuthRejectsExpiredAccessToken(
	t *testing.T,
) {
	router := newAuthRouter(
		t,
		-time.Minute,
	)

	registered := registerUser(t, router)

	response := performJSONRequest(
		t,
		router,
		http.MethodGet,
		"/me",
		nil,
		registered.AccessToken,
	)

	testutil.AssertStructuredError(
		t,
		response,
		http.StatusUnauthorized,
		"unauthorized",
	)
}

func newAuthRouter(
	t *testing.T,
	accessTokenTTL time.Duration,
) http.Handler {
	t.Helper()

	database :=
		testutil.OpenTestDatabase(t)

	testutil.ResetDatabase(t, database)

	repository := auth.NewRepository(database)

	service := auth.NewService(
		repository,
		"nimbus-test-access-secret-that-is-not-production",
		"nimbus-test-refresh-secret-that-is-not-production",
		accessTokenTTL,
		24*time.Hour,
	)

	handler := auth.NewHandler(service)

	return auth.Routes(
		handler,
		service,
		nil,
	)
}

func registerUser(
	t *testing.T,
	router http.Handler,
) authResponse {
	t.Helper()

	response := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/register",
		map[string]any{
			"name":     testName,
			"email":    testEmail,
			"password": testPassword,
		},
		"",
	)

	testutil.AssertStatus(
		t,
		response,
		http.StatusCreated,
	)

	payload := testutil.DecodeJSON[authResponse](t, response)

	assertTokensPresent(t, payload)

	return payload
}

func performJSONRequest(
	t *testing.T,
	router http.Handler,
	method string,
	target string,
	body any,
	accessToken string,
) *httptest.ResponseRecorder {
	t.Helper()

	request := testutil.JSONRequest(
		t,
		method,
		target,
		body,
	)

	if accessToken != "" {
		request.Header.Set(
			"Authorization",
			"Bearer "+accessToken,
		)
	}

	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	return response
}

func assertTokensPresent(
	t *testing.T,
	response authResponse,
) {
	t.Helper()

	if response.AccessToken == "" {
		t.Fatal("access token was empty")
	}

	if response.RefreshToken == "" {
		t.Fatal("refresh token was empty")
	}
}

func assertNoPasswordExposure(
	t *testing.T,
	body string,
) {
	t.Helper()

	for _, forbidden := range []string{
		testPassword,
		"password_hash",
		"passwordHash",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf(
				"response exposed %q\nbody: %s",
				forbidden,
				body,
			)
		}
	}
}

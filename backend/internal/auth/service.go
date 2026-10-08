package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid token")
	ErrExpiredToken       = errors.New("token expired")
	ErrRevokedSession     = errors.New("session revoked")
	ErrInvalidInput       = errors.New("invalid input")
)

type Service struct {
	repository      *Repository
	accessSecret    []byte
	refreshSecret   []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

type jwtClaims struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id,omitempty"`
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

func NewService(
	repository *Repository,
	accessSecret string,
	refreshSecret string,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
) *Service {
	return &Service{
		repository:      repository,
		accessSecret:    []byte(accessSecret),
		refreshSecret:   []byte(refreshSecret),
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
	}
}

func (s *Service) Register(
	ctx context.Context,
	request RegisterRequest,
) (AuthResponse, error) {
	name := strings.TrimSpace(request.Name)
	email := strings.ToLower(strings.TrimSpace(request.Email))
	password := request.Password

	if err := validateRegistration(name, email, password); err != nil {
		return AuthResponse{}, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return AuthResponse{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.repository.CreateUser(
		ctx,
		name,
		email,
		string(passwordHash),
	)
	if err != nil {
		return AuthResponse{}, err
	}

	return s.createAuthenticatedSession(ctx, user)
}

func (s *Service) Login(
	ctx context.Context,
	request LoginRequest,
) (AuthResponse, error) {
	email := strings.ToLower(strings.TrimSpace(request.Email))

	if email == "" || request.Password == "" {
		return AuthResponse{}, ErrInvalidCredentials
	}

	user, err := s.repository.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return AuthResponse{}, ErrInvalidCredentials
		}

		return AuthResponse{}, err
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(request.Password),
	)
	if err != nil {
		return AuthResponse{}, ErrInvalidCredentials
	}

	return s.createAuthenticatedSession(ctx, user)
}

func (s *Service) Refresh(
	ctx context.Context,
	refreshToken string,
) (AuthResponse, error) {
	claims, err := s.parseToken(
		refreshToken,
		s.refreshSecret,
		"refresh",
	)
	if err != nil {
		return AuthResponse{}, err
	}

	sessionID, err := uuid.Parse(claims.SessionID)
	if err != nil {
		return AuthResponse{}, ErrInvalidToken
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return AuthResponse{}, ErrInvalidToken
	}

	session, err := s.repository.FindSessionByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return AuthResponse{}, ErrInvalidToken
		}

		return AuthResponse{}, err
	}

	if session.RevokedAt != nil {
		return AuthResponse{}, ErrRevokedSession
	}

	if time.Now().After(session.ExpiresAt) {
		return AuthResponse{}, ErrExpiredToken
	}

	if session.UserID != userID {
		return AuthResponse{}, ErrInvalidToken
	}

	expectedHash := hashToken(refreshToken)
	if session.RefreshTokenHash != expectedHash {
		return AuthResponse{}, ErrInvalidToken
	}

	user, err := s.repository.FindUserByID(ctx, userID)
	if err != nil {
		return AuthResponse{}, err
	}

	accessToken, err := s.generateToken(
		user.ID,
		session.ID,
		"access",
		s.accessSecret,
		s.accessTokenTTL,
	)
	if err != nil {
		return AuthResponse{}, err
	}

	return AuthResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *Service) Logout(
	ctx context.Context,
	refreshToken string,
) error {
	claims, err := s.parseToken(
		refreshToken,
		s.refreshSecret,
		"refresh",
	)
	if err != nil {
		return err
	}

	sessionID, err := uuid.Parse(claims.SessionID)
	if err != nil {
		return ErrInvalidToken
	}

	err = s.repository.RevokeSession(ctx, sessionID)
	if errors.Is(err, ErrSessionNotFound) {
		return ErrInvalidToken
	}

	return err
}

func (s *Service) GetUser(
	ctx context.Context,
	userID uuid.UUID,
) (User, error) {
	return s.repository.FindUserByID(ctx, userID)
}

func (s *Service) ParseAccessToken(token string) (uuid.UUID, error) {
	claims, err := s.parseToken(
		token,
		s.accessSecret,
		"access",
	)
	if err != nil {
		return uuid.Nil, err
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}

	return userID, nil
}

func (s *Service) createAuthenticatedSession(
	ctx context.Context,
	user User,
) (AuthResponse, error) {
	sessionID := uuid.New()
	sessionExpiry := time.Now().Add(s.refreshTokenTTL)

	refreshToken, err := s.generateToken(
		user.ID,
		sessionID,
		"refresh",
		s.refreshSecret,
		s.refreshTokenTTL,
	)
	if err != nil {
		return AuthResponse{}, err
	}

	session, err := s.repository.CreateSessionWithID(
		ctx,
		sessionID,
		user.ID,
		hashToken(refreshToken),
		sessionExpiry,
	)
	if err != nil {
		return AuthResponse{}, err
	}

	accessToken, err := s.generateToken(
		user.ID,
		session.ID,
		"access",
		s.accessSecret,
		s.accessTokenTTL,
	)
	if err != nil {
		return AuthResponse{}, err
	}

	return AuthResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *Service) generateToken(
	userID uuid.UUID,
	sessionID uuid.UUID,
	tokenType string,
	secret []byte,
	ttl time.Duration,
) (string, error) {
	now := time.Now()

	claims := jwtClaims{
		UserID:    userID.String(),
		SessionID: sessionID.String(),
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "nimbus-api",
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return signedToken, nil
}

func (s *Service) parseToken(
	tokenString string,
	secret []byte,
	expectedType string,
) (jwtClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&jwtClaims{},
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, ErrInvalidToken
			}

			return secret, nil
		},
	)

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return jwtClaims{}, ErrExpiredToken
		}

		return jwtClaims{}, ErrInvalidToken
	}

	claims, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return jwtClaims{}, ErrInvalidToken
	}

	if claims.TokenType != expectedType {
		return jwtClaims{}, ErrInvalidToken
	}

	return *claims, nil
}

func validateRegistration(
	name string,
	email string,
	password string,
) error {
	if len(name) < 2 || len(name) > 120 {
		return fmt.Errorf("%w: name must be between 2 and 120 characters", ErrInvalidInput)
	}

	parsedEmail, err := mail.ParseAddress(email)
	if err != nil || parsedEmail.Address != email {
		return fmt.Errorf("%w: invalid email address", ErrInvalidInput)
	}

	if len(password) < 8 {
		return fmt.Errorf("%w: password must contain at least 8 characters", ErrInvalidInput)
	}

	if len(password) > 72 {
		return fmt.Errorf("%w: password must not exceed 72 characters", ErrInvalidInput)
	}

	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateExternalSession is called only after a server-side identity provider flow
// verifies the identity and consumes a browser-bound, single-use exchange.
func (s *Service) CreateExternalSession(ctx context.Context, userID uuid.UUID) (AuthResponse, error) {
	user, err := s.repository.FindUserByID(ctx, userID)
	if err != nil {
		return AuthResponse{}, err
	}
	return s.createAuthenticatedSession(ctx, user)
}

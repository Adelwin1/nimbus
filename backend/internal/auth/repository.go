package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrEmailTaken      = errors.New("email already registered")
	ErrSessionNotFound = errors.New("session not found")
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(
	ctx context.Context,
	name string,
	email string,
	passwordHash string,
) (User, error) {
	const query = `
		INSERT INTO users (
			name,
			email,
			password_hash
		)
		VALUES ($1, $2, $3)
		RETURNING
			id,
			name,
			email,
			password_hash,
			created_at,
			updated_at
	`

	var user User

	err := r.db.QueryRow(
		ctx,
		query,
		name,
		email,
		passwordHash,
	).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrEmailTaken
		}

		return User{}, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (r *Repository) FindUserByEmail(
	ctx context.Context,
	email string,
) (User, error) {
	const query = `
		SELECT
			id,
			name,
			email,
			password_hash,
			created_at,
			updated_at
		FROM users
		WHERE email = $1
	`

	var user User

	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}

	if err != nil {
		return User{}, fmt.Errorf("find user by email: %w", err)
	}

	return user, nil
}

func (r *Repository) FindUserByID(
	ctx context.Context,
	userID uuid.UUID,
) (User, error) {
	const query = `
		SELECT
			id,
			name,
			email,
			password_hash,
			created_at,
			updated_at
		FROM users
		WHERE id = $1
	`

	var user User

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}

	if err != nil {
		return User{}, fmt.Errorf("find user by id: %w", err)
	}

	return user, nil
}

func (r *Repository) CreateSessionWithID(
	ctx context.Context,
	sessionID uuid.UUID,
	userID uuid.UUID,
	refreshTokenHash string,
	expiresAt time.Time,
) (Session, error) {
	const query = `
		INSERT INTO sessions (
			id,
			user_id,
			refresh_token_hash,
			expires_at
		)
		VALUES ($1, $2, $3, $4)
		RETURNING
			id,
			user_id,
			refresh_token_hash,
			expires_at,
			revoked_at,
			created_at
	`

	var session Session

	err := r.db.QueryRow(
		ctx,
		query,
		sessionID,
		userID,
		refreshTokenHash,
		expiresAt,
	).Scan(
		&session.ID,
		&session.UserID,
		&session.RefreshTokenHash,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.CreatedAt,
	)

	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}

	return session, nil
}

func (r *Repository) FindSessionByID(
	ctx context.Context,
	sessionID uuid.UUID,
) (Session, error) {
	const query = `
		SELECT
			id,
			user_id,
			refresh_token_hash,
			expires_at,
			revoked_at,
			created_at
		FROM sessions
		WHERE id = $1
	`

	var session Session

	err := r.db.QueryRow(ctx, query, sessionID).Scan(
		&session.ID,
		&session.UserID,
		&session.RefreshTokenHash,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}

	if err != nil {
		return Session{}, fmt.Errorf("find session by id: %w", err)
	}

	return session, nil
}

func (r *Repository) RevokeSession(
	ctx context.Context,
	sessionID uuid.UUID,
) error {
	const query = `
		UPDATE sessions
		SET revoked_at = NOW()
		WHERE id = $1
		  AND revoked_at IS NULL
	`

	commandTag, err := r.db.Exec(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}

	return nil
}

func isUniqueViolation(err error) bool {
	type sqlStateError interface {
		SQLState() string
	}

	var databaseError sqlStateError

	if errors.As(err, &databaseError) {
		return databaseError.SQLState() == "23505"
	}

	return false
}

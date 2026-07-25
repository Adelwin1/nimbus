package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type AccessTokenParser interface {
	ParseAccessToken(token string) (uuid.UUID, error)
}

const UserIDKey contextKey = "user_id"

func Authenticate(
	tokenParser AccessTokenParser,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authorization := r.Header.Get("Authorization")

			if authorization == "" {
				writeUnauthorized(w, r)
				return
			}

			parts := strings.SplitN(authorization, " ", 2)

			if len(parts) != 2 ||
				!strings.EqualFold(parts[0], "Bearer") ||
				strings.TrimSpace(parts[1]) == "" {
				writeUnauthorized(w, r)
				return
			}

			userID, err := tokenParser.ParseAccessToken(
				strings.TrimSpace(parts[1]),
			)
			if err != nil {
				writeUnauthorized(w, r)
				return
			}

			ctx := context.WithValue(
				r.Context(),
				UserIDKey,
				userID,
			)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserID(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(UserIDKey).(uuid.UUID)
	return userID, ok
}

func writeUnauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)

	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":       "unauthorized",
			"message":    "Authentication is required.",
			"request_id": GetRequestID(r.Context()),
		},
	})
}

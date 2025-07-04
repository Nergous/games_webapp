package middleware

import (
	"context"
	"net/http"
	"strings"

	"games_webapp/internal/clients/sso/grpc"
)

type AuthMiddleware struct {
	ssoClient *grpc.Client
}

func NewAuthMiddleware(client *grpc.Client) *AuthMiddleware {
	return &AuthMiddleware{ssoClient: client}
}

type contextKey string

const UserIDKey = contextKey("userID")

func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(UserIDKey).(int64)
	return id, ok
}

func (m *AuthMiddleware) ValidateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Missing of invalid Authorization header", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		userID, valid, err := m.ssoClient.ValidateToken(r.Context(), token)
		if err != nil || !valid {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

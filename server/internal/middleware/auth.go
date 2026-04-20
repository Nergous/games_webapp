// internal/middleware/auth.go
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"games_webapp/internal/client/sso/grpc"
)

type AuthMiddleware struct {
	ssoClient *grpc.Client
	appID     uint32
	log       *slog.Logger
}

func NewAuthMiddleware(client *grpc.Client, appID uint32, log *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{ssoClient: client, appID: appID, log: log}
}

type contextKey string

const (
	UserIDKey  = contextKey("userID")
	IsAdminKey = contextKey("isAdmin")
)

func UserIDFromContext(ctx context.Context) (int, bool) {
	id, ok := ctx.Value(UserIDKey).(int)
	return id, ok
}

func (m *AuthMiddleware) ValidateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Отсутствует или неправильный заголовок авторизации", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		userID, valid, err := m.ssoClient.ValidateToken(r.Context(), token)
		if err != nil || !valid {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		isAdmin, err := m.ssoClient.IsAdmin(r.Context(), userID, m.appID)
		if err != nil {
			m.log.Warn("IsAdmin check failed, defaulting to non-admin",
				slog.Any("userID", userID),
				slog.Any("error", err))
			isAdmin = false
		}

		ctx := context.WithValue(r.Context(), UserIDKey, int(userID))
		ctx = context.WithValue(ctx, IsAdminKey, isAdmin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

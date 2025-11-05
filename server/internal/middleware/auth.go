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

const (
	UserIDKey  = contextKey("userID")
	IsAdminKey = contextKey("isAdmin")
)

func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(UserIDKey).(int64)
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

		isAdmin, err := m.ssoClient.IsAdmin(r.Context(), userID)
		if err != nil {
			http.Error(w, "ошибка распознавания прав", http.StatusInternalServerError)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, IsAdminKey, isAdmin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

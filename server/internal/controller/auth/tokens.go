// internal/controller/auth/tokens.go
package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"games_webapp/internal/controller"
	g_errors "games_webapp/internal/errors"
)

const (
	refreshTokenCookieName = "refresh_token"
	refreshTokenMaxAge     = 30 * 24 * 60 * 60
)

type TokensController struct {
	log    *slog.Logger
	client GRPCTokensClient
}

type GRPCTokensClient interface {
	RefreshToken(ctx context.Context, refreshToken string) (string, string, error)
}

func NewTokensController(log *slog.Logger, client GRPCTokensClient) *TokensController {
	return &TokensController{log: log, client: client}
}

type RefreshResponse struct {
	AccessToken string `json:"access_token"`
}

func (c *TokensController) Refresh(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.auth.Refresh"
	log := c.log.With(slog.String("operation", op))

	refreshCookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		se := g_errors.Wrap(op, g_errors.CodeUnauthorized, "", err)
		controller.WriteError(w, log, se)
		return
	}

	if refreshCookie.Value == "" {
		se := g_errors.NewWithInfo(op, g_errors.CodeUnauthorized, "",
			map[string]any{"info": g_errors.InvalidRefreshToken})
		controller.WriteError(w, log, se)
		return
	}

	accessToken, newRefreshToken, err := c.client.RefreshToken(r.Context(), refreshCookie.Value)
	if err != nil {
		se := g_errors.NewFromGRPC(op, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("err", se.Err))

		// сбрасываем невалидный refresh token
		http.SetCookie(w, &http.Cookie{
			Name:        refreshTokenCookieName,
			Value:       "",
			Path:        "/",
			MaxAge:      -1,
			HttpOnly:    true,
			Secure:      true,
			SameSite:    http.SameSiteNoneMode,
			Partitioned: true,
		})

		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:        refreshTokenCookieName,
		Value:       newRefreshToken,
		Path:        "/",
		MaxAge:      refreshTokenMaxAge,
		HttpOnly:    true,
		Secure:      true,
		SameSite:    http.SameSiteNoneMode,
		Partitioned: true,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(RefreshResponse{AccessToken: accessToken})
}

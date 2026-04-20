// internal/controller/auth/auth.go
package auth

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"games_webapp/internal/controller"
	g_errors "games_webapp/internal/errors"
)

const maxUploadSize = 10 << 20 // 10 MiB

type AuthController struct {
	log     *slog.Logger
	client  GRPCAuthClient
	uploads IUploadsAuth
	appID   uint32
}

type GRPCAuthClient interface {
	Login(ctx context.Context, email, password string, appID uint32) (string, string, error)
	Logout(ctx context.Context, token string) error
	Register(ctx context.Context, email, password, steamURL, pathToPhoto string) (uint32, error)
}

type IUploadsAuth interface {
	SaveImage(image []byte, filename string) error
	GenerateImageFilename(url, contentType string) string
}

func NewAuthController(log *slog.Logger, client GRPCAuthClient, uploads IUploadsAuth, appID uint32) *AuthController {
	return &AuthController{log: log, client: client, uploads: uploads, appID: appID}
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	SteamURL string `json:"steam_url"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	AppId    uint32 `json:"app_id"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
}

func (c *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.auth.Register"
	log := c.log.With(slog.String("operation", op))

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		controller.WriteError(w, log, se)
		return
	}

	request := RegisterRequest{
		Email:    r.FormValue("email"),
		Password: r.FormValue("password"),
		SteamURL: r.FormValue("steam_url"),
	}

	if err := c.validateRegister(&request); err != nil {
		se, _ := g_errors.AsServiceError(err)
		controller.WriteError(w, log, se)
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidImage, err)
		controller.WriteError(w, log, se)
		return
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInternal, g_errors.InvalidImage, err)
		controller.WriteError(w, log, se)
		return
	}

	imageFilename := c.uploads.GenerateImageFilename(request.Email, http.DetectContentType(imageData))
	if err := c.uploads.SaveImage(imageData, imageFilename); err != nil {
		se, _ := g_errors.AsServiceError(err)
		controller.WriteError(w, log, se)
		return
	}

	cleanedEmail := strings.ToLower(strings.TrimSpace(request.Email))

	userID, err := c.client.Register(r.Context(), cleanedEmail, request.Password, request.SteamURL, imageFilename)
	if err != nil {
		se := g_errors.NewFromGRPC(op, err) // ← единая точка обработки gRPC-ошибок
		controller.WriteError(w, log, se)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(userID)
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.auth.Login"
	log := c.log.With(slog.String("operation", op))

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		controller.WriteError(w, log, se)
		return
	}

	if err := c.validateLogin(&req); err != nil {
		se, _ := g_errors.AsServiceError(err)
		controller.WriteError(w, log, se)
		return
	}

	cleanedEmail := strings.ToLower(strings.TrimSpace(req.Email))

	accessToken, refreshToken, err := c.client.Login(r.Context(), cleanedEmail, req.Password, req.AppId)
	if err != nil {
		se := g_errors.NewFromGRPC(op, err)
		controller.WriteError(w, log, se)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:        refreshTokenCookieName,
		Value:       refreshToken,
		Path:        "/",
		MaxAge:      refreshTokenMaxAge,
		HttpOnly:    true,
		Secure:      true,
		SameSite:    http.SameSiteNoneMode,
		Partitioned: true,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(LoginResponse{AccessToken: accessToken})
}

func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.auth.Logout"
	log := c.log.With(slog.String("operation", op))

	refreshCookie, err := r.Cookie(refreshTokenCookieName)
	if err == nil && refreshCookie.Value != "" {
		err = c.client.Logout(r.Context(), refreshCookie.Value)
		if err != nil {
			se := g_errors.NewFromGRPC(op, err)
			log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("err", se.Err))
			http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
			return
		}
	}

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

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "logged out successfully"})
}

// ============================================================================
// VALIDATION
// ============================================================================

func (c *AuthController) validateRegister(request *RegisterRequest) error {
	const op = "controllers.auth.validateRegister"

	if strings.TrimSpace(request.Email) == "" {
		return g_errors.NewWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidEmail,
			map[string]any{"email": request.Email})
	}
	if request.Password == "" {
		return g_errors.NewWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidPassword,
			map[string]any{"password": "[REDACTED]"})
	}
	if strings.TrimSpace(request.SteamURL) == "" {
		return g_errors.NewWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidSteamURL,
			map[string]any{"steam_url": request.SteamURL})
	}

	return nil
}

func (c *AuthController) validateLogin(request *LoginRequest) error {
	const op = "controllers.auth.validateLogin"

	if strings.TrimSpace(request.Email) == "" {
		return g_errors.NewWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidEmail,
			map[string]any{"email": request.Email})
	}
	if request.Password == "" {
		return g_errors.NewWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidPassword,
			map[string]any{"password": "[REDACTED]"})
	}
	if request.AppId != c.appID {
		return g_errors.NewWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidAppID,
			map[string]any{"app_id": request.AppId})
	}

	return nil
}

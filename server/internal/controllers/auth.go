package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"games_webapp/internal/middleware"
	"games_webapp/internal/storage/uploads"

	ssov1 "github.com/Nergous/sso_protos/gen/go/sso"
)

type AuthController struct {
	log     *slog.Logger
	client  GRPCClient
	uploads uploads.IUploads
}

type GRPCClient interface {
	Login(ctx context.Context, email, password string, appID int32) (string, string, error)
	Logout(ctx context.Context, token string) error
	Register(ctx context.Context, email, password, steamURL, pathToPhoto string) (int64, error)
	GetUserInfo(ctx context.Context, userID int64) (email, steamURL, pathToPhoto string, err error)
	GetUsers(ctx context.Context) (*ssov1.GetAllUsersResponse, error)
	UpdateUser(ctx context.Context, user *ssov1.UpdateUserRequest) (*ssov1.UpdateUserResponse, error)
	RefreshToken(ctx context.Context, refreshToken string) (string, string, error)
	DeleteUser(ctx context.Context, user *ssov1.DeleteUserRequest) (*ssov1.DeleteUserResponse, error)
}

func NewAuthController(log *slog.Logger, client GRPCClient, uploads uploads.IUploads) *AuthController {
	return &AuthController{log: log, client: client, uploads: uploads}
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	SteamURL string `json:"steam_url"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
}

type RefreshResponse struct {
	AccessToken string `json:"access_token"`
}

const (
	refreshTokenCookieName = "refresh_token"
	refreshTokenMaxAge     = 30 * 24 * 60 * 60
)

func (c *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.auth.Register"

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		c.log.Error(ErrParsingForm.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrRegister.Error(), http.StatusBadRequest)
		return
	}

	request := RegisterRequest{
		Email:    r.FormValue("email"),
		Password: r.FormValue("password"),
		SteamURL: r.FormValue("steam_url"),
	}

	if request.Email == "" {
		c.log.Error(ErrMissingEmail.Error(), slog.String("operation", op))
		http.Error(w, ErrRegister.Error(), http.StatusBadRequest)
		return
	}

	if request.Password == "" {
		c.log.Error(ErrMissingPassword.Error(), slog.String("operation", op))
		http.Error(w, ErrRegister.Error(), http.StatusBadRequest)
		return
	}

	if request.SteamURL == "" {
		c.log.Error(ErrMissingSteamURL.Error(), slog.String("operation", op))
		http.Error(w, ErrRegister.Error(), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		c.log.Error(ErrMissingImage.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrRegister.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		c.log.Error(ErrReadImage.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrRegister.Error(), http.StatusInternalServerError)
		return
	}

	imageFilename := generatePhotoFilename(request.Email)
	if err := c.uploads.SaveImage(imageData, imageFilename); err != nil {
		c.log.Error(ErrSaveImage.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrRegister.Error(), http.StatusInternalServerError)
		return
	}

	cleanedEmail := strings.ToLower(strings.TrimSpace(request.Email))

	userID, err := c.client.Register(r.Context(), cleanedEmail, request.Password, request.SteamURL, imageFilename)
	if err != nil {
		c.log.Error("sso.Register failed", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrRegister.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(userID); err != nil {
		c.log.Error("encoding response", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrRegister.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.auth.Login"

	var req ssov1.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.log.Error(ErrParsingJSON.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrLogin.Error(), http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" || req.AppId == 0 {
		c.log.Error(ErrInvalidRequest.Error(), slog.String("operation", op))
		http.Error(w, ErrLogin.Error(), http.StatusBadRequest)
		return
	}

	cleanedEmail := strings.ToLower(strings.TrimSpace(req.Email))

	accessToken, refreshToken, err := c.client.Login(r.Context(), cleanedEmail, req.Password, req.AppId)
	if err != nil {
		c.log.Error("sso.Login failed", slog.String("error", err.Error()), slog.String("operation", op))
		http.Error(w, ErrLogin.Error(), http.StatusInternalServerError)
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

	response := LoginResponse{
		AccessToken: accessToken,
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		c.log.Error(ErrLogin.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrLogin.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	// Получаем refresh token из cookie для удаления его из базы
	refreshCookie, err := r.Cookie(refreshTokenCookieName)
	if err == nil && refreshCookie.Value != "" {
		c.client.Logout(r.Context(), refreshCookie.Value)
	}

	// Удаляем refresh token cookie
	http.SetCookie(w, &http.Cookie{
		Name:        refreshTokenCookieName,
		Value:       "",
		Path:        "/",
		MaxAge:      -1, // Удалить cookie
		HttpOnly:    true,
		Secure:      true,
		SameSite:    http.SameSiteNoneMode,
		Partitioned: true,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "logged out successfully"})
}

func (c *AuthController) Refresh(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.auth.Refresh"

	// Получаем refresh token из cookie
	refreshCookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		c.log.Error("refresh token cookie not found", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, "refresh token required", http.StatusUnauthorized)
		return
	}

	refreshToken := refreshCookie.Value
	if refreshToken == "" {
		http.Error(w, "refresh token is empty", http.StatusUnauthorized)
		return
	}

	// Обновляем токены
	accessToken, newRefreshToken, err := c.client.RefreshToken(r.Context(), refreshToken)
	if err != nil {
		c.log.Error("sso.Refresh failed", slog.String("error", err.Error()))

		// Если refresh token невалидный, удаляем cookie
		http.SetCookie(w, &http.Cookie{
			Name:        refreshTokenCookieName,
			Value:       "",
			Path:        "/",
			MaxAge:      -1, // Удалить cookie
			HttpOnly:    true,
			Secure:      true,
			SameSite:    http.SameSiteNoneMode,
			Partitioned: true,
		})

		http.Error(w, "failed to refresh tokens", http.StatusUnauthorized)
		return
	}

	// Устанавливаем новый refresh token в cookie
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

	// Возвращаем новый access token
	response := RefreshResponse{
		AccessToken: accessToken,
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		c.log.Error("failed to encode response", slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

type GetUserInfoResponse struct {
	Email    string `json:"email"`
	SteamURL string `json:"steam_url"`
	Photo    string `json:"photo"`
}

func (c *AuthController) GetUserInfo(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	var user GetUserInfoResponse
	var err error

	user.Email, user.SteamURL, user.Photo, err = c.client.GetUserInfo(r.Context(), userID)
	if err != nil {
		c.log.Error("sso.GetUserInfo failed", slog.String("error", err.Error()))
		http.Error(w, ErrGetUserInfo.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(user); err != nil {
		c.log.Error(ErrGetUserInfo.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetUserInfo.Error(), http.StatusInternalServerError)
		return
	}
}

type User struct {
	Id          int64  `json:"id"`
	Email       string `json:"email"`
	SteamURL    string `json:"steam_url"`
	PathToPhoto string `json:"path_to_photo"`
	IsAdmin     bool   `json:"is_admin"`
}

type GetUsersResponse struct {
	Users []User `json:"users"`
}

func (c *AuthController) GetUsers(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	isAdmin, ok := r.Context().Value(middleware.IsAdminKey).(bool)
	if !ok {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	if !isAdmin {
		http.Error(w, ErrForbidden.Error(), http.StatusForbidden)
		return
	}

	var users GetUsersResponse
	var err error

	resp, err := c.client.GetUsers(r.Context())
	if err != nil {
		c.log.Error("sso.GetUsers failed", slog.String("error", err.Error()))
		http.Error(w, ErrGetUsers.Error(), http.StatusInternalServerError)
		return
	}

	for _, user := range resp.User {
		users.Users = append(users.Users, User{
			Id:          user.Id,
			Email:       user.Email,
			SteamURL:    user.SteamUrl,
			PathToPhoto: user.PathToPhoto,
			IsAdmin:     user.IsAdmin,
		})
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(users); err != nil {
		c.log.Error(ErrGetUserInfo.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetUserInfo.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *AuthController) UpdateUser(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	isAdmin, ok := r.Context().Value(middleware.IsAdminKey).(bool)
	if !ok {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	if !isAdmin {
		http.Error(w, ErrForbidden.Error(), http.StatusForbidden)
		return
	}

	var user *ssov1.UpdateUserRequest

	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		c.log.Error("ошибка парсинга JSON тела", slog.String("error", err.Error()))
		http.Error(w, ErrUpdateUser.Error(), http.StatusBadRequest)
		return
	}

	_, err := c.client.UpdateUser(r.Context(), user)
	if err != nil {
		c.log.Error("sso.UpdateUser failed", slog.String("error", err.Error()))
		http.Error(w, ErrUpdateUser.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (c *AuthController) DeleteUser(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	isAdmin, ok := r.Context().Value(middleware.IsAdminKey).(bool)
	if !ok {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	if !isAdmin {
		http.Error(w, ErrForbidden.Error(), http.StatusForbidden)
		return
	}

	var user *ssov1.DeleteUserRequest

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		c.log.Error(ErrInvalidURL.Error(), slog.String("operation", "controllers.auth.DeleteUser"))
		http.Error(w, ErrInvalidURL.Error(), http.StatusBadRequest)
		return
	}
	id := parts[3]

	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.log.Error(
			ErrInvalidID.Error(),
			slog.String("operation", "controllers.auth.DeleteUser"),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrInvalidID.Error(), http.StatusBadRequest)
		return
	}

	user = &ssov1.DeleteUserRequest{
		Id: idInt,
	}

	_, err = c.client.DeleteUser(r.Context(), user)
	if err != nil {
		c.log.Error("sso.DeleteUser failed", slog.String("error", err.Error()))
		http.Error(w, ErrDeleteUser.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func generatePhotoFilename(email string) string {
	// Удаляем все недопустимые символы из email для имени файла
	cleanEmail := strings.Map(func(r rune) rune {
		switch {
		case r == '@' || r == '.':
			return '_'
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			return r
		default:
			return -1
		}
	}, email)

	timestamp := time.Now().Format("20060102150405")
	hash := sha256.Sum256([]byte(cleanEmail + timestamp))
	cleanEmail = fmt.Sprintf("%x", hash[:8])

	return cleanEmail + ".jpg"
}

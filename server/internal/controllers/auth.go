package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	Login(ctx context.Context, email, password string, appID int32) (string, error)
	Register(ctx context.Context, email, password, steamURL, pathToPhoto string) (int64, error)
	GetUserInfo(ctx context.Context, userID int64) (email, steamURL, pathToPhoto string, err error)
}

func NewAuthController(log *slog.Logger, client GRPCClient, uploads uploads.IUploads) *AuthController {
	return &AuthController{log: log, client: client, uploads: uploads}
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	SteamURL string `json:"steam_url"`
}

func (c *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.auth.Register"

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		c.log.Error(ErrParsingForm.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrParsingForm.Error(), http.StatusBadRequest)
		return
	}

	request := RegisterRequest{
		Email:    r.FormValue("email"),
		Password: r.FormValue("password"),
		SteamURL: r.FormValue("steam_url"),
	}

	if request.Email == "" {
		http.Error(w, ErrMissingEmail.Error(), http.StatusBadRequest)
		return
	}

	if request.Password == "" {
		http.Error(w, ErrMissingPassword.Error(), http.StatusBadRequest)
		return
	}

	if request.SteamURL == "" {
		http.Error(w, ErrMissingSteamURL.Error(), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		c.log.Error("image not provided", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrMissingImage.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		c.log.Error("failed to read image", slog.String("error", err.Error()))
		http.Error(w, ErrReadImage.Error(), http.StatusInternalServerError)
		return
	}

	imageFilename := generatePhotoFilename(request.Email)
	if err := c.uploads.SaveImage(imageData, imageFilename); err != nil {
		c.log.Error("failed to save image", slog.String("error", err.Error()))
		http.Error(w, ErrSaveImage.Error(), http.StatusInternalServerError)
		return
	}

	userID, err := c.client.Register(r.Context(), request.Email, request.Password, request.SteamURL, imageFilename)
	if err != nil {
		c.log.Error("sso.Register failed", slog.String("error", err.Error()))
		http.Error(w, ErrRegister.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(userID); err != nil {
		c.log.Error(ErrRegister.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrRegister.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.auth.Login"

	var req ssov1.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.log.Error("ошибка парсинга JSON тела", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrParsingJSON.Error(), http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" || req.AppId == 0 {
		http.Error(w, ErrInvalidRequest.Error(), http.StatusBadRequest)
		return
	}

	token, err := c.client.Login(r.Context(), req.Email, req.Password, req.AppId)
	if err != nil {
		c.log.Error("sso.Login failed", slog.String("error", err.Error()))
		http.Error(w, ErrLogin.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(token); err != nil {
		c.log.Error(ErrLogin.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrLogin.Error(), http.StatusInternalServerError)
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

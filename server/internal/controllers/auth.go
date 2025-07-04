package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

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
		c.log.Error(ErrCreate.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, "cannot parse form", http.StatusBadRequest)
		return
	}

	request := RegisterRequest{
		Email:    r.FormValue("email"),
		Password: r.FormValue("password"),
		SteamURL: r.FormValue("steam_url"),
	}

	if request.Password == "" {
		http.Error(w, "missing password", http.StatusBadRequest)
		return
	}

	if request.SteamURL == "" {
		http.Error(w, "missing steam url", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		c.log.Error("image not provided", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, "image not provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		c.log.Error("failed to read image", slog.String("error", err.Error()))
		http.Error(w, "failed to read image", http.StatusInternalServerError)
		return
	}

	imageFilename := generatePhotoFilename(request.Email)
	if err := c.uploads.SaveImage(imageData, imageFilename); err != nil {
		c.log.Error("failed to save image", slog.String("error", err.Error()))
		http.Error(w, "failed to save image", http.StatusInternalServerError)
		return
	}

	userID, err := c.client.Register(r.Context(), request.Email, request.Password, request.SteamURL, imageFilename)
	if err != nil {
		c.log.Error("sso.Register failed", slog.String("error", err.Error()))
		http.Error(w, "failed to register", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(userID); err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.auth.Login"

	var req ssov1.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.log.Error("ошибка парсинга JSON тела", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" || req.AppId == 0 {
		http.Error(w, "missing email or password or app id", http.StatusBadRequest)
		return
	}

	token, err := c.client.Login(r.Context(), req.Email, req.Password, req.AppId)
	if err != nil {
		c.log.Error("sso.Login failed", slog.String("error", err.Error()))
		http.Error(w, "failed to login", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "token",
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(token); err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

type GetUserInfoResponse struct {
	Email    string `json:"email"`
	SteamURL string `json:"steam_url"`
	Photo    string `json:"photo"`
}

func (c *AuthController) GetUserInfo(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("user_id").(int64)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var user GetUserInfoResponse
	var err error

	user.Email, user.SteamURL, user.Photo, err = c.client.GetUserInfo(r.Context(), userID)
	if err != nil {
		c.log.Error("sso.GetUserInfo failed", slog.String("error", err.Error()))
		http.Error(w, "failed to get user info", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(user); err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
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

	fmt.Println("Original email:", email)
	fmt.Println("Cleaned email:", cleanEmail)

	return cleanEmail + ".jpg"
}

// internal/controller/auth/user_info.go
package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	ssov1 "github.com/Nergous/sso_protos/gen/go/sso"
	"github.com/go-chi/chi/v5"

	"games_webapp/internal/controller"
	g_errors "games_webapp/internal/errors"
)

type UserInfoController struct {
	log    *slog.Logger
	client GRPCUserInfoClient
	appID  uint32
}

type GRPCUserInfoClient interface {
	GetUserInfo(ctx context.Context, userID uint32) (email, steamURL, pathToPhoto string, err error)
	GetUsers(ctx context.Context) (*ssov1.GetAllUsersResponse, error)
	UpdateUser(ctx context.Context, id uint32, email, password, steamURL, pathToPhoto string) (*ssov1.UpdateUserResponse, error)
	DeleteUser(ctx context.Context, user *ssov1.DeleteUserRequest) (*ssov1.DeleteUserResponse, error)
	GetUsersForApp(ctx context.Context, appID uint32) (*ssov1.GetAllUsersForAppResponse, error)
}

func NewUserInfoController(log *slog.Logger, client GRPCUserInfoClient, appID uint32) *UserInfoController {
	return &UserInfoController{log: log, client: client, appID: appID}
}

// ============================================================================
// TYPES
// ============================================================================

type GetUserInfoResponse struct {
	Email    string `json:"email"`
	SteamURL string `json:"steam_url"`
	Photo    string `json:"photo"`
}

type User struct {
	Id          int    `json:"id"`
	Email       string `json:"email"`
	SteamURL    string `json:"steam_url"`
	PathToPhoto string `json:"path_to_photo"`
	IsAdmin     bool   `json:"is_admin"`
}

type GetUsersResponse struct {
	Users []User `json:"users"`
}

type UpdateUserRequest struct {
	Id          uint32 `json:"id"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	SteamURL    string `json:"steam_url"`
	PathToPhoto string `json:"path_to_photo"`
}

// ============================================================================
// HANDLERS
// ============================================================================

func (c *UserInfoController) GetUserInfo(w http.ResponseWriter, r *http.Request) {
	const op = "controller.auth.GetUserInfo"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		controller.WriteError(w, log, se)
		return
	}

	var user GetUserInfoResponse
	user.Email, user.SteamURL, user.Photo, err = c.client.GetUserInfo(r.Context(), uint32(userID))
	if err != nil {
		se := g_errors.NewFromGRPC(op, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("err", se.Err))
		http.Error(w, g_errors.PublicMessage(g_errors.New(op, g_errors.CodeInternal, "")), http.StatusInternalServerError)
		return
	}

	controller.WriteJSON(w, log, http.StatusOK, user)
}

func (c *UserInfoController) GetUsers(w http.ResponseWriter, r *http.Request) {
	const op = "controller.auth.GetUsers"
	log := c.log.With(slog.String("operation", op))

	isAdmin := controller.GetIsAdmin(r.Context())
	if !isAdmin {
		se := g_errors.New(op, g_errors.CodeForbidden, g_errors.NotAdminOrCreator)
		controller.WriteError(w, log, se)
		return
	}

	resp, err := c.client.GetUsersForApp(r.Context(), c.appID)
	if err != nil {
		se := g_errors.NewFromGRPC(op, err)
		controller.WriteError(w, log, se)
		return
	}

	var users GetUsersResponse
	for _, u := range resp.Users {
		users.Users = append(users.Users, User{
			Id:          int(u.Id),
			Email:       u.Email,
			SteamURL:    u.SteamUrl,
			PathToPhoto: u.PathToPhoto,
			IsAdmin:     u.IsAdmin,
		})
	}

	controller.WriteJSON(w, log, http.StatusOK, users)
}

func (c *UserInfoController) UpdateUser(w http.ResponseWriter, r *http.Request) {
	const op = "controller.auth.UpdateUser"
	log := c.log.With(slog.String("operation", op))

	isAdmin := controller.GetIsAdmin(r.Context())
	if !isAdmin {
		se := g_errors.New(op, g_errors.CodeForbidden, g_errors.NotAdminOrCreator)
		controller.WriteError(w, log, se)
		return
	}

	var user UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		controller.WriteError(w, log, se)
		return
	}

	if _, err := c.client.UpdateUser(r.Context(), user.Id, user.Email, user.Password, user.SteamURL, user.PathToPhoto); err != nil {
		se := g_errors.NewFromGRPC(op, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("err", se.Err))
		http.Error(w, g_errors.PublicMessage(g_errors.New(op, g_errors.CodeInternal, "")), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (c *UserInfoController) DeleteUser(w http.ResponseWriter, r *http.Request) {
	const op = "controller.auth.DeleteUser"
	log := c.log.With(slog.String("operation", op))

	isAdmin := controller.GetIsAdmin(r.Context())
	if !isAdmin {
		se := g_errors.New(op, g_errors.CodeForbidden, g_errors.NotAdminOrCreator)
		controller.WriteError(w, log, se)
		return
	}

	idStr := chi.URLParam(r, "id")
	idInt, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidUserID, map[string]any{"id": idStr}, err)
		controller.WriteError(w, log, se)
		return
	}

	if _, err := c.client.DeleteUser(r.Context(), &ssov1.DeleteUserRequest{Id: uint32(idInt)}); err != nil {
		se := g_errors.NewFromGRPC(op, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("err", se.Err))
		http.Error(w, g_errors.PublicMessage(g_errors.New(op, g_errors.CodeInternal, "")), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

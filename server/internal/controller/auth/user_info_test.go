// internal/controller/auth/user_info_test.go
package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"games_webapp/internal/controller/auth"
	"games_webapp/internal/middleware"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ssov1 "github.com/Nergous/sso_protos/gen/go/sso"
	"github.com/go-chi/chi/v5"
)

// ============================================================================
// Mock
// ============================================================================

type mockGRPCUserInfoClient struct {
	getUserInfo    func(ctx context.Context, userID uint32) (string, string, string, error)
	getUsers       func(ctx context.Context) (*ssov1.GetAllUsersResponse, error)
	updateUser     func(ctx context.Context, id uint32, email, password, steamURL, pathToPhoto string) (*ssov1.UpdateUserResponse, error)
	deleteUser     func(ctx context.Context, user *ssov1.DeleteUserRequest) (*ssov1.DeleteUserResponse, error)
	getUsersForApp func(ctx context.Context, appID uint32) (*ssov1.GetAllUsersForAppResponse, error)
}

func (m *mockGRPCUserInfoClient) GetUserInfo(ctx context.Context, userID uint32) (string, string, string, error) {
	if m.getUserInfo == nil {
		panic("unexpected call to GetUserInfo")
	}
	return m.getUserInfo(ctx, userID)
}

func (m *mockGRPCUserInfoClient) GetUsers(ctx context.Context) (*ssov1.GetAllUsersResponse, error) {
	if m.getUsers == nil {
		panic("unexpected call to GetUsers")
	}
	return m.getUsers(ctx)
}

func (m *mockGRPCUserInfoClient) UpdateUser(ctx context.Context, id uint32, email, password, steamURL, pathToPhoto string) (*ssov1.UpdateUserResponse, error) {
	if m.updateUser == nil {
		panic("unexpected call to UpdateUser")
	}
	return m.updateUser(ctx, id, email, password, steamURL, pathToPhoto)
}

func (m *mockGRPCUserInfoClient) DeleteUser(ctx context.Context, user *ssov1.DeleteUserRequest) (*ssov1.DeleteUserResponse, error) {
	if m.deleteUser == nil {
		panic("unexpected call to DeleteUser")
	}
	return m.deleteUser(ctx, user)
}

func (m *mockGRPCUserInfoClient) GetUsersForApp(ctx context.Context, appID uint32) (*ssov1.GetAllUsersForAppResponse, error) {
	if m.getUsersForApp == nil {
		panic("unexpected call to GetUsersForApp")
	}
	return m.getUsersForApp(ctx, appID)
}

// ============================================================================
// Helpers
// ============================================================================

func newUserInfoController(client *mockGRPCUserInfoClient) *auth.UserInfoController {
	return auth.NewUserInfoController(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		client,
		1,
	)
}

// withAdminCtx инжектирует userID и isAdmin в контекст, имитируя middleware.
func withAdminCtx(r *http.Request, userID int, isAdmin bool) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserIDKey, userID)
	ctx = context.WithValue(ctx, middleware.IsAdminKey, isAdmin)
	return r.WithContext(ctx)
}

// withChiIDParam добавляет chi URL-параметр "id".
func withChiIDParam(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// ============================================================================
// GetUserInfo
// ============================================================================

func TestGetUserInfo_Unauthorized(t *testing.T) {
	c := newUserInfoController(&mockGRPCUserInfoClient{})

	r := httptest.NewRequest(http.MethodGet, "/users/me", nil)
	w := httptest.NewRecorder()
	c.GetUserInfo(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestGetUserInfo_ClientError(t *testing.T) {
	client := &mockGRPCUserInfoClient{
		getUserInfo: func(_ context.Context, _ uint32) (string, string, string, error) {
			return "", "", "", errors.New("grpc unavailable")
		},
	}
	c := newUserInfoController(client)

	r := withAdminCtx(httptest.NewRequest(http.MethodGet, "/users/me", nil), 10, false)
	w := httptest.NewRecorder()
	c.GetUserInfo(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

func TestGetUserInfo_Success(t *testing.T) {
	client := &mockGRPCUserInfoClient{
		getUserInfo: func(_ context.Context, userID uint32) (string, string, string, error) {
			if userID != 10 {
				t.Errorf("userID: got %d, want 10", userID)
			}
			return "user@test.com", "http://steam.com/user", "photo.jpg", nil
		},
	}
	c := newUserInfoController(client)

	r := withAdminCtx(httptest.NewRequest(http.MethodGet, "/users/me", nil), 10, false)
	w := httptest.NewRecorder()
	c.GetUserInfo(w, r)

	assertStatus(t, w.Code, http.StatusOK)

	var resp auth.GetUserInfoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Email != "user@test.com" {
		t.Errorf("Email: got %q", resp.Email)
	}
	if resp.SteamURL != "http://steam.com/user" {
		t.Errorf("SteamURL: got %q", resp.SteamURL)
	}
	if resp.Photo != "photo.jpg" {
		t.Errorf("Photo: got %q", resp.Photo)
	}
}

// ============================================================================
// GetUsers
// ============================================================================

func TestGetUsers_Forbidden(t *testing.T) {
	c := newUserInfoController(&mockGRPCUserInfoClient{})

	r := withAdminCtx(httptest.NewRequest(http.MethodGet, "/users", nil), 10, false)
	w := httptest.NewRecorder()
	c.GetUsers(w, r)

	assertStatus(t, w.Code, http.StatusForbidden)
}

func TestGetUsers_ClientError(t *testing.T) {
	client := &mockGRPCUserInfoClient{
		getUsersForApp: func(_ context.Context, _ uint32) (*ssov1.GetAllUsersForAppResponse, error) {
			return nil, errors.New("grpc unavailable")
		},
	}
	c := newUserInfoController(client)

	r := withAdminCtx(httptest.NewRequest(http.MethodGet, "/users", nil), 10, true)
	w := httptest.NewRecorder()
	c.GetUsers(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

func TestGetUsers_Success(t *testing.T) {
	client := &mockGRPCUserInfoClient{
		getUsersForApp: func(_ context.Context, appID uint32) (*ssov1.GetAllUsersForAppResponse, error) {
			if appID != 1 {
				t.Errorf("appID: got %d, want 1", appID)
			}
			return &ssov1.GetAllUsersForAppResponse{
				Users: []*ssov1.AppUser{
					{Id: 1, Email: "a@test.com", SteamUrl: "http://steam.com/a", PathToPhoto: "a.jpg", IsAdmin: false},
					{Id: 2, Email: "b@test.com", SteamUrl: "http://steam.com/b", PathToPhoto: "b.jpg", IsAdmin: true},
				},
			}, nil
		},
	}
	c := newUserInfoController(client)

	r := withAdminCtx(httptest.NewRequest(http.MethodGet, "/users", nil), 10, true)
	w := httptest.NewRecorder()
	c.GetUsers(w, r)

	assertStatus(t, w.Code, http.StatusOK)

	var resp auth.GetUsersResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Users) != 2 {
		t.Fatalf("users count: got %d, want 2", len(resp.Users))
	}
	if resp.Users[0].Email != "a@test.com" {
		t.Errorf("Users[0].Email: got %q", resp.Users[0].Email)
	}
	if !resp.Users[1].IsAdmin {
		t.Error("Users[1].IsAdmin: want true")
	}
}

// ============================================================================
// UpdateUser
// ============================================================================

func TestUpdateUser_Forbidden(t *testing.T) {
	c := newUserInfoController(&mockGRPCUserInfoClient{})

	body, _ := json.Marshal(auth.UpdateUserRequest{Id: 1, Email: "x@test.com"})
	r := withAdminCtx(httptest.NewRequest(http.MethodPut, "/users/1", strings.NewReader(string(body))), 10, false)
	w := httptest.NewRecorder()
	c.UpdateUser(w, r)

	assertStatus(t, w.Code, http.StatusForbidden)
}

func TestUpdateUser_InvalidBody(t *testing.T) {
	c := newUserInfoController(&mockGRPCUserInfoClient{})

	r := httptest.NewRequest(http.MethodPut, "/users/1", strings.NewReader("not-json{{{"))
	r.Header.Set("Content-Type", "application/json")
	r = withAdminCtx(r, 10, true)
	w := httptest.NewRecorder()
	c.UpdateUser(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestUpdateUser_ClientError(t *testing.T) {
	client := &mockGRPCUserInfoClient{
		updateUser: func(_ context.Context, _ uint32, _, _, _, _ string) (*ssov1.UpdateUserResponse, error) {
			return nil, errors.New("grpc unavailable")
		},
	}
	c := newUserInfoController(client)

	body, _ := json.Marshal(auth.UpdateUserRequest{Id: 1, Email: "x@test.com"})
	r := httptest.NewRequest(http.MethodPut, "/users/1", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r = withAdminCtx(r, 10, true)
	w := httptest.NewRecorder()
	c.UpdateUser(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

func TestUpdateUser_Success(t *testing.T) {
	var gotID uint32
	var gotEmail string
	client := &mockGRPCUserInfoClient{
		updateUser: func(_ context.Context, id uint32, email, _, _, _ string) (*ssov1.UpdateUserResponse, error) {
			gotID = id
			gotEmail = email
			return &ssov1.UpdateUserResponse{}, nil
		},
	}
	c := newUserInfoController(client)

	body, _ := json.Marshal(auth.UpdateUserRequest{
		Id:    5,
		Email: "updated@test.com",
	})
	r := httptest.NewRequest(http.MethodPut, "/users/5", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r = withAdminCtx(r, 10, true)
	w := httptest.NewRecorder()
	c.UpdateUser(w, r)

	assertStatus(t, w.Code, http.StatusOK)
	if gotID != 5 {
		t.Errorf("id: got %d, want 5", gotID)
	}
	if gotEmail != "updated@test.com" {
		t.Errorf("email: got %q", gotEmail)
	}
}

// ============================================================================
// DeleteUser
// ============================================================================

func TestDeleteUser_Forbidden(t *testing.T) {
	c := newUserInfoController(&mockGRPCUserInfoClient{})

	r := withAdminCtx(
		withChiIDParam(httptest.NewRequest(http.MethodDelete, "/users/1", nil), "1"),
		10, false,
	)
	w := httptest.NewRecorder()
	c.DeleteUser(w, r)

	assertStatus(t, w.Code, http.StatusForbidden)
}

func TestDeleteUser_InvalidID(t *testing.T) {
	c := newUserInfoController(&mockGRPCUserInfoClient{})

	r := withAdminCtx(
		withChiIDParam(httptest.NewRequest(http.MethodDelete, "/users/abc", nil), "abc"),
		10, true,
	)
	w := httptest.NewRecorder()
	c.DeleteUser(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestDeleteUser_ClientError(t *testing.T) {
	client := &mockGRPCUserInfoClient{
		deleteUser: func(_ context.Context, _ *ssov1.DeleteUserRequest) (*ssov1.DeleteUserResponse, error) {
			return nil, errors.New("grpc unavailable")
		},
	}
	c := newUserInfoController(client)

	r := withAdminCtx(
		withChiIDParam(httptest.NewRequest(http.MethodDelete, "/users/5", nil), "5"),
		10, true,
	)
	w := httptest.NewRecorder()
	c.DeleteUser(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

func TestDeleteUser_Success(t *testing.T) {
	var gotID uint32
	client := &mockGRPCUserInfoClient{
		deleteUser: func(_ context.Context, req *ssov1.DeleteUserRequest) (*ssov1.DeleteUserResponse, error) {
			gotID = req.Id
			return &ssov1.DeleteUserResponse{}, nil
		},
	}
	c := newUserInfoController(client)

	r := withAdminCtx(
		withChiIDParam(httptest.NewRequest(http.MethodDelete, "/users/7", nil), "7"),
		10, true,
	)
	w := httptest.NewRecorder()
	c.DeleteUser(w, r)

	assertStatus(t, w.Code, http.StatusNoContent)
	if gotID != 7 {
		t.Errorf("deleted id: got %d, want 7", gotID)
	}
}

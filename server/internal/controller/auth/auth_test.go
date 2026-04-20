// internal/controller/auth/auth_test.go
package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"games_webapp/internal/controller/auth"
	g_errors "games_webapp/internal/errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ============================================================================
// Mocks
// ============================================================================

type mockGRPCAuthClient struct {
	login    func(ctx context.Context, email, password string, appID uint32) (string, string, error)
	logout   func(ctx context.Context, token string) error
	register func(ctx context.Context, email, password, steamURL, pathToPhoto string) (uint32, error)
}

func (m *mockGRPCAuthClient) Login(ctx context.Context, email, password string, appID uint32) (string, string, error) {
	if m.login == nil {
		panic("unexpected call to Login")
	}
	return m.login(ctx, email, password, appID)
}

func (m *mockGRPCAuthClient) Logout(ctx context.Context, token string) error {
	if m.logout == nil {
		return nil
	}
	return m.logout(ctx, token)
}

func (m *mockGRPCAuthClient) Register(ctx context.Context, email, password, steamURL, pathToPhoto string) (uint32, error) {
	if m.register == nil {
		panic("unexpected call to Register")
	}
	return m.register(ctx, email, password, steamURL, pathToPhoto)
}

type mockUploadsAuth struct {
	saveImage             func(image []byte, filename string) error
	generateImageFilename func(url, contentType string) string
}

func (m *mockUploadsAuth) SaveImage(image []byte, filename string) error {
	if m.saveImage == nil {
		return nil
	}
	return m.saveImage(image, filename)
}

func (m *mockUploadsAuth) GenerateImageFilename(url, contentType string) string {
	if m.generateImageFilename == nil {
		return "generated.jpg"
	}
	return m.generateImageFilename(url, contentType)
}

// ============================================================================
// Helpers
// ============================================================================

func newAuthController(client *mockGRPCAuthClient, uploads *mockUploadsAuth) *auth.AuthController {
	return auth.NewAuthController(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		client,
		uploads,
		1,
	)
}

func assertStatus(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("status: got %d, want %d", got, want)
	}
}

func newMultipartRequest(t *testing.T, fields map[string]string, fileField, filename string, fileData []byte) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("write field %q: %v", k, err)
		}
	}
	if fileField != "" {
		part, err := w.CreateFormFile(fileField, filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		part.Write(fileData)
	}
	w.Close()

	r := httptest.NewRequest(http.MethodPost, "/", &buf)
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r
}

func grpcErr(code codes.Code, msg string) error {
	return status.Error(code, msg)
}

// ============================================================================
// Register
// ============================================================================

func TestRegister_InvalidMultipartForm(t *testing.T) {
	c := newAuthController(&mockGRPCAuthClient{}, &mockUploadsAuth{})

	r := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("not-multipart"))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=BOUNDARY")
	w := httptest.NewRecorder()
	c.Register(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestRegister_ValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]string
	}{
		{
			name:   "empty email",
			fields: map[string]string{"email": "", "password": "pass", "steam_url": "http://steam.com"},
		},
		{
			name:   "empty password",
			fields: map[string]string{"email": "user@test.com", "password": "", "steam_url": "http://steam.com"},
		},
		{
			name:   "empty steam_url",
			fields: map[string]string{"email": "user@test.com", "password": "pass", "steam_url": ""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newAuthController(&mockGRPCAuthClient{}, &mockUploadsAuth{})

			r := newMultipartRequest(t, tc.fields, "image", "photo.jpg", []byte("fake"))
			w := httptest.NewRecorder()
			c.Register(w, r)

			assertStatus(t, w.Code, http.StatusBadRequest)
		})
	}
}

func TestRegister_MissingImage(t *testing.T) {
	c := newAuthController(&mockGRPCAuthClient{}, &mockUploadsAuth{})

	r := newMultipartRequest(t,
		map[string]string{"email": "user@test.com", "password": "pass", "steam_url": "http://steam.com"},
		"", "", nil, // нет файла
	)
	w := httptest.NewRecorder()
	c.Register(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestRegister_SaveImageError(t *testing.T) {
	up := &mockUploadsAuth{
		generateImageFilename: func(_, _ string) string { return "photo.jpg" },
		saveImage: func(_ []byte, _ string) error {
			return g_errors.New("op", g_errors.CodeInternal, g_errors.CannotCreateFile)
		},
	}
	c := newAuthController(&mockGRPCAuthClient{}, up)

	r := newMultipartRequest(t,
		map[string]string{"email": "user@test.com", "password": "pass", "steam_url": "http://steam.com"},
		"image", "photo.jpg", []byte("fake"),
	)
	w := httptest.NewRecorder()
	c.Register(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

func TestRegister_GRPCError_InvalidArgument(t *testing.T) {
	client := &mockGRPCAuthClient{
		register: func(_ context.Context, _, _, _, _ string) (uint32, error) {
			return 0, grpcErr(codes.InvalidArgument, "invalid email format")
		},
	}
	up := &mockUploadsAuth{
		generateImageFilename: func(_, _ string) string { return "photo.jpg" },
		saveImage:             func(_ []byte, _ string) error { return nil },
	}
	c := newAuthController(client, up)

	r := newMultipartRequest(t,
		map[string]string{"email": "user@test.com", "password": "pass", "steam_url": "http://steam.com"},
		"image", "photo.jpg", []byte("fake"),
	)
	w := httptest.NewRecorder()
	c.Register(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestRegister_GRPCError_AlreadyExists(t *testing.T) {
	client := &mockGRPCAuthClient{
		register: func(_ context.Context, _, _, _, _ string) (uint32, error) {
			return 0, grpcErr(codes.AlreadyExists, "user already exists")
		},
	}
	up := &mockUploadsAuth{
		generateImageFilename: func(_, _ string) string { return "photo.jpg" },
		saveImage:             func(_ []byte, _ string) error { return nil },
	}
	c := newAuthController(client, up)

	r := newMultipartRequest(t,
		map[string]string{"email": "user@test.com", "password": "pass", "steam_url": "http://steam.com"},
		"image", "photo.jpg", []byte("fake"),
	)
	w := httptest.NewRecorder()
	c.Register(w, r)

	assertStatus(t, w.Code, http.StatusConflict)
}

func TestRegister_GRPCError_DeadlineExceeded(t *testing.T) {
	client := &mockGRPCAuthClient{
		register: func(_ context.Context, _, _, _, _ string) (uint32, error) {
			return 0, grpcErr(codes.DeadlineExceeded, "timeout")
		},
	}
	up := &mockUploadsAuth{
		generateImageFilename: func(_, _ string) string { return "photo.jpg" },
		saveImage:             func(_ []byte, _ string) error { return nil },
	}
	c := newAuthController(client, up)

	r := newMultipartRequest(t,
		map[string]string{"email": "user@test.com", "password": "pass", "steam_url": "http://steam.com"},
		"image", "photo.jpg", []byte("fake"),
	)
	w := httptest.NewRecorder()
	c.Register(w, r)

	assertStatus(t, w.Code, http.StatusGatewayTimeout)
}

func TestRegister_Success(t *testing.T) {
	var gotEmail, gotSteamURL, gotPhoto string
	client := &mockGRPCAuthClient{
		register: func(_ context.Context, email, _, steamURL, photo string) (uint32, error) {
			gotEmail = email
			gotSteamURL = steamURL
			gotPhoto = photo
			return 42, nil
		},
	}
	up := &mockUploadsAuth{
		generateImageFilename: func(_, _ string) string { return "photo.jpg" },
		saveImage:             func(_ []byte, _ string) error { return nil },
	}
	c := newAuthController(client, up)

	r := newMultipartRequest(t,
		map[string]string{
			"email":     "  User@Test.COM  ", // проверяем trim+lower
			"password":  "secret",
			"steam_url": "http://steam.com/user",
		},
		"image", "photo.jpg", []byte("fake"),
	)
	w := httptest.NewRecorder()
	c.Register(w, r)

	assertStatus(t, w.Code, http.StatusCreated)

	// email должен быть очищен
	if gotEmail != "user@test.com" {
		t.Errorf("email: got %q, want %q", gotEmail, "user@test.com")
	}
	if gotSteamURL != "http://steam.com/user" {
		t.Errorf("steamURL: got %q", gotSteamURL)
	}
	if gotPhoto != "photo.jpg" {
		t.Errorf("photo: got %q", gotPhoto)
	}

	var userID uint32
	if err := json.NewDecoder(w.Body).Decode(&userID); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if userID != 42 {
		t.Errorf("userID: got %d, want 42", userID)
	}
}

// ============================================================================
// Login
// ============================================================================

func TestLogin_InvalidBody(t *testing.T) {
	c := newAuthController(&mockGRPCAuthClient{}, &mockUploadsAuth{})

	r := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("not-json{{{"))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.Login(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestLogin_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		body map[string]any
	}{
		{
			name: "empty email",
			body: map[string]any{"email": "", "password": "pass", "app_id": 1},
		},
		{
			name: "empty password",
			body: map[string]any{"email": "user@test.com", "password": "", "app_id": 1},
		},
		{
			name: "wrong app_id",
			body: map[string]any{"email": "user@test.com", "password": "pass", "app_id": 99},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newAuthController(&mockGRPCAuthClient{}, &mockUploadsAuth{})

			body, _ := json.Marshal(tc.body)
			r := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			c.Login(w, r)

			assertStatus(t, w.Code, http.StatusBadRequest)
		})
	}
}

func TestLogin_GRPCError_NotFound(t *testing.T) {
	client := &mockGRPCAuthClient{
		login: func(_ context.Context, _, _ string, _ uint32) (string, string, error) {
			return "", "", grpcErr(codes.NotFound, "user not found")
		},
	}
	c := newAuthController(client, &mockUploadsAuth{})

	body, _ := json.Marshal(map[string]any{"email": "user@test.com", "password": "pass", "app_id": 1})
	r := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.Login(w, r)

	assertStatus(t, w.Code, http.StatusNotFound)
}

func TestLogin_GRPCError_InvalidArgument(t *testing.T) {
	client := &mockGRPCAuthClient{
		login: func(_ context.Context, _, _ string, _ uint32) (string, string, error) {
			return "", "", grpcErr(codes.InvalidArgument, "wrong password")
		},
	}
	c := newAuthController(client, &mockUploadsAuth{})

	body, _ := json.Marshal(map[string]any{"email": "user@test.com", "password": "wrong", "app_id": 1})
	r := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.Login(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestLogin_GRPCError_DeadlineExceeded(t *testing.T) {
	client := &mockGRPCAuthClient{
		login: func(_ context.Context, _, _ string, _ uint32) (string, string, error) {
			return "", "", grpcErr(codes.DeadlineExceeded, "timeout")
		},
	}
	c := newAuthController(client, &mockUploadsAuth{})

	body, _ := json.Marshal(map[string]any{"email": "user@test.com", "password": "pass", "app_id": 1})
	r := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.Login(w, r)

	assertStatus(t, w.Code, http.StatusGatewayTimeout)
}

func TestLogin_Success(t *testing.T) {
	var gotEmail string
	client := &mockGRPCAuthClient{
		login: func(_ context.Context, email, _ string, _ uint32) (string, string, error) {
			gotEmail = email
			return "access-token", "refresh-token", nil
		},
	}
	c := newAuthController(client, &mockUploadsAuth{})

	body, _ := json.Marshal(map[string]any{
		"email":    "  User@Test.COM  ", // trim+lower
		"password": "secret",
		"app_id":   1,
	})
	r := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.Login(w, r)

	assertStatus(t, w.Code, http.StatusOK)

	// email очищен
	if gotEmail != "user@test.com" {
		t.Errorf("email: got %q, want %q", gotEmail, "user@test.com")
	}

	// refresh token в cookie
	cookies := w.Result().Cookies()
	var refreshCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "refresh_token" {
			refreshCookie = cookie
			break
		}
	}
	if refreshCookie == nil {
		t.Fatal("refresh_token cookie not set")
	}
	if refreshCookie.Value != "refresh-token" {
		t.Errorf("refresh_token: got %q, want %q", refreshCookie.Value, "refresh-token")
	}
	if !refreshCookie.HttpOnly {
		t.Error("refresh_token cookie must be HttpOnly")
	}

	// access token в теле
	var resp auth.LoginResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AccessToken != "access-token" {
		t.Errorf("access_token: got %q, want %q", resp.AccessToken, "access-token")
	}
}

// ============================================================================
// Logout
// ============================================================================

func TestLogout_WithValidCookie(t *testing.T) {
	logoutCalled := false
	client := &mockGRPCAuthClient{
		logout: func(_ context.Context, token string) error {
			logoutCalled = true
			if token != "my-refresh-token" {
				t.Errorf("logout token: got %q, want %q", token, "my-refresh-token")
			}
			return nil
		},
	}
	c := newAuthController(client, &mockUploadsAuth{})

	r := httptest.NewRequest(http.MethodPost, "/logout", nil)
	r.AddCookie(&http.Cookie{Name: "refresh_token", Value: "my-refresh-token"})
	w := httptest.NewRecorder()
	c.Logout(w, r)

	assertStatus(t, w.Code, http.StatusOK)
	if !logoutCalled {
		t.Error("client.Logout must be called when refresh_token cookie is present")
	}

	// cookie должна быть сброшена
	cookies := w.Result().Cookies()
	var refreshCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "refresh_token" {
			refreshCookie = cookie
			break
		}
	}
	if refreshCookie == nil {
		t.Fatal("refresh_token cookie not found in response")
	}
	if refreshCookie.Value != "" {
		t.Errorf("refresh_token cookie value: got %q, want empty", refreshCookie.Value)
	}
	if refreshCookie.MaxAge != -1 {
		t.Errorf("refresh_token MaxAge: got %d, want -1", refreshCookie.MaxAge)
	}
}

func TestLogout_WithoutCookie(t *testing.T) {
	// client.Logout не должен вызываться если cookie нет
	client := &mockGRPCAuthClient{
		logout: func(_ context.Context, _ string) error {
			t.Error("client.Logout must not be called without cookie")
			return nil
		},
	}
	c := newAuthController(client, &mockUploadsAuth{})

	r := httptest.NewRequest(http.MethodPost, "/logout", nil)
	w := httptest.NewRecorder()
	c.Logout(w, r)

	assertStatus(t, w.Code, http.StatusOK)
}

func TestLogout_WithEmptyCookie(t *testing.T) {
	// cookie есть, но пустая — Logout не вызываем
	client := &mockGRPCAuthClient{
		logout: func(_ context.Context, _ string) error {
			t.Error("client.Logout must not be called with empty cookie value")
			return nil
		},
	}
	c := newAuthController(client, &mockUploadsAuth{})

	r := httptest.NewRequest(http.MethodPost, "/logout", nil)
	r.AddCookie(&http.Cookie{Name: "refresh_token", Value: ""})
	w := httptest.NewRecorder()
	c.Logout(w, r)

	assertStatus(t, w.Code, http.StatusOK)

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["message"] != "logged out successfully" {
		t.Errorf("message: got %q", resp["message"])
	}
}

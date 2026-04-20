// internal/controller/auth/tokens_test.go
package auth_test

import (
	"context"
	"encoding/json"
	"games_webapp/internal/controller/auth"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/codes"
)

// ============================================================================
// Mock
// ============================================================================

type mockGRPCTokensClient struct {
	refreshToken func(ctx context.Context, refreshToken string) (string, string, error)
}

func (m *mockGRPCTokensClient) RefreshToken(ctx context.Context, refreshToken string) (string, string, error) {
	if m.refreshToken == nil {
		panic("unexpected call to RefreshToken")
	}
	return m.refreshToken(ctx, refreshToken)
}

// ============================================================================
// Helper
// ============================================================================

func newTokensController(client *mockGRPCTokensClient) *auth.TokensController {
	return auth.NewTokensController(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		client,
	)
}

func getRefreshCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "refresh_token" {
			return cookie
		}
	}
	return nil
}

// ============================================================================
// Refresh
// ============================================================================

func TestRefresh_NoCookie(t *testing.T) {
	c := newTokensController(&mockGRPCTokensClient{})

	r := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	w := httptest.NewRecorder()
	c.Refresh(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestRefresh_EmptyCookieValue(t *testing.T) {
	c := newTokensController(&mockGRPCTokensClient{})

	r := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	r.AddCookie(&http.Cookie{Name: "refresh_token", Value: ""})
	w := httptest.NewRecorder()
	c.Refresh(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestRefresh_GRPCError_ClearsCookie(t *testing.T) {
	client := &mockGRPCTokensClient{
		refreshToken: func(_ context.Context, _ string) (string, string, error) {
			return "", "", grpcErr(codes.Unauthenticated, "token expired")
		},
	}
	c := newTokensController(client)

	r := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	r.AddCookie(&http.Cookie{Name: "refresh_token", Value: "expired-token"})
	w := httptest.NewRecorder()
	c.Refresh(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)

	cookie := getRefreshCookie(t, w)
	if cookie == nil {
		t.Fatal("refresh_token cookie must be present in response to clear it")
	}
	if cookie.Value != "" {
		t.Errorf("cookie value: got %q, want empty", cookie.Value)
	}
	if cookie.MaxAge != -1 {
		t.Errorf("cookie MaxAge: got %d, want -1", cookie.MaxAge)
	}
}

func TestRefresh_GRPCError_PassedTokenForwarded(t *testing.T) {
	var gotToken string
	client := &mockGRPCTokensClient{
		refreshToken: func(_ context.Context, token string) (string, string, error) {
			gotToken = token
			return "", "", grpcErr(codes.Unauthenticated, "expired")
		},
	}
	c := newTokensController(client)

	r := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	r.AddCookie(&http.Cookie{Name: "refresh_token", Value: "my-token"})
	w := httptest.NewRecorder()
	c.Refresh(w, r)

	if gotToken != "my-token" {
		t.Errorf("token passed to RefreshToken: got %q, want %q", gotToken, "my-token")
	}
}
func TestRefresh_Success(t *testing.T) {
	client := &mockGRPCTokensClient{
		refreshToken: func(_ context.Context, _ string) (string, string, error) {
			return "new-access-token", "new-refresh-token", nil
		},
	}
	c := newTokensController(client)

	r := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	r.AddCookie(&http.Cookie{Name: "refresh_token", Value: "old-refresh-token"})
	w := httptest.NewRecorder()
	c.Refresh(w, r)

	assertStatus(t, w.Code, http.StatusOK)

	// новый refresh token в cookie
	cookie := getRefreshCookie(t, w)
	if cookie == nil {
		t.Fatal("refresh_token cookie not set")
	}
	if cookie.Value != "new-refresh-token" {
		t.Errorf("cookie value: got %q, want %q", cookie.Value, "new-refresh-token")
	}
	if cookie.MaxAge != 30*24*60*60 {
		t.Errorf("cookie MaxAge: got %d, want %d", cookie.MaxAge, 30*24*60*60)
	}
	if !cookie.HttpOnly {
		t.Error("cookie must be HttpOnly")
	}

	// новый access token в теле
	var resp auth.RefreshResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.AccessToken != "new-access-token" {
		t.Errorf("access_token: got %q, want %q", resp.AccessToken, "new-access-token")
	}
}

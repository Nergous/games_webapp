// internal/controller/games/igdb_test.go
package games_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"games_webapp/internal/controller/games"
	g_errors "games_webapp/internal/errors"
	"games_webapp/internal/models"
)

// ============================================================================
// Mocks
// ============================================================================

type mockUploadsIGDB struct {
	saveImage            func([]byte, string) error
	deleteImage          func(string) error
	replaceImage         func([]byte, string, string) error
	downloadAndSaveImage func(string) (string, error)
}

func (m *mockUploadsIGDB) SaveImage(img []byte, fn string) error {
	if m.saveImage == nil {
		return nil
	}
	return m.saveImage(img, fn)
}
func (m *mockUploadsIGDB) DeleteImage(fn string) error {
	if m.deleteImage == nil {
		return nil
	}
	return m.deleteImage(fn)
}
func (m *mockUploadsIGDB) ReplaceImage(img []byte, old, newFn string) error {
	if m.replaceImage == nil {
		return nil
	}
	return m.replaceImage(img, old, newFn)
}
func (m *mockUploadsIGDB) DownloadAndSaveImage(url string) (string, error) {
	if m.downloadAndSaveImage == nil {
		return "cover.jpg", nil
	}
	return m.downloadAndSaveImage(url)
}

// ============================================================================
// Helpers
// ============================================================================

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func twitchTokenResponse() []byte {
	b, _ := json.Marshal(map[string]any{
		"access_token": "test-access-token",
		"expires_in":   3600,
		"token_type":   "bearer",
	})
	return b
}

func igdbGameResponse() []byte {
	b, _ := json.Marshal([]map[string]any{
		{
			"id":                 1,
			"name":               "Half-Life 2",
			"summary":            "A great game",
			"url":                "https://igdb.com/games/half-life-2",
			"first_release_date": 1099612800,
			"cover": map[string]any{
				"url": "//images.igdb.com/igdb/image/upload/t_thumb/co1234.jpg",
			},
			"involved_companies": []map[string]any{
				{
					"company":   map[string]any{"name": "Valve"},
					"developer": true,
					"publisher": true,
				},
			},
			"genres": []map[string]any{
				{"name": "Shooter"},
			},
		},
	})
	return b
}

func newIGDBController(
	t *testing.T,
	twitchHandler http.HandlerFunc,
	igdbHandler http.HandlerFunc,
	svc *mockGameService,
	up *mockUploadsIGDB,
) *games.IGDBGamesController {
	t.Helper()

	twitchSrv := httptest.NewServer(twitchHandler)
	igdbSrv := httptest.NewServer(igdbHandler)
	t.Cleanup(func() {
		twitchSrv.Close()
		igdbSrv.Close()
	})

	return games.NewIGDBGamesController(
		svc,
		newLogger(),
		up,
		igdbSrv.URL,
		twitchSrv.URL,
		"client-id",
		"client-secret",
	)
}

func defaultHandlers() (http.HandlerFunc, http.HandlerFunc) {
	twitch := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(twitchTokenResponse())
	}
	igdb := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(igdbGameResponse())
	}
	return twitch, igdb
}

func igdbRequest(t *testing.T, gamesList []map[string]any) *http.Request {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"games": gamesList})
	r := httptest.NewRequest(http.MethodPost, "/games/twitch", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return withAuth(r, 10, false)
}

func decodeIGDBResponse(t *testing.T, w *httptest.ResponseRecorder) games.MultiGameResponse {
	t.Helper()
	var resp games.MultiGameResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode MultiGameResponse: %v", err)
	}
	return resp
}

// ============================================================================
// CreateMultiGamesIGDB — валидация
// ============================================================================

func TestCreateMultiGamesIGDB_Unauthorized(t *testing.T) {
	twitch, igdb := defaultHandlers()
	c := newIGDBController(t, twitch, igdb, &mockGameService{}, &mockUploadsIGDB{})

	body, _ := json.Marshal(map[string]any{"games": []map[string]any{{"name": "Portal"}}})
	r := httptest.NewRequest(http.MethodPost, "/games/twitch", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	// без withAuth
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestCreateMultiGamesIGDB_InvalidBody(t *testing.T) {
	twitch, igdb := defaultHandlers()
	c := newIGDBController(t, twitch, igdb, &mockGameService{}, &mockUploadsIGDB{})

	r := httptest.NewRequest(http.MethodPost, "/games/twitch", strings.NewReader("not-json{{{"))
	r.Header.Set("Content-Type", "application/json")
	r = withAuth(r, 10, false)
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestCreateMultiGamesIGDB_EmptyGames(t *testing.T) {
	twitch, igdb := defaultHandlers()
	c := newIGDBController(t, twitch, igdb, &mockGameService{}, &mockUploadsIGDB{})

	r := igdbRequest(t, []map[string]any{})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestCreateMultiGamesIGDB_TooManyGames(t *testing.T) {
	twitch, igdb := defaultHandlers()
	c := newIGDBController(t, twitch, igdb, &mockGameService{}, &mockUploadsIGDB{})

	gamesList := make([]map[string]any, 101)
	for i := range gamesList {
		gamesList[i] = map[string]any{"name": "game"}
	}
	r := igdbRequest(t, gamesList)
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestCreateMultiGamesIGDB_EmptyNameAndURL(t *testing.T) {
	twitch, igdb := defaultHandlers()
	c := newIGDBController(t, twitch, igdb, &mockGameService{}, &mockUploadsIGDB{})

	r := igdbRequest(t, []map[string]any{{"name": "", "url": ""}})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

// ============================================================================
// CreateMultiGamesIGDB — ошибка токена Twitch
// ============================================================================

func TestCreateMultiGamesIGDB_TwitchTokenError(t *testing.T) {
	twitchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// невалидный JSON — Unmarshal упадёт
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not-json"))
	}))
	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	t.Cleanup(func() { twitchSrv.Close(); igdbSrv.Close() })

	c := games.NewIGDBGamesController(
		&mockGameService{},
		newLogger(),
		&mockUploadsIGDB{},
		igdbSrv.URL,
		twitchSrv.URL,
		"client-id",
		"client-secret",
	)

	r := igdbRequest(t, []map[string]any{{"name": "Portal"}})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

// ============================================================================
// CreateMultiGamesIGDB — результаты
// ============================================================================

func TestCreateMultiGamesIGDB_AllSuccess(t *testing.T) {
	twitch, igdb := defaultHandlers()
	svc := &mockGameService{
		create: func(_ context.Context, game *models.Game, _ *models.UserGame) (*models.Game, error) {
			game.ID = 1
			return game, nil
		},
	}
	up := &mockUploadsIGDB{
		downloadAndSaveImage: func(_ string) (string, error) { return "cover.jpg", nil },
	}
	c := newIGDBController(t, twitch, igdb, svc, up)

	r := igdbRequest(t, []map[string]any{{"name": "Half-Life 2"}})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusCreated)

	resp := decodeIGDBResponse(t, w)
	if len(resp.Success) != 1 {
		t.Errorf("success: got %d, want 1", len(resp.Success))
	}
	if len(resp.Errors) != 0 {
		t.Errorf("errors: got %d, want 0", len(resp.Errors))
	}
	if resp.Success[0].Title != "Half-Life 2" {
		t.Errorf("title: got %q", resp.Success[0].Title)
	}
}

func TestCreateMultiGamesIGDB_AllFailed(t *testing.T) {
	twitch, _ := defaultHandlers()
	igdb := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]")) // игра не найдена
	}
	c := newIGDBController(t, twitch, igdb, &mockGameService{}, &mockUploadsIGDB{})

	r := igdbRequest(t, []map[string]any{{"name": "NonExistentGame"}})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)

	resp := decodeIGDBResponse(t, w)
	if len(resp.Errors) != 1 {
		t.Errorf("errors: got %d, want 1", len(resp.Errors))
	}
	if resp.Errors[0].Name != "NonExistentGame" {
		t.Errorf("error name: got %q", resp.Errors[0].Name)
	}
}

func TestCreateMultiGamesIGDB_PartialSuccess(t *testing.T) {
	twitch, _ := defaultHandlers()

	// первый запрос — успех, второй — не найдено
	callCount := 0
	igdb := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount == 1 {
			w.Write(igdbGameResponse())
		} else {
			w.Write([]byte("[]"))
		}
	}
	svc := &mockGameService{
		create: func(_ context.Context, game *models.Game, _ *models.UserGame) (*models.Game, error) {
			game.ID = 1
			return game, nil
		},
	}
	up := &mockUploadsIGDB{
		downloadAndSaveImage: func(_ string) (string, error) { return "cover.jpg", nil },
	}
	c := newIGDBController(t, twitch, igdb, svc, up)

	r := igdbRequest(t, []map[string]any{
		{"name": "Half-Life 2"},
		{"name": "NonExistentGame"},
	})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusMultiStatus)

	resp := decodeIGDBResponse(t, w)
	if len(resp.Success) != 1 {
		t.Errorf("success: got %d, want 1", len(resp.Success))
	}
	if len(resp.Errors) != 1 {
		t.Errorf("errors: got %d, want 1", len(resp.Errors))
	}
}

func TestCreateMultiGamesIGDB_ByURL_SlugExtracted(t *testing.T) {
	twitch, _ := defaultHandlers()

	var gotBody string
	igdb := func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write(igdbGameResponse())
	}
	svc := &mockGameService{
		create: func(_ context.Context, game *models.Game, _ *models.UserGame) (*models.Game, error) {
			game.ID = 1
			return game, nil
		},
	}
	up := &mockUploadsIGDB{
		downloadAndSaveImage: func(_ string) (string, error) { return "cover.jpg", nil },
	}
	c := newIGDBController(t, twitch, igdb, svc, up)

	r := igdbRequest(t, []map[string]any{
		{"url": "https://www.igdb.com/games/half-life-2"},
	})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	assertStatus(t, w.Code, http.StatusCreated)
	if !strings.Contains(gotBody, `slug = "half-life-2"`) {
		t.Errorf("IGDB query should contain slug, got: %s", gotBody)
	}
}

func TestCreateMultiGamesIGDB_ServiceError_DeletesImage(t *testing.T) {
	twitch, igdb := defaultHandlers()
	deleteCalled := false
	svc := &mockGameService{
		create: func(_ context.Context, _ *models.Game, _ *models.UserGame) (*models.Game, error) {
			return nil, g_errors.New("op", g_errors.CodeConflict, g_errors.GameAlreadyExists)
		},
	}
	up := &mockUploadsIGDB{
		downloadAndSaveImage: func(_ string) (string, error) { return "cover.jpg", nil },
		deleteImage: func(_ string) error {
			deleteCalled = true
			return nil
		},
	}
	c := newIGDBController(t, twitch, igdb, svc, up)

	r := igdbRequest(t, []map[string]any{{"name": "Half-Life 2"}})
	w := httptest.NewRecorder()
	c.CreateMultiGamesIGDB(w, r)

	// все упали → 500
	assertStatus(t, w.Code, http.StatusInternalServerError)
	if !deleteCalled {
		t.Error("DeleteImage must be called when service.Create fails")
	}
}

// ============================================================================
// getTwitchToken — кеш
// ============================================================================

func TestGetTwitchToken_Cached(t *testing.T) {
	callCount := 0
	twitchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write(twitchTokenResponse())
	}))
	t.Cleanup(twitchSrv.Close)

	c := games.NewIGDBGamesController(
		&mockGameService{},
		newLogger(),
		&mockUploadsIGDB{},
		"http://igdb.example.com",
		twitchSrv.URL,
		"cid", "csecret",
	)

	if _, err := c.GetTwitchToken(); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := c.GetTwitchToken(); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if callCount != 1 {
		t.Errorf("loginTwitch called %d times, want 1", callCount)
	}
}

func TestGetTwitchToken_RefreshesExpired(t *testing.T) {
	callCount := 0
	twitchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write(twitchTokenResponse())
	}))
	t.Cleanup(twitchSrv.Close)

	c := games.NewIGDBGamesController(
		&mockGameService{},
		newLogger(),
		&mockUploadsIGDB{},
		"http://igdb.example.com",
		twitchSrv.URL,
		"cid", "csecret",
	)

	c.SetTokenForTest("old-token", time.Now().Add(-1*time.Hour))

	if _, err := c.GetTwitchToken(); err != nil {
		t.Fatalf("refresh call: %v", err)
	}
	if callCount != 1 {
		t.Errorf("loginTwitch called %d times, want 1", callCount)
	}
}

// ============================================================================
// extractSlug
// ============================================================================

func TestExtractSlug(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "normal url",
			url:  "https://www.igdb.com/games/half-life-2",
			want: "half-life-2",
		},
		{
			name: "trailing slash",
			url:  "https://www.igdb.com/games/half-life-2/",
			want: "half-life-2",
		},
		{
			name:    "no separator",
			url:     "half-life-2",
			wantErr: true,
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := games.ExtractSlug(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Error("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("slug: got %q, want %q", got, tc.want)
			}
		})
	}
}

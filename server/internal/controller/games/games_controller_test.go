// internal/controller/games/games_controller_test.go
package games_test

import (
	"bytes"
	"context"
	"encoding/json"
	"games_webapp/internal/controller"
	"games_webapp/internal/controller/games"
	g_errors "games_webapp/internal/errors"
	"games_webapp/internal/middleware"
	"games_webapp/internal/models"
	"games_webapp/internal/repository"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// ============================================================================
// Mocks
// ============================================================================
type mockGameService struct {
	getByID        func(ctx context.Context, id int) (*models.Game, error)
	getAll         func(ctx context.Context, userID int, params repository.GetAllParams) ([]models.UserGameResponse, int, error)
	searchAllGames func(ctx context.Context, query string) ([]models.Game, error)
	getUserGame    func(ctx context.Context, userID, gameID int) (*models.UserGame, error)
	getUserGames   func(ctx context.Context, userID int, params repository.GetUserGamesParams) ([]models.UserGameResponse, int, error)
	getGameStats   func(ctx context.Context, userID int) (repository.StatusCounts, error)
	create         func(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error)
	addUserGame    func(ctx context.Context, userID, gameID int) error
	update         func(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error)
	updateStatus   func(ctx context.Context, userID, gameID int, status models.GameStatus) error
	updatePriority func(ctx context.Context, userID, gameID int, priority int) error
	delete         func(ctx context.Context, gameID int) error
	deleteUserGame func(ctx context.Context, userID, gameID int) error
}

func (m *mockGameService) GetByID(ctx context.Context, id int) (*models.Game, error) {
	if m.getByID == nil {
		panic("unexpected call to GetByID")
	}
	return m.getByID(ctx, id)
}

func (m *mockGameService) GetAll(ctx context.Context, userID int, params repository.GetAllParams) ([]models.UserGameResponse, int, error) {
	if m.getAll == nil {
		panic("unexpected call to GetAll")
	}
	return m.getAll(ctx, userID, params)
}

func (m *mockGameService) SearchAllGames(ctx context.Context, query string) ([]models.Game, error) {
	if m.searchAllGames == nil {
		panic("unexpected call to SearchAllGames")
	}
	return m.searchAllGames(ctx, query)
}

func (m *mockGameService) GetUserGame(ctx context.Context, userID, gameID int) (*models.UserGame, error) {
	if m.getUserGame == nil {
		panic("unexpected call to GetUserGame")
	}
	return m.getUserGame(ctx, userID, gameID)
}

func (m *mockGameService) GetUserGames(ctx context.Context, userID int, params repository.GetUserGamesParams) ([]models.UserGameResponse, int, error) {
	if m.getUserGames == nil {
		panic("unexpected call to GetUserGames")
	}
	return m.getUserGames(ctx, userID, params)
}

func (m *mockGameService) GetGameStats(ctx context.Context, userID int) (repository.StatusCounts, error) {
	if m.getGameStats == nil {
		panic("unexpected call to GetGameStats")
	}
	return m.getGameStats(ctx, userID)
}

func (m *mockGameService) Create(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error) {
	if m.create == nil {
		panic("unexpected call to Create")
	}
	return m.create(ctx, game, userGame)
}

func (m *mockGameService) AddUserGame(ctx context.Context, userID, gameID int) error {
	if m.addUserGame == nil {
		panic("unexpected call to AddUserGame")
	}
	return m.addUserGame(ctx, userID, gameID)
}

func (m *mockGameService) Update(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error) {
	if m.update == nil {
		panic("unexpected call to Update")
	}
	return m.update(ctx, game, userGame)
}

func (m *mockGameService) UpdateStatus(ctx context.Context, userID, gameID int, status models.GameStatus) error {
	if m.updateStatus == nil {
		panic("unexpected call to UpdateStatus")
	}
	return m.updateStatus(ctx, userID, gameID, status)
}

func (m *mockGameService) UpdatePriority(ctx context.Context, userID, gameID int, priority int) error {
	if m.updatePriority == nil {
		panic("unexpected call to UpdatePriority")
	}
	return m.updatePriority(ctx, userID, gameID, priority)
}

func (m *mockGameService) Delete(ctx context.Context, gameID int) error {
	if m.delete == nil {
		panic("unexpected call to Delete")
	}
	return m.delete(ctx, gameID)
}

func (m *mockGameService) DeleteUserGame(ctx context.Context, userID, gameID int) error {
	if m.deleteUserGame == nil {
		panic("unexpected call to DeleteUserGame")
	}
	return m.deleteUserGame(ctx, userID, gameID)
}

type mockUploads struct {
	saveImage             func(image []byte, filename string) error
	deleteImage           func(filename string) error
	replaceImage          func(image []byte, oldFilename, newFilename string) error
	generateImageFilename func(url, contentType string) string
}

func (m *mockUploads) SaveImage(image []byte, filename string) error {
	if m.saveImage == nil {
		return nil
	}
	return m.saveImage(image, filename)
}

func (m *mockUploads) DeleteImage(filename string) error {
	if m.deleteImage == nil {
		return nil
	}
	return m.deleteImage(filename)
}

func (m *mockUploads) ReplaceImage(image []byte, old, new string) error {
	if m.replaceImage == nil {
		return nil
	}
	return m.replaceImage(image, old, new)
}

func (m *mockUploads) GenerateImageFilename(url, contentType string) string {
	if m.generateImageFilename == nil {
		return "generated.jpg"
	}
	return m.generateImageFilename(url, contentType)
}

// ============================================================================
// Test helpers
// ============================================================================

// withAuth инжектирует userID и isAdmin в контекст запроса,
// имитируя работу AuthMiddleware.
func withAuth(r *http.Request, userID int, isAdmin bool) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserIDKey, userID)
	ctx = context.WithValue(ctx, middleware.IsAdminKey, isAdmin)
	return r.WithContext(ctx)
}

// withChiParam добавляет URL-параметры chi в контекст.
func withChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// withAuthAndParam — комбинированный хелпер для большинства тестов.
func withAuthAndParam(r *http.Request, userID int, isAdmin bool, paramKey, paramValue string) *http.Request {
	return withChiParam(withAuth(r, userID, isAdmin), paramKey, paramValue)
}

func newController(svc *mockGameService, up *mockUploads) *games.GameController {
	return games.NewGameController(svc, noopLogger(), up)
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func stubGame() *models.Game {
	now := time.Now()
	return &models.Game{
		ID:        1,
		Title:     "Half-Life 2",
		URL:       "https://store.steampowered.com/app/220",
		Creator:   10,
		Image:     "hl2.jpg",
		CreatedAt: &now,
		UpdatedAt: &now,
	}
}

// decodeJSON читает тело ответа в структуру.
func decodeJSON(t *testing.T, body io.Reader, v any) {
	t.Helper()
	if err := json.NewDecoder(body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// assertStatus проверяет HTTP-статус.
func assertStatus(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("status: got %d, want %d", got, want)
	}
}

// newMultipartRequest создаёт multipart/form-data запрос с полями и файлом.
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

// ============================================================================
// GetByID
// ============================================================================

func TestGetByID_Success(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, id int) (*models.Game, error) {
			if id != 1 {
				t.Errorf("GetByID id: got %d, want 1", id)
			}
			return stubGame(), nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := withChiParam(httptest.NewRequest(http.MethodGet, "/games/1", nil), "id", "1")
	w := httptest.NewRecorder()
	c.GetByID(w, r)

	assertStatus(t, w.Code, http.StatusOK)

	var resp models.Game
	decodeJSON(t, w.Body, &resp)
	if resp.ID != 1 {
		t.Errorf("ID: got %d, want 1", resp.ID)
	}
}

func TestGetByID_InvalidID(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withChiParam(httptest.NewRequest(http.MethodGet, "/games/abc", nil), "id", "abc")
	w := httptest.NewRecorder()
	c.GetByID(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestGetByID_NotFound(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return nil, g_errors.New("op", g_errors.CodeNotFound, g_errors.GameNotFound)
		},
	}
	c := newController(svc, &mockUploads{})

	r := withChiParam(httptest.NewRequest(http.MethodGet, "/games/99", nil), "id", "99")
	w := httptest.NewRecorder()
	c.GetByID(w, r)

	assertStatus(t, w.Code, http.StatusNotFound)
}

// ============================================================================
// GetAll
// ============================================================================

func TestGetAll_Success(t *testing.T) {
	svc := &mockGameService{
		getAll: func(_ context.Context, userID int, params repository.GetAllParams) ([]models.UserGameResponse, int, error) {
			return []models.UserGameResponse{{Game: *stubGame()}}, 1, nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuth(httptest.NewRequest(http.MethodGet, "/games?page=1&page_size=10", nil), 10, false)
	w := httptest.NewRecorder()
	c.GetAll(w, r)

	assertStatus(t, w.Code, http.StatusOK)

	var resp controller.PaginationResponse
	decodeJSON(t, w.Body, &resp)
	if resp.Total != 1 {
		t.Errorf("Total: got %d, want 1", resp.Total)
	}
}

func TestGetAll_Unauthorized(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	// запрос без контекста авторизации
	r := httptest.NewRequest(http.MethodGet, "/games", nil)
	w := httptest.NewRecorder()
	c.GetAll(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestGetAll_TotalPagesBug(t *testing.T) {
	// 25 элементов, page_size=10 → должно быть 3 страницы
	svc := &mockGameService{
		getAll: func(_ context.Context, _ int, params repository.GetAllParams) ([]models.UserGameResponse, int, error) {
			return make([]models.UserGameResponse, params.PageSize), 25, nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuth(httptest.NewRequest(http.MethodGet, "/games?page=1&page_size=10", nil), 10, false)
	w := httptest.NewRecorder()
	c.GetAll(w, r)

	assertStatus(t, w.Code, http.StatusOK)

	var resp controller.PaginationResponse
	decodeJSON(t, w.Body, &resp)

	// Текущий баг: (25/10 - 1) / 10 = 0 вместо 3
	// Этот тест падает пока баг не исправлен.
	if resp.Pages != 3 {
		t.Errorf("Pages: got %d, want 3 (totalPages calculation bug)", resp.Pages)
	}
}

func TestGetAll_ServiceError(t *testing.T) {
	svc := &mockGameService{
		getAll: func(_ context.Context, _ int, _ repository.GetAllParams) ([]models.UserGameResponse, int, error) {
			return nil, 0, g_errors.New("op", g_errors.CodeInternal, "")
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuth(httptest.NewRequest(http.MethodGet, "/games", nil), 10, false)
	w := httptest.NewRecorder()
	c.GetAll(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

// ============================================================================
// SearchAllGames
// ============================================================================

func TestSearchAllGames_Success(t *testing.T) {
	svc := &mockGameService{
		searchAllGames: func(_ context.Context, query string) ([]models.Game, error) {
			if query != "Half" {
				t.Errorf("query: got %q, want %q", query, "Half")
			}
			return []models.Game{*stubGame()}, nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := httptest.NewRequest(http.MethodGet, "/games/search?title=Half", nil)
	w := httptest.NewRecorder()
	c.SearchAllGames(w, r)

	assertStatus(t, w.Code, http.StatusOK)
}

func TestSearchAllGames_ServiceError(t *testing.T) {
	svc := &mockGameService{
		searchAllGames: func(_ context.Context, _ string) ([]models.Game, error) {
			return nil, g_errors.New("op", g_errors.CodeInvalidInput, g_errors.EmptyQuery)
		},
	}
	c := newController(svc, &mockUploads{})

	r := httptest.NewRequest(http.MethodGet, "/games/search?title=", nil)
	w := httptest.NewRecorder()
	c.SearchAllGames(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

// ============================================================================
// GetUserGame
// ============================================================================

func TestGetUserGame_Success(t *testing.T) {
	svc := &mockGameService{
		getUserGame: func(_ context.Context, userID, gameID int) (*models.UserGame, error) {
			return &models.UserGame{UserID: userID, GameID: gameID, Status: models.StatusPlaying}, nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodGet, "/games/1/user-game", nil), 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.GetUserGame(w, r)

	assertStatus(t, w.Code, http.StatusOK)

	var resp models.UserGame
	decodeJSON(t, w.Body, &resp)
	if resp.Status != models.StatusPlaying {
		t.Errorf("Status: got %q, want playing", resp.Status)
	}
}

func TestGetUserGame_InvalidID(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodGet, "/games/abc/user-game", nil), 10, false, "id", "abc")
	w := httptest.NewRecorder()
	c.GetUserGame(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestGetUserGame_Unauthorized(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	// запрос без контекста авторизации — GetUserID вернёт ошибку
	r := withChiParam(httptest.NewRequest(http.MethodGet, "/games/1/user-game", nil), "id", "1")
	w := httptest.NewRecorder()
	c.GetUserGame(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestGetUserGame_ServiceError(t *testing.T) {
	svc := &mockGameService{
		getUserGame: func(_ context.Context, _, _ int) (*models.UserGame, error) {
			return nil, g_errors.New("op", g_errors.CodeNotFound, g_errors.GameNotFound)
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodGet, "/games/1/user-game", nil), 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.GetUserGame(w, r)

	assertStatus(t, w.Code, http.StatusNotFound)
}

// ============================================================================
// GetUserGames
// ============================================================================

func TestGetUserGames_Success(t *testing.T) {
	svc := &mockGameService{
		getUserGames: func(_ context.Context, _ int, _ repository.GetUserGamesParams) ([]models.UserGameResponse, int, error) {
			return []models.UserGameResponse{{Game: *stubGame()}}, 1, nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuth(httptest.NewRequest(http.MethodGet, "/games/user?page=1&page_size=10", nil), 10, false)
	w := httptest.NewRecorder()
	c.GetUserGames(w, r)

	assertStatus(t, w.Code, http.StatusOK)

	var resp controller.PaginationResponse
	decodeJSON(t, w.Body, &resp)
	if resp.Total != 1 {
		t.Errorf("Total: got %d, want 1", resp.Total)
	}
}

func TestGetUserGames_WithStatusFilter(t *testing.T) {
	var gotStatus *models.GameStatus
	svc := &mockGameService{
		getUserGames: func(_ context.Context, _ int, params repository.GetUserGamesParams) ([]models.UserGameResponse, int, error) {
			gotStatus = params.Status
			return nil, 0, nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuth(httptest.NewRequest(http.MethodGet, "/games/user?status=finished&page=1&page_size=10", nil), 10, false)
	w := httptest.NewRecorder()
	c.GetUserGames(w, r)

	assertStatus(t, w.Code, http.StatusOK)
	if gotStatus == nil || *gotStatus != models.StatusFinished {
		t.Errorf("Status filter: got %v, want finished", gotStatus)
	}
}

func TestGetUserGames_Unauthorized(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := httptest.NewRequest(http.MethodGet, "/games/user", nil)
	w := httptest.NewRecorder()
	c.GetUserGames(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestGetUserGames_ServiceError(t *testing.T) {
	svc := &mockGameService{
		getUserGames: func(_ context.Context, _ int, _ repository.GetUserGamesParams) ([]models.UserGameResponse, int, error) {
			return nil, 0, g_errors.New("op", g_errors.CodeInvalidInput, g_errors.InvalidStatus)
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuth(httptest.NewRequest(http.MethodGet, "/games/user?status=bad", nil), 10, false)
	w := httptest.NewRecorder()
	c.GetUserGames(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

// ============================================================================
// GetGameStats
// ============================================================================

func TestGetGameStats_Success(t *testing.T) {
	svc := &mockGameService{
		getGameStats: func(_ context.Context, _ int) (repository.StatusCounts, error) {
			return repository.StatusCounts{Finished: 5, Playing: 2}, nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuth(httptest.NewRequest(http.MethodGet, "/games/user/stats", nil), 10, false)
	w := httptest.NewRecorder()
	c.GetGameStats(w, r)

	assertStatus(t, w.Code, http.StatusOK)

	var resp repository.StatusCounts
	decodeJSON(t, w.Body, &resp)
	if resp.Finished != 5 {
		t.Errorf("Finished: got %d, want 5", resp.Finished)
	}
}

func TestGetGameStats_Unauthorized(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := httptest.NewRequest(http.MethodGet, "/games/user/stats", nil)
	w := httptest.NewRecorder()
	c.GetGameStats(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestGetGameStats_ServiceError(t *testing.T) {
	svc := &mockGameService{
		getGameStats: func(_ context.Context, _ int) (repository.StatusCounts, error) {
			return repository.StatusCounts{}, g_errors.New("op", g_errors.CodeInternal, "")
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuth(httptest.NewRequest(http.MethodGet, "/games/user/stats", nil), 10, false)
	w := httptest.NewRecorder()
	c.GetGameStats(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

// ============================================================================
// Create
// ============================================================================

func TestCreate_Success(t *testing.T) {
	svc := &mockGameService{
		create: func(_ context.Context, game *models.Game, _ *models.UserGame) (*models.Game, error) {
			game.ID = 42
			return game, nil
		},
	}
	up := &mockUploads{
		generateImageFilename: func(_, _ string) string { return "game.jpg" },
		saveImage:             func(_ []byte, _ string) error { return nil },
	}
	c := newController(svc, up)

	r := newMultipartRequest(t,
		map[string]string{
			"title":     "Portal",
			"url":       "http://portal.com",
			"status":    "planned",
			"priority":  "1",
			"developer": "Valve",
		},
		"image", "portal.jpg", []byte("fake-image-data"),
	)
	r = withAuth(r, 10, false)
	w := httptest.NewRecorder()
	c.Create(w, r)

	assertStatus(t, w.Code, http.StatusCreated)

	var resp models.Game
	decodeJSON(t, w.Body, &resp)
	if resp.ID != 42 {
		t.Errorf("ID: got %d, want 42", resp.ID)
	}
}

func TestCreate_Unauthorized(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := httptest.NewRequest(http.MethodPost, "/games", nil)
	w := httptest.NewRecorder()
	c.Create(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestCreate_InvalidMultipartForm(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	// тело не является валидным multipart — ParseMultipartForm вернёт ошибку
	r := httptest.NewRequest(http.MethodPost, "/games", strings.NewReader("not-a-multipart"))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=BOUNDARY")
	r = withAuth(r, 10, false)
	w := httptest.NewRecorder()
	c.Create(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestCreate_MissingImage(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	// валидный multipart, но поле "image" отсутствует
	r := newMultipartRequest(t,
		map[string]string{"title": "Portal", "url": "http://portal.com"},
		"", "", nil,
	)
	r = withAuth(r, 10, false)
	w := httptest.NewRecorder()
	c.Create(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestCreate_SaveImageError(t *testing.T) {
	up := &mockUploads{
		generateImageFilename: func(_, _ string) string { return "game.jpg" },
		saveImage: func(_ []byte, _ string) error {
			return g_errors.New("op", g_errors.CodeInternal, g_errors.CannotCreateFile)
		},
	}
	c := newController(&mockGameService{}, up)

	r := newMultipartRequest(t,
		map[string]string{"title": "Portal", "url": "http://portal.com", "status": "planned"},
		"image", "portal.jpg", []byte("fake"),
	)
	r = withAuth(r, 10, false)
	w := httptest.NewRecorder()
	c.Create(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

func TestCreate_ServiceError_DeletesImage(t *testing.T) {
	deleteCalled := false
	svc := &mockGameService{
		create: func(_ context.Context, _ *models.Game, _ *models.UserGame) (*models.Game, error) {
			return nil, g_errors.New("op", g_errors.CodeConflict, g_errors.GameAlreadyExists)
		},
	}
	up := &mockUploads{
		generateImageFilename: func(_, _ string) string { return "game.jpg" },
		saveImage:             func(_ []byte, _ string) error { return nil },
		deleteImage: func(filename string) error {
			deleteCalled = true
			if filename != "game.jpg" {
				t.Errorf("deleteImage: got %q, want %q", filename, "game.jpg")
			}
			return nil
		},
	}
	c := newController(svc, up)

	r := newMultipartRequest(t,
		map[string]string{"title": "Portal", "url": "http://portal.com", "status": "planned"},
		"image", "portal.jpg", []byte("fake"),
	)
	r = withAuth(r, 10, false)
	w := httptest.NewRecorder()
	c.Create(w, r)

	assertStatus(t, w.Code, http.StatusConflict)
	if !deleteCalled {
		t.Error("DeleteImage must be called when service.Create fails")
	}
}

// ============================================================================
// AddUserGame
// ============================================================================

func TestAddUserGame_Unauthorized(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withChiParam(httptest.NewRequest(http.MethodPost, "/games/5/add", nil), "id", "5")
	w := httptest.NewRecorder()
	c.AddUserGame(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestAddUserGame_InvalidID(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodPost, "/games/abc/add", nil), 10, false, "id", "abc")
	w := httptest.NewRecorder()
	c.AddUserGame(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestAddUserGame_ServiceError(t *testing.T) {
	svc := &mockGameService{
		addUserGame: func(_ context.Context, _, _ int) error {
			return g_errors.New("op", g_errors.CodeNotFound, g_errors.GameNotFound)
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodPost, "/games/5/add", nil), 10, false, "id", "5")
	w := httptest.NewRecorder()
	c.AddUserGame(w, r)

	assertStatus(t, w.Code, http.StatusNotFound)
}

func TestAddUserGame_Success(t *testing.T) {
	svc := &mockGameService{
		addUserGame: func(_ context.Context, userID, gameID int) error {
			if userID != 10 || gameID != 5 {
				t.Errorf("AddUserGame(%d, %d), want (10, 5)", userID, gameID)
			}
			return nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodPost, "/games/5/add", nil), 10, false, "id", "5")
	w := httptest.NewRecorder()
	c.AddUserGame(w, r)

	assertStatus(t, w.Code, http.StatusCreated)
}

// ============================================================================
// Update
// ============================================================================

func TestUpdate_Unauthorized(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withChiParam(httptest.NewRequest(http.MethodPut, "/games/1", nil), "id", "1")
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestUpdate_InvalidID(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodPut, "/games/abc", nil), 10, false, "id", "abc")
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestUpdate_GameNotFound(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return nil, g_errors.New("op", g_errors.CodeNotFound, g_errors.GameNotFound)
		},
	}
	c := newController(svc, &mockUploads{})

	body, _ := json.Marshal(map[string]any{"title": "x", "url": "http://x.com"})
	r := httptest.NewRequest(http.MethodPut, "/games/99", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 10, false, "id", "99")
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusNotFound)
}

func TestUpdate_Forbidden(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil // creator=10
		},
	}
	c := newController(svc, &mockUploads{})

	body, _ := json.Marshal(map[string]any{"title": "x", "url": "http://x.com"})
	r := httptest.NewRequest(http.MethodPut, "/games/1", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 99, false, "id", "1") // userID=99, creator=10, не admin
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusForbidden)
}

func TestUpdate_InvalidMultipartForm(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := httptest.NewRequest(http.MethodPut, "/games/1", strings.NewReader("not-a-multipart"))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=BOUNDARY")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestUpdate_Multipart_WithImage_ReplaceError(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil
		},
	}
	up := &mockUploads{
		generateImageFilename: func(_, _ string) string { return "new.jpg" },
		replaceImage: func(_ []byte, _, _ string) error {
			return g_errors.New("op", g_errors.CodeInternal, g_errors.CannotWriteFile)
		},
	}
	c := newController(svc, up)

	r := newMultipartRequest(t,
		map[string]string{"title": "HL2", "url": "http://hl2.com"},
		"image", "new.jpg", []byte("fake"),
	)
	r.Method = http.MethodPut
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

func TestUpdate_Multipart_WithoutImage_UsesExisting(t *testing.T) {
	var gotGame *models.Game
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil // image="hl2.jpg"
		},
		update: func(_ context.Context, game *models.Game, _ *models.UserGame) (*models.Game, error) {
			gotGame = game
			return game, nil
		},
	}
	c := newController(svc, &mockUploads{})

	// multipart без поля "image"
	r := newMultipartRequest(t,
		map[string]string{"title": "HL2", "url": "http://hl2.com", "status": "planned"},
		"", "", nil,
	)
	r.Method = http.MethodPut
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusOK)
	if gotGame.Image != "hl2.jpg" {
		t.Errorf("Image: got %q, want %q", gotGame.Image, "hl2.jpg")
	}
}

func TestUpdate_JSON_InvalidBody(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := httptest.NewRequest(http.MethodPut, "/games/1", strings.NewReader("not-json{{{"))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestUpdate_JSON_EmptyImageUsesExisting(t *testing.T) {
	var gotGame *models.Game
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil // image="hl2.jpg"
		},
		update: func(_ context.Context, game *models.Game, _ *models.UserGame) (*models.Game, error) {
			gotGame = game
			return game, nil
		},
	}
	c := newController(svc, &mockUploads{})

	// image не передаём — должен подставиться existingGame.Image
	body, _ := json.Marshal(map[string]any{"title": "HL2", "url": "http://hl2.com"})
	r := httptest.NewRequest(http.MethodPut, "/games/1", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusOK)
	if gotGame.Image != "hl2.jpg" {
		t.Errorf("Image: got %q, want %q", gotGame.Image, "hl2.jpg")
	}
}

func TestUpdate_UnsupportedContentType(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := httptest.NewRequest(http.MethodPut, "/games/1", strings.NewReader("data"))
	r.Header.Set("Content-Type", "text/plain")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestUpdate_ServiceError(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil
		},
		update: func(_ context.Context, _ *models.Game, _ *models.UserGame) (*models.Game, error) {
			return nil, g_errors.New("op", g_errors.CodeConflict, g_errors.GameAlreadyExists)
		},
	}
	c := newController(svc, &mockUploads{})

	body, _ := json.Marshal(map[string]any{"title": "HL2", "url": "http://hl2.com"})
	r := httptest.NewRequest(http.MethodPut, "/games/1", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusConflict)
}

func TestUpdate_AdminCanUpdate(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil // creator=10
		},
		update: func(_ context.Context, game *models.Game, _ *models.UserGame) (*models.Game, error) {
			return game, nil
		},
	}
	c := newController(svc, &mockUploads{})

	body, _ := json.Marshal(map[string]any{"title": "HL2", "url": "http://hl2.com"})
	r := httptest.NewRequest(http.MethodPut, "/games/1", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 99, true, "id", "1") // isAdmin=true, userID≠creator
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusOK)
}

func TestUpdate_JSON_StatusAndPriorityParsed(t *testing.T) {
	var gotUserGame *models.UserGame
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil
		},
		update: func(_ context.Context, game *models.Game, ug *models.UserGame) (*models.Game, error) {
			gotUserGame = ug
			return game, nil
		},
	}
	c := newController(svc, &mockUploads{})

	body, _ := json.Marshal(map[string]any{
		"title":    "HL2",
		"url":      "http://hl2.com",
		"status":   "finished",
		"priority": 7,
	})
	r := httptest.NewRequest(http.MethodPut, "/games/1", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.Update(w, r)

	assertStatus(t, w.Code, http.StatusOK)
	if gotUserGame.Status != models.StatusFinished {
		t.Errorf("Status: got %q, want finished", gotUserGame.Status)
	}
	if gotUserGame.Priority != 7 {
		t.Errorf("Priority: got %d, want 7", gotUserGame.Priority)
	}
}

// ============================================================================
// UpdateStatus
// ============================================================================

func TestUpdateStatus_Unauthorized(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withChiParam(httptest.NewRequest(http.MethodPut, "/games/1/status", nil), "id", "1")
	w := httptest.NewRecorder()
	c.UpdateStatus(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestUpdateStatus_InvalidID(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodPut, "/games/abc/status", nil), 10, false, "id", "abc")
	w := httptest.NewRecorder()
	c.UpdateStatus(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestUpdateStatus_InvalidBody(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := httptest.NewRequest(http.MethodPut, "/games/1/status", strings.NewReader("not-json"))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.UpdateStatus(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestUpdateStatus_ServiceError(t *testing.T) {
	svc := &mockGameService{
		updateStatus: func(_ context.Context, _, _ int, _ models.GameStatus) error {
			return g_errors.New("op", g_errors.CodeInvalidInput, g_errors.InvalidStatus)
		},
	}
	c := newController(svc, &mockUploads{})

	body, _ := json.Marshal(map[string]string{"status": "bad"})
	r := httptest.NewRequest(http.MethodPut, "/games/1/status", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.UpdateStatus(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestUpdateStatus_Success(t *testing.T) {
	var gotStatus models.GameStatus
	svc := &mockGameService{
		updateStatus: func(_ context.Context, _, _ int, status models.GameStatus) error {
			gotStatus = status
			return nil
		},
	}
	c := newController(svc, &mockUploads{})

	body, _ := json.Marshal(map[string]string{"status": "finished"})
	r := httptest.NewRequest(http.MethodPut, "/games/1/status", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.UpdateStatus(w, r)

	assertStatus(t, w.Code, http.StatusNoContent)
	if gotStatus != models.StatusFinished {
		t.Errorf("Status: got %q, want finished", gotStatus)
	}
}

// ============================================================================
// UpdatePriority
// ============================================================================

func TestUpdatePriority_Unauthorized(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withChiParam(httptest.NewRequest(http.MethodPut, "/games/1/priority", nil), "id", "1")
	w := httptest.NewRecorder()
	c.UpdatePriority(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestUpdatePriority_InvalidID(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodPut, "/games/abc/priority", nil), 10, false, "id", "abc")
	w := httptest.NewRecorder()
	c.UpdatePriority(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestUpdatePriority_InvalidBody(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := httptest.NewRequest(http.MethodPut, "/games/1/priority", strings.NewReader("not-json"))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.UpdatePriority(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestUpdatePriority_ServiceError(t *testing.T) {
	svc := &mockGameService{
		updatePriority: func(_ context.Context, _, _, _ int) error {
			return g_errors.New("op", g_errors.CodeInvalidInput, g_errors.InvalidPriority)
		},
	}
	c := newController(svc, &mockUploads{})

	body, _ := json.Marshal(map[string]int{"priority": 15})
	r := httptest.NewRequest(http.MethodPut, "/games/1/priority", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.UpdatePriority(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestUpdatePriority_Success(t *testing.T) {
	var gotPriority int
	svc := &mockGameService{
		updatePriority: func(_ context.Context, _, _ int, priority int) error {
			gotPriority = priority
			return nil
		},
	}
	c := newController(svc, &mockUploads{})

	body, _ := json.Marshal(map[string]int{"priority": 7})
	r := httptest.NewRequest(http.MethodPut, "/games/1/priority", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r = withAuthAndParam(r, 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.UpdatePriority(w, r)

	assertStatus(t, w.Code, http.StatusNoContent)
	if gotPriority != 7 {
		t.Errorf("Priority: got %d, want 7", gotPriority)
	}
}

// ============================================================================
// Delete
// ============================================================================

func TestDelete_Unauthorized(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withChiParam(httptest.NewRequest(http.MethodDelete, "/games/1", nil), "id", "1")
	w := httptest.NewRecorder()
	c.Delete(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestDelete_InvalidID(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodDelete, "/games/abc", nil), 10, false, "id", "abc")
	w := httptest.NewRecorder()
	c.Delete(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestDelete_GameNotFound(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return nil, g_errors.New("op", g_errors.CodeNotFound, g_errors.GameNotFound)
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodDelete, "/games/99", nil), 10, false, "id", "99")
	w := httptest.NewRecorder()
	c.Delete(w, r)

	assertStatus(t, w.Code, http.StatusNotFound)
}

func TestDelete_Forbidden(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil // creator=10
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodDelete, "/games/1", nil), 99, false, "id", "1")
	w := httptest.NewRecorder()
	c.Delete(w, r)

	assertStatus(t, w.Code, http.StatusForbidden)
}

func TestDelete_UploadError(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil
		},
	}
	up := &mockUploads{
		deleteImage: func(_ string) error {
			return g_errors.New("op", g_errors.CodeInternal, g_errors.CannotDeleteFile)
		},
	}
	c := newController(svc, up)

	r := withAuthAndParam(httptest.NewRequest(http.MethodDelete, "/games/1", nil), 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.Delete(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

func TestDelete_ServiceError(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil
		},
		delete: func(_ context.Context, _ int) error {
			return g_errors.New("op", g_errors.CodeInternal, "")
		},
	}
	up := &mockUploads{
		deleteImage: func(_ string) error { return nil },
	}
	c := newController(svc, up)

	r := withAuthAndParam(httptest.NewRequest(http.MethodDelete, "/games/1", nil), 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.Delete(w, r)

	assertStatus(t, w.Code, http.StatusInternalServerError)
}

func TestDelete_Success(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil
		},
		delete: func(_ context.Context, _ int) error { return nil },
	}
	up := &mockUploads{
		deleteImage: func(_ string) error { return nil },
	}
	c := newController(svc, up)

	r := withAuthAndParam(httptest.NewRequest(http.MethodDelete, "/games/1", nil), 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.Delete(w, r)

	assertStatus(t, w.Code, http.StatusNoContent)
}

func TestDelete_AdminCanDelete(t *testing.T) {
	svc := &mockGameService{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil // creator=10
		},
		delete: func(_ context.Context, _ int) error { return nil },
	}
	up := &mockUploads{
		deleteImage: func(_ string) error { return nil },
	}
	c := newController(svc, up)

	r := withAuthAndParam(httptest.NewRequest(http.MethodDelete, "/games/1", nil), 99, true, "id", "1")
	w := httptest.NewRecorder()
	c.Delete(w, r)

	assertStatus(t, w.Code, http.StatusNoContent)
}

// ============================================================================
// DeleteUserGame
// ============================================================================

func TestDeleteUserGame_Unauthorized(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withChiParam(httptest.NewRequest(http.MethodDelete, "/games/1/user-game", nil), "id", "1")
	w := httptest.NewRecorder()
	c.DeleteUserGame(w, r)

	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestDeleteUserGame_InvalidID(t *testing.T) {
	c := newController(&mockGameService{}, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodDelete, "/games/abc/user-game", nil), 10, false, "id", "abc")
	w := httptest.NewRecorder()
	c.DeleteUserGame(w, r)

	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestDeleteUserGame_ServiceError(t *testing.T) {
	svc := &mockGameService{
		deleteUserGame: func(_ context.Context, _, _ int) error {
			return g_errors.New("op", g_errors.CodeNotFound, g_errors.GameNotFound)
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodDelete, "/games/1/user-game", nil), 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.DeleteUserGame(w, r)

	assertStatus(t, w.Code, http.StatusNotFound)
}

func TestDeleteUserGame_Success(t *testing.T) {
	svc := &mockGameService{
		deleteUserGame: func(_ context.Context, userID, gameID int) error {
			if userID != 10 || gameID != 1 {
				t.Errorf("DeleteUserGame(%d, %d), want (10, 1)", userID, gameID)
			}
			return nil
		},
	}
	c := newController(svc, &mockUploads{})

	r := withAuthAndParam(httptest.NewRequest(http.MethodDelete, "/games/1/user-game", nil), 10, false, "id", "1")
	w := httptest.NewRecorder()
	c.DeleteUserGame(w, r)

	assertStatus(t, w.Code, http.StatusNoContent)
}

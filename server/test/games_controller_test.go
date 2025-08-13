package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"games_webapp/internal/controllers"
	"games_webapp/internal/middleware"
	"games_webapp/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MockGameService реализует мок для services.GameService
type MockGameService struct {
	mock.Mock
}

func (m *MockGameService) GetAll() ([]models.Game, error) {
	args := m.Called()
	return args.Get(0).([]models.Game), args.Error(1)
}

func (m *MockGameService) GetAllPaginatedForUser(userID int64, page, pageSize int) ([]models.UserGameResponse, int, error) {
	args := m.Called(userID, page, pageSize)
	return args.Get(0).([]models.UserGameResponse), args.Get(1).(int), args.Error(2)
}

func (m *MockGameService) GetByID(id int64) (*models.Game, error) {
	args := m.Called(id)
	return args.Get(0).(*models.Game), args.Error(1)
}

func (m *MockGameService) SearchAllGames(query string) ([]models.Game, error) {
	args := m.Called(query)
	return args.Get(0).([]models.Game), args.Error(1)
}

func (m *MockGameService) SearchUserGames(userID int64, query string) ([]models.Game, error) {
	args := m.Called(userID, query)
	return args.Get(0).([]models.Game), args.Error(1)
}

func (m *MockGameService) Create(game *models.Game) (*models.Game, error) {
	args := m.Called(game)
	return args.Get(0).(*models.Game), args.Error(1)
}

func (m *MockGameService) Update(game *models.Game) (*models.Game, error) {
	args := m.Called(game)
	return args.Get(0).(*models.Game), args.Error(1)
}

func (m *MockGameService) Delete(id int64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockGameService) GetGameByURL(url string) error {
	args := m.Called(url)
	return args.Error(0)
}

func (m *MockGameService) CreateUserGame(ug *models.UserGames) error {
	args := m.Called(ug)
	return args.Error(0)
}

func (m *MockGameService) GetDroppedGames(id int64) (int, error) {
	args := m.Called(id)
	return args.Get(0).(int), args.Error(0)
}

func (m *MockGameService) GetFinishedGames(id int64) (int, error) {
	args := m.Called(id)
	return args.Get(0).(int), args.Error(0)
}

func (m *MockGameService) GetPlayingGames(id int64) (int, error) {
	args := m.Called(id)
	return args.Get(0).(int), args.Error(0)
}

func (m *MockGameService) GetPlannedGames(id int64) (int, error) {
	args := m.Called(id)
	return args.Get(0).(int), args.Error(0)
}

func (m *MockGameService) UpdateUserGame(ug *models.UserGames) error {
	args := m.Called(ug)
	return args.Error(0)
}

func (m *MockGameService) DeleteUserGame(userID, gameID int64) error {
	args := m.Called(userID, gameID)
	return args.Error(0)
}

func setupController() (*controllers.GameController, *MockGameService, *MockUploads) {
	mockService := &MockGameService{}
	mockUploads := &MockUploads{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	controller := controllers.NewGameController(
		mockService,
		logger,
		mockUploads,
	)

	return controller, mockService, mockUploads
}

func TestGameController_GetAll(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		expectedGames := []models.Game{
			{ID: 1, Title: "Game 1"},
			{ID: 2, Title: "Game 2"},
		}

		mockService.On("GetAll").Return(expectedGames, nil)

		req := httptest.NewRequest("GET", "/api/games", nil)
		w := httptest.NewRecorder()

		ctrl.GetAll(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var games []models.Game
		err := json.NewDecoder(resp.Body).Decode(&games)
		assert.NoError(t, err)
		assert.Equal(t, expectedGames, games)

		mockService.AssertExpectations(t)
	})

	t.Run("error", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		mockService.On("GetAll").Return([]models.Game{}, errors.New("db error"))

		req := httptest.NewRequest("GET", "/api/games", nil)
		w := httptest.NewRecorder()

		ctrl.GetAll(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		mockService.AssertExpectations(t)
	})
}

func TestGameController_GetAllPaginatedForUser(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		expectedGames := []models.UserGameResponse{
			{
				Game: models.Game{
					ID:    1,
					Title: "Game 1",
				},
				Priority: 1,
				Status:   models.StatusPlanned,
			},
			{
				Game: models.Game{
					ID:    2,
					Title: "Game 2",
				},
				Priority: 2,
				Status:   models.StatusPlaying,
			},
		}
		userID := int64(1)
		page := 1
		pageSize := 10
		total := 20

		mockService.On("GetAllPaginatedForUser", userID, page, pageSize).
			Return(expectedGames, total, nil)

		req := httptest.NewRequest("GET", "/api/games/user?page=1&page_size=10", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
		w := httptest.NewRecorder()

		ctrl.GetAllPaginatedForUser(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response controllers.PaginationResponse
		err := json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)

		// Просто сравниваем response.Data с expectedGames напрямую,
		// так как Data уже имеет тип []models.UserGameResponse
		assert.Equal(t, expectedGames, response.Data)
		assert.Equal(t, total, response.Total)
		assert.Equal(t, page, response.Current)
		assert.Equal(t, pageSize, response.Size)

		mockService.AssertExpectations(t)
	})

	t.Run("unauthorized", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		req := httptest.NewRequest("GET", "/api/games/user", nil)
		w := httptest.NewRecorder()

		ctrl.GetAllPaginatedForUser(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		mockService.AssertNotCalled(t, "GetAllPaginatedForUser")
	})

	t.Run("invalid pagination params", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		userID := int64(1)
		expectedGames := []models.UserGameResponse{
			{
				Game: models.Game{
					ID:    1,
					Title: "Game 1",
				},
				Priority: 0,
				Status:   models.StatusPlanned,
			},
		}
		total := 1

		// Should default to page=1, pageSize=10
		mockService.On("GetAllPaginatedForUser", userID, 1, 10).Return(expectedGames, total, nil)

		req := httptest.NewRequest("GET", "/api/games/user?page=invalid&page_size=invalid", nil)
		// Используем middleware.UserIDKey вместо строки "user_id"
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
		w := httptest.NewRecorder()

		ctrl.GetAllPaginatedForUser(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response controllers.PaginationResponse
		err := json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Equal(t, expectedGames, response.Data) // Теперь типы совпадают
		assert.Equal(t, total, response.Total)

		mockService.AssertExpectations(t)
	})
}

func TestGameController_GetByID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		expectedGame := &models.Game{ID: 1, Title: "Test Game"}
		mockService.On("GetByID", int64(1)).Return(expectedGame, nil)

		req := httptest.NewRequest("GET", "/api/games/1", nil)
		w := httptest.NewRecorder()

		ctrl.GetByID(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var game models.Game
		err := json.NewDecoder(resp.Body).Decode(&game)
		assert.NoError(t, err)
		assert.Equal(t, *expectedGame, game)

		mockService.AssertExpectations(t)
	})

	t.Run("invalid id", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		req := httptest.NewRequest("GET", "/api/games/invalid", nil)
		w := httptest.NewRecorder()

		ctrl.GetByID(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		mockService.AssertNotCalled(t, "GetByID")
	})

	t.Run("not found", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		mockService.On("GetByID", int64(999)).Return(&models.Game{}, gorm.ErrRecordNotFound)

		req := httptest.NewRequest("GET", "/api/games/999", nil)
		w := httptest.NewRecorder()

		ctrl.GetByID(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		mockService.AssertExpectations(t)
	})
}

func TestGameController_SearchAllGames(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		query := "test"
		expectedGames := []models.Game{
			{ID: 1, Title: "Test Game 1"},
			{ID: 2, Title: "Test Game 2"},
		}

		mockService.On("SearchAllGames", query).Return(expectedGames, nil)

		req := httptest.NewRequest("GET", "/api/games/search?title="+query, nil)
		w := httptest.NewRecorder()

		ctrl.SearchAllGames(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var games []models.Game
		err := json.NewDecoder(resp.Body).Decode(&games)
		assert.NoError(t, err)
		assert.Equal(t, expectedGames, games)

		mockService.AssertExpectations(t)
	})

	t.Run("missing query", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		req := httptest.NewRequest("GET", "/api/games/search", nil)
		w := httptest.NewRecorder()

		ctrl.SearchAllGames(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		mockService.AssertNotCalled(t, "SearchAllGames")
	})
}

func TestGameController_SearchUserGames(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		userID := int64(1)
		query := "test"
		expectedGames := []models.Game{
			{ID: 1, Title: "Test Game 1"},
		}

		mockService.On("SearchUserGames", userID, query).Return(expectedGames, nil)

		req := httptest.NewRequest("GET", "/api/games/user/search?title="+query, nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
		w := httptest.NewRecorder()

		ctrl.SearchUserGames(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var games []models.Game
		err := json.NewDecoder(resp.Body).Decode(&games)
		assert.NoError(t, err)
		assert.Equal(t, expectedGames, games)

		mockService.AssertExpectations(t)
	})

	t.Run("unauthorized", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		req := httptest.NewRequest("GET", "/api/games/user/search?title=test", nil)
		w := httptest.NewRecorder()

		ctrl.SearchUserGames(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		mockService.AssertNotCalled(t, "SearchUserGames")
	})

	t.Run("missing query", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		userID := int64(1)
		req := httptest.NewRequest("GET", "/api/games/user/search", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
		w := httptest.NewRecorder()

		ctrl.SearchUserGames(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		mockService.AssertNotCalled(t, "SearchUserGames")
	})
}

func TestGameController_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, mockUploads := setupController()

		userID := int64(1)
		now := time.Now()

		// Setup form data
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		// Add form fields
		_ = writer.WriteField("title", "New Game")
		_ = writer.WriteField("preambula", "Description")
		_ = writer.WriteField("developer", "Dev")
		_ = writer.WriteField("publisher", "Pub")
		_ = writer.WriteField("year", "2023")
		_ = writer.WriteField("genre", "Action")
		_ = writer.WriteField("status", "planned")
		_ = writer.WriteField("url", "http://example.com")
		_ = writer.WriteField("priority", "5")

		// Add file
		part, _ := writer.CreateFormFile("image", "test.jpg")
		_, _ = part.Write([]byte("test image content"))

		writer.Close()

		expectedGame := &models.Game{
			Title:     "New Game",
			Preambula: "Description",
			Image:     "uuid.jpg",
			Developer: "Dev",
			Publisher: "Pub",
			Year:      "2023",
			Genre:     "Action",
			URL:       "http://example.com",
			CreatedAt: &now,
			UpdatedAt: &now,
		}

		mockUploads.On("SaveImage", mock.Anything, mock.Anything).Return(nil)
		mockService.On("Create", mock.AnythingOfType("*models.Game")).Return(expectedGame, nil)
		mockService.On("CreateUserGame", mock.AnythingOfType("*models.UserGames")).Return(nil)

		req := httptest.NewRequest("POST", "/api/games", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
		w := httptest.NewRecorder()

		ctrl.Create(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var game models.Game
		err := json.NewDecoder(resp.Body).Decode(&game)
		assert.NoError(t, err)
		assert.Equal(t, expectedGame.Title, game.Title)

		mockService.AssertExpectations(t)
		mockUploads.AssertExpectations(t)
	})

	t.Run("unauthorized", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		req := httptest.NewRequest("POST", "/api/games", nil)
		w := httptest.NewRecorder()

		ctrl.Create(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		mockService.AssertNotCalled(t, "Create")
	})

	t.Run("invalid form data", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		userID := int64(1)
		req := httptest.NewRequest("POST", "/api/games", bytes.NewReader([]byte("invalid")))
		req.Header.Set("Content-Type", "multipart/form-data")
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
		w := httptest.NewRecorder()

		ctrl.Create(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		mockService.AssertNotCalled(t, "Create")
	})
}

func TestGameController_Update(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, mockUploads := setupController()

		userID := int64(1)
		now := time.Now()

		// Setup form data
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		// Add form fields (отправляем как отдельные поля формы, а не JSON)
		_ = writer.WriteField("id", "1")
		_ = writer.WriteField("title", "Updated Game")
		_ = writer.WriteField("preambula", "New Desc")
		_ = writer.WriteField("image", "existing.jpg")
		_ = writer.WriteField("developer", "New Dev")
		_ = writer.WriteField("publisher", "New Pub")
		_ = writer.WriteField("year", "2024")
		_ = writer.WriteField("genre", "RPG")
		_ = writer.WriteField("status", string(models.StatusPlanned))
		_ = writer.WriteField("url", "http://example.com")
		_ = writer.WriteField("priority", "5")
		_ = writer.WriteField("created_at", now.Format(time.RFC3339))

		// Add file (optional)
		part, _ := writer.CreateFormFile("image", "test.jpg")
		_, _ = part.Write([]byte("test image content"))

		writer.Close()

		expectedGame := &models.Game{
			ID:        1,
			Title:     "Updated Game",
			Preambula: "New Desc",
			Image:     "existing.jpg",
			Developer: "New Dev",
			Publisher: "New Pub",
			Year:      "2024",
			Genre:     "RPG",
			URL:       "http://example.com",
			CreatedAt: &now,
			UpdatedAt: &now,
		}

		mockUploads.On("ReplaceImage", mock.Anything, "existing.jpg").Return(nil)
		mockService.On("Update", mock.AnythingOfType("*models.Game")).Return(expectedGame, nil)
		mockService.On("UpdateUserGame", mock.AnythingOfType("*models.UserGames")).Return(nil)

		req := httptest.NewRequest("PUT", "/api/games", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
		w := httptest.NewRecorder()

		ctrl.Update(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var game models.Game
		err := json.NewDecoder(resp.Body).Decode(&game)
		assert.NoError(t, err)
		assert.Equal(t, expectedGame.Title, game.Title)

		mockService.AssertExpectations(t)
		mockUploads.AssertExpectations(t)
	})

	t.Run("unauthorized", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		req := httptest.NewRequest("PUT", "/api/games", nil)
		w := httptest.NewRecorder()

		ctrl.Update(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		mockService.AssertNotCalled(t, "Update")
	})

	t.Run("invalid form data", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		userID := int64(1)
		req := httptest.NewRequest("PUT", "/api/games", bytes.NewReader([]byte("invalid")))
		req.Header.Set("Content-Type", "multipart/form-data")
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
		w := httptest.NewRecorder()

		ctrl.Update(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		mockService.AssertNotCalled(t, "Update")
	})
}

func TestGameController_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, mockUploads := setupController()

		userID := int64(1)
		gameID := int64(1)
		game := &models.Game{
			ID:    gameID,
			Image: "test.jpg",
		}

		// Настраиваем все ожидаемые вызовы методов
		mockService.On("GetByID", gameID).Return(game, nil)
		mockUploads.On("DeleteImage", game.Image).Return(nil)
		mockService.On("Delete", gameID).Return(nil)
		mockService.On("DeleteUserGame", userID, gameID).Return(nil) // Добавляем этот мок

		req := httptest.NewRequest("DELETE", "/api/games/1", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
		w := httptest.NewRecorder()

		ctrl.Delete(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Проверяем все ожидаемые вызовы
		mockService.AssertExpectations(t)
		mockUploads.AssertExpectations(t)
	})

	t.Run("invalid id", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		req := httptest.NewRequest("DELETE", "/api/games/invalid", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, int64(1)))
		w := httptest.NewRecorder()

		ctrl.Delete(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		mockService.AssertNotCalled(t, "Delete")
	})

	t.Run("game not found", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		gameID := int64(999)
		mockService.On("GetByID", gameID).Return(&models.Game{}, gorm.ErrRecordNotFound)

		req := httptest.NewRequest("DELETE", "/api/games/999", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, int64(1)))
		w := httptest.NewRecorder()

		ctrl.Delete(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		mockService.AssertExpectations(t)
	})

	t.Run("delete error", func(t *testing.T) {
		ctrl, mockService, mockUploads := setupController()

		gameID := int64(1)
		game := &models.Game{
			ID:    gameID,
			Image: "test.jpg",
		}

		mockService.On("GetByID", gameID).Return(game, nil)
		mockUploads.On("DeleteImage", game.Image).Return(nil)
		mockService.On("Delete", gameID).Return(errors.New("db error"))

		req := httptest.NewRequest("DELETE", "/api/games/1", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, int64(1)))
		w := httptest.NewRecorder()

		ctrl.Delete(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		mockService.AssertExpectations(t)
		mockUploads.AssertExpectations(t)
	})
	t.Run("image delete error", func(t *testing.T) {
		ctrl, mockService, mockUploads := setupController()

		gameID := int64(1)
		game := &models.Game{
			ID:    gameID,
			Image: "test.jpg",
		}

		mockService.On("GetByID", gameID).Return(game, nil)
		mockUploads.On("DeleteImage", game.Image).Return(errors.New("delete error"))
		mockService.On("Delete", gameID).Return(nil)
		mockService.On("DeleteUserGame", int64(1), gameID).Return(nil)

		req := httptest.NewRequest("DELETE", "/api/games/1", nil)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, int64(1)))
		w := httptest.NewRecorder()

		ctrl.Delete(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode) // Image delete error shouldn't fail the whole operation
		mockService.AssertExpectations(t)
		mockUploads.AssertExpectations(t)
	})
}

func TestGameController_CreateMultiGamesDB(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, mockUploads := setupController()

		input := controllers.RequestData{
			Games: []controllers.RequestGame{
				{Name: "Witcher 3", Source: "Wiki"},
				{Name: "Portal 2", Source: "Steam"},
			},
		}

		// Моки вызываются для каждого RequestGame
		mockService.On("GetGameByURL", mock.Anything).Return(nil).Times(len(input.Games))
		mockUploads.On("SaveImage", mock.Anything, mock.Anything).Return(nil).Times(len(input.Games))
		mockService.On("Create", mock.AnythingOfType("*models.Game")).Return(&models.Game{}, nil).Times(len(input.Games))
		mockService.On("CreateUserGame", mock.AnythingOfType("*models.UserGames")).Return(nil).Times(len(input.Games))

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/api/games/multi", bytes.NewReader(body))

		ctx := context.WithValue(req.Context(), middleware.UserIDKey, int64(1))
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		ctrl.CreateMultiGamesDB(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var response controllers.MultiGameResponse
		err := json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Equal(t, len(input.Games), len(response.Success))
		assert.Empty(t, response.Errors)

		mockService.AssertExpectations(t)
		mockUploads.AssertExpectations(t)
	})

	t.Run("too many games", func(t *testing.T) {
		ctrl, _, _ := setupController()

		// Создаем 101 игру с одинаковым источником, чтобы пройти проверку длины
		var games []controllers.RequestGame
		for i := 0; i < 101; i++ {
			games = append(games, controllers.RequestGame{Name: fmt.Sprintf("Game %d", i), Source: "Wiki"})
		}

		input := controllers.RequestData{Games: games}

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/api/games/multi", bytes.NewReader(body))
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, int64(1))
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		ctrl.CreateMultiGamesDB(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("partial success", func(t *testing.T) {
		ctrl, mockService, mockUploads := setupController()

		input := controllers.RequestData{
			Games: []controllers.RequestGame{
				{Name: "Game 1", Source: "Wiki"},
				{Name: "Game 2", Source: "Steam"},
			},
		}

		// Первый успешно создается
		mockService.On("GetGameByURL", mock.Anything).Return(nil).Once()
		mockUploads.On("SaveImage", mock.Anything, mock.Anything).Return(nil).Once()
		mockService.On("Create", mock.AnythingOfType("*models.Game")).Return(&models.Game{}, nil).Once()
		mockService.On("CreateUserGame", mock.AnythingOfType("*models.UserGames")).Return(nil).Once()

		// Второй — ошибка (например, игра уже существует)
		mockService.On("GetGameByURL", mock.Anything).Return(errors.New("game exists")).Once()

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/api/games/multi", bytes.NewReader(body))
		ctx := context.WithValue(req.Context(), middleware.UserIDKey, int64(1))
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		ctrl.CreateMultiGamesDB(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusMultiStatus, resp.StatusCode)

		var response controllers.MultiGameResponse
		err := json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(response.Success))
		assert.Equal(t, 1, len(response.Errors))

		mockService.AssertExpectations(t)
		mockUploads.AssertExpectations(t)
	})

	t.Run("unauthorized", func(t *testing.T) {
		ctrl, _, _ := setupController()

		input := controllers.RequestData{
			Games: []controllers.RequestGame{
				{Name: "Game 1", Source: "Wiki"},
			},
		}

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/api/games/multi", bytes.NewReader(body))
		// НЕ добавляем UserID в контекст
		w := httptest.NewRecorder()

		ctrl.CreateMultiGamesDB(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode) // контроллер возвращает 500, т.к. ошибка "unauthorized"
	})
}

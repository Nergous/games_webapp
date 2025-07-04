package controllers

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

func (m *MockGameService) GetAllPaginatedForUser(userID int64, page, pageSize int) ([]models.Game, int, error) {
	args := m.Called(userID, page, pageSize)
	return args.Get(0).([]models.Game), args.Get(1).(int), args.Error(2)
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

func (m *MockGameService) UpdateUserGame(ug *models.UserGames) error {
	args := m.Called(ug)
	return args.Error(0)
}

// MockUploads реализует мок для uploads.Uploads
type MockUploads struct {
	mock.Mock
}

func (m *MockUploads) SaveImage(data []byte, filename string) error {
	args := m.Called(data, filename)
	return args.Error(0)
}

func (m *MockUploads) DeleteImage(filename string) error {
	args := m.Called(filename)
	return args.Error(0)
}

func (m *MockUploads) ReplaceImage(data []byte, filename string) error {
	args := m.Called(data, filename)
	return args.Error(0)
}

func setupController() (*GameController, *MockGameService, *MockUploads) {
	mockService := &MockGameService{}
	mockUploads := &MockUploads{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	controller := NewGameController(
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

		expectedGames := []models.Game{
			{ID: 1, Title: "Game 1"},
			{ID: 2, Title: "Game 2"},
		}
		userID := int64(1)
		page := 1
		pageSize := 10
		total := 20

		mockService.On("GetAllPaginatedForUser", userID, page, pageSize).Return(expectedGames, total, nil)

		req := httptest.NewRequest("GET", "/api/games/user?page=1&page_size=10", nil)
		ctx := context.WithValue(req.Context(), "user_id", userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		ctrl.GetAllPaginatedForUser(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response PaginationResponse
		err := json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
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
		expectedGames := []models.Game{
			{ID: 1, Title: "Game 1"},
		}
		total := 1

		// Should default to page=1, pageSize=10
		mockService.On("GetAllPaginatedForUser", userID, 1, 10).Return(expectedGames, total, nil)

		req := httptest.NewRequest("GET", "/api/games/user?page=invalid&page_size=invalid", nil)
		ctx := context.WithValue(req.Context(), "user_id", userID)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		ctrl.GetAllPaginatedForUser(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response PaginationResponse
		err := json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Equal(t, expectedGames, response.Data)

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
		ctx := context.WithValue(req.Context(), "user_id", userID)
		req = req.WithContext(ctx)
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
		ctx := context.WithValue(req.Context(), "user_id", userID)
		req = req.WithContext(ctx)
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
		ctx := context.WithValue(req.Context(), "user_id", userID)
		req = req.WithContext(ctx)
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
		ctx := context.WithValue(req.Context(), "user_id", userID)
		req = req.WithContext(ctx)
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

		// Add form fields
		updateReq := UpdateGameRequest{
			GameID:    1,
			CreatedAt: &now,
			CreateGameRequest: CreateGameRequest{
				Title:     "Updated Game",
				Preambula: "New Desc",
				Image:     "existing.jpg",
				Developer: "New Dev",
				Publisher: "New Pub",
				Year:      "2024",
				Genre:     "RPG",
				Status:    models.StatusPlanned,
				URL:       "http://example.com",
				Priority:  5,
			},
		}

		reqData, _ := json.Marshal(updateReq)
		_ = writer.WriteField("data", string(reqData))

		// Add file (optional)
		part, _ := writer.CreateFormFile("image", "test.jpg")
		_, _ = part.Write([]byte("test image content"))

		writer.Close()

		expectedGame := &models.Game{
			ID:        updateReq.GameID,
			Title:     updateReq.Title,
			Preambula: updateReq.Preambula,
			Image:     updateReq.Image,
			Developer: updateReq.Developer,
			Publisher: updateReq.Publisher,
			Year:      updateReq.Year,
			Genre:     updateReq.Genre,
			URL:       updateReq.URL,
			CreatedAt: updateReq.CreatedAt,
			UpdatedAt: &now,
		}

		mockUploads.On("ReplaceImage", mock.Anything, updateReq.Image).Return(nil)
		mockService.On("Update", mock.AnythingOfType("*models.Game")).Return(expectedGame, nil)
		mockService.On("UpdateUserGame", mock.AnythingOfType("*models.UserGames")).Return(nil)

		req := httptest.NewRequest("PUT", "/api/games", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		ctx := context.WithValue(req.Context(), "user_id", userID)
		req = req.WithContext(ctx)
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
		ctx := context.WithValue(req.Context(), "user_id", userID)
		req = req.WithContext(ctx)
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

		gameID := int64(1)
		game := &models.Game{
			ID:    gameID,
			Image: "test.jpg",
		}

		mockService.On("GetByID", gameID).Return(game, nil)
		mockUploads.On("DeleteImage", game.Image).Return(nil)
		mockService.On("Delete", gameID).Return(nil)

		req := httptest.NewRequest("DELETE", "/api/games/1", nil)
		w := httptest.NewRecorder()

		ctrl.Delete(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		mockService.AssertExpectations(t)
		mockUploads.AssertExpectations(t)
	})

	t.Run("invalid id", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		req := httptest.NewRequest("DELETE", "/api/games/invalid", nil)
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

		req := httptest.NewRequest("DELETE", "/api/games/1", nil)
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

		input := requestData{
			Names: []string{"Witcher 3", "Portal 2"},
		}

		// Mock for each game
		for range input.Names {
			mockService.On("GetGameByURL", mock.Anything).Return(nil)
			mockUploads.On("SaveImage", mock.Anything, mock.Anything).Return(nil)
			mockService.On("Create", mock.AnythingOfType("*models.Game")).Return(&models.Game{}, nil)
			mockService.On("CreateUserGame", mock.AnythingOfType("*models.UserGames")).Return(nil)
		}

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/api/games/multi", bytes.NewReader(body))

		ctx := context.WithValue(req.Context(), middleware.UserIDKey, int64(1))
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()

		ctrl.CreateMultiGamesDB(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var response MultiGameResponse
		err := json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Equal(t, len(input.Names), len(response.Success))
		assert.Empty(t, response.Errors)

		mockService.AssertExpectations(t)
		mockUploads.AssertExpectations(t)
	})

	t.Run("too many games", func(t *testing.T) {
		ctrl, _, _ := setupController()

		names := make([]string, 101)
		for i := range names {
			names[i] = fmt.Sprintf("Game %d", i)
		}

		input := requestData{
			Names: names,
		}

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

		input := requestData{
			Names: []string{"Game 1", "Game 2"},
		}

		// First game succeeds
		mockService.On("GetGameByURL", mock.Anything).Return(nil).Once()
		mockUploads.On("SaveImage", mock.Anything, mock.Anything).Return(nil).Once()
		mockService.On("Create", mock.AnythingOfType("*models.Game")).Return(&models.Game{}, nil).Once()
		mockService.On("CreateUserGame", mock.AnythingOfType("*models.UserGames")).Return(nil).Once()

		// Second game fails
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

		var response MultiGameResponse
		err := json.NewDecoder(resp.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(response.Success))
		assert.Equal(t, 1, len(response.Errors))

		mockService.AssertExpectations(t)
		mockUploads.AssertExpectations(t)
	})

	t.Run("unauthorized", func(t *testing.T) {
		ctrl, _, _ := setupController()
		input := requestData{
			Names: []string{"Game 1"},
		}

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/api/games/multi", bytes.NewReader(body))
		w := httptest.NewRecorder()

		ctrl.CreateMultiGamesDB(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

func TestGameController_checkURLInDB(t *testing.T) {
	t.Run("success - URL exists", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		testURL := "https://example.com/game"
		mockService.On("GetGameByURL", testURL).Return(nil) // Нет ошибки = URL существует

		err := ctrl.checkURLInDB(testURL)

		assert.NoError(t, err)
		mockService.AssertExpectations(t)
	})

	t.Run("not found - URL doesn't exist", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		testURL := "https://example.com/not-found"
		expectedErr := errors.New("game not found")
		mockService.On("GetGameByURL", testURL).Return(expectedErr)

		err := ctrl.checkURLInDB(testURL)

		assert.Error(t, err)
		assert.EqualError(t, err, expectedErr.Error())
		mockService.AssertExpectations(t)
	})
}

func TestGameController_createSingleGame(t *testing.T) {
	t.Run("context cancel", func(t *testing.T) {
		ctrl, _, _ := setupController()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := ctrl.createSingleGame(ctx, "test game")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestGameController_parseGameWiki(t *testing.T) {
	t.Run("invalid HTML", func(t *testing.T) {
		ctrl, _, _ := setupController()
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("<html><body>Invalid content</body></html>"))
		}))
		defer ts.Close()

		_, err := ctrl.parseGameWiki(ts.URL)

		assert.Error(t, err)
	})
}

func TestGameController_downloadAndSaveImage(t *testing.T) {
	t.Run("invalid URL", func(t *testing.T) {
		ctrl, _, _ := setupController()

		_, err := ctrl.downloadAndSaveImage("invalid-url")

		assert.Error(t, err)
	})

	t.Run("invalid image", func(t *testing.T) {
		ctrl, _, mockUploads := setupController()

		// Create a test server that returns non-image content
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("not an image"))
		}))
		defer ts.Close()

		mockUploads.On("SaveImage", mock.Anything, mock.Anything).Return(nil)

		_, err := ctrl.downloadAndSaveImage(ts.URL)

		assert.Error(t, err)
		mockUploads.AssertNotCalled(t, "SaveImage")
	})
}

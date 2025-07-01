package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func (m *MockGameService) GetByID(id int64) (*models.Game, error) {
	args := m.Called(id)
	return args.Get(0).(*models.Game), args.Error(1)
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

func setupController() (*GameController, *MockGameService, *MockUploads) {
	mockService := &MockGameService{}
	mockUploads := &MockUploads{}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

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

	t.Run("empty URL", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		testURL := ""
		expectedErr := errors.New("empty URL")
		mockService.On("GetGameByURL", testURL).Return(expectedErr)

		err := ctrl.checkURLInDB(testURL)

		assert.Error(t, err)
		assert.EqualError(t, err, expectedErr.Error())
		mockService.AssertExpectations(t)
	})
}

func TestGameController_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		now := time.Now()
		input := CreateGameRequest{
			Title:     "New Game",
			Preambula: "Description",
			Image:     "image.jpg",
			Developer: "Dev",
			Publisher: "Pub",
			Year:      "2023",
			Genre:     "Action",
			Status:    models.StatusPlanned,
		}

		expectedGame := &models.Game{
			Title:     input.Title,
			Preambula: input.Preambula,
			Image:     input.Image,
			Developer: input.Developer,
			Publisher: input.Publisher,
			Year:      input.Year,
			Genre:     input.Genre,
			Status:    input.Status,
			CreatedAt: &now,
			UpdatedAt: &now,
		}

		mockService.On("Create", mock.AnythingOfType("*models.Game")).Return(expectedGame, nil)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/api/games", bytes.NewReader(body))
		w := httptest.NewRecorder()

		ctrl.Create(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var game models.Game
		err := json.NewDecoder(resp.Body).Decode(&game)
		assert.NoError(t, err)
		assert.Equal(t, input.Title, game.Title)

		mockService.AssertExpectations(t)
	})

	t.Run("invalid input", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		// Явно указываем, что Create не должен вызываться
		mockService.AssertNotCalled(t, "Create")

		req := httptest.NewRequest("POST", "/api/games", bytes.NewReader([]byte("invalid")))
		w := httptest.NewRecorder()

		ctrl.Create(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		// Дополнительная проверка, что Create действительно не вызывался
		mockService.AssertExpectations(t)
	})
}

func TestGameController_Update(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		now := time.Now()
		input := UpdateGameRequest{
			ID:        1,
			CreatedAt: &now,
			CreateGameRequest: CreateGameRequest{
				Title:     "Updated Game",
				Preambula: "New Desc",
				Image:     "new.jpg",
				Developer: "New Dev",
				Publisher: "New Pub",
				Year:      "2024",
				Genre:     "RPG",
				Status:    models.StatusPlanned,
			},
		}

		expectedGame := &models.Game{
			ID:        input.ID,
			Title:     input.Title,
			Preambula: input.Preambula,
			Image:     input.Image,
			Developer: input.Developer,
			Publisher: input.Publisher,
			Year:      input.Year,
			Genre:     input.Genre,
			Status:    input.Status,
			CreatedAt: input.CreatedAt,
			UpdatedAt: &now,
		}

		mockService.On("Update", mock.AnythingOfType("*models.Game")).Return(expectedGame, nil)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("PUT", "/api/games", bytes.NewReader(body))
		w := httptest.NewRecorder()

		ctrl.Update(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var game models.Game
		err := json.NewDecoder(resp.Body).Decode(&game)
		assert.NoError(t, err)
		assert.Equal(t, input.Title, game.Title)

		mockService.AssertExpectations(t)
	})
}

func TestGameController_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		mockService.On("Delete", int64(1)).Return(nil)

		req := httptest.NewRequest("DELETE", "/api/games/1", nil)
		w := httptest.NewRecorder()

		ctrl.Delete(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		mockService.AssertExpectations(t)
	})

	t.Run("error", func(t *testing.T) {
		ctrl, mockService, _ := setupController()

		mockService.On("Delete", int64(1)).Return(errors.New("db error"))

		req := httptest.NewRequest("DELETE", "/api/games/1", nil)
		w := httptest.NewRecorder()

		ctrl.Delete(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		mockService.AssertExpectations(t)
	})
}

func TestGameController_CreateMultiGamesDB(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl, mockService, mockUploads := setupController()

		input := requestData{
			Names: []string{"Game 1", "Game 2"},
		}

		for range input.Names {
			mockService.On("GetGameByURL", mock.Anything).Return(nil)
			mockService.On("Create", mock.AnythingOfType("*models.Game")).Return(&models.Game{}, nil)
		}

		// Настройка моков для createSingleGame
		mockUploads.On("SaveImage", mock.Anything, mock.Anything).Return(nil)

		body, _ := json.Marshal(input)
		req := httptest.NewRequest("POST", "/api/games/multi", bytes.NewReader(body))
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
		w := httptest.NewRecorder()

		ctrl.CreateMultiGamesDB(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

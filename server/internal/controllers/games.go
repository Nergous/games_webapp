package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"games_webapp/internal/middleware"
	"games_webapp/internal/models"
	"games_webapp/internal/storage/uploads"
	"games_webapp/utils"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type GameServicer interface {
	GetAll() ([]models.Game, error)
	GetByID(id int64) (*models.Game, error)
	SearchAllGames(query string) ([]models.Game, error)
	GetUserGames(userID int64, status *models.GameStatus, search, sortBy, sortOrder string, page, pageSize int) ([]models.UserGameResponse, int, error)
	GetUserGame(userID, gameID int64) (*models.UserGames, error)

	Create(game *models.Game) (*models.Game, error)
	Update(game *models.Game) (*models.Game, error)
	Delete(id int64) error
	GetGameByURL(url string) error
	CreateUserGame(ug *models.UserGames) error
	UpdateUserGame(ug *models.UserGames) error
	DeleteUserGame(userID, gameID int64) error
	GetFinishedGames(userID int64) (int, error)
	GetPlayingGames(userID int64) (int, error)
	GetPlannedGames(userID int64) (int, error)
	GetDroppedGames(userID int64) (int, error)
}

type RequestGame struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type RequestData struct {
	Games []RequestGame `json:"games"`
}

type CreateGameRequest struct {
	Title     string            `json:"title"`
	Preambula string            `json:"preambula"`
	Image     string            `json:"image"`
	Developer string            `json:"developer"`
	Publisher string            `json:"publisher"`
	Year      string            `json:"year"`
	Genre     string            `json:"genre"`
	Status    models.GameStatus `json:"status"`
	URL       string            `json:"url"`
	Priority  int               `json:"priority"`
	Creator   int64             `json:"creator"`
}

type UpdateGameRequest struct {
	GameID    int64      `json:"id"`
	CreatedAt *time.Time `json:"created_at"`
	CreateGameRequest
}

type MultiGameResponse struct {
	Success []*models.Game `json:"success"`
	Errors  []string       `json:"errors"`
}

type PaginationResponse struct {
	Total   int                       `json:"total"`   // Общее кол-во элементов
	Pages   int                       `json:"pages"`   // Общее кол-во страниц
	Current int                       `json:"current"` // Текущая страница
	Size    int                       `json:"size"`    // Количество элементов на странице
	Data    []models.UserGameResponse `json:"data"`
}

type GameController struct {
	service GameServicer
	log     *slog.Logger
	uploads uploads.IUploads
}

func NewGameController(s GameServicer, log *slog.Logger, u uploads.IUploads) *GameController {
	return &GameController{
		service: s,
		log:     log,
		uploads: u,
	}
}

func (c *GameController) GetAll(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.GetAll"

	res, err := c.service.GetAll()
	if err != nil {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("operation", op),
			slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) GetByID(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.GetByID"
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, ErrInvalidURL.Error(), http.StatusBadRequest)
		return
	}
	id := parts[3]

	id_s, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.log.Error(
			ErrInvalidID.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrInvalidID.Error(), http.StatusBadRequest)
		return
	}
	res, err := c.service.GetByID(int64(id_s))
	if err != nil {
		c.log.Error(
			ErrGetGame.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrGetGame.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) SearchAllGames(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.SearchAllGames"

	query := r.URL.Query().Get("title")
	if query == "" {
		http.Error(w, ErrMissingTitle.Error(), http.StatusBadRequest)
		return
	}

	games, err := c.service.SearchAllGames(query)
	if err != nil {
		c.log.Error("ошибка поиска", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(games); err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) GetUserGames(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	query := r.URL.Query()

	var status *models.GameStatus
	if s := query.Get("status"); s != "" {
		st := models.GameStatus(s)
		status = &st
	}

	search := strings.TrimSpace(query.Get("search"))

	sortBy := query.Get("sort_by")
	sortOrder := query.Get("sort_order")

	page, _ := strconv.Atoi(query.Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(query.Get("page_size"))
	if pageSize < 1 {
		pageSize = 10
	} else if pageSize > 100 {
		pageSize = 100
	}

	games, total, err := c.service.GetUserGames(userID, status, search, sortBy, sortOrder, page, pageSize)
	if err != nil {
		c.log.Error(ErrGetUserGames.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetUserGames.Error(), http.StatusInternalServerError)
		return
	}

	totalPages := total / pageSize
	if total%pageSize != 0 {
		totalPages++
	}

	response := PaginationResponse{
		Total:   total,
		Pages:   totalPages,
		Current: page,
		Size:    pageSize,
		Data:    games,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		c.log.Error(ErrGetUserGames.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetUserGames.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) Create(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Create"
	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		c.log.Error(ErrCreateGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrParsingForm.Error(), http.StatusBadRequest)
		return
	}

	request := CreateGameRequest{
		Title:     r.FormValue("title"),
		Preambula: r.FormValue("preambula"),
		Developer: r.FormValue("developer"),
		Publisher: r.FormValue("publisher"),
		Year:      r.FormValue("year"),
		Genre:     r.FormValue("genre"),
		URL:       r.FormValue("url"),
		Creator:   userID,
	}

	var err error
	if request.Priority, err = strconv.Atoi(r.FormValue("priority")); err != nil {
		request.Priority = 0
	}

	if request.Priority > 10 {
		c.log.Error(ErrCreateGame.Error(), slog.String("operation", op), slog.String("error", "priority > 10"))
		http.Error(w, ErrInvalidPriority.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		c.log.Error("image not provided", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrMissingImage.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		c.log.Error("failed to read image", slog.String("error", err.Error()))
		http.Error(w, ErrReadImage.Error(), http.StatusInternalServerError)
		return
	}

	imageFilename := uuid.New().String() + filepath.Ext(header.Filename)
	if err := c.uploads.SaveImage(imageData, imageFilename); err != nil {
		c.log.Error("failed to save image", slog.String("error", err.Error()))
		http.Error(w, ErrSaveImage.Error(), http.StatusInternalServerError)
		return
	}

	timeNow := time.Now()
	game := &models.Game{
		Title:     request.Title,
		Preambula: request.Preambula,
		Image:     imageFilename,
		Developer: request.Developer,
		Publisher: request.Publisher,
		Year:      request.Year,
		Genre:     request.Genre,
		URL:       request.URL,
		Creator:   request.Creator,
		CreatedAt: &timeNow,
		UpdatedAt: &timeNow,
	}

	res, err := c.service.Create(game)
	if err != nil {
		_ = c.uploads.DeleteImage(imageFilename)
		c.log.Error(ErrCreateGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrCreateGame.Error(), http.StatusInternalServerError)
		return
	}

	usrGame := &models.UserGames{
		UserID:   userID,
		GameID:   res.ID,
		Priority: request.Priority,
		Status:   request.Status,
	}

	if err := c.service.CreateUserGame(usrGame); err != nil {
		c.log.Error(ErrCreateUserGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrCreateUserGame.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrCreateGame.Error(), slog.String("error", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) Update(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Update"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		c.log.Error(ErrUpdateGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrInvalidID.Error(), http.StatusBadRequest)
		return
	}

	existingGame, err := c.service.GetByID(gameID)
	if err != nil {
		c.log.Error(ErrGetGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
		return
	}

	isAdmin := r.Context().Value(middleware.IsAdminKey).(bool)
	if !isAdmin && existingGame.Creator != userID {
		c.log.Error(ErrUpdateGame.Error(), slog.String("operation", op), slog.String("error", "user is not admin"))
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	contentType := r.Header.Get("Content-Type")
	var filename string
	var gameData map[string]interface{}
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			c.log.Error("Ошибка парсинга формы", slog.String("operation", op), slog.String("error", err.Error()))
			http.Error(w, ErrParsingForm.Error(), http.StatusBadRequest)
			return
		}

		file, h, err := r.FormFile("image")
		if err == nil {
			defer file.Close()

			oldFilename, err := c.service.GetByID(gameID)
			if err != nil {
				c.log.Error("Ошибка получения игры", slog.String("operation", op), slog.String("error", err.Error()))
				http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
				return
			}

			imageData, err := io.ReadAll(file)
			if err != nil {
				c.log.Error("Ошибка чтения изображения", slog.String("operation", op), slog.String("error", err.Error()))
				http.Error(w, ErrReadImage.Error(), http.StatusBadRequest)
				return
			}

			// get filename

			filename = h.Filename
			filename = generateImageFilename(filename, h.Header.Get("Content-Type"))

			if err := c.uploads.ReplaceImage(imageData, oldFilename.Image, filename); err != nil {
				c.log.Error("Ошибка замены изображения", slog.String("operation", op), slog.String("error", err.Error()))
				http.Error(w, ErrSaveImage.Error(), http.StatusInternalServerError)
				return
			}
		}
	} else if strings.HasPrefix(contentType, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&gameData); err != nil {
			c.log.Error("Ошибка парсинга JSON тела", slog.String("operation", op), slog.String("error", err.Error()))
			http.Error(w, ErrParsingJSON.Error(), http.StatusBadRequest)
			return
		}
		if img, ok := gameData["image"].(string); ok {
			filename = img
		}
	} else {
		http.Error(w, ErrInvalidRequest.Error(), http.StatusBadRequest)
		return
	}

	priority, err := strconv.Atoi(getFormValue(r, gameData, "priority"))
	if err != nil {
		priority = 0
	}
	if priority > 10 {
		http.Error(w, ErrInvalidPriority.Error(), http.StatusBadRequest)
		return
	}

	var createdAt *time.Time
	if createdAtStr := getFormValue(r, gameData, "created_at"); createdAtStr != "" {
		t, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			c.log.Error("Ошибка парсинга даты создания", slog.String("operation", op), slog.String("error", err.Error()))
			http.Error(w, ErrInvalidRequest.Error(), http.StatusBadRequest)
			return
		}
		createdAt = &t
	}

	timeNow := time.Now()

	game := &models.Game{
		ID:        gameID,
		Title:     getFormValue(r, gameData, "title"),
		Preambula: getFormValue(r, gameData, "preambula"),
		Image:     filename,
		Developer: getFormValue(r, gameData, "developer"),
		Publisher: getFormValue(r, gameData, "publisher"),
		Year:      getFormValue(r, gameData, "year"),
		Genre:     getFormValue(r, gameData, "genre"),
		URL:       getFormValue(r, gameData, "url"),
		Creator:   existingGame.Creator,
		CreatedAt: createdAt,
		UpdatedAt: &timeNow,
	}

	res, err := c.service.Update(game)
	if err != nil {
		c.log.Error(ErrUpdateGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateGame.Error(), http.StatusInternalServerError)
		return
	}

	userGame := &models.UserGames{
		UserID:   userID,
		GameID:   res.ID,
		Priority: priority,
		Status:   models.GameStatus(getFormValue(r, gameData, "status")),
	}

	if err := c.service.UpdateUserGame(userGame); err != nil {
		c.log.Error(ErrUpdateUserGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateUserGame.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrUpdateGame.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateGame.Error(), http.StatusInternalServerError)
		return
	}
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

func (c *GameController) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.UpdateStatus"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		c.log.Error(ErrUpdateGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrInvalidID.Error(), http.StatusBadRequest)
		return
	}

	existingGame, err := c.service.GetByID(gameID)
	if err != nil {
		c.log.Error(ErrGetGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
		return
	}

	request := UpdateStatusRequest{Status: "planned"}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.log.Error("Ошибка парсинга JSON тела", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrParsingJSON.Error(), http.StatusBadRequest)
		return
	}
	existingUserGame, err := c.service.GetUserGame(userID, gameID)
	if err != nil {
		c.log.Error(ErrGetGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
		return
	}

	userGame := &models.UserGames{
		UserID:   userID,
		GameID:   existingGame.ID,
		Priority: existingUserGame.Priority,
		Status:   models.GameStatus(request.Status),
	}

	if err := c.service.UpdateUserGame(userGame); err != nil {
		c.log.Error(ErrUpdateUserGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateUserGame.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(userGame); err != nil {
		c.log.Error(ErrUpdateGame.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateGame.Error(), http.StatusInternalServerError)
		return
	}
}

type UpdatePriorityRequest struct {
	Priority int `json:"priority"`
}

func (c *GameController) UpdatePriority(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.UpdatePriority"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		c.log.Error(ErrUpdateGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrInvalidID.Error(), http.StatusBadRequest)
		return
	}

	existingGame, err := c.service.GetByID(gameID)
	if err != nil {
		c.log.Error(ErrGetGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
		return
	}

	request := UpdatePriorityRequest{Priority: 0}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.log.Error("Ошибка парсинга JSON тела", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrParsingJSON.Error(), http.StatusBadRequest)
		return
	}
	existingUserGame, err := c.service.GetUserGame(userID, gameID)
	if err != nil {
		c.log.Error(ErrGetGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
		return
	}

	userGame := &models.UserGames{
		UserID:   userID,
		GameID:   existingGame.ID,
		Priority: request.Priority,
		Status:   existingUserGame.Status,
	}

	if err := c.service.UpdateUserGame(userGame); err != nil {
		c.log.Error(ErrUpdateUserGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateUserGame.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(userGame); err != nil {
		c.log.Error(ErrUpdateGame.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateGame.Error(), http.StatusInternalServerError)
		return
	}
}

func getFormValue(r *http.Request, gameData map[string]interface{}, key string) string {
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		return r.FormValue(key)
	} else if strings.HasPrefix(contentType, "application/json") {
		if val, ok := gameData[key]; ok {
			switch v := val.(type) {
			case string:
				return v
			case float64: // JSON numbers are usually unmarshalled as float64
				return strconv.FormatFloat(v, 'f', -1, 64)
			case int:
				return strconv.Itoa(v)
			default:
				return fmt.Sprint(v)
			}
		}
		return ""
	}
	return ""
}

func (c *GameController) Delete(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Delete"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, ErrInvalidURL.Error(), http.StatusBadRequest)
		return
	}
	id := parts[3]

	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.log.Error(
			ErrInvalidID.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrInvalidID.Error(), http.StatusBadRequest)
		return
	}

	// Получаем игру по ID
	game, err := c.service.GetByID(idInt)
	if err != nil {
		c.log.Error(
			"Не удалось получить игру для удаления",
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusNotFound)
		return
	}

	isAdmin := r.Context().Value(middleware.IsAdminKey).(bool)

	if userID == game.Creator || isAdmin {
		if err := c.uploads.DeleteImage(game.Image); err != nil {
			// Логируем, но не прерываем выполнение — игра всё равно будет удалена
			c.log.Error(
				"Ошибка удаления изображения",
				slog.String("operation", op),
				slog.String("filename", game.Image),
				slog.String("error", err.Error()))
		}

		// Удаляем запись игры
		err = c.service.Delete(idInt)
		if err != nil {
			c.log.Error(
				ErrDeleteGame.Error(),
				slog.String("operation", op),
				slog.String("id", id),
				slog.String("error", err.Error()))
			http.Error(w, ErrDeleteGame.Error(), http.StatusInternalServerError)
			return
		}

	}

	err = c.service.DeleteUserGame(userID, idInt)
	if err != nil {
		c.log.Error(
			ErrDeleteUserGame.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrDeleteUserGame.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) CreateMultiGamesDB(w http.ResponseWriter, r *http.Request) {
	var request RequestData

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.log.Error(ErrParsingJSON.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrParsingJSON.Error(), http.StatusBadRequest)
		return
	}

	fmt.Println(request)

	if len(request.Games) == 0 {
		c.log.Error(ErrNoGamesNames.Error(), slog.String("error", "no games names"))
		http.Error(w, ErrNoGamesNames.Error(), http.StatusBadRequest)
		return
	}

	if len(request.Games) > 100 {
		c.log.Error(ErrTooManyGames.Error(), slog.String("error", "over 100 games"))
		http.Error(w, ErrTooManyGames.Error(), http.StatusBadRequest)
		return
	}

	var (
		maxWorkers  = 10
		sem         = make(chan struct{}, maxWorkers)
		wg          sync.WaitGroup
		errChan     = make(chan error, len(request.Games))
		resultsChan = make(chan *models.Game, len(request.Games))
	)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)

	defer cancel()

	for _, game := range request.Games {
		sem <- struct{}{}
		wg.Add(1)
		go func(name, source string) {
			defer func() {
				<-sem
				wg.Done()
			}()

			game, err := c.createSingleGame(ctx, name, source)
			if err != nil {
				errChan <- err
				return
			}
			resultsChan <- game
		}(game.Name, game.Source)
	}

	go func() {
		wg.Wait()
		close(errChan)
		close(resultsChan)
	}()

	var errors []string
	var createdGames []*models.Game

	for err := range errChan {
		errors = append(errors, err.Error())
	}

	for res := range resultsChan {
		createdGames = append(createdGames, res)
	}

	response := MultiGameResponse{
		Success: createdGames,
		Errors:  errors,
	}

	status := http.StatusCreated

	if len(errors) > 0 {
		if len(createdGames) == 0 {
			status = http.StatusInternalServerError
		} else {
			status = http.StatusMultiStatus
		}
		c.log.Warn(
			ErrPartialCreate.Error(),
			slog.Int("success_count", len(createdGames)),
			slog.Int("error_count", len(errors)),
		)
		for _, err := range errors {
			c.log.Warn(ErrPartialCreate.Error(), slog.String("error", err))
		}
	} else {
		c.log.Info(
			"games created",
			slog.Int("count", len(createdGames)))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		c.log.Error(ErrCreateGame.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrCreateGame.Error(), http.StatusInternalServerError)
	}
}

func (c *GameController) createSingleGame(ctx context.Context, name, source string) (*models.Game, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	userID, ok := ctx.Value(middleware.UserIDKey).(int64)

	if !ok || userID <= 0 {
		return nil, ErrUnauthorized
	}

	var resultMap map[string]string
	var err error

	if source != "Wiki" && source != "Steam" {
		return nil, ErrInvalidSource
	}

	switch source {
	case "Wiki":
		resultMap, err = utils.ProcessWiki(name, c.checkURLInDB, c.log)
		if err != nil {
			return nil, err
		}
	case "Steam":
		resultMap, err = utils.ProcessSteam(name, c.checkURLInDB, c.log)
		if err != nil {
			return nil, err
		}
	}

	imageFilename, err := c.downloadAndSaveImage(resultMap["image"])
	if err != nil {
		c.log.Error(
			"failed to save image",
			slog.String("error", err.Error()),
			slog.String("game", name),
			slog.String("url", resultMap["image"]),
		)
		imageFilename = ""
	}

	timeNow := time.Now()
	game := &models.Game{
		Title:     resultMap["title"],
		Preambula: resultMap["description"],
		Image:     imageFilename,
		Developer: resultMap["developer"],
		Publisher: resultMap["publisher"],
		Year:      resultMap["year"],
		Genre:     resultMap["genre"],
		URL:       resultMap["url"],
		CreatedAt: &timeNow,
		UpdatedAt: &timeNow,
	}

	createdGame, err := c.service.Create(game)
	if err != nil {
		if imageFilename != "" {
			if delErr := c.uploads.DeleteImage(imageFilename); delErr != nil {
				c.log.Error(
					"failed to delete image",
					slog.String("error", delErr.Error()),
					slog.String("filename", imageFilename),
				)
			}
		}
		c.log.Error(
			ErrCreateGame.Error(),
			slog.String("error", err.Error()),
			slog.String("game", name))
		return nil, fmt.Errorf(ErrCreateGame.Error()+" %s : %s", name, err)

	}

	userGame := &models.UserGames{
		UserID:   userID,
		GameID:   createdGame.ID,
		Status:   models.StatusPlanned,
		Priority: 0,
	}

	if err := c.service.CreateUserGame(userGame); err != nil {
		c.log.Error(
			ErrCreateGame.Error(),
			slog.String("error", err.Error()),
			slog.String("game", name))
		return nil, fmt.Errorf(ErrCreateGame.Error()+" %s : %s", name, err)
	}
	return game, nil
}

func (c *GameController) checkURLInDB(url string) error {
	if err := c.service.GetGameByURL(url); err != nil {
		return err
	}
	return nil
}

func (c *GameController) downloadAndSaveImage(url string) (string, error) {
	if url == "" {
		return "", ErrInvalidURL
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", ErrImageURL
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", ErrDownloadImage
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return "", ErrUnexpectedImageType
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", ErrReadImage
	}
	filename := generateImageFilename(url, contentType)

	if err := c.uploads.SaveImage(imageData, filename); err != nil {
		return "", ErrSaveImage
	}

	return filename, nil
}

func generateImageFilename(url, contentType string) string {
	// Извлекаем расширение из Content-Type
	ext := ".jpg"
	switch {
	case strings.Contains(contentType, "png"):
		ext = ".png"
	case strings.Contains(contentType, "gif"):
		ext = ".gif"
	case strings.Contains(contentType, "webp"):
		ext = ".webp"
	}

	// Get current timestamp
	unique_string := time.Now().Format("20060102150405") + url

	// Создаем хэш от URL для уникального имени
	hash := sha256.Sum256([]byte(unique_string))
	return fmt.Sprintf("%x%s", hash[:8], ext)
}

type GameStats struct {
	Finished int `json:"finished"`
	Playing  int `json:"playing"`
	Planned  int `json:"planned"`
	Dropped  int `json:"dropped"`
}

func (c *GameController) GetGameStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	gs := GameStats{
		Finished: 0,
		Playing:  0,
		Planned:  0,
		Dropped:  0,
	}

	finished, err := c.service.GetFinishedGames(userID)
	if err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		return
	}
	playing, err := c.service.GetPlayingGames(userID)
	if err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		return
	}
	planned, err := c.service.GetPlannedGames(userID)
	if err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		return
	}

	dropped, err := c.service.GetDroppedGames(userID)
	if err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		return
	}

	gs.Finished = finished
	gs.Playing = playing
	gs.Planned = planned
	gs.Dropped = dropped

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(gs); err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

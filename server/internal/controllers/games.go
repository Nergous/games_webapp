package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"games_webapp/internal/middleware"
	"games_webapp/internal/models"
	"games_webapp/internal/storage/uploads"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
)

type GameServicer interface {
	GetAll() ([]models.Game, error)
	GetAllPaginatedForUser(userID int64, page, pageSize int) ([]models.UserGameResponse, int, error)
	GetByID(id int64) (*models.Game, error)
	SearchAllGames(query string) ([]models.Game, error)
	SearchUserGames(userID int64, query string) ([]models.Game, error)
	Create(game *models.Game) (*models.Game, error)
	Update(game *models.Game) (*models.Game, error)
	Delete(id int64) error
	GetGameByURL(url string) error
	CreateUserGame(ug *models.UserGames) error
	UpdateUserGame(ug *models.UserGames) error
	DeleteUserGame(userID, gameID int64) error
}

type RequestGame struct {
	Name   string `json:"names"`
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

func (c *GameController) GetAllPaginatedForUser(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.GetAllPaginatedForUser"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	query := r.URL.Query()
	page, err := strconv.Atoi(query.Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(query.Get("page_size"))
	if err != nil || pageSize < 1 {
		pageSize = 10
	} else if pageSize > 100 {
		pageSize = 100
	}

	games, total, err := c.service.GetAllPaginatedForUser(userID, page, pageSize)
	if err != nil {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("operation", op),
			slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
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
			ErrDelete.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrDelete.Error(), http.StatusBadRequest)
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
		http.Error(w, "missing title query", http.StatusBadRequest)
		return
	}

	games, err := c.service.SearchAllGames(query)
	if err != nil {
		c.log.Error("ошибка поиска", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, "failed to search games", http.StatusInternalServerError)
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

func (c *GameController) SearchUserGames(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.SearchUserGames"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	query := r.URL.Query().Get("title")
	if query == "" {
		http.Error(w, "missing title query", http.StatusBadRequest)
		return
	}

	games, err := c.service.SearchUserGames(userID, query)
	if err != nil {
		c.log.Error("ошибка поиска", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, "failed to search games", http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(games); err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) Create(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Create"
	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		c.log.Error(ErrCreate.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, "cannot parse form", http.StatusBadRequest)
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
	}

	var err error
	if request.Priority, err = strconv.Atoi(r.FormValue("priority")); err != nil {
		request.Priority = 0
	}

	if request.Priority > 10 {
		c.log.Error(ErrCreate.Error(), slog.String("operation", op), slog.String("error", "priority > 10"))
		http.Error(w, "priority > 10", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		c.log.Error("image not provided", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, "image not provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		c.log.Error("failed to read image", slog.String("error", err.Error()))
		http.Error(w, "failed to read image", http.StatusInternalServerError)
		return
	}

	imageFilename := uuid.New().String() + filepath.Ext(header.Filename)
	if err := c.uploads.SaveImage(imageData, imageFilename); err != nil {
		c.log.Error("failed to save image", slog.String("error", err.Error()))
		http.Error(w, "failed to save image", http.StatusInternalServerError)
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
		CreatedAt: &timeNow,
		UpdatedAt: &timeNow,
	}

	res, err := c.service.Create(game)
	if err != nil {
		_ = c.uploads.DeleteImage(imageFilename)
		c.log.Error(ErrCreate.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	usrGame := &models.UserGames{
		UserID:   userID,
		GameID:   res.ID,
		Priority: request.Priority,
		Status:   request.Status,
	}

	if err := c.service.CreateUserGame(usrGame); err != nil {
		c.log.Error(ErrCreate.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrCreate.Error(), slog.String("error", err.Error()))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) Update(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Update"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		c.log.Error("Ошибка парсинга формы", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	gameID, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		c.log.Error("Ошибка парсинга ID игры", slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, "invalid game id", http.StatusBadRequest)
		return
	}

	priority, err := strconv.Atoi(r.FormValue("priority"))
	if err != nil {
		priority = 0
	}
	if priority > 10 {
		http.Error(w, "priority > 10", http.StatusBadRequest)
		return
	}

	var createdAt *time.Time
	if createdAtStr := r.FormValue("created_at"); createdAtStr != "" {
		t, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			c.log.Error("Ошибка парсинга даты создания", slog.String("operation", op), slog.String("error", err.Error()))
			http.Error(w, "invalid created_at", http.StatusBadRequest)
			return
		}
		createdAt = &t
	}

	filename := r.FormValue("image") // старое имя (можем заменить, если будет файл)
	file, _, err := r.FormFile("image")
	if err == nil {
		defer file.Close()

		imageData, err := io.ReadAll(file)
		if err != nil {
			c.log.Error("Ошибка чтения изображения", slog.String("operation", op), slog.String("error", err.Error()))
			http.Error(w, "failed to read image", http.StatusBadRequest)
			return
		}
		if err := c.uploads.ReplaceImage(imageData, filename); err != nil {
			c.log.Error("Ошибка замены изображения", slog.String("operation", op), slog.String("error", err.Error()))
			http.Error(w, "failed to save image", http.StatusInternalServerError)
			return
		}
	}
	timeNow := time.Now()

	game := &models.Game{
		ID:        gameID,
		Title:     r.FormValue("title"),
		Preambula: r.FormValue("preambula"),
		Image:     filename,
		Developer: r.FormValue("developer"),
		Publisher: r.FormValue("publisher"),
		Year:      r.FormValue("year"),
		Genre:     r.FormValue("genre"),
		URL:       r.FormValue("url"),
		CreatedAt: createdAt,
		UpdatedAt: &timeNow,
	}

	res, err := c.service.Update(game)
	if err != nil {
		c.log.Error(ErrUpdate.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdate.Error(), http.StatusInternalServerError)
		return
	}

	userGame := &models.UserGames{
		UserID:   userID,
		GameID:   res.ID,
		Priority: priority,
		Status:   models.GameStatus(r.FormValue("status")),
	}

	if err := c.service.UpdateUserGame(userGame); err != nil {
		c.log.Error(ErrUpdate.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdate.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrUpdate.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrUpdate.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) Delete(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Delete"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
			ErrDelete.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrDelete.Error(), http.StatusBadRequest)
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
		http.Error(w, "game not found", http.StatusNotFound)
		return
	}

	// Удаляем файл изображения
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
			ErrDelete.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrDelete.Error(), http.StatusInternalServerError)
		return
	}
	err = c.service.DeleteUserGame(userID, idInt)
	if err != nil {
		c.log.Error(
			ErrDelete.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrDelete.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) CreateMultiGamesDB(w http.ResponseWriter, r *http.Request) {
	var request RequestData

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.log.Error(ErrBadRequest.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrBadRequest.Error(), http.StatusBadRequest)
		return
	}

	if len(request.Games) == 0 {
		c.log.Error(ErrBadRequest.Error(), slog.String("error", "no games names"))
		http.Error(w, ErrBadRequest.Error(), http.StatusBadRequest)
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
		c.log.Error(ErrEncoding.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrEncoding.Error(), http.StatusInternalServerError)
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
		return nil, errors.New("unauthorized")
	}

	var resultMap map[string]string
	var err error
	url := ""

	if source != "Wiki" && source != "Steam" {
		return nil, errors.New("invalid source")
	}

	switch source {
	case "Wiki":
		resultMap, err = c.processWiki(name)
		if err != nil {
			return nil, err
		}
	case "Steam":
		resultMap, err = c.processSteam(name)
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
		Preambula: resultMap["preambula"],
		Image:     imageFilename,
		Developer: resultMap["developer"],
		Publisher: resultMap["publisher"],
		Year:      resultMap["year"],
		Genre:     resultMap["genre"],
		URL:       url,
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
			ErrCreate.Error(),
			slog.String("error", err.Error()),
			slog.String("game", name))
		return nil, fmt.Errorf(ErrCreate.Error()+" %s : %s", name, err)

	}

	userGame := &models.UserGames{
		UserID:   userID,
		GameID:   createdGame.ID,
		Status:   models.StatusPlanned,
		Priority: 0,
	}

	if err := c.service.CreateUserGame(userGame); err != nil {
		c.log.Error(
			ErrCreate.Error(),
			slog.String("error", err.Error()),
			slog.String("game", name))
		return nil, fmt.Errorf(ErrCreate.Error()+" %s : %s", name, err)
	}
	return game, nil
}

func (c *GameController) processWiki(name string) (map[string]string, error) {
	url, err := c.findGameWiki(name)
	if err != nil {
		c.log.Error(
			ErrGameWiki.Error(),
			slog.String("error", err.Error()),
			slog.String("game", name))
		return nil, fmt.Errorf(ErrGameWiki.Error()+" %s : %s", name, err)
	}

	if err := c.checkURLInDB(url); err != nil {
		return nil, fmt.Errorf("game already exists: %s", url)
	}

	resultMap, err := c.parseGameWiki(url)
	if err != nil {
		c.log.Error(
			ErrParsing.Error(),
			slog.String("error", err.Error()),
			slog.String("game", name),
			slog.String("url", url))
		return nil, fmt.Errorf(ErrParsing.Error()+" %s - %s : %s", name, url, err)
	}

	return resultMap, nil
}

func (c *GameController) processSteam(name string) (map[string]string, error) {
	url, err := c.findGameSteam(name)
	if err != nil {
		c.log.Error(
			ErrGameSteam.Error(),
			slog.String("error", err.Error()),
			slog.String("game", name))

		result, err := c.processWiki(name)
		if err != nil {
			return nil, err
		}

		return result, nil
	}

	if err := c.checkURLInDB(url); err != nil {
		return nil, fmt.Errorf("game already exists: %s", url)
	}

	resultMap, err := c.parseGameSteam(url)
	if err != nil {
		c.log.Error(
			ErrParsing.Error(),
			slog.String("error", err.Error()),
			slog.String("game", name),
			slog.String("url", url))
		return nil, fmt.Errorf(ErrParsing.Error()+" %s - %s : %s", name, url, err)
	}

	return resultMap, nil
}

func (c *GameController) findGameSteam(name string) (string, error) {
	steamSearchUrl := "https://store.steampowered.com/search/suggest"

	params := url.Values{}
	params.Add("term", name)
	params.Add("f", "games")
	params.Add("cc", "RU")
	params.Add("l", "russian")
	params.Add("realm", "1")

	req, err := http.NewRequest("GET", steamSearchUrl, nil)
	if err != nil {
		return "", err
	}
	req.URL.RawQuery = params.Encode()

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.36")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.8,en-US;q=0.6,en;q=0.4")

	cookies := map[string]string{
		"steamCountry":         "RU|Moscow",
		"birthtime":            "473385601",
		"wants_mature_content": "1",
		"Steam_Language":       "russian",
	}

	for k, v := range cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Парсим HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	// Находим первую игру
	firstLink := ""
	doc.Find("a.match").EachWithBreak(func(i int, s *goquery.Selection) bool {
		href, exists := s.Attr("href")
		if exists {
			firstLink = href
			return false // остановить после первой
		}
		return true
	})

	if firstLink == "" {
		return "", fmt.Errorf("no games found for '%s'", name)
	}

	return firstLink, nil
}

func (c *GameController) parseGameSteam(url string) (map[string]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get Steam page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam returned status: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Получаем весь текст из блока
	detailsText := doc.Find("div.details_block, #genresAndManufacturer").Text()
	detailsText = strings.ReplaceAll(detailsText, "\n", " ") // Упрощаем текст
	detailsText = regexp.MustCompile(`\s+`).ReplaceAllString(detailsText, " ")

	result := make(map[string]string)

	// Парсим поля по простым шаблонам
	parseField := func(detailsText, pattern string) string {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(detailsText)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
		return ""
	}

	result["title"] = parseField(detailsText, `Title:\s*([^ ]+.*?)Genre:`)
	result["genre"] = parseField(detailsText, `Genre:\s*([^ ]+.*?)Developer:`)
	result["developer"] = parseField(detailsText, `Developer:\s*([^ ]+.*?)Publisher:`)
	result["publisher"] = parseField(detailsText, `Publisher:\s*([^ ]+.*?)Release Date:`)
	result["release_date"] = parseField(detailsText, `Release Date:\s*([^ ]+.*?)$`)

	// Извлекаем год из даты
	if year := regexp.MustCompile(`(20\d{2}|19\d{2})`).FindString(result["release_date"]); year != "" {
		result["year"] = year
	}

	// Дополнительные поля
	result["description"] = strings.TrimSpace(doc.Find("div.game_description_snippet").Text())
	if img, ok := doc.Find("img.game_header_image_full").Attr("src"); ok {
		result["image"] = img
	}

	// Проверка обязательных полей
	if result["title"] == "" || result["developer"] == "" {
		return nil, fmt.Errorf("failed to parse required fields")
	}

	return result, nil
}

func (c *GameController) findGameWiki(gameName string) (string, error) {
	gameName = url.QueryEscape(gameName)
	response, err := http.Get("https://ru.wikipedia.org/w/api.php?action=opensearch&format=json&formatversion=2&search=" + gameName + "&namespace=0&limit=10")
	if err != nil {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("error", err.Error()),
			slog.String("game", gameName))
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("error", err.Error()),
			slog.String("game", gameName))
		return "", err
	}
	var data []interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("error", err.Error()),
			slog.String("game", gameName))
		return "", err
	}

	if len(data) >= 4 {
		links, ok := data[3].([]interface{})
		if !ok || len(links) == 0 {
			c.log.Error(
				ErrGetGames.Error(),
				slog.String("error", "no links"),
				slog.String("game", gameName))
			return "", fmt.Errorf(ErrGetGames.Error()+" %s : %s", gameName, "no links")
		}

		firstLink, ok := links[0].(string)
		if !ok {
			c.log.Error(
				ErrGetGames.Error(),
				slog.String("error", "no first link"),
				slog.String("game", gameName))
			return "", fmt.Errorf(ErrGetGames.Error()+" %s : %s", gameName, "no first link")
		}
		return firstLink, nil
	} else {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("error", "no data"),
			slog.String("game", gameName))
		return "", fmt.Errorf(ErrGetGames.Error()+" %s : %s", gameName, "no data")
	}
}

func (c *GameController) parseGameWiki(url string) (map[string]string, error) {
	response, err := http.Get(url)
	if err != nil {
		c.log.Error(
			ErrGetGames.Error(),
			slog.String("error", err.Error()),
			slog.String("url", url),
		)
		return nil, ErrGetGames
	}
	defer response.Body.Close()

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		fmt.Println("----------------------")
		fmt.Println(err.Error())
		fmt.Println("----------------------")
		return nil, ErrParsing
	}

	var (
		title     string
		imgSrc    string
		developer string
		publisher string
		genre     string
		year      string
	)

	infobox := doc.Find("table.infobox").First()
	// Название игры (верхняя строка таблицы)
	title = infobox.Find("th.infobox-above").Text()
	title = strings.Join(strings.Fields(title), " ") // Удаляет лишние пробелы и переносы
	title = strings.TrimSpace(title)
	// Разработчик
	if selection := infobox.Find("th:contains('Разработчик')"); selection.Length() > 0 {
		developer = selection.Next().Text()
		developer = strings.TrimSpace(developer)
	} else if selection := infobox.Find("th:contains('Разработчики')"); selection.Length() > 0 {
		developer = strings.Split(selection.Next().Text(), " ")[0]
		developer = strings.TrimSpace(developer)
	}

	// Издатель/ Издатели
	if selection := infobox.Find("th:contains('Издатель')"); selection.Length() > 0 {
		publisher = selection.Next().Text()
		publisher = strings.TrimSpace(publisher)
	} else if selection := infobox.Find("th:contains('Издатели')"); selection.Length() > 0 {
		publisher = strings.TrimSpace(selection.Next().Text())
	}
	// Жанр
	genre = infobox.Find("th:contains('Жанр')").Next().Text()
	genre = strings.TrimSpace(genre)
	// Картинка (src = относительный путь)
	imgSrc, _ = infobox.Find("td.infobox-image img").Attr("src")
	imgFull := "https:" + imgSrc

	var releaseText string
	if selection := infobox.Find("th:contains('Даты выпуска')"); selection.Length() > 0 {
		releaseText = selection.Next().Text()
	} else if selection := infobox.Find("th:contains('Дата выпуска')"); selection.Length() > 0 {
		releaseText = selection.Next().Text()
	}
	re := regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	if match := re.FindStringSubmatch(releaseText); len(match) > 1 {
		year = match[1]
	} else if len(match) == 1 {
		year = match[0]
	}

	// Ищем следующий <p> после таблицы
	firstParagraph := ""
	found := false
	infoboxParent := infobox.Parent()

	// Идём по всем дочерним элементам родителя
	infoboxParent.Children().EachWithBreak(func(i int, s *goquery.Selection) bool {
		// Как только находим infobox, начинаем искать <p> после неё
		if s.Is("table.infobox") {
			found = true
			return true // идём дальше
		}

		if found && s.Is("p") {
			firstParagraph = strings.TrimSpace(s.Text())
			return false // остановить итерацию
		}

		return true // продолжать
	})

	if title == "" || firstParagraph == "" || imgFull == "" || developer == "" || publisher == "" || year == "" || genre == "" {
		fmt.Println("----------------------")
		fmt.Println("no data")
		fmt.Println("title:", title)
		fmt.Println("firstParagraph:", firstParagraph)
		fmt.Println("imgFull:", imgFull)
		fmt.Println("developer:", developer)
		fmt.Println("publisher:", publisher)
		fmt.Println("year:", year)
		fmt.Println("genre:", genre)
		fmt.Println("----------------------")
		return nil, ErrParsing
	}

	resultMap := map[string]string{
		"title":     title,
		"preambula": firstParagraph,
		"image":     imgFull,
		"developer": developer,
		"publisher": publisher,
		"year":      year,
		"genre":     genre,
	}

	return resultMap, nil
}

func (c *GameController) checkURLInDB(url string) error {
	if err := c.service.GetGameByURL(url); err != nil {
		return err
	}
	return nil
}

func (c *GameController) downloadAndSaveImage(url string) (string, error) {
	if url == "" {
		return "", errors.New("image url is empty")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image: %s", resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("unexpected content type: %s", contentType)
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	filename := generateImageFilename(url, contentType)

	if err := c.uploads.SaveImage(imageData, filename); err != nil {
		return "", err
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

	// Создаем хэш от URL для уникального имени
	hash := sha256.Sum256([]byte(url))
	return fmt.Sprintf("%x%s", hash[:8], ext)
}

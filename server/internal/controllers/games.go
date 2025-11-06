package controllers

import (
	"bytes"
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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ======================
// MAIN INTERFACE
// ======================

type GameServicer interface {
	GetByID(id int64) (*models.Game, error)
	SearchAllGames(query string) ([]models.Game, error)
	GetUserGames(userID int64, status *models.GameStatus, search, sortBy, sortOrder string, page, pageSize int) ([]models.UserGameResponse, int, error)
	GetUserGame(userID, gameID int64) (*models.UserGames, error)
	GetGamesPaginated(userID int64, search, sortBy, sortOrder string, page, pageSize int) ([]models.UserGameResponse, int, error)
	GetFlex(userID int64, fields []string, where []models.WhereQuery, order []models.Sort, limit uint32, offset uint32) ([]models.UserGameResponse, error)

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

// ======================
// CONSTRUCTOR
// ======================

type GameController struct {
	service            GameServicer
	log                *slog.Logger
	uploads            uploads.IUploads
	twitchClientId     string
	twitchClientSecret string
}

func NewGameController(s GameServicer, log *slog.Logger, u uploads.IUploads, twitchClientId, twitchClientSecret string) *GameController {
	return &GameController{
		service:            s,
		log:                log,
		uploads:            u,
		twitchClientId:     twitchClientId,
		twitchClientSecret: twitchClientSecret,
	}
}

// ======================
// GETTERS
// ======================

type PaginationResponse struct {
	Total   int                       `json:"total"`   // Общее кол-во элементов
	Pages   int                       `json:"pages"`   // Общее кол-во страниц
	Current int                       `json:"current"` // Текущая страница
	Size    int                       `json:"size"`    // Количество элементов на странице
	Data    []models.UserGameResponse `json:"data"`
}

func (c *GameController) GetAll(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.GetAll"
	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok {
		c.log.Error(ErrUnauthorized.Error(), slog.String("operation", op))
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	query := r.URL.Query()
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

	games, total, err := c.service.GetGamesPaginated(userID, search, sortBy, sortOrder, page, pageSize)
	if err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("operation", op), slog.String("error", err.Error()))
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
		c.log.Error(ErrGetGames.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) GetByID(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.GetByID"
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		c.log.Error(ErrInvalidURL.Error(), slog.String("operation", op))
		http.Error(w, ErrGetGames.Error(), http.StatusBadRequest)
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
		http.Error(w, ErrGetGames.Error(), http.StatusBadRequest)
		return
	}
	res, err := c.service.GetByID(int64(id_s))
	if err != nil {
		c.log.Error(
			ErrGetGame.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrGetGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

func (c *GameController) GetUserGames(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.GetUserGames"
	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok {
		c.log.Error(ErrUnauthorized.Error(), slog.String("operation", op))
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
		c.log.Error(ErrGetUserGames.Error(), slog.String("operation", op), slog.String("error", err.Error()))
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
		c.log.Error(ErrGetUserGames.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

type FlexRequest struct {
	UserID int64               `json:"user_id"`
	Fields []string            `json:"fields"`
	Where  []models.WhereQuery `json:"where"`
	Order  []models.Sort       `json:"order"`
	Limit  uint32              `json:"limit"`
	Offset uint32              `json:"offset"`
}

func (c *GameController) GetFlex(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.GetFlex"

	var req FlexRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.log.Error(ErrInvalidRequest.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrInvalidRequest.Error(), http.StatusBadRequest)
		return
	}

	games, err := c.service.GetFlex(req.UserID, req.Fields, req.Where, req.Order, req.Limit, req.Offset)
	if err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(games); err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

// ======================
// SEARCH
// ======================

func (c *GameController) SearchAllGames(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.SearchAllGames"

	query := r.URL.Query().Get("title")
	if query == "" {
		c.log.Error(ErrMissingTitle.Error(), slog.String("operation", op))
		http.Error(w, ErrMissingTitle.Error(), http.StatusBadRequest)
		return
	}

	games, err := c.service.SearchAllGames(query)
	if err != nil {
		c.log.Error(ErrSearching.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrSearching.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(games); err != nil {
		c.log.Error(ErrSearching.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrSearching.Error(), http.StatusInternalServerError)
		return
	}
}

// ======================
// CREATE
// ======================

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

type RequestGame struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type RequestData struct {
	Games []RequestGame `json:"games"`
}

type GameError struct {
	Name string `json:"name"`
	Err  string `json:"error"`
}

type MultiGameResponse struct {
	Success []*models.Game `json:"success"`
	Errors  []*GameError   `json:"errors"`
}

func (c *GameController) Create(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Create"
	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		c.log.Error(ErrUnauthorized.Error(), slog.String("operation", op))
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		c.log.Error(ErrParsingForm.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrCreateGame.Error(), http.StatusBadRequest)
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
		c.log.Error(ErrInvalidPriority.Error(), slog.String("operation", op), slog.String("error", "priority > 10"))
		http.Error(w, ErrCreateGame.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		c.log.Error(ErrMissingImage.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrCreateGame.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		c.log.Error(ErrReadImage.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrCreateGame.Error(), http.StatusInternalServerError)
		return
	}

	imageFilename := uuid.New().String() + filepath.Ext(header.Filename)
	if err := c.uploads.SaveImage(imageData, imageFilename); err != nil {
		c.log.Error(ErrSaveImage.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrCreateGame.Error(), http.StatusInternalServerError)
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
		http.Error(w, ErrCreateGame.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		c.log.Error(ErrCreateGame.Error(), slog.String("error", err.Error()))
		http.Error(w, ErrCreateGame.Error(), http.StatusInternalServerError)
		return
	}
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

func (c *GameController) CreateMultiGamesIGDB(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.CreateMultiGamesIGDB"

	var request RequestData

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.log.Error(ErrParsingJSON.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrCreateGame.Error(), http.StatusBadRequest)
		return
	}

	if len(request.Games) == 0 {
		c.log.Error(ErrNoGamesNames.Error(), slog.String("operation", op), slog.String("error", "no games names"))
		http.Error(w, ErrCreateGame.Error(), http.StatusBadRequest)
		return
	}

	if len(request.Games) > 100 {
		c.log.Error(ErrTooManyGames.Error(), slog.String("operation", op), slog.String("error", "over 100 games"))
		http.Error(w, ErrTooManyGames.Error(), http.StatusBadRequest)
		return
	}

	access, err := c.loginTwitch()
	if err != nil {
		c.log.Error(ErrLoginTwitch.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrCreateGame.Error(), http.StatusInternalServerError)
		return
	}

	var (
		maxWorkers  = 10
		sem         = make(chan struct{}, maxWorkers)
		wg          sync.WaitGroup
		errChan     = make(chan GameError, len(request.Games))
		resultsChan = make(chan *models.Game, len(request.Games))
	)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)

	defer cancel()

	for _, game := range request.Games {
		sem <- struct{}{}
		wg.Add(1)
		go func(name, source string, access *TwitchLoginResponse) {
			defer func() {
				<-sem
				wg.Done()
			}()

			game, err := c.createThroughIGDB(ctx, name, access)
			if err != nil {
				errChan <- GameError{Name: name, Err: err.Error()}
				return
			}
			resultsChan <- game
		}(game.Name, game.Source, access)
	}

	go func() {
		wg.Wait()
		close(errChan)
		close(resultsChan)
	}()

	var errors []*GameError
	var createdGames []*models.Game

	for err := range errChan {
		errors = append(errors, &err)
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
			slog.String("operation", op),
			slog.Int("success_count", len(createdGames)),
			slog.Int("error_count", len(errors)),
		)
		for _, err := range errors {
			c.log.Warn(ErrPartialCreate.Error(), slog.String("operation", op), slog.String("error", err.Err))
		}
	} else {
		c.log.Info(
			"games created",
			slog.Int("count", len(createdGames)))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		c.log.Error(ErrCreateGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrCreateGame.Error(), http.StatusInternalServerError)
	}
}

func (c *GameController) createThroughIGDB(ctx context.Context, name string, access *TwitchLoginResponse) (*models.Game, error) {
	const op = "controllers.games.createThroughIGDB"
	select {
	case <-ctx.Done():
		return nil, ErrUnknown
	default:
	}

	userID, ok := ctx.Value(middleware.UserIDKey).(int64)

	if !ok || userID <= 0 {
		return nil, ErrUnauthorized
	}

	var err error

	result, err := c.getDataFromIGDB(name, access)
	if err != nil {
		return nil, ErrCreateGame
	}

	imageFilename, err := c.downloadAndSaveImage(result["cover_url"])
	if err != nil {
		c.log.Error(
			"failed to save image",
			slog.String("operation", op),
			slog.String("error", err.Error()),
			slog.String("game", name),
			slog.String("url", result["cover"]),
		)
		imageFilename = ""
	}

	releaseDate := result["release_date"]
	releaseDate = strings.Split(releaseDate, "-")[0]

	timeNow := time.Now()
	game := &models.Game{
		Title:     result["name"],
		Preambula: result["summary"],
		Image:     imageFilename,
		Developer: result["developers"],
		Publisher: result["publishers"],
		Year:      releaseDate,
		Genre:     result["genres"],
		URL:       result["url"],
		CreatedAt: &timeNow,
		UpdatedAt: &timeNow,
	}

	createdGame, err := c.service.Create(game)
	if err != nil {
		if imageFilename != "" {
			if delErr := c.uploads.DeleteImage(imageFilename); delErr != nil {
				c.log.Error(
					"failed to delete image",
					slog.String("operation", op),
					slog.String("error", delErr.Error()),
					slog.String("filename", imageFilename),
				)
			}
		}
		c.log.Error(
			ErrCreateGame.Error(),
			slog.String("operation", op),
			slog.String("error", err.Error()),
			slog.String("game", name))
		return nil, ErrCreateGame
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
			slog.String("operation", op),
			slog.String("error", err.Error()),
			slog.String("game", name))
		return nil, ErrCreateGame
	}
	return game, nil
}

func (c *GameController) getDataFromIGDB(name string, access *TwitchLoginResponse) (map[string]string, error) {
	const op = "controllers.games.getDataFromIGDB"

	url := "https://api.igdb.com/v4/games"

	body := fmt.Sprintf(`
		search "%s";
		fields
			name,
			summary,
			url,
			cover.url,
			involved_companies.company.name,
			involved_companies.publisher,
			involved_companies.developer,
			first_release_date,
			genres.name;
		where version_parent = null & game_type = (0, 8, 9, 10) & (aggregated_rating != null | (aggregated_rating = null & hypes != null & hypes > 10));
		limit 1;
	`, name)

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(body))
	if err != nil {
		c.log.Error("ошибка при создании запроса", slog.String("operation", op), slog.String("error", err.Error()))
		return nil, ErrCreateGame
	}

	req.Header.Set("Client-ID", c.twitchClientId)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", access.AccessToken))
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.log.Error("ошибка при выполнении запроса", slog.String("operation", op), slog.String("error", err.Error()))
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.log.Error("ошибка при чтении тела ответа", slog.String("operation", op), slog.String("error", err.Error()))
		return nil, ErrCreateGame
	}

	var result []struct {
		Name             string `json:"name"`
		Summary          string `json:"summary"`
		FirstReleaseDate int    `json:"first_release_date"`
		URL              string `json:"url"`
		Cover            *struct {
			URL string `json:"url"`
		} `json:"cover"`
		InvolvedCompanies []struct {
			Company *struct {
				Name string `json:"name"`
			} `json:"company"`
			Publisher bool `json:"publisher"`
			Developer bool `json:"developer"`
		} `json:"involved_companies"`
		Genres []struct {
			Name string `json:"name"`
		} `json:"genres"`
	}

	err = json.Unmarshal(bodyBytes, &result)
	if err != nil {
		c.log.Error("ошибка при парсинге тела ответа", slog.String("operation", op), slog.String("error", err.Error()))
		return nil, ErrCreateGame
	}

	if len(result) == 0 {
		c.log.Error("игра не найдена", slog.String("operation", op), slog.String("error", "game not found"))
		return nil, ErrGameNotFound
	}

	game := result[0]

	var developers, publishers []string

	for _, ic := range game.InvolvedCompanies {
		if ic.Developer {
			developers = append(developers, ic.Company.Name)
		}
		if ic.Publisher {
			publishers = append(publishers, ic.Company.Name)
		}
	}

	var releaseDate string
	if game.FirstReleaseDate != 0 {
		releaseDate = time.Unix(int64(game.FirstReleaseDate), 0).Format("2006-01-02")
	}

	coverURL := ""
	if game.Cover != nil {
		coverURL = "https:" + strings.Replace(game.Cover.URL, "t_thumb", "t_1080p", 1)
	}

	var genres []string
	for _, g := range game.Genres {
		genres = append(genres, g.Name)
	}

	data := map[string]string{
		"name":         game.Name,
		"summary":      game.Summary,
		"url":          game.URL,
		"developers":   strings.Join(developers, ", "),
		"publishers":   strings.Join(publishers, ", "),
		"release_date": releaseDate,
		"cover_url":    coverURL,
		"genres":       strings.Join(genres, ", "),
	}

	return data, nil
}

type TwitchLoginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func (c *GameController) loginTwitch() (*TwitchLoginResponse, error) {
	const op = "controllers.games.loginTwitch"
	twitchLoginUrl := "https://id.twitch.tv/oauth2/token"

	clientId := c.twitchClientId
	clientSecret := c.twitchClientSecret

	requestString := fmt.Sprintf(twitchLoginUrl+"?client_id=%s&client_secret=%s&grant_type=client_credentials", clientId, clientSecret)

	req, err := http.NewRequest("POST", requestString, nil)
	if err != nil {
		c.log.Error(ErrLoginTwitch.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		return nil, ErrLogin
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.log.Error(ErrLoginTwitch.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.log.Error(ErrLoginTwitch.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		return nil, err
	}

	var data TwitchLoginResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		c.log.Error(ErrLoginTwitch.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		return nil, err
	}

	return &data, nil
}

// ======================
// UPDATE
// ======================

type UpdateGameRequest struct {
	GameID    int64      `json:"id"`
	CreatedAt *time.Time `json:"created_at"`
	CreateGameRequest
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

type UpdatePriorityRequest struct {
	Priority int `json:"priority"`
}

func (c *GameController) Update(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Update"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		c.log.Error(ErrUnauthorized.Error(), slog.String("operation", op))
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		c.log.Error(ErrInvalidID.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateGame.Error(), http.StatusBadRequest)
		return
	}

	existingGame, err := c.service.GetByID(gameID)
	if err != nil {
		c.log.Error(ErrGetGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateGame.Error(), http.StatusInternalServerError)
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
			c.log.Error(ErrParsingForm.Error(), slog.String("operation", op), slog.String("error", err.Error()))
			http.Error(w, ErrUpdateGame.Error(), http.StatusBadRequest)
			return
		}

		file, h, err := r.FormFile("image")
		if err == nil {
			defer file.Close()

			oldFilename, err := c.service.GetByID(gameID)
			if err != nil {
				c.log.Error(ErrGetGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
				http.Error(w, ErrUpdateGame.Error(), http.StatusInternalServerError)
				return
			}

			imageData, err := io.ReadAll(file)
			if err != nil {
				c.log.Error(ErrReadImage.Error(), slog.String("operation", op), slog.String("error", err.Error()))
				http.Error(w, ErrUpdateGame.Error(), http.StatusBadRequest)
				return
			}

			// get filename

			filename = h.Filename
			filename = generateImageFilename(filename, h.Header.Get("Content-Type"))

			if err := c.uploads.ReplaceImage(imageData, oldFilename.Image, filename); err != nil {
				c.log.Error(ErrSaveImage.Error(), slog.String("operation", op), slog.String("error", err.Error()))
				http.Error(w, ErrUpdateGame.Error(), http.StatusInternalServerError)
				return
			}
		}
	} else if strings.HasPrefix(contentType, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&gameData); err != nil {
			c.log.Error(ErrParsingJSON.Error(), slog.String("operation", op), slog.String("error", err.Error()))
			http.Error(w, ErrUpdateGame.Error(), http.StatusBadRequest)
			return
		}
		if img, ok := gameData["image"].(string); ok {
			filename = img
		}
	} else {
		c.log.Error(ErrInvalidRequest.Error(), slog.String("operation", op))
		http.Error(w, ErrInvalidRequest.Error(), http.StatusBadRequest)
		return
	}

	priority, err := strconv.Atoi(getFormValue(r, gameData, "priority"))
	if err != nil {
		priority = 0
	}
	if priority > 10 {
		c.log.Error(ErrInvalidPriority.Error(), slog.String("operation", op))
		http.Error(w, ErrInvalidPriority.Error(), http.StatusBadRequest)
		return
	}

	var createdAt *time.Time
	if createdAtStr := getFormValue(r, gameData, "created_at"); createdAtStr != "" {
		t, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			c.log.Error(ErrInvalidRequest.Error(), slog.String("operation", op), slog.String("error", err.Error()))
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

func (c *GameController) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.UpdateStatus"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		c.log.Error(ErrUnauthorized.Error(), slog.String("operation", op))
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		c.log.Error(ErrInvalidID.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateGame.Error(), http.StatusBadRequest)
		return
	}

	existingGame, err := c.service.GetByID(gameID)
	fmt.Printf("%v", existingGame)
	if err != nil {
		c.log.Error(ErrGetGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
		return
	}

	request := UpdateStatusRequest{Status: "planned"}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.log.Error(ErrParsingJSON.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateGame.Error(), http.StatusBadRequest)
		return
	}

	userGame := models.UserGames{}

	if userID != existingGame.Creator {
		userGame = models.UserGames{
			UserID:   userID,
			GameID:   existingGame.ID,
			Priority: 0,
			Status:   models.GameStatus(request.Status),
		}
	} else {
		existingUserGame, err := c.service.GetUserGame(userID, gameID)
		if err != nil {
			c.log.Error(ErrGetGame.Error(), slog.String("operation", op), slog.String("error", err.Error()))
			http.Error(w, ErrGetGame.Error(), http.StatusInternalServerError)
			return
		}
		userGame = models.UserGames{
			UserID:   userID,
			GameID:   existingUserGame.GameID,
			Priority: existingUserGame.Priority,
			Status:   models.GameStatus(request.Status),
		}
	}

	if err := c.service.UpdateUserGame(&userGame); err != nil {
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

func (c *GameController) UpdatePriority(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.UpdatePriority"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		c.log.Error(ErrUnauthorized.Error(), slog.String("operation", op))
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		c.log.Error(ErrInvalidID.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateGame.Error(), http.StatusBadRequest)
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
		c.log.Error(ErrParsingJSON.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrUpdateGame.Error(), http.StatusBadRequest)
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

// ======================
// DELETE
// ======================

func (c *GameController) DeleteUserGame(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.DeleteUserGame"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		c.log.Error(ErrUnauthorized.Error(), slog.String("operation", op))
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		c.log.Error(ErrInvalidURL.Error(), slog.String("operation", op))
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
		http.Error(w, ErrDeleteGame.Error(), http.StatusBadRequest)
		return
	}

	// Получаем игру по ID
	game, err := c.service.GetByID(idInt)
	if err != nil {
		c.log.Error(
			ErrGetGame.Error(),
			slog.String("operation", op),
			slog.String("id", id),
			slog.String("error", err.Error()))
		http.Error(w, ErrGetGame.Error(), http.StatusNotFound)
		return
	}

	if game == nil {
		c.log.Error(
			ErrGetGame.Error(),
			slog.String("operation", op),
			slog.String("id", id))
		http.Error(w, ErrGetGame.Error(), http.StatusNotFound)
		return
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

	w.WriteHeader(http.StatusNoContent)
}

func (c *GameController) Delete(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.Delete"

	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		c.log.Error(ErrUnauthorized.Error(), slog.String("operation", op))
		http.Error(w, ErrUnauthorized.Error(), http.StatusUnauthorized)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		c.log.Error(ErrInvalidURL.Error(), slog.String("operation", op))
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
			ErrGetGame.Error(),
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

// ======================
// STATS
// ======================

type GameStats struct {
	Finished int `json:"finished"`
	Playing  int `json:"playing"`
	Planned  int `json:"planned"`
	Dropped  int `json:"dropped"`
}

func (c *GameController) GetGameStats(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.games.GetGameStats"
	userID, ok := r.Context().Value(middleware.UserIDKey).(int64)
	if !ok || userID <= 0 {
		c.log.Error(ErrUnauthorized.Error(), slog.String("operation", op))
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
		c.log.Error(ErrGetGames.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
	playing, err := c.service.GetPlayingGames(userID)
	if err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
	planned, err := c.service.GetPlannedGames(userID)
	if err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}

	dropped, err := c.service.GetDroppedGames(userID)
	if err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}

	gs.Finished = finished
	gs.Playing = playing
	gs.Planned = planned
	gs.Dropped = dropped

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(gs); err != nil {
		c.log.Error(ErrGetGames.Error(), slog.String("operation", op), slog.String("error", err.Error()))
		http.Error(w, ErrGetGames.Error(), http.StatusInternalServerError)
		return
	}
}

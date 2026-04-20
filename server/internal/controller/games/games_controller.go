// internal/controller/games/games_controller.go
package games

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"games_webapp/internal/controller"
	g_errors "games_webapp/internal/errors"
	"games_webapp/internal/models"
	"games_webapp/internal/repository"
	"games_webapp/internal/service"

	"github.com/go-chi/chi/v5"
)

type GameController struct {
	service service.GameService
	log     *slog.Logger
	uploads IUploadsGames
}

type IUploadsGames interface {
	SaveImage(image []byte, filename string) error
	DeleteImage(filename string) error
	ReplaceImage(image []byte, oldFilename, newFilename string) error
	GenerateImageFilename(url, contentType string) string
}

func NewGameController(s service.GameService, log *slog.Logger, u IUploadsGames) *GameController {
	return &GameController{
		service: s,
		log:     log,
		uploads: u,
	}
}

// ============================================================================
// SETTERS
// ============================================================================

// SetService replaces the GameService used by the controller.
func (c *GameController) SetService(s service.GameService) { c.service = s }

// SetLogger replaces the structured logger used by the controller.
func (c *GameController) SetLogger(log *slog.Logger) { c.log = log }

// SetUploads replaces the file upload handler used by the controller.
func (c *GameController) SetUploads(u IUploadsGames) { c.uploads = u }

func (c *GameController) GetByID(w http.ResponseWriter, r *http.Request) {
	const op = "controller.PublicGamesController.GetByID"
	log := c.log.With(slog.String("operation", op))

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	res, err := c.service.GetByID(r.Context(), int(gameID))
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
}

func (c *GameController) GetAll(w http.ResponseWriter, r *http.Request) {
	const op = "controller.PublicGamesController.GetAll"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	query := r.URL.Query()
	page, _ := strconv.Atoi(query.Get("page"))
	pageSize, _ := strconv.Atoi(query.Get("page_size"))

	params := repository.GetAllParams{
		Search:    query.Get("search"),
		SortBy:    query.Get("sort_by"),
		SortOrder: query.Get("sort_order"),
		Page:      page,
		PageSize:  pageSize,
	}

	games, total, err := c.service.GetAll(r.Context(), userID, params)
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	var totalPages int
	if params.PageSize > 0 {
		totalPages = (total / params.PageSize)
		if total%params.PageSize != 0 {
			totalPages++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(controller.PaginationResponse{
		Total:   total,
		Pages:   totalPages,
		Current: params.Page,
		Size:    params.PageSize,
		Data:    games,
	})
}

func (c *GameController) SearchAllGames(w http.ResponseWriter, r *http.Request) {
	const op = "controller.PublicGamesController.SearchAllGames"
	log := c.log.With(slog.String("operation", op))

	query := r.URL.Query().Get("title")

	games, err := c.service.SearchAllGames(r.Context(), query)
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(games)
}

func (c *GameController) GetUserGame(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.GetUserGame"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	result, err := c.service.GetUserGame(r.Context(), userID, int(gameID))
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}

func (c *GameController) GetUserGames(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.GetUserGames"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	query := r.URL.Query()
	page, _ := strconv.Atoi(query.Get("page"))
	pageSize, _ := strconv.Atoi(query.Get("page_size"))

	var status *models.GameStatus
	if s := query.Get("status"); s != "" {
		st := models.GameStatus(s)
		status = &st
	}

	params := repository.GetUserGamesParams{
		Status:    status,
		Search:    query.Get("search"),
		SortBy:    query.Get("sort_by"),
		SortOrder: query.Get("sort_order"),
		Page:      page,
		PageSize:  pageSize,
	}

	games, total, err := c.service.GetUserGames(r.Context(), userID, params)
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	totalPages := total / params.PageSize
	if total%params.PageSize != 0 {
		totalPages++
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(controller.PaginationResponse{
		Total:   total,
		Pages:   totalPages,
		Current: params.Page,
		Size:    params.PageSize,
		Data:    games,
	})
}

func (c *GameController) GetGameStats(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.GetGameStats"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	counts, err := c.service.GetGameStats(r.Context(), userID)
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(counts)
}

func (c *GameController) Create(w http.ResponseWriter, r *http.Request) {
	const op = "controller.games.Create"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	// парсинг priority — если не число, дефолт 0
	priority, err := strconv.Atoi(r.FormValue("priority"))
	if err != nil {
		priority = 0
	}

	// парсинг изображения
	file, header, err := r.FormFile("image")
	if err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidImage, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInternal, "", err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	imageFilename := c.uploads.GenerateImageFilename(header.Filename, header.Header.Get("Content-Type"))
	if err := c.uploads.SaveImage(imageData, imageFilename); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInternal, "", err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	now := time.Now()
	game := &models.Game{
		Title:     r.FormValue("title"),
		Preambula: r.FormValue("preambula"),
		Image:     imageFilename,
		Developer: r.FormValue("developer"),
		Publisher: r.FormValue("publisher"),
		Year:      r.FormValue("year"),
		Genre:     r.FormValue("genre"),
		URL:       r.FormValue("url"),
		Creator:   userID,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	userGame := &models.UserGame{
		UserID:   userID,
		Priority: priority,
		Status:   models.GameStatus(r.FormValue("status")),
	}

	res, err := c.service.Create(r.Context(), game, userGame)
	if err != nil {
		_ = c.uploads.DeleteImage(imageFilename)
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(res)
}

func (c *GameController) AddUserGame(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.AddUserGame"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	if err := c.service.AddUserGame(r.Context(), userID, int(gameID)); err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (c *GameController) Update(w http.ResponseWriter, r *http.Request) {
	const op = "controller.games.Update"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	existingGame, err := c.service.GetByID(r.Context(), int(gameID))
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	isAdmin := controller.GetIsAdmin(r.Context())
	if !isAdmin && existingGame.Creator != userID {
		se := g_errors.NewWithInfo(op, g_errors.CodeForbidden, g_errors.NotAdminOrCreator,
			map[string]any{"userID": userID, "creator": existingGame.Creator})
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	contentType := r.Header.Get("Content-Type")
	var imageFilename string
	var priority int
	var status models.GameStatus
	var title, preambula, developer, publisher, year, genre, url string

	switch {
	case strings.HasPrefix(contentType, "multipart/form-data"):
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
			log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
			http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
			return
		}

		title = r.FormValue("title")
		preambula = r.FormValue("preambula")
		developer = r.FormValue("developer")
		publisher = r.FormValue("publisher")
		year = r.FormValue("year")
		genre = r.FormValue("genre")
		url = r.FormValue("url")
		status = models.GameStatus(r.FormValue("status"))
		priority, err = strconv.Atoi(r.FormValue("priority"))
		if err != nil {
			priority = 0
		}

		file, header, err := r.FormFile("image")
		if err == nil {
			defer file.Close()
			imageData, err := io.ReadAll(file)
			if err != nil {
				se := g_errors.Wrap(op, g_errors.CodeInternal, "", err)
				log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
				http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
				return
			}
			imageFilename = c.uploads.GenerateImageFilename(header.Filename, header.Header.Get("Content-Type"))
			if err := c.uploads.ReplaceImage(imageData, existingGame.Image, imageFilename); err != nil {
				se := g_errors.Wrap(op, g_errors.CodeInternal, "", err)
				log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
				http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
				return
			}
		} else {
			imageFilename = existingGame.Image
		}

	case strings.HasPrefix(contentType, "application/json"):
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
			log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
			http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
			return
		}
		title, _ = body["title"].(string)
		preambula, _ = body["preambula"].(string)
		developer, _ = body["developer"].(string)
		publisher, _ = body["publisher"].(string)
		year, _ = body["year"].(string)
		genre, _ = body["genre"].(string)
		url, _ = body["url"].(string)
		imageFilename, _ = body["image"].(string)

		if imageFilename == "" {
			imageFilename = existingGame.Image
		}
		if s, ok := body["status"].(string); ok {
			status = models.GameStatus(s)
		}
		if p, ok := body["priority"].(float64); ok {
			priority = int(p)
		}

	default:
		se := g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	now := time.Now()
	game := &models.Game{
		ID:        int(gameID),
		Title:     title,
		Preambula: preambula,
		Image:     imageFilename,
		Developer: developer,
		Publisher: publisher,
		Year:      year,
		Genre:     genre,
		URL:       url,
		Creator:   existingGame.Creator,
		CreatedAt: existingGame.CreatedAt,
		UpdatedAt: &now,
	}
	userGame := &models.UserGame{
		UserID:   userID,
		GameID:   int(gameID),
		Priority: priority,
		Status:   status,
	}

	res, err := c.service.Update(r.Context(), game, userGame)
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
}

type UpdatePriorityRequest struct {
	Priority int `json:"priority"`
}

func (c *GameController) UpdatePriority(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.UpdatePriority"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	var req UpdatePriorityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	if err := c.service.UpdatePriority(r.Context(), userID, int(gameID), req.Priority); err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

func (c *GameController) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.UpdateStatus"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	if err := c.service.UpdateStatus(r.Context(), userID, int(gameID), models.GameStatus(req.Status)); err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *GameController) Delete(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.Delete"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID,
			map[string]any{"id": gameIDStr}, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	game, err := c.service.GetByID(r.Context(), int(gameID))
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	isAdmin := controller.GetIsAdmin(r.Context())
	if !isAdmin && game.Creator != userID {
		se := g_errors.NewWithInfo(op, g_errors.CodeForbidden, g_errors.NotAdminOrCreator,
			map[string]any{"userID": userID, "creator": game.Creator})
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	if err := c.uploads.DeleteImage(game.Image); err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInternal, g_errors.CannotDeleteFile, map[string]any{"image": game.Image}, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	if err := c.service.Delete(r.Context(), int(gameID)); err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *GameController) DeleteUserGame(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.DeleteUserGame"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	if err := c.service.DeleteUserGame(r.Context(), userID, int(gameID)); err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

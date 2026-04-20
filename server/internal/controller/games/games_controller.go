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
		controller.WriteError(w, log, se)
		return
	}

	res, err := c.service.GetByID(r.Context(), int(gameID))
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	controller.WriteJSON(w, log, http.StatusOK, res)
}

func (c *GameController) GetAll(w http.ResponseWriter, r *http.Request) {
	const op = "controller.PublicGamesController.GetAll"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
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
		controller.WriteError(w, log, err)
		return
	}

	var totalPages int
	if params.PageSize > 0 {
		totalPages = (total / params.PageSize)
		if total%params.PageSize != 0 {
			totalPages++
		}
	}

	controller.WriteJSON(w, log, http.StatusOK, controller.PaginationResponse{
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
		controller.WriteError(w, log, err)
		return
	}

	controller.WriteJSON(w, log, http.StatusOK, games)
}

func (c *GameController) GetUserGame(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.GetUserGame"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		controller.WriteError(w, log, se)
		return
	}

	result, err := c.service.GetUserGame(r.Context(), userID, int(gameID))
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	controller.WriteJSON(w, log, http.StatusOK, result)
}

func (c *GameController) GetUserGames(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.GetUserGames"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
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
		controller.WriteError(w, log, err)
		return
	}

	totalPages := total / params.PageSize
	if total%params.PageSize != 0 {
		totalPages++
	}

	controller.WriteJSON(w, log, http.StatusOK, controller.PaginationResponse{
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
		controller.WriteError(w, log, err)
		return
	}

	counts, err := c.service.GetGameStats(r.Context(), userID)
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	controller.WriteJSON(w, log, http.StatusOK, counts)
}

func (c *GameController) Create(w http.ResponseWriter, r *http.Request) {
	const op = "controller.games.Create"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		controller.WriteError(w, log, se)
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
		controller.WriteError(w, log, se)
		return
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInternal, "", err)
		controller.WriteError(w, log, se)
		return
	}

	imageFilename := c.uploads.GenerateImageFilename(header.Filename, header.Header.Get("Content-Type"))
	if err := c.uploads.SaveImage(imageData, imageFilename); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInternal, "", err)
		controller.WriteError(w, log, se)
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
		controller.WriteError(w, log, err)
		return
	}

	controller.WriteJSON(w, log, http.StatusCreated, res)
}

func (c *GameController) AddUserGame(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.AddUserGame"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		controller.WriteError(w, log, se)
		return
	}

	if err := c.service.AddUserGame(r.Context(), userID, int(gameID)); err != nil {
		controller.WriteError(w, log, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// gameUpdateFields is the assembled payload from either multipart or JSON
// Update requests, with the image filename already resolved against the
// existing game (fallback applied when a new image isn't provided).
type gameUpdateFields struct {
	Title     string
	Preambula string
	Developer string
	Publisher string
	Year      string
	Genre     string
	URL       string
	Image     string
	Priority  int
	Status    models.GameStatus
}

func (c *GameController) Update(w http.ResponseWriter, r *http.Request) {
	const op = "controller.games.Update"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	gameID, err := parseGameID(r, op)
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	existingGame, err := c.service.GetByID(r.Context(), gameID)
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	if !controller.GetIsAdmin(r.Context()) && existingGame.Creator != userID {
		controller.WriteError(w, log, g_errors.NewWithInfo(op, g_errors.CodeForbidden, g_errors.NotAdminOrCreator,
			map[string]any{"userID": userID, "creator": existingGame.Creator}))
		return
	}

	fields, err := c.parseUpdateRequest(r, existingGame, op)
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	now := time.Now()
	game := &models.Game{
		ID:        gameID,
		Title:     fields.Title,
		Preambula: fields.Preambula,
		Image:     fields.Image,
		Developer: fields.Developer,
		Publisher: fields.Publisher,
		Year:      fields.Year,
		Genre:     fields.Genre,
		URL:       fields.URL,
		Creator:   existingGame.Creator,
		CreatedAt: existingGame.CreatedAt,
		UpdatedAt: &now,
	}
	userGame := &models.UserGame{
		UserID:   userID,
		GameID:   gameID,
		Priority: fields.Priority,
		Status:   fields.Status,
	}

	res, err := c.service.Update(r.Context(), game, userGame)
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	controller.WriteJSON(w, log, http.StatusOK, res)
}

// parseUpdateRequest dispatches to the appropriate parser by Content-Type.
func (c *GameController) parseUpdateRequest(r *http.Request, existing *models.Game, op string) (gameUpdateFields, error) {
	contentType := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(contentType, "multipart/form-data"):
		return c.parseMultipartUpdate(r, existing, op)
	case strings.HasPrefix(contentType, "application/json"):
		return parseJSONUpdate(r, existing, op)
	default:
		return gameUpdateFields{}, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm)
	}
}

// parseMultipartUpdate parses a multipart/form-data Update request. If a new
// image is supplied, it replaces the existing one on disk; otherwise the
// existing image filename is preserved.
func (c *GameController) parseMultipartUpdate(r *http.Request, existing *models.Game, op string) (gameUpdateFields, error) {
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		return gameUpdateFields{}, g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
	}

	priority, err := strconv.Atoi(r.FormValue("priority"))
	if err != nil {
		priority = 0
	}

	fields := gameUpdateFields{
		Title:     r.FormValue("title"),
		Preambula: r.FormValue("preambula"),
		Developer: r.FormValue("developer"),
		Publisher: r.FormValue("publisher"),
		Year:      r.FormValue("year"),
		Genre:     r.FormValue("genre"),
		URL:       r.FormValue("url"),
		Status:    models.GameStatus(r.FormValue("status")),
		Priority:  priority,
		Image:     existing.Image,
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		return fields, nil
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		return gameUpdateFields{}, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	newName := c.uploads.GenerateImageFilename(header.Filename, header.Header.Get("Content-Type"))
	if err := c.uploads.ReplaceImage(imageData, existing.Image, newName); err != nil {
		return gameUpdateFields{}, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}
	fields.Image = newName
	return fields, nil
}

// parseJSONUpdate parses an application/json Update request. A missing or
// empty "image" field falls back to the existing image filename.
func parseJSONUpdate(r *http.Request, existing *models.Game, op string) (gameUpdateFields, error) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return gameUpdateFields{}, g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
	}

	fields := gameUpdateFields{}
	fields.Title, _ = body["title"].(string)
	fields.Preambula, _ = body["preambula"].(string)
	fields.Developer, _ = body["developer"].(string)
	fields.Publisher, _ = body["publisher"].(string)
	fields.Year, _ = body["year"].(string)
	fields.Genre, _ = body["genre"].(string)
	fields.URL, _ = body["url"].(string)
	fields.Image, _ = body["image"].(string)
	if fields.Image == "" {
		fields.Image = existing.Image
	}
	if s, ok := body["status"].(string); ok {
		fields.Status = models.GameStatus(s)
	}
	if p, ok := body["priority"].(float64); ok {
		fields.Priority = int(p)
	}
	return fields, nil
}

// parseGameID extracts and validates the "id" chi URL parameter.
func parseGameID(r *http.Request, op string) (int, error) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID,
			map[string]any{"id": idStr}, err)
	}
	return int(id), nil
}

type UpdatePriorityRequest struct {
	Priority int `json:"priority"`
}

func (c *GameController) UpdatePriority(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.UpdatePriority"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		controller.WriteError(w, log, se)
		return
	}

	var req UpdatePriorityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		controller.WriteError(w, log, se)
		return
	}

	if err := c.service.UpdatePriority(r.Context(), userID, int(gameID), req.Priority); err != nil {
		controller.WriteError(w, log, err)
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
		controller.WriteError(w, log, err)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		controller.WriteError(w, log, se)
		return
	}

	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		controller.WriteError(w, log, se)
		return
	}

	if err := c.service.UpdateStatus(r.Context(), userID, int(gameID), models.GameStatus(req.Status)); err != nil {
		controller.WriteError(w, log, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *GameController) Delete(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.Delete"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID,
			map[string]any{"id": gameIDStr}, err)
		controller.WriteError(w, log, se)
		return
	}

	game, err := c.service.GetByID(r.Context(), int(gameID))
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	isAdmin := controller.GetIsAdmin(r.Context())
	if !isAdmin && game.Creator != userID {
		se := g_errors.NewWithInfo(op, g_errors.CodeForbidden, g_errors.NotAdminOrCreator,
			map[string]any{"userID": userID, "creator": game.Creator})
		controller.WriteError(w, log, se)
		return
	}

	if err := c.uploads.DeleteImage(game.Image); err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInternal, g_errors.CannotDeleteFile, map[string]any{"image": game.Image}, err)
		controller.WriteError(w, log, se)
		return
	}

	if err := c.service.Delete(r.Context(), int(gameID)); err != nil {
		controller.WriteError(w, log, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *GameController) DeleteUserGame(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.DeleteUserGame"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	gameIDStr := chi.URLParam(r, "id")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		se := g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID, map[string]any{"id": gameIDStr}, err)
		controller.WriteError(w, log, se)
		return
	}

	if err := c.service.DeleteUserGame(r.Context(), userID, int(gameID)); err != nil {
		controller.WriteError(w, log, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

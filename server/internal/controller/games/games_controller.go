// internal/controller/games/games_controller.go
package games

import (
	"context"
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

	"github.com/go-chi/chi/v5"
)

// defaultMaxUploadBytes is the fallback cap used when NewGameController is
// called with a non-positive value (tests, defensive code). Production wires
// the real limit from config.Uploads.MaxUploadBytes.
const defaultMaxUploadBytes int64 = 10 << 20 // 10 MiB

// gameService is the subset of the application's game service that the CRUD
// controller depends on. Declared in the consumer package (Go idiom: accept
// interfaces, return structs) so the service can evolve without touching the
// HTTP layer.
type gameService interface {
	GetByID(ctx context.Context, id int) (*models.Game, error)
	GetAll(ctx context.Context, userID int, params repository.GetAllParams) ([]models.UserGameResponse, int, error)
	SearchAllGames(ctx context.Context, query string) ([]models.Game, error)

	GetUserGame(ctx context.Context, userID, gameID int) (*models.UserGame, error)
	GetUserGames(ctx context.Context, userID int, params repository.GetUserGamesParams) ([]models.UserGameResponse, int, error)
	GetGameStats(ctx context.Context, userID int) (repository.StatusCounts, error)

	Create(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error)
	AddUserGame(ctx context.Context, userID, gameID int) error

	Update(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error)
	UpdateStatus(ctx context.Context, userID, gameID int, status models.GameStatus) error
	UpdatePriority(ctx context.Context, userID, gameID int, priority int) error

	Delete(ctx context.Context, gameID int) error
	DeleteUserGame(ctx context.Context, userID, gameID int) error
}

// IUploadsGames describes the Uploads methods the CRUD controller depends on.
type IUploadsGames interface {
	SaveImage(image []byte, filename string) error
	DeleteImage(filename string) error
	ReplaceImage(image []byte, oldFilename, newFilename string) error
	GenerateImageFilename(url, contentType string) string
}

type GameController struct {
	service        gameService
	log            *slog.Logger
	uploads        IUploadsGames
	maxUploadBytes int64
}

// NewGameController wires the controller. maxUploadBytes caps incoming
// multipart uploads; pass 0 to use defaultMaxUploadBytes (handy in tests).
func NewGameController(s gameService, log *slog.Logger, u IUploadsGames, maxUploadBytes int64) *GameController {
	if maxUploadBytes <= 0 {
		maxUploadBytes = defaultMaxUploadBytes
	}
	return &GameController{
		service:        s,
		log:            log,
		uploads:        u,
		maxUploadBytes: maxUploadBytes,
	}
}

func (c *GameController) GetByID(w http.ResponseWriter, r *http.Request) {
	const op = "controller.PublicGamesController.GetByID"
	log := controller.HandlerLog(r, c.log, op)

	gameID, err := parseGameID(r, op)
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	res, err := c.service.GetByID(r.Context(), gameID)
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	controller.WriteJSON(w, log, http.StatusOK, res)
}

func (c *GameController) GetAll(w http.ResponseWriter, r *http.Request) {
	const op = "controller.PublicGamesController.GetAll"
	log := controller.HandlerLog(r, c.log, op)

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

	controller.WriteJSON(w, log, http.StatusOK, controller.PaginationResponse{
		Total:   total,
		Pages:   paginationPages(total, params.PageSize),
		Current: params.Page,
		Size:    params.PageSize,
		Data:    games,
	})
}

func (c *GameController) SearchAllGames(w http.ResponseWriter, r *http.Request) {
	const op = "controller.PublicGamesController.SearchAllGames"
	log := controller.HandlerLog(r, c.log, op)

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
	log := controller.HandlerLog(r, c.log, op)

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

	result, err := c.service.GetUserGame(r.Context(), userID, gameID)
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	controller.WriteJSON(w, log, http.StatusOK, result)
}

func (c *GameController) GetUserGames(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.GetUserGames"
	log := controller.HandlerLog(r, c.log, op)

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

	controller.WriteJSON(w, log, http.StatusOK, controller.PaginationResponse{
		Total:   total,
		Pages:   paginationPages(total, params.PageSize),
		Current: params.Page,
		Size:    params.PageSize,
		Data:    games,
	})
}

func (c *GameController) GetGameStats(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.GetGameStats"
	log := controller.HandlerLog(r, c.log, op)

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
	log := controller.HandlerLog(r, c.log, op)

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	if err := r.ParseMultipartForm(c.maxUploadBytes); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		controller.WriteError(w, log, se)
		return
	}

	priority, err := strconv.Atoi(r.FormValue("priority"))
	if err != nil {
		priority = 0
	}

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
		controller.WriteError(w, log, err)
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
	log := controller.HandlerLog(r, c.log, op)

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

	if err := c.service.AddUserGame(r.Context(), userID, gameID); err != nil {
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
	log := controller.HandlerLog(r, c.log, op)

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

	fields, err := c.parseUpdateRequest(w, r, existingGame, op)
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
func (c *GameController) parseUpdateRequest(w http.ResponseWriter, r *http.Request, existing *models.Game, op string) (gameUpdateFields, error) {
	contentType := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(contentType, "multipart/form-data"):
		return c.parseMultipartUpdate(r, existing, op)
	case strings.HasPrefix(contentType, "application/json"):
		return parseJSONUpdate(w, r, existing, op)
	default:
		return gameUpdateFields{}, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm)
	}
}

// parseMultipartUpdate parses a multipart/form-data Update request. If a new
// image is supplied, it replaces the existing one on disk; otherwise the
// existing image filename is preserved.
func (c *GameController) parseMultipartUpdate(r *http.Request, existing *models.Game, op string) (gameUpdateFields, error) {
	if err := r.ParseMultipartForm(c.maxUploadBytes); err != nil {
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
		return gameUpdateFields{}, err
	}
	fields.Image = newName
	return fields, nil
}

// parseJSONUpdate parses an application/json Update request and merges it
// with the current game state. Any field the caller omits keeps its existing
// value, enabling true partial updates (e.g. bumping only priority without
// wiping title).
//
// Requires at least one known field in the body — an empty payload is a
// no-op and almost always a client bug.
func parseJSONUpdate(w http.ResponseWriter, r *http.Request, existing *models.Game, op string) (gameUpdateFields, error) {
	var body map[string]any
	if err := controller.DecodeJSON(w, r, &body); err != nil {
		return gameUpdateFields{}, g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
	}

	fields := gameUpdateFields{
		Title:     existing.Title,
		Preambula: existing.Preambula,
		Developer: existing.Developer,
		Publisher: existing.Publisher,
		Year:      existing.Year,
		Genre:     existing.Genre,
		URL:       existing.URL,
		Image:     existing.Image,
	}

	touched := overlayString(body, "title", &fields.Title)
	touched = overlayString(body, "preambula", &fields.Preambula) || touched
	touched = overlayString(body, "developer", &fields.Developer) || touched
	touched = overlayString(body, "publisher", &fields.Publisher) || touched
	touched = overlayString(body, "year", &fields.Year) || touched
	touched = overlayString(body, "genre", &fields.Genre) || touched
	touched = overlayString(body, "url", &fields.URL) || touched
	touched = overlayString(body, "image", &fields.Image) || touched
	if s, ok := body["status"].(string); ok {
		fields.Status = models.GameStatus(s)
		touched = true
	}
	if p, ok := body["priority"].(float64); ok {
		fields.Priority = int(p)
		touched = true
	}

	if !touched {
		return gameUpdateFields{}, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm)
	}
	return fields, nil
}

// overlayString copies body[key] into *dst when present as a string, and
// reports whether the field was touched.
func overlayString(body map[string]any, key string, dst *string) bool {
	v, ok := body[key].(string)
	if !ok {
		return false
	}
	*dst = v
	return true
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

// paginationPages returns the total number of pages for a paginated query.
// Returns 0 when pageSize is non-positive, guarding against a division-by-zero
// panic on unsanitized input (the service layer clamps pageSize, but the
// controller reads it back and should be robust in isolation).
func paginationPages(total, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	return (total + pageSize - 1) / pageSize
}

type UpdatePriorityRequest struct {
	Priority int `json:"priority"`
}

func (c *GameController) UpdatePriority(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.UpdatePriority"
	log := controller.HandlerLog(r, c.log, op)

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

	var req UpdatePriorityRequest
	if err := controller.DecodeJSON(w, r, &req); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		controller.WriteError(w, log, se)
		return
	}

	if err := c.service.UpdatePriority(r.Context(), userID, gameID, req.Priority); err != nil {
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
	log := controller.HandlerLog(r, c.log, op)

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

	var req UpdateStatusRequest
	if err := controller.DecodeJSON(w, r, &req); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		controller.WriteError(w, log, se)
		return
	}

	if err := c.service.UpdateStatus(r.Context(), userID, gameID, models.GameStatus(req.Status)); err != nil {
		controller.WriteError(w, log, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *GameController) Delete(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.Delete"
	log := controller.HandlerLog(r, c.log, op)

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

	game, err := c.service.GetByID(r.Context(), gameID)
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

	// Delete the DB row first: if the DB fails, the image stays and the row
	// stays — we can retry. Deleting the image first would risk leaving an
	// orphan DB row pointing at a missing file when the DB delete fails.
	// Image cleanup is best-effort: a stale file is cheaper than a stale row.
	if err := c.service.Delete(r.Context(), gameID); err != nil {
		controller.WriteError(w, log, err)
		return
	}

	if err := c.uploads.DeleteImage(game.Image); err != nil {
		log.Warn("image cleanup after delete failed",
			slog.String("image", game.Image),
			slog.Any("error", err))
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *GameController) DeleteUserGame(w http.ResponseWriter, r *http.Request) {
	const op = "controller.GameController.DeleteUserGame"
	log := controller.HandlerLog(r, c.log, op)

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

	if err := c.service.DeleteUserGame(r.Context(), userID, gameID); err != nil {
		controller.WriteError(w, log, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

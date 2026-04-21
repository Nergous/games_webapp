// internal/controller/games/igdb.go
package games

import (
	"context"
	"log/slog"
	"net/http"

	"games_webapp/internal/controller"
	g_errors "games_webapp/internal/errors"
	"games_webapp/internal/models"
	"games_webapp/internal/service"
)

// igdbImportService is the narrow interface the IGDB handler depends on:
// just the batch import. Declared here (consumer side) so the controller is
// decoupled from unrelated service capabilities.
type igdbImportService interface {
	BatchImportFromIGDB(ctx context.Context, userID int, requests []service.ImportGameRequest) (*service.ImportResult, error)
}

// IGDBGamesController is the HTTP adapter for the IGDB batch-import flow. All
// orchestration (worker pool, cover download, rollback) lives in the service
// layer; this handler's job is to decode JSON, invoke the service, and map
// the result to an HTTP status.
type IGDBGamesController struct {
	service igdbImportService
	log     *slog.Logger
}

func NewIGDBGamesController(s igdbImportService, log *slog.Logger) *IGDBGamesController {
	return &IGDBGamesController{service: s, log: log}
}

// GameError is the public shape of a per-item failure in the JSON response.
type GameError struct {
	Name string `json:"name"`
	Err  string `json:"error"`
}

// RequestGame mirrors one entry in the incoming JSON body.
type RequestGame struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// RequestData is the incoming JSON body.
type RequestData struct {
	Games []RequestGame `json:"games"`
}

// MultiGameResponse is the outgoing JSON body. Errors contains one entry per
// failed input; Success contains the persisted games for the rest.
type MultiGameResponse struct {
	Success []*models.Game `json:"success"`
	Errors  []*GameError   `json:"errors"`
}

func (c *IGDBGamesController) CreateMultiGamesIGDB(w http.ResponseWriter, r *http.Request) {
	const op = "controller.games.IGDBGamesController.CreateMultiGamesIGDB"
	log := controller.HandlerLog(r, c.log, op)

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	var request RequestData
	if err := controller.DecodeJSON(w, r, &request); err != nil {
		controller.WriteError(w, log, g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err))
		return
	}

	reqs := make([]service.ImportGameRequest, 0, len(request.Games))
	for _, g := range request.Games {
		reqs = append(reqs, service.ImportGameRequest{Name: g.Name, URL: g.URL})
	}

	result, err := c.service.BatchImportFromIGDB(r.Context(), userID, reqs)
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	gameErrors := make([]*GameError, 0, len(result.Errors))
	for i := range result.Errors {
		gameErrors = append(gameErrors, &GameError{
			Name: result.Errors[i].Name,
			Err:  result.Errors[i].Err,
		})
	}

	status := http.StatusCreated
	switch {
	case len(gameErrors) > 0 && len(result.Success) == 0:
		status = http.StatusInternalServerError
	case len(gameErrors) > 0:
		status = http.StatusMultiStatus
	}

	if len(gameErrors) > 0 {
		log.Warn(g_errors.PartialCreate,
			slog.Int("success", len(result.Success)),
			slog.Int("errors", len(gameErrors)),
		)
	} else {
		log.Info("games created", slog.Int("count", len(result.Success)))
	}

	controller.WriteJSON(w, log, status, MultiGameResponse{
		Success: result.Success,
		Errors:  gameErrors,
	})
}

// internal/controller/games/igdb.go
package games

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	igdbclient "games_webapp/internal/client/igdb"
	"games_webapp/internal/controller"
	g_errors "games_webapp/internal/errors"
	"games_webapp/internal/models"
	"games_webapp/internal/service"
)

const (
	maxIGDBWorkers     = 10
	maxGamesPerRequest = 100
	maxUploadSize      = 10 << 20 // 10 MiB
	igdbBatchTimeout   = 30 * time.Second
)

type IGDBGamesController struct {
	service service.GameService
	log     *slog.Logger
	uploads IUploadsIGDB
	igdb    *igdbclient.Client
}

type IUploadsIGDB interface {
	SaveImage(image []byte, filename string) error
	DeleteImage(filename string) error
	ReplaceImage(image []byte, oldFilename, newFilename string) error
	DownloadAndSaveImage(ctx context.Context, url string) (string, error)
}

func NewIGDBGamesController(
	s service.GameService,
	log *slog.Logger,
	u IUploadsIGDB,
	client *igdbclient.Client,
) *IGDBGamesController {
	return &IGDBGamesController{
		service: s,
		log:     log,
		uploads: u,
		igdb:    client,
	}
}

type GameError struct {
	Name string `json:"name"`
	Err  string `json:"error"`
}

type RequestGame struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type RequestData struct {
	Games []RequestGame `json:"games"`
}

type MultiGameResponse struct {
	Success []*models.Game `json:"success"`
	Errors  []*GameError   `json:"errors"`
}

func (c *IGDBGamesController) CreateMultiGamesIGDB(w http.ResponseWriter, r *http.Request) {
	const op = "controller.games.IGDBGamesController.CreateMultiGamesIGDB"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		controller.WriteError(w, log, err)
		return
	}

	var request RequestData
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		controller.WriteError(w, log, g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err))
		return
	}

	if len(request.Games) == 0 {
		controller.WriteError(w, log, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.EmptyQuery))
		return
	}
	if len(request.Games) > maxGamesPerRequest {
		controller.WriteError(w, log, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.TooMuchGamesInRequest))
		return
	}
	for _, g := range request.Games {
		if strings.TrimSpace(g.Name) == "" && strings.TrimSpace(g.URL) == "" {
			controller.WriteError(w, log, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidInput))
			return
		}
	}

	// Warm the token cache once up front so token-fetch failures surface before
	// spawning workers.
	if _, err := c.igdb.Token(r.Context()); err != nil {
		controller.WriteError(w, log, err)
		return
	}

	// Result/error channels are drained only after every worker has finished
	// (see wg.Wait + close below), so buffers must accommodate the whole
	// batch or workers will block and deadlock. Memory overhead is minimal —
	// the batch is capped at maxGamesPerRequest.
	var (
		sem         = make(chan struct{}, maxIGDBWorkers)
		wg          sync.WaitGroup
		errChan     = make(chan GameError, len(request.Games))
		resultsChan = make(chan *models.Game, len(request.Games))
	)

	ctx, cancel := context.WithTimeout(r.Context(), igdbBatchTimeout)
	defer cancel()

	for _, g := range request.Games {
		sem <- struct{}{}
		wg.Add(1)
		go func(req RequestGame) {
			defer func() {
				<-sem
				wg.Done()
			}()

			game, err := c.createThroughIGDB(ctx, req, userID)
			if err != nil {
				se, _ := g_errors.AsServiceError(err)
				label := req.URL
				if label == "" {
					label = req.Name
				}
				errChan <- GameError{
					Name: label,
					Err:  fmt.Sprintf("%s: %v", se.Details.Op, se.Details.Info),
				}
				return
			}
			resultsChan <- game
		}(g)
	}

	go func() {
		wg.Wait()
		close(errChan)
		close(resultsChan)
	}()

	gameErrors := make([]*GameError, 0, len(request.Games))
	createdGames := make([]*models.Game, 0, len(request.Games))
	for err := range errChan {
		e := err
		gameErrors = append(gameErrors, &e)
	}
	for res := range resultsChan {
		createdGames = append(createdGames, res)
	}

	status := http.StatusCreated
	if len(gameErrors) > 0 {
		if len(createdGames) == 0 {
			status = http.StatusInternalServerError
		} else {
			status = http.StatusMultiStatus
		}
		log.Warn(g_errors.PartialCreate,
			slog.Int("success", len(createdGames)),
			slog.Int("errors", len(gameErrors)),
		)
	} else {
		log.Info("games created", slog.Int("count", len(createdGames)))
	}

	controller.WriteJSON(w, log, status, MultiGameResponse{
		Success: createdGames,
		Errors:  gameErrors,
	})
}

func (c *IGDBGamesController) createThroughIGDB(ctx context.Context, req RequestGame, userID int) (*models.Game, error) {
	const op = "controller.games.IGDBGamesController.createThroughIGDB"

	select {
	case <-ctx.Done():
		return nil, g_errors.New(op, g_errors.CodeTimeout, "")
	default:
	}

	var slug string
	if req.URL != "" {
		s, err := igdbclient.ExtractSlug(req.URL)
		if err != nil {
			return nil, g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidURL,
				map[string]any{"url": req.URL}, err)
		}
		slug = s
	}

	info, err := c.igdb.GetGame(ctx, req.Name, slug)
	if err != nil {
		return nil, err
	}

	imageFilename, err := c.uploads.DownloadAndSaveImage(ctx, info.CoverURL)
	if err != nil {
		return nil, g_errors.WrapWithInfo(op, g_errors.CodeInternal, g_errors.CannotDownloadImage,
			map[string]any{"url": info.CoverURL}, err)
	}

	releaseYear, _, _ := strings.Cut(info.ReleaseDate, "-")
	now := time.Now()

	game := &models.Game{
		Title:     info.Name,
		Preambula: info.Summary,
		Image:     imageFilename,
		Developer: info.Developers,
		Publisher: info.Publishers,
		Year:      releaseYear,
		Genre:     info.Genres,
		URL:       info.URL,
		Creator:   userID,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	userGame := &models.UserGame{
		UserID:   userID,
		Priority: 0,
		Status:   models.StatusPlanned,
	}

	createdGame, err := c.service.Create(ctx, game, userGame)
	if err != nil {
		_ = c.uploads.DeleteImage(imageFilename)
		return nil, err
	}

	return createdGame, nil
}

// ExtractSlug is preserved for callers still importing it through this package.
// New code should use igdbclient.ExtractSlug directly.
func ExtractSlug(url string) (string, error) {
	return igdbclient.ExtractSlug(url)
}

// func (c *IGDBGamesController) searchThroughIGDB(name string, access *TwitchLoginResponse) (*FoundGames, error) {
// 	const op = "controllers.games.searchThroughIGDB"

// 	fields := []string{
// 		"name",
// 		"url",
// 		"image",
// 	}
// 	where := []models.WhereQuery{
// 		{
// 			Field:     "name",
// 			Condition: "like",
// 			Value:     name,
// 		},
// 	}

// 	internalGames, err := c.service.GetFlex(0, fields, where, nil, 5, 0)
// 	if err != nil {
// 		return nil, err
// 	}

// 	url := "https://api.igdb.com/v4/games"

// 	body := fmt.Sprintf(`
// 		search "%s";
// 		fields
// 			name,
// 			url,
// 			cover.url,
// 		where version_parent = null & game_type = (0, 8, 9, 10) & (aggregated_rating != null | (aggregated_rating = null & hypes != null & hypes > 10));
// 		limit 5;
// 	`, name)

// 	req, err := http.NewRequest("POST", url, bytes.NewBufferString(body))
// 	if err != nil {
// 		c.log.Error("ошибка при создании запроса", slog.String("operation", op), slog.String("error", err.Error()))
// 		return nil, controllers.ErrCreateGame
// 	}

// 	req.Header.Set("Client-ID", c.twitchClientId)
// 	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", access.AccessToken))
// 	req.Header.Set("Accept", "application/json")

// 	resp, err := http.DefaultClient.Do(req)
// 	if err != nil {
// 		c.log.Error("ошибка при выполнении запроса", slog.String("operation", op), slog.String("error", err.Error()))
// 		return nil, err
// 	}
// 	defer resp.Body.Close()

// 	bodyBytes, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		c.log.Error("ошибка при чтении тела ответа", slog.String("operation", op), slog.String("error", err.Error()))
// 		return nil, controllers.ErrCreateGame
// 	}

// 	var result []IGDBGame

// 	if err := json.Unmarshal(bodyBytes, &result); err != nil {
// 		c.log.Error("ошибка при декодировании JSON", slog.String("operation", op), slog.String("error", err.Error()))
// 		return nil, controllers.ErrCreateGame
// 	}

// 	if len(result) == 0 {
// 		c.log.Error("игра не найдена", slog.String("operation", op), slog.String("error", "game not found"))
// 		return nil, controllers.ErrGameNotFound
// 	}

// 	intGames := make([]game, len(internalGames))
// 	for i := range internalGames {
// 		intGames[i] = game{
// 			name:  internalGames[i].Title,
// 			url:   internalGames[i].URL,
// 			image: internalGames[i].Image,
// 		}
// 	}

// 	urlSet := make(map[string]struct{}, len(intGames))
// 	for _, g := range intGames {
// 		urlSet[g.url] = struct{}{}
// 	}

// 	igdbGames := make([]game, 0, len(result))
// 	for i := range result {
// 		url := result[i].URL
// 		if _, exists := urlSet[url]; exists {
// 			continue
// 		}
// 		igdbGames = append(igdbGames, game{
// 			name:  result[i].Name,
// 			url:   result[i].URL,
// 			image: result[i].Cover.URL,
// 		})
// 	}

// 	return &FoundGames{
// 		InternalGames: intGames,
// 		IGDBGames:     igdbGames,
// 	}, nil
// }

// func (c *IGDBGamesController) processRequest(body io.ReadCloser) (req RequestData, err error) {
// 	if err := json.NewDecoder(body).Decode(&req); err != nil {
// 		return req, err
// 	}

// 	if len(req.Games) == 0 {
// 		return req, err
// 	}

// 	if len(req.Games) > 100 {
// 		return req, err
// 	}
// 	return req, nil
// }

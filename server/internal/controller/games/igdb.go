// internal/controller/games/igdb.go
package games

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

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
	igdbHTTPTimeout    = 10 * time.Second
	twitchAuthTimeout  = 5 * time.Second
	tokenRefreshBuffer = 60 * time.Second
)

type IGDBGamesController struct {
	service            service.GameService
	log                *slog.Logger
	uploads            IUploadsIGDB
	apiURL             string // (typically https://api.igdb.com/v4).
	twitchAuthURL      string // (typically https://id.twitch.tv/oauth2/token).
	twitchClientId     string
	twitchClientSecret string

	httpClient *http.Client

	// Token cache
	tokenMu        sync.RWMutex
	cachedToken    string
	tokenExpiresAt time.Time
}

type IUploadsIGDB interface {
	SaveImage(image []byte, filename string) error
	DeleteImage(filename string) error
	ReplaceImage(image []byte, oldFilename, newFilename string) error
	DownloadAndSaveImage(url string) (string, error)
}

func NewIGDBGamesController(
	s service.GameService,
	log *slog.Logger,
	u IUploadsIGDB,
	api,
	twitchAuthURL,
	twitchClientId,
	twitchClientSecret string,
) *IGDBGamesController {
	return &IGDBGamesController{
		service:            s,
		log:                log,
		uploads:            u,
		apiURL:             api,
		twitchAuthURL:      twitchAuthURL,
		twitchClientId:     twitchClientId,
		twitchClientSecret: twitchClientSecret,
		httpClient:         &http.Client{Timeout: igdbHTTPTimeout},
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

type TwitchLoginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func (c *IGDBGamesController) CreateMultiGamesIGDB(w http.ResponseWriter, r *http.Request) {
	const op = "controllers.IGDBGamesController.CreateMultiGamesIGDB"
	log := c.log.With(slog.String("operation", op))

	userID, err := controller.GetUserID(r.Context())
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	var request RequestData
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		se := g_errors.Wrap(op, g_errors.CodeInvalidInput, g_errors.InvalidRequestForm, err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	if len(request.Games) == 0 {
		se := g_errors.New(op, g_errors.CodeInvalidInput, g_errors.EmptyQuery)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	if len(request.Games) > maxGamesPerRequest {
		se := g_errors.New(op, g_errors.CodeInvalidInput, g_errors.TooMuchGamesInRequest)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

	// каждый элемент должен иметь хотя бы name или url
	for _, g := range request.Games {
		if strings.TrimSpace(g.Name) == "" && strings.TrimSpace(g.URL) == "" {
			se := g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidInput)
			log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
			http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
			return
		}
	}

	access, err := c.getTwitchToken()
	if err != nil {
		se, _ := g_errors.AsServiceError(err)
		log.Error(se.Details.Op, slog.Any("info", se.Details.Info), slog.Any("error", err))
		http.Error(w, g_errors.PublicMessage(se), g_errors.HttpStatusFromErr(se))
		return
	}

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

			game, err := c.createThroughIGDB(ctx, req, userID, access)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(MultiGameResponse{
		Success: createdGames,
		Errors:  gameErrors,
	})
}

func extractSlug(igdbURL string) (string, error) {
	parts := strings.Split(strings.TrimRight(igdbURL, "/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid igdb url: %s", igdbURL)
	}
	slug := parts[len(parts)-1]
	if slug == "" {
		return "", fmt.Errorf("empty slug in url: %s", igdbURL)
	}
	return slug, nil
}

func (c *IGDBGamesController) createThroughIGDB(ctx context.Context, req RequestGame, userID int, access *TwitchLoginResponse) (*models.Game, error) {
	const op = "controllers.IGDBGamesController.createThroughIGDB"

	select {
	case <-ctx.Done():
		return nil, g_errors.New(op, g_errors.CodeTimeout, "")
	default:
	}

	var slug string
	if req.URL != "" {
		var err error
		slug, err = extractSlug(req.URL)
		if err != nil {
			return nil, g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidURL,
				map[string]any{"url": req.URL}, err)
		}
	}

	data, err := c.getDataFromIGDB(ctx, req.Name, slug, access)
	if err != nil {
		return nil, err
	}

	imageFilename, err := c.uploads.DownloadAndSaveImage(data["cover_url"])
	if err != nil {
		return nil, g_errors.WrapWithInfo(op, g_errors.CodeInternal, g_errors.CannotDownloadImage,
			map[string]any{"url": data["cover_url"]}, err)
	}

	releaseYear, _, _ := strings.Cut(data["release_date"], "-")
	now := time.Now()

	game := &models.Game{
		Title:     data["name"],
		Preambula: data["summary"],
		Image:     imageFilename,
		Developer: data["developers"],
		Publisher: data["publishers"],
		Year:      releaseYear,
		Genre:     data["genres"],
		URL:       data["url"],
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

func (c *IGDBGamesController) getDataFromIGDB(ctx context.Context, name, slug string, access *TwitchLoginResponse) (map[string]string, error) {
	const op = "controllers.IGDBGamesController.getDataFromIGDB"

	var body string
	if slug != "" {
		body = fmt.Sprintf(`
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
			where slug = "%s";
			limit 1;
		`, slug)
	} else {
		body = fmt.Sprintf(`
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
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL, bytes.NewBufferString(body))
	if err != nil {
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	req.Header.Set("Client-ID", c.twitchClientId)
	req.Header.Set("Authorization", "Bearer "+access.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, g_errors.IGDBInternalError, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	var results []struct {
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

	if err := json.Unmarshal(bodyBytes, &results); err != nil {
		return nil, g_errors.WrapWithInfo(op, g_errors.CodeInternal, "",
			map[string]any{"body": string(bodyBytes)}, err)
	}

	if len(results) == 0 {
		return nil, g_errors.NewWithInfo(op, g_errors.CodeNotFound, g_errors.GameNotFound,
			map[string]any{"name": name, "slug": slug})
	}

	g := results[0]

	var developers, publishers, genres []string
	for _, ic := range g.InvolvedCompanies {
		if ic.Company == nil {
			continue
		}
		if ic.Developer {
			developers = append(developers, ic.Company.Name)
		}
		if ic.Publisher {
			publishers = append(publishers, ic.Company.Name)
		}
	}
	for _, genre := range g.Genres {
		genres = append(genres, genre.Name)
	}

	var releaseDate string
	if g.FirstReleaseDate != 0 {
		releaseDate = time.Unix(int64(g.FirstReleaseDate), 0).Format("2006-01-02")
	}

	coverURL := ""
	if g.Cover != nil {
		coverURL = "https:" + strings.Replace(g.Cover.URL, "t_thumb", "t_1080p", 1)
	}

	return map[string]string{
		"name":         g.Name,
		"summary":      g.Summary,
		"url":          g.URL,
		"developers":   strings.Join(developers, ", "),
		"publishers":   strings.Join(publishers, ", "),
		"release_date": releaseDate,
		"cover_url":    coverURL,
		"genres":       strings.Join(genres, ", "),
	}, nil
}

func (c *IGDBGamesController) loginTwitch() (*TwitchLoginResponse, error) {
	const op = "controllers.IGDBGamesController.loginTwitch"

	url := fmt.Sprintf("%s?client_id=%s&client_secret=%s&grant_type=client_credentials",
		c.twitchAuthURL, c.twitchClientId, c.twitchClientSecret)

	ctx, cancel := context.WithTimeout(context.Background(), twitchAuthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, g_errors.IGDBInternalError, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	var data TwitchLoginResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, g_errors.WrapWithInfo(op, g_errors.CodeInternal, "",
			map[string]any{"body": string(body)}, err)
	}

	return &data, nil
}

func (c *IGDBGamesController) getTwitchToken() (*TwitchLoginResponse, error) {
	const op = "controllers.IGDBGamesController.getTwitchToken"

	// Fast path: token is still valid (with 60s buffer)
	c.tokenMu.RLock()
	if c.cachedToken != "" && time.Now().Add(tokenRefreshBuffer).Before(c.tokenExpiresAt) {
		c.tokenMu.RUnlock()
		return &TwitchLoginResponse{AccessToken: c.cachedToken}, nil
	}
	c.tokenMu.RUnlock()

	// Slow path: fetch new token
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Double-check after acquiring write lock
	if c.cachedToken != "" && time.Now().Add(tokenRefreshBuffer).Before(c.tokenExpiresAt) {
		return &TwitchLoginResponse{AccessToken: c.cachedToken}, nil
	}

	resp, err := c.loginTwitch()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	c.cachedToken = resp.AccessToken
	c.tokenExpiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)

	return resp, nil
}

func (c *IGDBGamesController) GetTwitchToken() (*TwitchLoginResponse, error) {
	return c.getTwitchToken()
}

func (c *IGDBGamesController) SetTokenForTest(token string, expiresAt time.Time) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.cachedToken = token
	c.tokenExpiresAt = expiresAt
}

func ExtractSlug(url string) (string, error) {
	return extractSlug(url)
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
// 		c.log.Error("игра не найдена", slog.String("operation", op), slog.String("error", "game not found"))
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

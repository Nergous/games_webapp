// internal/service/games/igdb_import.go
package games

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	igdbclient "games_webapp/internal/client/igdb"
	g_errors "games_webapp/internal/errors"
	"games_webapp/internal/models"
	"games_webapp/internal/service"
)

// Defaults applied when IGDBSettings aren't supplied (tests, zero-valued
// config). Production wires real values from config.IGDB via
// GameService.SetIGDBSettings at composition time.
const (
	defaultIGDBWorkers      = 10
	defaultIGDBBatchTimeout = 30 * time.Second

	// MaxGamesPerRequest is the public default hard cap on a single batch.
	// Kept exported for the HTTP layer and tests; runtime override lives on
	// GameService.maxGames (set via SetIGDBSettings).
	MaxGamesPerRequest = 100
)

// IGDBSettings carries tunables for BatchImportFromIGDB. Passed through
// GameService.SetIGDBSettings so the service has no import on the config
// package and stays testable in isolation.
type IGDBSettings struct {
	MaxWorkers         int
	MaxGamesPerRequest int
	BatchTimeout       time.Duration
}

// IGDBFetcher is the subset of the IGDB client that BatchImportFromIGDB
// depends on. Declared in the consumer package so it can be swapped in tests.
type IGDBFetcher interface {
	GetGame(ctx context.Context, name, slug string) (*igdbclient.GameInfo, error)
	Token(ctx context.Context) (string, error)
}

// ImageDownloader abstracts the filesystem/HTTP side of cover handling so the
// service can be tested without touching disk.
type ImageDownloader interface {
	DownloadAndSaveImage(ctx context.Context, url string) (string, error)
	DeleteImage(filename string) error
}

// BatchImportFromIGDB fetches game metadata and covers from IGDB for each
// request and persists both the Game and the user_games row. Requests are
// processed concurrently (bounded by s.maxWorkers via errgroup) and partial
// failures are returned via ImportResult.Errors rather than aborting the batch.
//
// The returned error is non-nil only for failures that prevent the batch from
// running at all (invalid input, IGDB token fetch failure); per-item failures
// live in ImportResult.Errors.
func (s *GameService) BatchImportFromIGDB(
	ctx context.Context,
	userID int,
	requests []service.ImportGameRequest,
) (*service.ImportResult, error) {
	const op = "services.games.BatchImportFromIGDB"

	if userID <= 0 {
		return nil, g_errors.New(op, g_errors.CodeUnauthorized, g_errors.MissingUserID)
	}
	if len(requests) == 0 {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.EmptyQuery)
	}
	if len(requests) > s.maxGames {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.TooMuchGamesInRequest)
	}
	for _, req := range requests {
		if strings.TrimSpace(req.Name) == "" && strings.TrimSpace(req.URL) == "" {
			return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidInput)
		}
	}
	if s.igdb == nil || s.uploads == nil {
		return nil, g_errors.New(op, g_errors.CodeInternal, "igdb/uploads not configured")
	}

	// Warm the token cache once so token-fetch failures surface before we
	// spawn workers (each worker would otherwise hit the same failure).
	if _, err := s.igdb.Token(ctx); err != nil {
		return nil, err
	}

	batchCtx, cancel := context.WithTimeout(ctx, s.batchTimeout)
	defer cancel()

	result := &service.ImportResult{
		Success: make([]*models.Game, 0, len(requests)),
		Errors:  make([]service.ImportError, 0, len(requests)),
	}
	var mu sync.Mutex

	// errgroup.SetLimit honours batchCtx cancellation inside Go(), so an
	// in-flight shutdown aborts pending work instead of blocking on a full
	// semaphore. importOne errors are folded into result.Errors; fatal
	// programming errors (should be none here) would bubble up via Wait.
	g, gCtx := errgroup.WithContext(batchCtx)
	g.SetLimit(s.maxWorkers)

	for _, r := range requests {
		req := r
		g.Go(func() error {
			game, err := s.importOne(gCtx, req, userID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.Errors = append(result.Errors, toImportError(req, err))
				return nil
			}
			result.Success = append(result.Success, game)
			return nil
		})
	}
	_ = g.Wait()

	return result, nil
}

// importOne runs the metadata fetch, cover download, and persistence for a
// single request. The cover is rolled back if persistence fails.
func (s *GameService) importOne(ctx context.Context, req service.ImportGameRequest, userID int) (*models.Game, error) {
	const op = "services.games.importOne"

	select {
	case <-ctx.Done():
		return nil, g_errors.New(op, g_errors.CodeTimeout, "")
	default:
	}

	var slug string
	if req.URL != "" {
		extracted, err := igdbclient.ExtractSlug(req.URL)
		if err != nil {
			return nil, g_errors.WrapWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidURL,
				map[string]any{"url": req.URL}, err)
		}
		slug = extracted
	}

	info, err := s.igdb.GetGame(ctx, req.Name, slug)
	if err != nil {
		return nil, err
	}

	imageFilename, err := s.uploads.DownloadAndSaveImage(ctx, info.CoverURL)
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

	created, err := s.Create(ctx, game, userGame)
	if err != nil {
		_ = s.uploads.DeleteImage(imageFilename)
		return nil, err
	}
	return created, nil
}

// toImportError flattens a ServiceError into the transport-friendly
// ImportError. The label prefers the original URL so callers can correlate
// failures with exactly what they submitted.
func toImportError(req service.ImportGameRequest, err error) service.ImportError {
	label := req.URL
	if label == "" {
		label = req.Name
	}
	if se, ok := g_errors.AsServiceError(err); ok && se != nil {
		return service.ImportError{
			Name: label,
			Err:  fmt.Sprintf("%s: %v", se.Details.Op, se.Details.Info),
		}
	}
	return service.ImportError{Name: label, Err: err.Error()}
}

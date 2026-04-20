// internal/service/games/igdb_import_test.go
package games_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	igdbclient "games_webapp/internal/client/igdb"
	g_errors "games_webapp/internal/errors"
	"games_webapp/internal/models"
	"games_webapp/internal/repository"
	"games_webapp/internal/service"
	"games_webapp/internal/service/games"
)

// ============================================================================
// Mocks
// ============================================================================

type mockIGDB struct {
	token   func(ctx context.Context) (string, error)
	getGame func(ctx context.Context, name, slug string) (*igdbclient.GameInfo, error)
}

func (m *mockIGDB) Token(ctx context.Context) (string, error) {
	if m.token == nil {
		return "test-token", nil
	}
	return m.token(ctx)
}

func (m *mockIGDB) GetGame(ctx context.Context, name, slug string) (*igdbclient.GameInfo, error) {
	if m.getGame == nil {
		return &igdbclient.GameInfo{Name: name, URL: "https://igdb.com/games/" + slug, CoverURL: "https://igdb.com/cover.jpg"}, nil
	}
	return m.getGame(ctx, name, slug)
}

type mockUploader struct {
	download func(ctx context.Context, url string) (string, error)
	delete   func(filename string) error
}

func (m *mockUploader) DownloadAndSaveImage(ctx context.Context, url string) (string, error) {
	if m.download == nil {
		return "cover.jpg", nil
	}
	return m.download(ctx, url)
}

func (m *mockUploader) DeleteImage(filename string) error {
	if m.delete == nil {
		return nil
	}
	return m.delete(filename)
}

// ============================================================================
// Helpers
// ============================================================================

// newImportService returns a service wired with the given IGDB fetcher,
// uploader, and a CreateWithUserGame stub on the repo (repo must always be
// involved because BatchImportFromIGDB calls through to Create).
func newImportService(repo repository.GamesRepository, igdb games.IGDBFetcher, up games.ImageDownloader) *games.GameService {
	return games.NewGameService(repo, igdb, up)
}

// defaultRepo returns a mockRepo whose CreateWithUserGame succeeds and returns
// the incoming game with ID=1. Tests that care about Create behavior should
// override it.
func defaultRepo() *mockRepo {
	return &mockRepo{
		createWithUserGame: func(_ context.Context, g *models.Game, _ *models.UserGame) (*models.Game, error) {
			g.ID = 1
			return g, nil
		},
	}
}

// ============================================================================
// Input validation — short-circuits before any IGDB call
// ============================================================================

func TestBatchImport_UnauthorizedUser(t *testing.T) {
	s := newImportService(&mockRepo{}, &mockIGDB{}, &mockUploader{})
	_, err := s.BatchImportFromIGDB(context.Background(), 0, []service.ImportGameRequest{{Name: "A"}})
	assertCode(t, err, g_errors.CodeUnauthorized)
}

func TestBatchImport_EmptyRequests(t *testing.T) {
	s := newImportService(&mockRepo{}, &mockIGDB{}, &mockUploader{})
	_, err := s.BatchImportFromIGDB(context.Background(), 1, nil)
	assertCode(t, err, g_errors.CodeInvalidInput)
}

func TestBatchImport_TooManyRequests(t *testing.T) {
	s := newImportService(&mockRepo{}, &mockIGDB{}, &mockUploader{})
	reqs := make([]service.ImportGameRequest, games.MaxGamesPerRequest+1)
	for i := range reqs {
		reqs[i] = service.ImportGameRequest{Name: "x"}
	}
	_, err := s.BatchImportFromIGDB(context.Background(), 1, reqs)
	assertCode(t, err, g_errors.CodeInvalidInput)
}

func TestBatchImport_EmptyNameAndURL(t *testing.T) {
	s := newImportService(&mockRepo{}, &mockIGDB{}, &mockUploader{})
	_, err := s.BatchImportFromIGDB(context.Background(), 1,
		[]service.ImportGameRequest{{Name: "", URL: ""}})
	assertCode(t, err, g_errors.CodeInvalidInput)
}

func TestBatchImport_MissingDependencies(t *testing.T) {
	s := games.NewGameService(&mockRepo{}, nil, nil)
	_, err := s.BatchImportFromIGDB(context.Background(), 1,
		[]service.ImportGameRequest{{Name: "A"}})
	assertCode(t, err, g_errors.CodeInternal)
}

// ============================================================================
// Token warm-up failure — aborts the batch before spawning workers
// ============================================================================

func TestBatchImport_TokenError(t *testing.T) {
	var getGameCalls int32
	igdb := &mockIGDB{
		token: func(_ context.Context) (string, error) {
			return "", errors.New("twitch down")
		},
		getGame: func(_ context.Context, _, _ string) (*igdbclient.GameInfo, error) {
			atomic.AddInt32(&getGameCalls, 1)
			return nil, nil
		},
	}
	s := newImportService(defaultRepo(), igdb, &mockUploader{})

	_, err := s.BatchImportFromIGDB(context.Background(), 1,
		[]service.ImportGameRequest{{Name: "A"}})
	if err == nil {
		t.Fatal("expected token error")
	}
	if atomic.LoadInt32(&getGameCalls) != 0 {
		t.Errorf("GetGame must not be called when token fetch fails")
	}
}

// ============================================================================
// Orchestration: success, partial, all-failed
// ============================================================================

func TestBatchImport_AllSuccess(t *testing.T) {
	igdb := &mockIGDB{
		getGame: func(_ context.Context, name, _ string) (*igdbclient.GameInfo, error) {
			return &igdbclient.GameInfo{Name: name, URL: "u", CoverURL: "c"}, nil
		},
	}
	s := newImportService(defaultRepo(), igdb, &mockUploader{})

	res, err := s.BatchImportFromIGDB(context.Background(), 1,
		[]service.ImportGameRequest{{Name: "A"}, {Name: "B"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Success) != 2 || len(res.Errors) != 0 {
		t.Errorf("counts: success=%d errors=%d", len(res.Success), len(res.Errors))
	}
}

func TestBatchImport_PartialFailure(t *testing.T) {
	igdb := &mockIGDB{
		getGame: func(_ context.Context, name, _ string) (*igdbclient.GameInfo, error) {
			if name == "Bad" {
				return nil, g_errors.New("op", g_errors.CodeNotFound, g_errors.GameNotFound)
			}
			return &igdbclient.GameInfo{Name: name, URL: "u", CoverURL: "c"}, nil
		},
	}
	s := newImportService(defaultRepo(), igdb, &mockUploader{})

	res, err := s.BatchImportFromIGDB(context.Background(), 1,
		[]service.ImportGameRequest{{Name: "A"}, {Name: "Bad"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Success) != 1 || len(res.Errors) != 1 {
		t.Errorf("counts: success=%d errors=%d", len(res.Success), len(res.Errors))
	}
	if res.Errors[0].Name != "Bad" {
		t.Errorf("error label: got %q, want %q", res.Errors[0].Name, "Bad")
	}
}

func TestBatchImport_URLProducesSlug(t *testing.T) {
	var gotSlug string
	var mu sync.Mutex
	igdb := &mockIGDB{
		getGame: func(_ context.Context, _, slug string) (*igdbclient.GameInfo, error) {
			mu.Lock()
			gotSlug = slug
			mu.Unlock()
			return &igdbclient.GameInfo{Name: "Half-Life 2", URL: "u", CoverURL: "c"}, nil
		},
	}
	s := newImportService(defaultRepo(), igdb, &mockUploader{})

	_, err := s.BatchImportFromIGDB(context.Background(), 1,
		[]service.ImportGameRequest{{URL: "https://igdb.com/games/half-life-2"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotSlug != "half-life-2" {
		t.Errorf("slug: got %q, want %q", gotSlug, "half-life-2")
	}
}

func TestBatchImport_InvalidURL(t *testing.T) {
	s := newImportService(defaultRepo(), &mockIGDB{}, &mockUploader{})
	res, err := s.BatchImportFromIGDB(context.Background(), 1,
		[]service.ImportGameRequest{{URL: "no-slash"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Errors) != 1 || res.Errors[0].Name != "no-slash" {
		t.Errorf("expected 1 error labeled by URL, got %+v", res.Errors)
	}
}

// ============================================================================
// Rollback: failed persistence must delete the freshly downloaded cover
// ============================================================================

func TestBatchImport_CreateFailure_DeletesCover(t *testing.T) {
	repo := &mockRepo{
		createWithUserGame: func(_ context.Context, _ *models.Game, _ *models.UserGame) (*models.Game, error) {
			return nil, repository.ErrAlreadyExists
		},
	}
	var deleted int32
	up := &mockUploader{
		delete: func(_ string) error { atomic.AddInt32(&deleted, 1); return nil },
	}
	s := newImportService(repo, &mockIGDB{}, up)

	res, err := s.BatchImportFromIGDB(context.Background(), 1,
		[]service.ImportGameRequest{{Name: "A"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("want 1 error, got %d", len(res.Errors))
	}
	if atomic.LoadInt32(&deleted) != 1 {
		t.Error("DeleteImage must run on Create failure")
	}
}

func TestBatchImport_ImageDownloadFailure(t *testing.T) {
	up := &mockUploader{
		download: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("disk full")
		},
	}
	s := newImportService(defaultRepo(), &mockIGDB{}, up)

	res, err := s.BatchImportFromIGDB(context.Background(), 1,
		[]service.ImportGameRequest{{Name: "A"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Success) != 0 || len(res.Errors) != 1 {
		t.Errorf("counts: success=%d errors=%d", len(res.Success), len(res.Errors))
	}
}

// ============================================================================
// Concurrency smoke: bounded workers, every request produces exactly one
// outcome.
// ============================================================================

func TestBatchImport_Concurrent(t *testing.T) {
	var live, peak int32
	igdb := &mockIGDB{
		getGame: func(_ context.Context, name, _ string) (*igdbclient.GameInfo, error) {
			cur := atomic.AddInt32(&live, 1)
			defer atomic.AddInt32(&live, -1)
			for {
				p := atomic.LoadInt32(&peak)
				if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
					break
				}
			}
			return &igdbclient.GameInfo{Name: name, URL: "u", CoverURL: "c"}, nil
		},
	}
	s := newImportService(defaultRepo(), igdb, &mockUploader{})

	reqs := make([]service.ImportGameRequest, 25)
	for i := range reqs {
		reqs[i] = service.ImportGameRequest{Name: "g"}
	}
	res, err := s.BatchImportFromIGDB(context.Background(), 1, reqs)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Success)+len(res.Errors) != len(reqs) {
		t.Errorf("accounting mismatch: success=%d errors=%d requests=%d",
			len(res.Success), len(res.Errors), len(reqs))
	}
	if atomic.LoadInt32(&peak) > 10 {
		t.Errorf("peak concurrency %d exceeded maxIGDBWorkers=10", peak)
	}
}

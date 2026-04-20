package games_test

import (
	"context"
	"errors"
	g_errors "games_webapp/internal/errors"
	"games_webapp/internal/models"
	"games_webapp/internal/repository"
	"games_webapp/internal/service/games"
	"testing"
	"time"
)

// ============================================================================
// Mock репозитория — реализует repository.GamesRepository вручную.
// Mock of repository - manually implements repository.GamesRepository
// Every method is controlled by a field-function: if the field is nil, the
// method should not be called in this test (call = panic with a clear message).
// ============================================================================

type mockRepo struct {
	getByID                 func(ctx context.Context, id int) (*models.Game, error)
	getAllPaginated         func(ctx context.Context, userID int, params repository.GetAllParams) ([]models.UserGameResponse, int, error)
	getUserGame             func(ctx context.Context, userID, gameID int) (*models.UserGame, error)
	getUserGames            func(ctx context.Context, userID int, params repository.GetUserGamesParams) ([]models.UserGameResponse, int, error)
	getUserGameStatusCounts func(ctx context.Context, userID int) (repository.StatusCounts, error)
	searchAllGames          func(ctx context.Context, query string) ([]models.Game, error)
	createWithUserGame      func(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error)
	addUserGame             func(ctx context.Context, userGame *models.UserGame) error
	updateWithUserGame      func(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error)
	updateUserGameStatus    func(ctx context.Context, userID, gameID int, status models.GameStatus) error
	updateUserGamePriority  func(ctx context.Context, userID, gameID int, priority int) error
	delete                  func(ctx context.Context, id int) error
	deleteUserGame          func(ctx context.Context, userID, gameID int) error
}

func (m *mockRepo) GetByID(ctx context.Context, id int) (*models.Game, error) {
	if m.getByID == nil {
		panic("unexpected call to GetByID")
	}
	return m.getByID(ctx, id)
}

func (m *mockRepo) GetAllPaginated(ctx context.Context, userID int, params repository.GetAllParams) ([]models.UserGameResponse, int, error) {
	if m.getAllPaginated == nil {
		panic("unexpected call to GetAllPaginated")
	}
	return m.getAllPaginated(ctx, userID, params)
}

func (m *mockRepo) GetUserGame(ctx context.Context, userID, gameID int) (*models.UserGame, error) {
	if m.getUserGame == nil {
		panic("unexpected call to GetUserGame")
	}
	return m.getUserGame(ctx, userID, gameID)
}

func (m *mockRepo) GetUserGames(ctx context.Context, userID int, params repository.GetUserGamesParams) ([]models.UserGameResponse, int, error) {
	if m.getUserGames == nil {
		panic("unexpected call to GetUserGames")
	}
	return m.getUserGames(ctx, userID, params)
}

func (m *mockRepo) GetUserGameStatusCounts(ctx context.Context, userID int) (repository.StatusCounts, error) {
	if m.getUserGameStatusCounts == nil {
		panic("unexpected call to GetUserGameStatusCounts")
	}
	return m.getUserGameStatusCounts(ctx, userID)
}

func (m *mockRepo) SearchAllGames(ctx context.Context, query string) ([]models.Game, error) {
	if m.searchAllGames == nil {
		panic("unexpected call to SearchAllGames")
	}
	return m.searchAllGames(ctx, query)
}

func (m *mockRepo) CreateWithUserGame(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error) {
	if m.createWithUserGame == nil {
		panic("unexpected call to CreateWithUserGame")
	}
	return m.createWithUserGame(ctx, game, userGame)
}

func (m *mockRepo) AddUserGame(ctx context.Context, userGame *models.UserGame) error {
	if m.addUserGame == nil {
		panic("unexpected call to AddUserGame")
	}
	return m.addUserGame(ctx, userGame)
}

func (m *mockRepo) UpdateWithUserGame(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error) {
	if m.updateWithUserGame == nil {
		panic("unexpected call to UpdateWithUserGame")
	}
	return m.updateWithUserGame(ctx, game, userGame)
}

func (m *mockRepo) UpdateUserGameStatus(ctx context.Context, userID, gameID int, status models.GameStatus) error {
	if m.updateUserGameStatus == nil {
		panic("unexpected call to UpdateUserGameStatus")
	}
	return m.updateUserGameStatus(ctx, userID, gameID, status)
}

func (m *mockRepo) UpdateUserGamePriority(ctx context.Context, userID, gameID int, priority int) error {
	if m.updateUserGamePriority == nil {
		panic("unexpected call to UpdateUserGamePriority")
	}
	return m.updateUserGamePriority(ctx, userID, gameID, priority)
}

func (m *mockRepo) Delete(ctx context.Context, id int) error {
	if m.delete == nil {
		panic("unexpected call to Delete")
	}
	return m.delete(ctx, id)
}

func (m *mockRepo) DeleteUserGame(ctx context.Context, userID, gameID int) error {
	if m.deleteUserGame == nil {
		panic("unexpected call to DeleteUserGame")
	}
	return m.deleteUserGame(ctx, userID, gameID)
}

// ============================================================================
// Helpers
// ============================================================================

func newService(repo repository.GamesRepository) *games.GameService {
	return games.NewGameService(repo)
}

func assertCode(t *testing.T, err error, want g_errors.Code) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	got := g_errors.GetCode(err)
	if got != want {
		t.Errorf("error code: got %q, want %q (err: %v)", got, want, err)
	}
}

func assertReason(t *testing.T, err error, want string) {
	t.Helper()
	se, ok := g_errors.AsServiceError(err)
	if !ok {
		t.Fatalf("expected *ServiceError, got %T: %v", err, err)
	}
	if se.Reason != want {
		t.Errorf("error reason: got %q, want %q", se.Reason, want)
	}
}

func stubGame() *models.Game {
	now := time.Now()
	return &models.Game{
		ID:        1,
		Title:     "Half-Life 2",
		URL:       "https://store.steampowered.com/app/220",
		Creator:   10,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
}

func stubUserGame() *models.UserGame {
	return &models.UserGame{
		UserID:   10,
		GameID:   1,
		Priority: 5,
		Status:   models.StatusPlaying,
	}
}

// ============================================================================
// GetByID
// ============================================================================

func TestGetByID_Success(t *testing.T) {
	want := stubGame()
	svc := newService(&mockRepo{
		getByID: func(_ context.Context, id int) (*models.Game, error) {
			if id != 1 {
				t.Errorf("GetByID called with id=%d, want 1", id)
			}
			return want, nil
		},
	})

	got, err := svc.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("game ID: got %d, want %d", got.ID, want.ID)
	}
}

func TestGetByID_InvalidID(t *testing.T) {
	svc := newService(&mockRepo{})

	for _, id := range []int{0, -1, -100} {
		_, err := svc.GetByID(context.Background(), id)
		assertCode(t, err, g_errors.CodeInvalidInput)
		assertReason(t, err, g_errors.InvalidGameID)
	}
}

func TestGetByID_NotFound_WithSentinel(t *testing.T) {
	svc := newService(&mockRepo{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return nil, errors.Join(errors.New("storage.Op"), repository.ErrNotFound)
		},
	})

	_, err := svc.GetByID(context.Background(), 99)
	assertCode(t, err, g_errors.CodeNotFound)
	assertReason(t, err, g_errors.GameNotFound)
}

func TestGetByID_RepoError(t *testing.T) {
	svc := newService(&mockRepo{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return nil, errors.New("connection refused")
		},
	})

	_, err := svc.GetByID(context.Background(), 1)
	assertCode(t, err, g_errors.CodeInternal)
}

// ============================================================================
// GetAll
// ============================================================================

func TestGetAll_InvalidUserID(t *testing.T) {
	svc := newService(&mockRepo{})

	for _, uid := range []int{0, -5} {
		_, _, err := svc.GetAll(context.Background(), uid, repository.GetAllParams{})
		assertCode(t, err, g_errors.CodeUnauthorized)
		assertReason(t, err, g_errors.MissingUserID)
	}
}

func TestGetAll_NormalizesPageDefaults(t *testing.T) {
	var gotParams repository.GetAllParams
	svc := newService(&mockRepo{
		getAllPaginated: func(_ context.Context, _ int, p repository.GetAllParams) ([]models.UserGameResponse, int, error) {
			gotParams = p
			return nil, 0, nil
		},
	})

	_, _, err := svc.GetAll(context.Background(), 1, repository.GetAllParams{Page: 0, PageSize: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotParams.Page != 1 {
		t.Errorf("Page: got %d, want 1", gotParams.Page)
	}
	if gotParams.PageSize != 10 {
		t.Errorf("PageSize: got %d, want 10", gotParams.PageSize)
	}
}

func TestGetAll_CapsPageSize(t *testing.T) {
	var gotParams repository.GetAllParams
	svc := newService(&mockRepo{
		getAllPaginated: func(_ context.Context, _ int, p repository.GetAllParams) ([]models.UserGameResponse, int, error) {
			gotParams = p
			return nil, 0, nil
		},
	})

	_, _, _ = svc.GetAll(context.Background(), 1, repository.GetAllParams{PageSize: 999})
	if gotParams.PageSize != 100 {
		t.Errorf("PageSize: got %d, want 100 (capped)", gotParams.PageSize)
	}
}

func TestGetAll_DefaultSortOrderAsc(t *testing.T) {
	var gotParams repository.GetAllParams
	svc := newService(&mockRepo{
		getAllPaginated: func(_ context.Context, _ int, p repository.GetAllParams) ([]models.UserGameResponse, int, error) {
			gotParams = p
			return nil, 0, nil
		},
	})

	_, _, _ = svc.GetAll(context.Background(), 1, repository.GetAllParams{SortOrder: "INVALID"})
	if gotParams.SortOrder != "asc" {
		t.Errorf("SortOrder: got %q, want %q", gotParams.SortOrder, "asc")
	}
}

func TestGetAll_AllowsDesc(t *testing.T) {
	var gotParams repository.GetAllParams
	svc := newService(&mockRepo{
		getAllPaginated: func(_ context.Context, _ int, p repository.GetAllParams) ([]models.UserGameResponse, int, error) {
			gotParams = p
			return nil, 0, nil
		},
	})

	_, _, _ = svc.GetAll(context.Background(), 1, repository.GetAllParams{SortOrder: "desc"})
	if gotParams.SortOrder != "desc" {
		t.Errorf("SortOrder: got %q, want %q", gotParams.SortOrder, "desc")
	}
}

func TestGetAll_TrimsSearch(t *testing.T) {
	var gotParams repository.GetAllParams
	svc := newService(&mockRepo{
		getAllPaginated: func(_ context.Context, _ int, p repository.GetAllParams) ([]models.UserGameResponse, int, error) {
			gotParams = p
			return nil, 0, nil
		},
	})

	_, _, _ = svc.GetAll(context.Background(), 1, repository.GetAllParams{Search: "  Half-Life  "})
	if gotParams.Search != "Half-Life" {
		t.Errorf("Search: got %q, want %q", gotParams.Search, "Half-Life")
	}
}

// ============================================================================
// SearchAllGames
// ============================================================================

func TestSearchAllGames_EmptyQuery(t *testing.T) {
	svc := newService(&mockRepo{})

	for _, q := range []string{"", "   ", "\t"} {
		_, err := svc.SearchAllGames(context.Background(), q)
		assertCode(t, err, g_errors.CodeInvalidInput)
		assertReason(t, err, g_errors.EmptyQuery)
	}
}

func TestSearchAllGames_Success(t *testing.T) {
	want := []models.Game{{ID: 1, Title: "Half-Life 2"}}
	svc := newService(&mockRepo{
		searchAllGames: func(_ context.Context, query string) ([]models.Game, error) {
			if query != "Half-Life" {
				t.Errorf("query: got %q, want %q", query, "Half-Life")
			}
			return want, nil
		},
	})

	got, err := svc.SearchAllGames(context.Background(), "Half-Life")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != 1 {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestSearchAllGames_RepoError(t *testing.T) {
	svc := newService(&mockRepo{
		searchAllGames: func(_ context.Context, _ string) ([]models.Game, error) {
			return nil, errors.New("db error")
		},
	})

	_, err := svc.SearchAllGames(context.Background(), "Portal")
	assertCode(t, err, g_errors.CodeInternal)
}

// ============================================================================
// GetUserGame
// ============================================================================

func TestGetUserGame_InvalidUserID(t *testing.T) {
	svc := newService(&mockRepo{})
	_, err := svc.GetUserGame(context.Background(), 0, 1)
	assertCode(t, err, g_errors.CodeUnauthorized)
}

func TestGetUserGame_InvalidGameID(t *testing.T) {
	svc := newService(&mockRepo{})
	_, err := svc.GetUserGame(context.Background(), 1, 0)
	assertCode(t, err, g_errors.CodeInvalidInput)
	assertReason(t, err, g_errors.InvalidGameID)
}

func TestGetUserGame_Success(t *testing.T) {
	want := stubUserGame()
	svc := newService(&mockRepo{
		getUserGame: func(_ context.Context, _, _ int) (*models.UserGame, error) {
			return want, nil
		},
	})

	got, err := svc.GetUserGame(context.Background(), 10, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != models.StatusPlaying {
		t.Errorf("Status: got %q, want %q", got.Status, models.StatusPlaying)
	}
}

// ============================================================================
// GetGameStats
// ============================================================================

func TestGetGameStats_InvalidUserID(t *testing.T) {
	svc := newService(&mockRepo{})
	_, err := svc.GetGameStats(context.Background(), 0)
	assertCode(t, err, g_errors.CodeUnauthorized)
}

func TestGetGameStats_Success(t *testing.T) {
	want := repository.StatusCounts{Finished: 3, Playing: 1, Planned: 10, Dropped: 2}
	svc := newService(&mockRepo{
		getUserGameStatusCounts: func(_ context.Context, _ int) (repository.StatusCounts, error) {
			return want, nil
		},
	})

	got, err := svc.GetGameStats(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("counts: got %+v, want %+v", got, want)
	}
}

func TestGetGameStats_RepoError(t *testing.T) {
	svc := newService(&mockRepo{
		getUserGameStatusCounts: func(_ context.Context, _ int) (repository.StatusCounts, error) {
			return repository.StatusCounts{}, errors.New("db error")
		},
	})

	_, err := svc.GetGameStats(context.Background(), 1)
	assertCode(t, err, g_errors.CodeInternal)
}

// ============================================================================
// Create
// ============================================================================

func TestCreate_ValidationErrors(t *testing.T) {
	svc := newService(&mockRepo{})
	now := time.Now()

	tests := []struct {
		name     string
		game     *models.Game
		userGame *models.UserGame
		reason   string
	}{
		{
			name:     "empty title",
			game:     &models.Game{Title: "   ", URL: "http://x.com"},
			userGame: &models.UserGame{Priority: 0},
			reason:   g_errors.InvalidGameTitle,
		},
		{
			name:     "empty url",
			game:     &models.Game{Title: "Title", URL: ""},
			userGame: &models.UserGame{Priority: 0},
			reason:   g_errors.InvalidURL,
		},
		{
			name:     "priority below 0",
			game:     &models.Game{Title: "Title", URL: "http://x.com", CreatedAt: &now, UpdatedAt: &now},
			userGame: &models.UserGame{Priority: -1},
			reason:   g_errors.InvalidPriority,
		},
		{
			name:     "priority above 10",
			game:     &models.Game{Title: "Title", URL: "http://x.com", CreatedAt: &now, UpdatedAt: &now},
			userGame: &models.UserGame{Priority: 11},
			reason:   g_errors.InvalidPriority,
		},
		{
			name:     "invalid status",
			game:     &models.Game{Title: "Title", URL: "http://x.com", CreatedAt: &now, UpdatedAt: &now},
			userGame: &models.UserGame{Priority: 0, Status: "invalid_status"},
			reason:   g_errors.InvalidInput,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), tc.game, tc.userGame)
			assertCode(t, err, g_errors.CodeInvalidInput)
			assertReason(t, err, tc.reason)
		})
	}
}

func TestCreate_SetsDefaultStatus(t *testing.T) {
	var gotUserGame *models.UserGame
	svc := newService(&mockRepo{
		createWithUserGame: func(_ context.Context, g *models.Game, ug *models.UserGame) (*models.Game, error) {
			gotUserGame = ug
			return g, nil
		},
	})

	now := time.Now()
	game := &models.Game{Title: "Portal", URL: "http://portal.com", CreatedAt: &now, UpdatedAt: &now}
	ug := &models.UserGame{Priority: 0, Status: ""}

	_, err := svc.Create(context.Background(), game, ug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUserGame.Status != models.StatusPlanned {
		t.Errorf("Status: got %q, want %q", gotUserGame.Status, models.StatusPlanned)
	}
}

func TestCreate_Conflict(t *testing.T) {
	svc := newService(&mockRepo{
		createWithUserGame: func(_ context.Context, _ *models.Game, _ *models.UserGame) (*models.Game, error) {
			return nil, repository.ErrAlreadyExists
		},
	})

	now := time.Now()
	game := &models.Game{Title: "Portal", URL: "http://portal.com", CreatedAt: &now, UpdatedAt: &now}
	ug := &models.UserGame{Priority: 0}

	_, err := svc.Create(context.Background(), game, ug)
	assertCode(t, err, g_errors.CodeConflict)
	assertReason(t, err, g_errors.GameAlreadyExists)
}

func TestCreate_Success(t *testing.T) {
	want := stubGame()
	svc := newService(&mockRepo{
		createWithUserGame: func(_ context.Context, _ *models.Game, _ *models.UserGame) (*models.Game, error) {
			return want, nil
		},
	})

	now := time.Now()
	game := &models.Game{Title: "Half-Life 2", URL: "https://store.steampowered.com/app/220", CreatedAt: &now, UpdatedAt: &now}
	ug := &models.UserGame{Priority: 3, Status: models.StatusPlanned}

	got, err := svc.Create(context.Background(), game, ug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID: got %d, want %d", got.ID, want.ID)
	}
}

// ============================================================================
// Update
// ============================================================================

func TestUpdate_ValidationErrors(t *testing.T) {
	svc := newService(&mockRepo{})
	now := time.Now()

	tests := []struct {
		name     string
		game     *models.Game
		userGame *models.UserGame
		reason   string
	}{
		{
			name:     "invalid game id",
			game:     &models.Game{ID: 0, Title: "Title", URL: "http://x.com"},
			userGame: &models.UserGame{Priority: 0},
			reason:   g_errors.InvalidGameID,
		},
		{
			name:     "empty title",
			game:     &models.Game{ID: 1, Title: "", URL: "http://x.com"},
			userGame: &models.UserGame{Priority: 0},
			reason:   g_errors.InvalidGameTitle,
		},
		{
			name:     "empty url",
			game:     &models.Game{ID: 1, Title: "Title", URL: ""},
			userGame: &models.UserGame{Priority: 0},
			reason:   g_errors.InvalidURL,
		},
		{
			name:     "invalid priority",
			game:     &models.Game{ID: 1, Title: "Title", URL: "http://x.com", CreatedAt: &now, UpdatedAt: &now},
			userGame: &models.UserGame{Priority: 15},
			reason:   g_errors.InvalidPriority,
		},
		{
			name:     "invalid status",
			game:     &models.Game{ID: 1, Title: "Title", URL: "http://x.com", CreatedAt: &now, UpdatedAt: &now},
			userGame: &models.UserGame{Priority: 0, Status: "bad_status"},
			reason:   g_errors.InvalidStatus,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Update(context.Background(), tc.game, tc.userGame)
			assertCode(t, err, g_errors.CodeInvalidInput)
			assertReason(t, err, tc.reason)
		})
	}
}

func TestUpdate_Success(t *testing.T) {
	svc := newService(&mockRepo{
		updateWithUserGame: func(_ context.Context, g *models.Game, _ *models.UserGame) (*models.Game, error) {
			return g, nil
		},
	})

	now := time.Now()
	game := &models.Game{ID: 1, Title: "Half-Life 2 Updated", URL: "http://hl2.com", CreatedAt: &now, UpdatedAt: &now}
	ug := &models.UserGame{Priority: 5, Status: models.StatusFinished}

	got, err := svc.Update(context.Background(), game, ug)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != 1 {
		t.Errorf("ID: got %d, want 1", got.ID)
	}
}

func TestUpdate_Conflict(t *testing.T) {
	svc := newService(&mockRepo{
		updateWithUserGame: func(_ context.Context, _ *models.Game, _ *models.UserGame) (*models.Game, error) {
			return nil, repository.ErrAlreadyExists
		},
	})

	now := time.Now()
	game := &models.Game{ID: 1, Title: "Title", URL: "http://x.com", CreatedAt: &now, UpdatedAt: &now}
	ug := &models.UserGame{Priority: 0}

	_, err := svc.Update(context.Background(), game, ug)
	assertCode(t, err, g_errors.CodeConflict)
}

func TestUpdate_NotFound(t *testing.T) {
	svc := newService(&mockRepo{
		updateWithUserGame: func(_ context.Context, _ *models.Game, _ *models.UserGame) (*models.Game, error) {
			return nil, repository.ErrNotFound
		},
	})

	now := time.Now()
	game := &models.Game{ID: 99, Title: "Title", URL: "http://x.com", CreatedAt: &now, UpdatedAt: &now}
	ug := &models.UserGame{Priority: 0}

	_, err := svc.Update(context.Background(), game, ug)
	assertCode(t, err, g_errors.CodeNotFound)
}

// ============================================================================
// UpdateStatus
// ============================================================================

func TestUpdateStatus_ValidationErrors(t *testing.T) {
	svc := newService(&mockRepo{})

	tests := []struct {
		name   string
		userID int
		gameID int
		status models.GameStatus
		code   g_errors.Code
	}{
		{"invalid userID", 0, 1, models.StatusPlaying, g_errors.CodeUnauthorized},
		{"invalid gameID", 1, 0, models.StatusPlaying, g_errors.CodeInvalidInput},
		{"invalid status", 1, 1, "unknown", g_errors.CodeInvalidInput},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.UpdateStatus(context.Background(), tc.userID, tc.gameID, tc.status)
			assertCode(t, err, tc.code)
		})
	}
}

func TestUpdateStatus_AllValidStatuses(t *testing.T) {
	for _, status := range []models.GameStatus{
		models.StatusPlanned,
		models.StatusPlaying,
		models.StatusFinished,
		models.StatusDropped,
	} {
		status := status
		t.Run(string(status), func(t *testing.T) {
			svc := newService(&mockRepo{
				updateUserGameStatus: func(_ context.Context, _, _ int, s models.GameStatus) error {
					if s != status {
						t.Errorf("status: got %q, want %q", s, status)
					}
					return nil
				},
			})
			if err := svc.UpdateStatus(context.Background(), 1, 1, status); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestUpdateStatus_NotFound(t *testing.T) {
	svc := newService(&mockRepo{
		updateUserGameStatus: func(_ context.Context, _, _ int, _ models.GameStatus) error {
			return repository.ErrNoRowsAffected
		},
	})

	err := svc.UpdateStatus(context.Background(), 1, 1, models.StatusFinished)
	assertCode(t, err, g_errors.CodeNotFound)
}

// ============================================================================
// UpdatePriority
// ============================================================================

func TestUpdatePriority_ValidationErrors(t *testing.T) {
	svc := newService(&mockRepo{})

	tests := []struct {
		name     string
		userID   int
		gameID   int
		priority int
		code     g_errors.Code
	}{
		{"invalid userID", 0, 1, 5, g_errors.CodeUnauthorized},
		{"invalid gameID", 1, 0, 5, g_errors.CodeInvalidInput},
		{"priority -1", 1, 1, -1, g_errors.CodeInvalidInput},
		{"priority 11", 1, 1, 11, g_errors.CodeInvalidInput},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.UpdatePriority(context.Background(), tc.userID, tc.gameID, tc.priority)
			assertCode(t, err, tc.code)
		})
	}
}

func TestUpdatePriority_BoundaryValues(t *testing.T) {
	for _, priority := range []int{0, 1, 10} {
		priority := priority
		t.Run("priority ok", func(t *testing.T) {
			svc := newService(&mockRepo{
				updateUserGamePriority: func(_ context.Context, _, _ int, p int) error {
					if p != priority {
						t.Errorf("priority: got %d, want %d", p, priority)
					}
					return nil
				},
			})
			if err := svc.UpdatePriority(context.Background(), 1, 1, priority); err != nil {
				t.Errorf("priority %d: unexpected error: %v", priority, err)
			}
		})
	}
}

func TestUpdatePriority_NotFound(t *testing.T) {
	svc := newService(&mockRepo{
		updateUserGamePriority: func(_ context.Context, _, _, _ int) error {
			return repository.ErrNoRowsAffected
		},
	})

	err := svc.UpdatePriority(context.Background(), 1, 1, 5)
	assertCode(t, err, g_errors.CodeNotFound)
}

// ============================================================================
// Delete
// ============================================================================

func TestDelete_InvalidID(t *testing.T) {
	svc := newService(&mockRepo{})
	err := svc.Delete(context.Background(), 0)
	assertCode(t, err, g_errors.CodeInvalidInput)
	assertReason(t, err, g_errors.InvalidGameID)
}

func TestDelete_Success(t *testing.T) {
	svc := newService(&mockRepo{
		delete: func(_ context.Context, id int) error {
			if id != 42 {
				t.Errorf("Delete called with id=%d, want 42", id)
			}
			return nil
		},
	})
	if err := svc.Delete(context.Background(), 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	svc := newService(&mockRepo{
		delete: func(_ context.Context, _ int) error {
			return repository.ErrNoRowsAffected
		},
	})
	err := svc.Delete(context.Background(), 1)
	assertCode(t, err, g_errors.CodeNotFound)
}

// ============================================================================
// DeleteUserGame
// ============================================================================

func TestDeleteUserGame_InvalidInputs(t *testing.T) {
	svc := newService(&mockRepo{})

	tests := []struct {
		name   string
		userID int
		gameID int
		code   g_errors.Code
	}{
		{"userID 0", 0, 1, g_errors.CodeUnauthorized},
		{"gameID 0", 1, 0, g_errors.CodeInvalidInput},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.DeleteUserGame(context.Background(), tc.userID, tc.gameID)
			assertCode(t, err, tc.code)
		})
	}
}

func TestDeleteUserGame_Success(t *testing.T) {
	svc := newService(&mockRepo{
		deleteUserGame: func(_ context.Context, userID, gameID int) error {
			if userID != 5 || gameID != 10 {
				t.Errorf("DeleteUserGame(%d, %d), want (5, 10)", userID, gameID)
			}
			return nil
		},
	})
	if err := svc.DeleteUserGame(context.Background(), 5, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteUserGame_NotFound(t *testing.T) {
	svc := newService(&mockRepo{
		deleteUserGame: func(_ context.Context, _, _ int) error {
			return repository.ErrNoRowsAffected
		},
	})
	err := svc.DeleteUserGame(context.Background(), 1, 1)
	assertCode(t, err, g_errors.CodeNotFound)
}

// ============================================================================
// AddUserGame
// ============================================================================

func TestAddUserGame_InvalidInputs(t *testing.T) {
	svc := newService(&mockRepo{})

	tests := []struct {
		name   string
		userID int
		gameID int
		code   g_errors.Code
	}{
		{"userID 0", 0, 1, g_errors.CodeUnauthorized},
		{"gameID 0", 1, 0, g_errors.CodeInvalidInput},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.AddUserGame(context.Background(), tc.userID, tc.gameID)
			assertCode(t, err, tc.code)
		})
	}
}

func TestAddUserGame_GameNotFound(t *testing.T) {
	svc := newService(&mockRepo{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return nil, repository.ErrNotFound
		},
	})

	err := svc.AddUserGame(context.Background(), 1, 99)
	assertCode(t, err, g_errors.CodeNotFound)
}

func TestAddUserGame_AlreadyInLibrary(t *testing.T) {
	svc := newService(&mockRepo{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil
		},
		addUserGame: func(_ context.Context, _ *models.UserGame) error {
			return repository.ErrAlreadyExists
		},
	})

	err := svc.AddUserGame(context.Background(), 1, 1)
	assertCode(t, err, g_errors.CodeConflict)
}

func TestAddUserGame_SetsDefaultPriorityAndStatus(t *testing.T) {
	var gotUG *models.UserGame
	svc := newService(&mockRepo{
		getByID: func(_ context.Context, _ int) (*models.Game, error) {
			return stubGame(), nil
		},
		addUserGame: func(_ context.Context, ug *models.UserGame) error {
			gotUG = ug
			return nil
		},
	})

	if err := svc.AddUserGame(context.Background(), 7, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUG.Priority != 0 {
		t.Errorf("Priority: got %d, want 0", gotUG.Priority)
	}
	if gotUG.Status != models.StatusPlanned {
		t.Errorf("Status: got %q, want %q", gotUG.Status, models.StatusPlanned)
	}
	if gotUG.UserID != 7 {
		t.Errorf("UserID: got %d, want 7", gotUG.UserID)
	}
}

// ============================================================================
// GetUserGames
// ============================================================================

func TestGetUserGames_InvalidUserID(t *testing.T) {
	svc := newService(&mockRepo{})
	_, _, err := svc.GetUserGames(context.Background(), 0, repository.GetUserGamesParams{})
	assertCode(t, err, g_errors.CodeUnauthorized)
}

func TestGetUserGames_InvalidStatus(t *testing.T) {
	svc := newService(&mockRepo{})
	invalid := models.GameStatus("nonexistent")
	_, _, err := svc.GetUserGames(context.Background(), 1, repository.GetUserGamesParams{
		Status: &invalid,
	})
	assertCode(t, err, g_errors.CodeInvalidInput)
	assertReason(t, err, g_errors.InvalidStatus)
}

func TestGetUserGames_NormalizesParams(t *testing.T) {
	var gotParams repository.GetUserGamesParams
	svc := newService(&mockRepo{
		getUserGames: func(_ context.Context, _ int, p repository.GetUserGamesParams) ([]models.UserGameResponse, int, error) {
			gotParams = p
			return nil, 0, nil
		},
	})

	_, _, _ = svc.GetUserGames(context.Background(), 1, repository.GetUserGamesParams{
		Page:      0,
		PageSize:  0,
		SortOrder: "WRONG",
		Search:    "  portal  ",
	})

	if gotParams.Page != 1 {
		t.Errorf("Page: got %d, want 1", gotParams.Page)
	}
	if gotParams.PageSize != 10 {
		t.Errorf("PageSize: got %d, want 10", gotParams.PageSize)
	}
	if gotParams.SortOrder != "asc" {
		t.Errorf("SortOrder: got %q, want %q", gotParams.SortOrder, "asc")
	}
	if gotParams.Search != "portal" {
		t.Errorf("Search: got %q, want %q", gotParams.Search, "portal")
	}
}

func TestGetUserGames_ValidStatusPassed(t *testing.T) {
	status := models.StatusFinished
	var gotParams repository.GetUserGamesParams
	svc := newService(&mockRepo{
		getUserGames: func(_ context.Context, _ int, p repository.GetUserGamesParams) ([]models.UserGameResponse, int, error) {
			gotParams = p
			return nil, 0, nil
		},
	})

	_, _, err := svc.GetUserGames(context.Background(), 1, repository.GetUserGamesParams{Status: &status})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotParams.Status == nil || *gotParams.Status != models.StatusFinished {
		t.Errorf("Status: got %v, want %q", gotParams.Status, models.StatusFinished)
	}
}

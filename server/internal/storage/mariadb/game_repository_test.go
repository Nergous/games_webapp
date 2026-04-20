// internal/storage/mariadb/game_repository_test.go
package mariadb_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"games_webapp/internal/models"
	"games_webapp/internal/repository"
	"games_webapp/internal/storage/mariadb"
	"os"
	"testing"
	"time"
)

// ============================================================================
// Test infrastructure
// ============================================================================

func testDSN() string {
	if dsn := os.Getenv("TEST_DB_DSN"); dsn != "" {
		return dsn
	}
	return "root:@tcp(localhost:3306)/test-games?parseTime=true"
}

func setupDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("mysql", testDSN())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	if err := db.Ping(); err != nil {
		t.Skipf("database unavailable, skipping: %v", err)
	}

	t.Cleanup(func() { db.Close() })

	// Пересоздаём схему перед каждым тестовым прогоном
	dropAndMigrate(t, db)

	return db
}

func dropAndMigrate(t *testing.T, db *sql.DB) {
	t.Helper()

	stmts := []string{
		"DROP TABLE IF EXISTS user_games",
		"DROP TABLE IF EXISTS games",
		`CREATE TABLE games (
            id         INT AUTO_INCREMENT PRIMARY KEY,
            title      VARCHAR(255) NOT NULL,
            preambula  TEXT,
            image      VARCHAR(255) DEFAULT '',
            developer  VARCHAR(255) DEFAULT '',
            publisher  VARCHAR(255) DEFAULT '',
            year       VARCHAR(10)  DEFAULT '',
            genre      VARCHAR(100) DEFAULT '',
            creator    INT NOT NULL DEFAULT 0,
            url        VARCHAR(255) NOT NULL,
            created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
            UNIQUE KEY idx_games_url (url)
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE user_games (
            id       INT AUTO_INCREMENT PRIMARY KEY,
            user_id  INT NOT NULL,
            game_id  INT NOT NULL,
            priority INT NOT NULL DEFAULT 0,
            status   VARCHAR(20) NOT NULL DEFAULT 'planned',
            INDEX idx_user_id (user_id),
            INDEX idx_game_id (game_id),
            UNIQUE KEY idx_unique (user_id, game_id),
            CONSTRAINT fk_game FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE
        ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("migrate: %v\nSQL: %s", err, s)
		}
	}
}

// truncate быстро чистит данные между тестами внутри одного прогона.
func truncate(t *testing.T, db *sql.DB) {
	t.Helper()
	db.Exec("DELETE FROM user_games")
	db.Exec("DELETE FROM games")
}

// ============================================================================
// Fixtures
// ============================================================================

func insertGame(t *testing.T, db *sql.DB, title, url string, creatorID int) *models.Game {
	t.Helper()

	now := time.Now()
	result, err := db.Exec(
		"INSERT INTO games (title, preambula, image, developer, publisher, year, genre, creator, url) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		title, "desc", "img.jpg", "dev", "pub", "2024", "action", creatorID, url,
	)
	if err != nil {
		t.Fatalf("insertGame %q: %v", title, err)
	}
	id, _ := result.LastInsertId()
	return &models.Game{
		ID:        int(id),
		Title:     title,
		URL:       url,
		Creator:   creatorID,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
}

func insertUserGame(t *testing.T, db *sql.DB, userID, gameID, priority int, status models.GameStatus) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO user_games (user_id, game_id, priority, status) VALUES (?, ?, ?, ?)",
		userID, gameID, priority, status,
	)
	if err != nil {
		t.Fatalf("insertUserGame user=%d game=%d: %v", userID, gameID, err)
	}
}

// newRepo создаёт репозиторий поверх тестовой БД.
func newRepo(db *sql.DB) *mariadb.GamesRepository {
	return mariadb.NewGamesRepository(db)
}

// ============================================================================
// GetByID
// ============================================================================

func TestGetByID_Found(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Half-Life 2", "https://store.steampowered.com/app/220", 1)
	repo := newRepo(db)

	got, err := repo.GetByID(context.Background(), game.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != game.ID {
		t.Errorf("ID: got %d, want %d", got.ID, game.ID)
	}
	if got.Title != "Half-Life 2" {
		t.Errorf("Title: got %q, want %q", got.Title, "Half-Life 2")
	}
	if got.URL != game.URL {
		t.Errorf("URL: got %q, want %q", got.URL, game.URL)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	db := setupDB(t)
	repo := newRepo(db)

	_, err := repo.GetByID(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isErrNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ============================================================================
// GetUserGame
// ============================================================================

func TestGetUserGame_Found(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "https://store.steampowered.com/app/400", 1)
	insertUserGame(t, db, 10, game.ID, 3, models.StatusPlaying)
	repo := newRepo(db)

	got, err := repo.GetUserGame(context.Background(), 10, game.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.UserID != 10 {
		t.Errorf("UserID: got %d, want 10", got.UserID)
	}
	if got.Priority != 3 {
		t.Errorf("Priority: got %d, want 3", got.Priority)
	}
	if got.Status != models.StatusPlaying {
		t.Errorf("Status: got %q, want %q", got.Status, models.StatusPlaying)
	}
}

func TestGetUserGame_NotFound(t *testing.T) {
	db := setupDB(t)
	repo := newRepo(db)

	_, err := repo.GetUserGame(context.Background(), 1, 99999)
	if !isErrNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// ============================================================================
// GetUserGameStatusCounts
// ============================================================================

func TestGetUserGameStatusCounts(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	userID := 5
	games := []struct {
		title  string
		url    string
		status models.GameStatus
	}{
		{"Game1", "http://g1.com", models.StatusFinished},
		{"Game2", "http://g2.com", models.StatusFinished},
		{"Game3", "http://g3.com", models.StatusPlaying},
		{"Game4", "http://g4.com", models.StatusPlanned},
		{"Game5", "http://g5.com", models.StatusDropped},
	}

	for _, g := range games {
		game := insertGame(t, db, g.title, g.url, 1)
		insertUserGame(t, db, userID, game.ID, 0, g.status)
	}

	repo := newRepo(db)
	counts, err := repo.GetUserGameStatusCounts(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if counts.Finished != 2 {
		t.Errorf("Finished: got %d, want 2", counts.Finished)
	}
	if counts.Playing != 1 {
		t.Errorf("Playing: got %d, want 1", counts.Playing)
	}
	if counts.Planned != 1 {
		t.Errorf("Planned: got %d, want 1", counts.Planned)
	}
	if counts.Dropped != 1 {
		t.Errorf("Dropped: got %d, want 1", counts.Dropped)
	}
}

func TestGetUserGameStatusCounts_EmptyUser(t *testing.T) {
	db := setupDB(t)
	repo := newRepo(db)

	counts, err := repo.GetUserGameStatusCounts(context.Background(), 9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts != (repository.StatusCounts{}) {
		t.Errorf("expected zero counts, got %+v", counts)
	}
}

// ============================================================================
// SearchAllGames
// ============================================================================

func TestSearchAllGames(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	insertGame(t, db, "Half-Life 2", "http://hl2.com", 1)
	insertGame(t, db, "Half-Life: Alyx", "http://hla.com", 1)
	insertGame(t, db, "Portal 2", "http://p2.com", 1)
	repo := newRepo(db)

	got, err := repo.SearchAllGames(context.Background(), "Half")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len: got %d, want 2", len(got))
	}
}

func TestSearchAllGames_NoResults(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	insertGame(t, db, "Portal 2", "http://p2.com", 1)
	repo := newRepo(db)

	got, err := repo.SearchAllGames(context.Background(), "zzznomatch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

// ============================================================================
// CreateWithUserGame
// ============================================================================

func TestCreateWithUserGame_Success(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	now := time.Now()
	game := &models.Game{
		Title:     "Portal",
		Preambula: "test game",
		Image:     "portal.jpg",
		Developer: "Valve",
		Publisher: "Valve",
		Year:      "2007",
		Genre:     "Puzzle",
		Creator:   1,
		URL:       "https://store.steampowered.com/app/400",
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	userGame := &models.UserGame{
		UserID:   10,
		Priority: 2,
		Status:   models.StatusPlanned,
	}

	repo := newRepo(db)
	created, err := repo.CreateWithUserGame(context.Background(), game, userGame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if created.ID == 0 {
		t.Error("ID must be set after create")
	}
	if created.Title != "Portal" {
		t.Errorf("Title: got %q, want %q", created.Title, "Portal")
	}

	// Проверяем что user_game тоже создан
	ug, err := repo.GetUserGame(context.Background(), 10, created.ID)
	if err != nil {
		t.Fatalf("GetUserGame after create: %v", err)
	}
	if ug.Priority != 2 {
		t.Errorf("Priority: got %d, want 2", ug.Priority)
	}
	if ug.Status != models.StatusPlanned {
		t.Errorf("Status: got %q, want %q", ug.Status, models.StatusPlanned)
	}
}

func TestCreateWithUserGame_DuplicateURL(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	now := time.Now()
	game := &models.Game{
		Title: "Portal", URL: "http://portal.com",
		Creator: 1, CreatedAt: &now, UpdatedAt: &now,
	}
	ug := &models.UserGame{UserID: 1, Status: models.StatusPlanned}

	repo := newRepo(db)
	if _, err := repo.CreateWithUserGame(context.Background(), game, ug); err != nil {
		t.Fatalf("first create: %v", err)
	}

	// второй раз с тем же URL
	game2 := &models.Game{
		Title: "Portal Clone", URL: "http://portal.com",
		Creator: 2, CreatedAt: &now, UpdatedAt: &now,
	}
	ug2 := &models.UserGame{UserID: 2, Status: models.StatusPlanned}

	_, err := repo.CreateWithUserGame(context.Background(), game2, ug2)
	if !isErrAlreadyExists(err) {
		t.Errorf("expected ErrAlreadyExists, got: %v", err)
	}
}

// ============================================================================
// AddUserGame
// ============================================================================

func TestAddUserGame_Success(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "http://portal.com", 1)
	repo := newRepo(db)

	err := repo.AddUserGame(context.Background(), &models.UserGame{
		UserID:   42,
		GameID:   game.ID,
		Priority: 1,
		Status:   models.StatusPlanned,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ug, err := repo.GetUserGame(context.Background(), 42, game.ID)
	if err != nil {
		t.Fatalf("GetUserGame: %v", err)
	}
	if ug.Priority != 1 {
		t.Errorf("Priority: got %d, want 1", ug.Priority)
	}
}

func TestAddUserGame_Duplicate(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "http://portal.com", 1)
	insertUserGame(t, db, 42, game.ID, 0, models.StatusPlanned)
	repo := newRepo(db)

	err := repo.AddUserGame(context.Background(), &models.UserGame{
		UserID: 42, GameID: game.ID, Status: models.StatusPlanned,
	})
	if !isErrAlreadyExists(err) {
		t.Errorf("expected ErrAlreadyExists, got: %v", err)
	}
}

// ============================================================================
// UpdateWithUserGame
// ============================================================================

func TestUpdateWithUserGame_Success(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "http://portal.com", 1)
	insertUserGame(t, db, 10, game.ID, 0, models.StatusPlanned)
	repo := newRepo(db)

	now := time.Now()
	updatedGame := &models.Game{
		ID:        game.ID,
		Title:     "Portal Updated",
		URL:       "http://portal-updated.com",
		Developer: "Valve Updated",
		Publisher: "Valve",
		UpdatedAt: &now,
	}
	updatedUG := &models.UserGame{
		UserID:   10,
		GameID:   game.ID,
		Priority: 7,
		Status:   models.StatusFinished,
	}

	got, err := repo.UpdateWithUserGame(context.Background(), updatedGame, updatedUG)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Portal Updated" {
		t.Errorf("Title: got %q, want %q", got.Title, "Portal Updated")
	}

	// Проверяем user_game
	ug, err := repo.GetUserGame(context.Background(), 10, game.ID)
	if err != nil {
		t.Fatalf("GetUserGame: %v", err)
	}
	if ug.Priority != 7 {
		t.Errorf("Priority: got %d, want 7", ug.Priority)
	}
	if ug.Status != models.StatusFinished {
		t.Errorf("Status: got %q, want %q", ug.Status, models.StatusFinished)
	}
}

func TestUpdateWithUserGame_DuplicateURL(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	_ = insertGame(t, db, "Portal", "http://portal.com", 1)
	game2 := insertGame(t, db, "HL2", "http://hl2.com", 1)
	insertUserGame(t, db, 10, game2.ID, 0, models.StatusPlanned)
	repo := newRepo(db)

	now := time.Now()
	// пытаемся дать game2 URL от game1
	_, err := repo.UpdateWithUserGame(context.Background(), &models.Game{
		ID:        game2.ID,
		Title:     "HL2",
		URL:       "http://portal.com", // конфликт
		UpdatedAt: &now,
	}, &models.UserGame{UserID: 10, GameID: game2.ID})

	if !isErrAlreadyExists(err) {
		t.Errorf("expected ErrAlreadyExists, got: %v", err)
	}
}

// ============================================================================
// UpdateUserGameStatus
// ============================================================================

func TestUpdateUserGameStatus_Success(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "http://portal.com", 1)
	insertUserGame(t, db, 10, game.ID, 0, models.StatusPlanned)
	repo := newRepo(db)

	err := repo.UpdateUserGameStatus(context.Background(), 10, game.ID, models.StatusFinished)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ug, _ := repo.GetUserGame(context.Background(), 10, game.ID)
	if ug.Status != models.StatusFinished {
		t.Errorf("Status: got %q, want %q", ug.Status, models.StatusFinished)
	}
}

func TestUpdateUserGameStatus_NotFound(t *testing.T) {
	db := setupDB(t)
	repo := newRepo(db)

	err := repo.UpdateUserGameStatus(context.Background(), 999, 999, models.StatusFinished)
	if !isErrNoRowsAffected(err) {
		t.Errorf("expected ErrNoRowsAffected, got: %v", err)
	}
}

// ============================================================================
// UpdateUserGamePriority
// ============================================================================

func TestUpdateUserGamePriority_Success(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "http://portal.com", 1)
	insertUserGame(t, db, 10, game.ID, 0, models.StatusPlanned)
	repo := newRepo(db)

	err := repo.UpdateUserGamePriority(context.Background(), 10, game.ID, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ug, _ := repo.GetUserGame(context.Background(), 10, game.ID)
	if ug.Priority != 8 {
		t.Errorf("Priority: got %d, want 8", ug.Priority)
	}
}

func TestUpdateUserGamePriority_NotFound(t *testing.T) {
	db := setupDB(t)
	repo := newRepo(db)

	err := repo.UpdateUserGamePriority(context.Background(), 999, 999, 5)
	if !isErrNoRowsAffected(err) {
		t.Errorf("expected ErrNoRowsAffected, got: %v", err)
	}
}

// ============================================================================
// Delete
// ============================================================================

func TestDelete_Success(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "http://portal.com", 1)
	repo := newRepo(db)

	if err := repo.Delete(context.Background(), game.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := repo.GetByID(context.Background(), game.ID)
	if !isErrNotFound(err) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestDelete_CascadesToUserGames(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "http://portal.com", 1)
	insertUserGame(t, db, 10, game.ID, 0, models.StatusPlanned)
	insertUserGame(t, db, 20, game.ID, 0, models.StatusPlaying)
	repo := newRepo(db)

	if err := repo.Delete(context.Background(), game.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// user_games должны удалиться каскадом
	var count int
	db.QueryRow("SELECT COUNT(*) FROM user_games WHERE game_id = ?", game.ID).Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 user_games after cascade delete, got %d", count)
	}
}

func TestDelete_NotFound(t *testing.T) {
	db := setupDB(t)
	repo := newRepo(db)

	err := repo.Delete(context.Background(), 99999)
	if !isErrNoRowsAffected(err) {
		t.Errorf("expected ErrNoRowsAffected, got: %v", err)
	}
}

// ============================================================================
// DeleteUserGame
// ============================================================================

func TestDeleteUserGame_Success(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "http://portal.com", 1)
	insertUserGame(t, db, 10, game.ID, 0, models.StatusPlanned)
	repo := newRepo(db)

	if err := repo.DeleteUserGame(context.Background(), 10, game.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := repo.GetUserGame(context.Background(), 10, game.ID)
	if !isErrNotFound(err) {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestDeleteUserGame_NotFound(t *testing.T) {
	db := setupDB(t)
	repo := newRepo(db)

	err := repo.DeleteUserGame(context.Background(), 999, 999)
	if !isErrNoRowsAffected(err) {
		t.Errorf("expected ErrNoRowsAffected, got: %v", err)
	}
}

func TestDeleteUserGame_DoesNotDeleteGame(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "http://portal.com", 1)
	insertUserGame(t, db, 10, game.ID, 0, models.StatusPlanned)
	repo := newRepo(db)

	if err := repo.DeleteUserGame(context.Background(), 10, game.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Сама игра должна остаться
	got, err := repo.GetByID(context.Background(), game.ID)
	if err != nil {
		t.Fatalf("game must still exist: %v", err)
	}
	if got.ID != game.ID {
		t.Errorf("game ID: got %d, want %d", got.ID, game.ID)
	}
}

// ============================================================================
// GetAllPaginated
// ============================================================================

func TestGetAllPaginated_Pagination(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	// вставляем 5 игр
	for i := range 5 {
		insertGame(t, db, fmt.Sprintf("Game %d", i), fmt.Sprintf("http://game%d.com", i), 1)
	}
	repo := newRepo(db)

	games, total, err := repo.GetAllPaginated(context.Background(), 1, repository.GetAllParams{
		Page:      1,
		PageSize:  2,
		SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Errorf("total: got %d, want 5", total)
	}
	if len(games) != 2 {
		t.Errorf("page size: got %d, want 2", len(games))
	}
}

func TestGetAllPaginated_ShowsUserGameData(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "http://portal.com", 1)
	insertUserGame(t, db, 10, game.ID, 5, models.StatusPlaying)
	repo := newRepo(db)

	games, _, err := repo.GetAllPaginated(context.Background(), 10, repository.GetAllParams{
		Page: 1, PageSize: 10, SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(games))
	}
	if games[0].Priority != 5 {
		t.Errorf("Priority: got %d, want 5", games[0].Priority)
	}
	if games[0].Status != models.StatusPlaying {
		t.Errorf("Status: got %q, want %q", games[0].Status, models.StatusPlaying)
	}
}

func TestGetAllPaginated_NullStatusForOtherUser(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	game := insertGame(t, db, "Portal", "http://portal.com", 1)
	insertUserGame(t, db, 10, game.ID, 5, models.StatusPlaying)
	repo := newRepo(db)

	// запрашиваем от имени другого пользователя — status/priority должны быть дефолтными
	games, _, err := repo.GetAllPaginated(context.Background(), 99, repository.GetAllParams{
		Page: 1, PageSize: 10, SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("expected 1 game, got %d", len(games))
	}
	if games[0].Priority != 0 {
		t.Errorf("Priority for other user: got %d, want 0", games[0].Priority)
	}
	if games[0].Status != "" {
		t.Errorf("Status for other user: got %q, want empty", games[0].Status)
	}
}

func TestGetAllPaginated_Search(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	insertGame(t, db, "Half-Life 2", "http://hl2.com", 1)
	insertGame(t, db, "Half-Life: Alyx", "http://hla.com", 1)
	insertGame(t, db, "Portal", "http://portal.com", 1)
	repo := newRepo(db)

	games, total, err := repo.GetAllPaginated(context.Background(), 1, repository.GetAllParams{
		Search:    "Half",
		Page:      1,
		PageSize:  10,
		SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("total: got %d, want 2", total)
	}
	if len(games) != 2 {
		t.Errorf("len: got %d, want 2", len(games))
	}
}

// ============================================================================
// GetUserGames
// ============================================================================

func TestGetUserGames_FilterByStatus(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	userID := 10
	g1 := insertGame(t, db, "Game1", "http://g1.com", 1)
	g2 := insertGame(t, db, "Game2", "http://g2.com", 1)
	g3 := insertGame(t, db, "Game3", "http://g3.com", 1)
	insertUserGame(t, db, userID, g1.ID, 0, models.StatusFinished)
	insertUserGame(t, db, userID, g2.ID, 0, models.StatusFinished)
	insertUserGame(t, db, userID, g3.ID, 0, models.StatusPlaying)
	repo := newRepo(db)

	status := models.StatusFinished
	games, total, err := repo.GetUserGames(context.Background(), userID, repository.GetUserGamesParams{
		Status:    &status,
		Page:      1,
		PageSize:  10,
		SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("total: got %d, want 2", total)
	}
	if len(games) != 2 {
		t.Errorf("len: got %d, want 2", len(games))
	}
	for _, g := range games {
		if g.Status != models.StatusFinished {
			t.Errorf("game %d has status %q, want finished", g.ID, g.Status)
		}
	}
}

func TestGetUserGames_OnlyOwnGames(t *testing.T) {
	db := setupDB(t)
	defer truncate(t, db)

	g1 := insertGame(t, db, "Game1", "http://g1.com", 1)
	g2 := insertGame(t, db, "Game2", "http://g2.com", 1)
	insertUserGame(t, db, 10, g1.ID, 0, models.StatusPlanned)
	insertUserGame(t, db, 20, g2.ID, 0, models.StatusPlanned) // другой пользователь
	repo := newRepo(db)

	games, total, err := repo.GetUserGames(context.Background(), 10, repository.GetUserGamesParams{
		Page: 1, PageSize: 10, SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("total: got %d, want 1 (only user 10's games)", total)
	}
	if len(games) != 1 || games[0].ID != g1.ID {
		t.Errorf("unexpected games: %+v", games)
	}
}

// ============================================================================
// Sentinel error helpers
// ============================================================================

func isErrNotFound(err error) bool {
	return errors.Is(err, repository.ErrNotFound)
}

func isErrAlreadyExists(err error) bool {
	return errors.Is(err, repository.ErrAlreadyExists)
}

func isErrNoRowsAffected(err error) bool {
	return errors.Is(err, repository.ErrNoRowsAffected)
}

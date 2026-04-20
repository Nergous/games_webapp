// internal/storage/mariadb/game_repository.go
package mariadb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"games_webapp/internal/models"
	"games_webapp/internal/repository"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type GamesRepository struct {
	db *sql.DB
}

func NewGamesRepository(db *sql.DB) *GamesRepository {
	return &GamesRepository{db: db}
}

// ============================================================================
// READ
// ============================================================================

func (r *GamesRepository) GetByID(ctx context.Context, id int) (*models.Game, error) {
	const op = "storage.mariadb.GamesRepository.GetByID"

	query := `
		SELECT id, title, preambula, image, developer, publisher, year, genre, creator, url, created_at, updated_at
		FROM games
		WHERE id = ?`

	var g models.Game
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&g.ID, &g.Title, &g.Preambula, &g.Image,
		&g.Developer, &g.Publisher, &g.Year, &g.Genre,
		&g.Creator, &g.URL, &g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, wrapNotFound(op, err)
	}

	return &g, nil
}

func (r *GamesRepository) GetAllPaginated(ctx context.Context, userID int, params repository.GetAllParams) ([]models.UserGameResponse, int, error) {
	const op = "storage.mariadb.GamesRepository.GetAllPaginated"

	allowedSort := map[string]string{
		"title": "games.title",
		"year":  "games.year",
	}

	sortField, ok := allowedSort[params.SortBy]
	if !ok {
		sortField = "games.title"
	}
	if strings.ToLower(params.SortOrder) != "desc" {
		params.SortOrder = "asc"
	}

	baseQuery := `
		FROM games
		LEFT JOIN user_games ON user_games.game_id = games.id AND user_games.user_id = ?`
	args := []any{userID}

	if params.Search != "" {
		baseQuery += " WHERE games.title LIKE ?"
		args = append(args, "%"+params.Search+"%")
	}

	// count
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+baseQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("%s: count: %w", op, err)
	}

	// data
	offset := (params.Page - 1) * params.PageSize
	dataQuery := fmt.Sprintf(`
		SELECT
			games.*,
			COALESCE(user_games.priority, 0) as priority,
			COALESCE(user_games.status, '')   as status
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?`,
		baseQuery, sortField, params.SortOrder,
	)
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: query: %w", op, err)
	}
	defer rows.Close()

	var results []models.UserGameResponse
	for rows.Next() {
		var g models.UserGameResponse
		if err := rows.Scan(
			&g.ID, &g.Title, &g.Preambula, &g.Image,
			&g.Developer, &g.Publisher, &g.Year, &g.Genre,
			&g.Creator, &g.URL, &g.CreatedAt, &g.UpdatedAt,
			&g.Priority, &g.Status,
		); err != nil {
			return nil, 0, fmt.Errorf("%s: scan: %w", op, err)
		}
		results = append(results, g)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("%s: rows: %w", op, err)
	}

	return results, total, nil
}

func (r *GamesRepository) GetUserGame(ctx context.Context, userID, gameID int) (*models.UserGame, error) {
	const op = "storage.mariadb.GamesRepository.GetUserGame"

	query := `
		SELECT id, user_id, game_id, priority, status
		FROM user_games
		WHERE user_id = ? AND game_id = ?`

	var ug models.UserGame
	err := r.db.QueryRowContext(ctx, query, userID, gameID).Scan(
		&ug.ID,
		&ug.UserID,
		&ug.GameID,
		&ug.Priority,
		&ug.Status,
	)
	if err != nil {
		return nil, wrapNotFound(op, err)
	}

	return &ug, nil
}

func (r *GamesRepository) GetUserGames(ctx context.Context, userID int, params repository.GetUserGamesParams) ([]models.UserGameResponse, int, error) {
	const op = "storage.mariadb.GamesRepository.GetUserGames"

	allowedSort := map[string]string{
		"title":    "games.title",
		"year":     "games.year",
		"priority": "user_games.priority",
	}
	sortField, ok := allowedSort[params.SortBy]
	if !ok {
		sortField = "games.title"
	}
	if strings.ToLower(params.SortOrder) != "desc" {
		params.SortOrder = "asc"
	}

	baseQuery := `
		FROM games
		INNER JOIN user_games ON user_games.game_id = games.id
		WHERE user_games.user_id = ?`

	args := []any{userID}

	if params.Status != nil {
		baseQuery += " AND user_games.status = ?"
		args = append(args, *params.Status)
	}
	if params.Search != "" {
		baseQuery += " AND games.title LIKE ?"
		args = append(args, "%"+params.Search+"%")
	}

	// count
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+baseQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("%s: count: %w", op, err)
	}

	// data
	offset := (params.Page - 1) * params.PageSize
	dataQuery := fmt.Sprintf(`
		SELECT
			games.id, games.title, games.preambula, games.image,
			games.developer, games.publisher, games.year, games.genre,
			games.creator, games.url, games.created_at, games.updated_at,
			user_games.priority, user_games.status
		%s
		ORDER BY %s %s
		LIMIT ? OFFSET ?`,
		baseQuery, sortField, params.SortOrder,
	)
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: query: %w", op, err)
	}
	defer rows.Close()

	var results []models.UserGameResponse
	for rows.Next() {
		var g models.UserGameResponse
		if err := rows.Scan(
			&g.ID, &g.Title, &g.Preambula, &g.Image,
			&g.Developer, &g.Publisher, &g.Year, &g.Genre,
			&g.Creator, &g.URL, &g.CreatedAt, &g.UpdatedAt,
			&g.Priority, &g.Status,
		); err != nil {
			return nil, 0, fmt.Errorf("%s: scan: %w", op, err)
		}
		results = append(results, g)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("%s: rows: %w", op, err)
	}

	return results, total, nil
}

func (r *GamesRepository) SearchAllGames(ctx context.Context, query string) ([]models.Game, error) {
	const op = "storage.mariadb.GamesRepository.SearchAllGames"

	q := `
		SELECT id, title, preambula, image, developer, publisher, year, genre, creator, url, created_at, updated_at
		FROM games
		WHERE title LIKE ?
		ORDER BY title ASC
		LIMIT 20`

	rows, err := r.db.QueryContext(ctx, q, "%"+query+"%")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var results []models.Game
	for rows.Next() {
		var g models.Game
		if err := rows.Scan(
			&g.ID, &g.Title, &g.Preambula, &g.Image,
			&g.Developer, &g.Publisher, &g.Year, &g.Genre,
			&g.Creator, &g.URL, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		results = append(results, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: rows: %w", op, err)
	}

	return results, nil
}

func (r *GamesRepository) GetUserGameStatusCounts(ctx context.Context, userID int) (repository.StatusCounts, error) {
	const op = "storage.mariadb.GamesRepository.GetUserGameStatusCounts"

	query := `
		SELECT
			COALESCE(SUM(status = 'finished'), 0) as finished,
			COALESCE(SUM(status = 'playing'),  0) as playing,
			COALESCE(SUM(status = 'planned'),  0) as planned,
			COALESCE(SUM(status = 'dropped'),  0) as dropped
		FROM user_games
		WHERE user_id = ?`

	var counts repository.StatusCounts
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&counts.Finished,
		&counts.Playing,
		&counts.Planned,
		&counts.Dropped,
	)
	if err != nil {
		return repository.StatusCounts{}, fmt.Errorf("%s: %w", op, err)
	}

	return counts, nil
}

// ============================================================================
// WRITE
// ============================================================================

func (r *GamesRepository) CreateWithUserGame(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error) {
	const op = "storage.mariadb.GamesRepository.CreateWithUserGame"

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO games (title, preambula, image, developer, publisher, year, genre, creator, url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		game.Title, game.Preambula, game.Image, game.Developer,
		game.Publisher, game.Year, game.Genre, game.Creator, game.URL,
	)
	if err != nil {
		return nil, wrapMySQLErr(op, err)
	}

	gameID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("%s: last insert id: %w", op, err)
	}
	game.ID = int(gameID)
	userGame.GameID = int(gameID)

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_games (user_id, game_id, priority, status)
		VALUES (?, ?, ?, ?)`,
		userGame.UserID, userGame.GameID, userGame.Priority, userGame.Status,
	); err != nil {
		return nil, wrapMySQLErr(op, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s: commit: %w", op, err)
	}

	return game, nil
}

func (r *GamesRepository) AddUserGame(ctx context.Context, userGame *models.UserGame) error {
	const op = "storage.mariadb.GamesRepository.AddUserGame"

	_, err := r.db.ExecContext(ctx,
		"INSERT INTO user_games (user_id, game_id, priority, status) VALUES (?, ?, ?, ?)",
		userGame.UserID, userGame.GameID, userGame.Priority, userGame.Status,
	)
	if err != nil {
		return wrapMySQLErr(op, err)
	}

	return nil
}

func (r *GamesRepository) UpdateWithUserGame(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error) {
	const op = "storage.mariadb.GamesRepository.UpdateWithUserGame"

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE games
		SET title = ?, preambula = ?, image = ?, developer = ?, publisher = ?,
		    year = ?, genre = ?, url = ?, updated_at = ?
		WHERE id = ?`,
		game.Title, game.Preambula, game.Image, game.Developer,
		game.Publisher, game.Year, game.Genre, game.URL, game.UpdatedAt, game.ID,
	); err != nil {
		return nil, wrapMySQLErr(op, err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE user_games
		SET priority = ?, status = ?
		WHERE user_id = ? AND game_id = ?`,
		userGame.Priority, userGame.Status, userGame.UserID, userGame.GameID,
	); err != nil {
		return nil, fmt.Errorf("%s: update user_game: %w", op, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%s: commit: %w", op, err)
	}

	return game, nil
}

func (r *GamesRepository) UpdateUserGameStatus(ctx context.Context, userID, gameID int, status models.GameStatus) error {
	const op = "storage.mariadb.GamesRepository.UpdateUserGameStatus"

	result, err := r.db.ExecContext(ctx,
		"UPDATE user_games SET status = ? WHERE user_id = ? AND game_id = ?",
		status, userID, gameID,
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return checkRowsAffected(op, result)
}

func (r *GamesRepository) UpdateUserGamePriority(ctx context.Context, userID, gameID int, priority int) error {
	const op = "storage.mariadb.GamesRepository.UpdateUserGamePriority"

	result, err := r.db.ExecContext(ctx,
		"UPDATE user_games SET priority = ? WHERE user_id = ? AND game_id = ?",
		priority, userID, gameID,
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return checkRowsAffected(op, result)
}

func (r *GamesRepository) Delete(ctx context.Context, id int) error {
	const op = "storage.mariadb.GamesRepository.Delete"

	result, err := r.db.ExecContext(ctx, "DELETE FROM games WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return checkRowsAffected(op, result)
}

func (r *GamesRepository) DeleteUserGame(ctx context.Context, userID, gameID int) error {
	const op = "storage.mariadb.GamesRepository.DeleteUserGame"

	result, err := r.db.ExecContext(ctx,
		"DELETE FROM user_games WHERE user_id = ? AND game_id = ?",
		userID, gameID,
	)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return checkRowsAffected(op, result)
}

// ============================================================================
// HELPERS
// ============================================================================

// wrapMySQLErr translate known MySQL-errors to repository sentitel-errors
// All other errors are wrapped via fmt.Errorf
func wrapMySQLErr(op string, err error) error {
	var mySQLErr *mysql.MySQLError
	if errors.As(err, &mySQLErr) && mySQLErr.Number == 1062 {
		return fmt.Errorf("%s: %w", op, repository.ErrAlreadyExists)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// wrapNotFound returns ErrNotFound if err == sql.ErrNoRows,
// otherwise returns wrapped err
func wrapNotFound(op string, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", op, repository.ErrNotFound)
	}

	return fmt.Errorf("%s: %w", op, err)
}

func checkRowsAffected(op string, result sql.Result) error {
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: rows affected: %w", op, err)
	}

	if n == 0 {
		return fmt.Errorf("%s: %w", op, repository.ErrNoRowsAffected)
	}

	return nil
}

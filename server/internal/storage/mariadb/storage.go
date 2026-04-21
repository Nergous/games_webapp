// internal/storage/mariadb/storage.go
package mariadb

import (
	"context"
	"database/sql"
	"fmt"

	"games_webapp/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

type Storage struct {
	db *sql.DB
}

func New(cfg config.Database) (*Storage, error) {
	const op = "storage.mariadb.New"

	dsn := cfg.GetDSN()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// Apply pool limits BEFORE the Ping so a misconfigured pool doesn't
	// quietly race the first request after startup.
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s: ping: %w", op, err)
	}

	return &Storage{db: db}, nil
}

func NewTest() (*Storage, error) {
	const op = "storage.mariadb.NewTest"

	dsn := config.GetTestDSN()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s: ping: %w", op, err)
	}

	return &Storage{db: db}, nil
}

func (s *Storage) GetDB() *sql.DB {
	return s.db
}

func (s *Storage) Close() error {
	const op = "storage.mariadb.Close"

	if s.db == nil {
		return fmt.Errorf("%s: db is nil", op)
	}

	if err := s.db.Close(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// Migrate runs the full schema migration using a background context. Kept
// for callers that don't manage a context themselves (mainly tests).
func (s *Storage) Migrate() error {
	return s.MigrateContext(context.Background())
}

// MigrateContext runs the full schema migration under ctx, so a stuck DB
// fails the command instead of blocking indefinitely.
func (s *Storage) MigrateContext(ctx context.Context) error {
	const op = "storage.mariadb.Migrate"

	if s.db == nil {
		return fmt.Errorf("%s: db is nil", op)
	}

	gameTable := `
		CREATE TABLE IF NOT EXISTS games (
			id 			INT AUTO_INCREMENT PRIMARY KEY,
			title 		VARCHAR(255) NOT NULL,
			preambula 	TEXT,
			image 		VARCHAR(255) DEFAULT '',
			developer 	VARCHAR(255) DEFAULT '',
			publisher 	VARCHAR(255) DEFAULT '',
			year 		VARCHAR(10) DEFAULT '',
			genre 		VARCHAR(100) DEFAULT '',
			creator 	INT NOT NULL DEFAULT 0,
			url 		VARCHAR(255) NOT NULL,
			created_at 	TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at 	TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY 	idx_games_url (url),
			-- FULLTEXT on title replaces the O(n) LIKE '%s%' scans in game search.
			-- Boolean mode is used at query time so callers can append '*' for
			-- prefix matching on the last token.
			FULLTEXT KEY ft_games_title (title)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	userGamesTable := `
		CREATE TABLE IF NOT EXISTS user_games (
			id 			INT AUTO_INCREMENT PRIMARY KEY,
			user_id 	INT NOT NULL,
			game_id 	INT NOT NULL,
			priority 	INT NOT NULL DEFAULT 0,
			status 		VARCHAR(20) NOT NULL DEFAULT 'planned',
			INDEX 		idx_user_games_user_id (user_id),
			INDEX 		idx_user_games_game_id (game_id),
			UNIQUE KEY 	idx_user_games_unique (user_id, game_id),
			CONSTRAINT 	fk_user_games_game_id
				FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	if _, err := s.db.ExecContext(ctx, gameTable); err != nil {
		return fmt.Errorf("%s: create games table: %w", op, err)
	}

	if _, err := s.db.ExecContext(ctx, userGamesTable); err != nil {
		return fmt.Errorf("%s: create user_games table: %w", op, err)
	}

	// Ensure the FULLTEXT index exists on pre-existing databases where
	// CREATE TABLE IF NOT EXISTS above is a no-op. MariaDB 10.0+ and
	// MySQL 8.0+ both support CREATE FULLTEXT INDEX IF NOT EXISTS.
	if _, err := s.db.ExecContext(ctx,
		`CREATE FULLTEXT INDEX IF NOT EXISTS ft_games_title ON games(title)`,
	); err != nil {
		return fmt.Errorf("%s: create ft index: %w", op, err)
	}

	return nil
}

func (s *Storage) MigrateRefresh(ctx context.Context) error {
	const op = "storage.mariadb.MigrateRefresh"

	if s.db == nil {
		return fmt.Errorf("%s: db is nil", op)
	}

	if _, err := s.db.ExecContext(ctx, "DELETE FROM user_games"); err != nil {
		return fmt.Errorf("%s: failed to clear user_games: %w", op, err)
	}

	if _, err := s.db.ExecContext(ctx, "DELETE FROM games"); err != nil {
		return fmt.Errorf("%s: failed to clear games: %w", op, err)
	}

	if err := s.MigrateContext(ctx); err != nil {
		return fmt.Errorf("%s: migrate failed: %w", op, err)
	}

	return nil
}

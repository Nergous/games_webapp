package mariadb

import (
	"database/sql"
	"fmt"

	"games_webapp/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

type Storage struct {
	DB *sql.DB
}

func New(cfg config.Database) (*Storage, error) {
	const op = "storage.maradb.New"

	dsn := cfg.GetDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &Storage{DB: db}, nil
}

func (s *Storage) Close() error {
	return s.DB.Close()
}

func (s *Storage) CreateGamesTable() error {
	const op = "storage.mariadb.create-games-table"

	query := `
		CREATE TABLE IF NOT EXISTS games (
			id INT AUTO_INCREMENT PRIMARY KEY,
			title VARCHAR(255) NOT NULL,
			preambula BLOB,
			image VARCHAR(500),
			developer VARCHAR(500) NOT NULL,
			publisher VARCHAR(500) NOT NULL,
			year VARCHAR(20) NOT NULL,
			genre VARCHAR(100),
			status ENUM('planned', 'completed', 'dropped'),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		);
	`

	_, err := s.DB.Exec(query)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

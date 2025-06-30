package mariadb

import (
	"fmt"

	"games_webapp/internal/config"
	"games_webapp/internal/models"

	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type Storage struct {
	DB *gorm.DB
}

func New(cfg config.Database) (*Storage, error) {
	const op = "storage.maradb.New"

	dsn := cfg.GetDSN()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &Storage{DB: db}, nil
}

func (s *Storage) Close() error {
	const op = "storage.mariadb.Close"
	db, err := s.DB.DB()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	db.Close()
	return nil
}

func (s *Storage) Migrate() error {
	const op = "storage.mariadb.Migrate"
	err := s.DB.AutoMigrate(&models.Game{})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

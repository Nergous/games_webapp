package services

import (
	"fmt"
	"log/slog"

	"games_webapp/internal/models"
	"games_webapp/internal/storage/mariadb"
)

type GameServicer interface {
	GetAll() ([]models.Game, error)
	GetByID(id int64) (*models.Game, error)
	Create(game *models.Game) (*models.Game, error)
	Update(game *models.Game) (*models.Game, error)
	Delete(id int64) error
	GetGameByURL(url string) error
}

type GameService struct {
	storage *mariadb.Storage
	log     *slog.Logger
}

func NewGameService(s *mariadb.Storage, log *slog.Logger) *GameService {
	return &GameService{
		storage: s,
		log:     log,
	}
}

func (s *GameService) GetAll() ([]models.Game, error) {
	const op = "services.games.GetAll"

	var results []models.Game
	rows := s.storage.DB.Find(&results)
	if rows.Error != nil {
		return nil, fmt.Errorf("%s: %w", op, rows.Error)
	}

	return results, nil
}

func (s *GameService) GetByID(id int64) (*models.Game, error) {
	const op = "services.games.GetByID"

	var g models.Game

	rows := s.storage.DB.First(&g, id)
	if rows.Error != nil {
		return nil, fmt.Errorf("%s: %w", op, rows.Error)
	}

	return &g, nil
}

func (s *GameService) Create(g *models.Game) (*models.Game, error) {
	const op = "services.games.Create"
	tx := s.storage.DB.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("%s: %w", op, tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(g).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("%s: %w", op, tx.Error)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("%s: %w", op, tx.Error)
	}

	return g, nil
}

func (s *GameService) Update(g *models.Game) (*models.Game, error) {
	const op = "services.games.Update"

	tx := s.storage.DB.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("%s: %w", op, tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var existing models.Game
	if err := tx.First(&existing, g.ID).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if err := tx.Model(&models.Game{}).Where("id = ?", g.ID).Updates(g).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return g, nil
}

func (s *GameService) Delete(id int64) error {
	const op = "services.games.Delete"

	tx := s.storage.DB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("%s: %w", op, tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Delete(&models.Game{}, id).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("%s: %w", op, tx.Error)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("%s: %w", op, tx.Error)
	}

	return nil
}

func (s *GameService) GetGameByURL(url string) error {
	const op = "services.games.GetGameByURL"

	if url == "" {
		return fmt.Errorf("%s: url is empty", op)
	}

	rows := s.storage.DB.Where("url = ?", url).First(&models.Game{})
	if rows.Error != nil {
		return fmt.Errorf("%s: %w", op, rows.Error)
	}

	return nil
}

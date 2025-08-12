package services

import (
	"errors"
	"fmt"
	"log/slog"

	"games_webapp/internal/models"
	"games_webapp/internal/storage/mariadb"

	"gorm.io/gorm"
)

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

func (s *GameService) GetAllPaginatedForUser(userID int64, page, pageSize int) ([]models.UserGameResponse, int, error) {
	const op = "services.games.GetAllPaginatedForUser"

	var results []models.UserGameResponse
	var count int64

	offset := (page - 1) * pageSize

	if err := s.storage.DB.
		Model(&models.UserGames{}).
		Where("user_id = ?", userID).
		Count(&count).Error; err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}

	if err := s.storage.DB.
		Table("games").
		Select("games.*, user_games.priority, user_games.status").
		Joins("JOIN user_games ON user_games.game_id = games.id").
		Where("user_games.user_id = ?", userID).
		Offset(offset).
		Limit(pageSize).
		Find(&results).Error; err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}

	return results, int(count), nil
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

func (s *GameService) SearchAllGames(query string) ([]models.Game, error) {
	const op = "services.games.SearchAllGames"

	var results []models.Game
	rows := s.storage.DB.Where("title LIKE ?", "%"+query+"%").Find(&results)
	if rows.Error != nil {
		return nil, fmt.Errorf("%s: %w", op, rows.Error)
	}

	return results, nil
}

func (s *GameService) SearchUserGames(userID int64, query string) ([]models.Game, error) {
	const op = "services.games.SearchUserGames"

	var results []models.Game
	rows := s.storage.DB.
		Model(&models.Game{}).
		Joins("JOIN user_games ON user_games.game_id = games.id").
		Where("user_games.user_id = ?", userID).
		Where("games.title LIKE ?", "%"+query+"%").
		Find(&results)
	if rows.Error != nil {
		return nil, fmt.Errorf("%s: %w", op, rows.Error)
	}

	return results, nil
}

func (s *GameService) Create(g *models.Game) (*models.Game, error) {
	const op = "services.games.Create"

	err := s.GetGameByURL(g.URL)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

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

	fmt.Printf("url: %s \n op: %s\n", url, op)

	if url == "" {
		return fmt.Errorf("%s: url is empty", op)
	}

	rows := s.storage.DB.Where("url = ?", url).First(&models.Game{})
	fmt.Println(rows)
	if rows.Error != nil && !errors.Is(rows.Error, gorm.ErrRecordNotFound) {
		return nil
	}
	if rows.Error == nil {
		return fmt.Errorf("%s: %w", op, errors.New("game already exists"))
	}

	return nil
}

func (s *GameService) CreateUserGame(ug *models.UserGames) error {
	const op = "services.games.CreateUserGame"

	var existing models.UserGames
	err := s.storage.DB.Where(
		"user_id = ? AND game_id = ?",
		ug.UserID,
		ug.GameID,
	).First(&existing).Error
	if err == nil {
		return nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.storage.DB.Create(ug).Error; err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *GameService) UpdateUserGame(ug *models.UserGames) error {
	const op = "services.games.UpdateUserGame"

	var existing models.UserGames
	if err := s.storage.DB.Where("user_id = ? AND game_id = ?", ug.UserID, ug.GameID).First(&existing).Error; err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	existing.Priority = ug.Priority
	existing.Status = ug.Status

	if err := s.storage.DB.Save(&existing).Error; err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *GameService) DeleteUserGame(userID, gameID int64) error {
	const op = "services.games.DeleteUserGame"

	if err := s.storage.DB.Where("user_id = ? AND game_id = ?", userID, gameID).Delete(&models.UserGames{}).Error; err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *GameService) GetFinishedGames(userID int64) (int, error) {
	const op = "services.games.GetFinishedGames"

	var count int64
	if err := s.storage.DB.
		Model(&models.UserGames{}).
		Where("user_id = ?", userID).
		Where("status = ?", "finished").
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return int(count), nil
}

func (s *GameService) GetPlayingGames(userID int64) (int, error) {
	const op = "services.games.GetPlayingGames"

	var count int64
	if err := s.storage.DB.
		Model(&models.UserGames{}).
		Where("user_id = ?", userID).
		Where("status = ?", "playing").
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return int(count), nil
}

func (s *GameService) GetPlannedGames(userID int64) (int, error) {
	const op = "services.games.GetPlannedGames"

	var count int64
	if err := s.storage.DB.
		Model(&models.UserGames{}).
		Where("user_id = ?", userID).
		Where("status = ?", "planned").
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return int(count), nil
}

func (s *GameService) GetDroppedGames(userID int64) (int, error) {
	const op = "services.games.GetDroppedGames"

	var count int64
	if err := s.storage.DB.
		Model(&models.UserGames{}).
		Where("user_id = ?", userID).
		Where("status = ?", "dropped").
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return int(count), nil
}

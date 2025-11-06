package services

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

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

func (s *GameService) GetGamesPaginated(userID int, search, sortBy, sortOrder string, page, pageSize int) ([]models.UserGameResponse, int, error) {
	const op = "services.games.GetAllGames"

	var results []models.UserGameResponse
	var count int64

	offset := (page - 1) * pageSize

	db := s.storage.DB.Table("games").
		Select("games.*, COALESCE(user_games.priority, 0) as priority, COALESCE(user_games.status, '') as status").
		Joins("LEFT JOIN user_games ON user_games.game_id = games.id AND user_games.user_id = ?", userID)

	if search != "" {
		db = db.Where("games.title LIKE ?", "%"+search+"%")
	}

	if err := db.Count(&count).Error; err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}

	allowedSort := map[string]string{
		"title": "games.title",
		"year":  "games.year",
	}

	sortField, ok := allowedSort[sortBy]
	if !ok {
		sortField = "games.title"
	}

	if strings.ToLower(sortOrder) != "desc" {
		sortOrder = "asc"
	}

	if err := db.
		Order(fmt.Sprintf("%s %s", sortField, sortOrder)).
		Offset(offset).
		Limit(pageSize).
		Scan(&results).Error; err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}

	return results, int(count), nil
}

func (s *GameService) GetByID(id int) (*models.Game, error) {
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

func (s *GameService) GetUserGame(userID, gameID int) (*models.UserGames, error) {
	const op = "services.games.GetUserGame"

	var g models.UserGames

	rows := s.storage.DB.Where("user_id = ? AND game_id = ?", userID, gameID).First(&g)
	if rows.Error != nil {
		return nil, fmt.Errorf("%s: %w", op, rows.Error)
	}

	return &g, nil
}

func (s *GameService) GetUserGames(userID int, status *models.GameStatus, search, sortBy, sortOrder string, page, pageSize int) ([]models.UserGameResponse, int, error) {
	const op = "services.games.GetUserGames"

	var results []models.UserGameResponse
	var count int64

	offset := (page - 1) * pageSize

	db := s.storage.DB.
		Table("games").
		Select("games.*, user_games.priority, user_games.status").
		Joins("JOIN user_games ON user_games.game_id = games.id").
		Where("user_games.user_id = ?", userID)

	if status != nil {
		db = db.Where("user_games.status = ?", status)
	}

	if search != "" {
		db = db.Where("games.title LIKE ?", "%"+search+"%")
	}

	if err := db.Count(&count).Error; err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}

	allowedSort := map[string]string{
		"title":    "games.title",
		"year":     "games.year",
		"priority": "user_games.priority",
	}

	sortField, ok := allowedSort[sortBy]
	if !ok {
		sortField = "games.title"
	}

	if strings.ToLower(sortOrder) != "desc" {
		sortOrder = "asc"
	}

	if err := db.
		Order(fmt.Sprintf("%s %s", sortField, sortOrder)).
		Offset(offset).
		Limit(pageSize).
		Find(&results).Error; err != nil {
		return nil, 0, fmt.Errorf("%s: %w", op, err)
	}

	return results, int(count), nil
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

func (s *GameService) Delete(id int) error {
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
	fmt.Println("ТУТАЧКИ")
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := s.storage.DB.Create(ug).Error; err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
		fmt.Println("ВСЁ НОРМ")
		return nil

	} else if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (s *GameService) UpdateUserGame(ug *models.UserGames) error {
	const op = "services.games.UpdateUserGame"
	fmt.Println("ОБНОВЛЕНИЕ")

	var existing models.UserGames

	fmt.Printf("%v", ug)
	err := s.storage.DB.
		Table("user_games").
		Where("user_id = ? AND game_id = ?", ug.UserID, ug.GameID).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fmt.Println("СОЗДАНИЕ")
		return s.CreateUserGame(ug)
	} else if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	existing.Priority = ug.Priority
	existing.Status = ug.Status

	if err := s.storage.DB.Table("user_games").Save(&existing).Error; err != nil {
		fmt.Println("НУ Я ТУТ")
		return fmt.Errorf("%s: %w", op, err)
	}
	fmt.Printf("%v", existing)
	fmt.Println("ВСЁ ЧЕТЕНЬКО")
	return nil
}

func (s *GameService) DeleteUserGame(userID, gameID int) error {
	const op = "services.games.DeleteUserGame"

	if err := s.storage.DB.Where("user_id = ? AND game_id = ?", userID, gameID).Delete(&models.UserGames{}).Error; err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *GameService) GetFinishedGames(userID int) (int, error) {
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

func (s *GameService) GetPlayingGames(userID int) (int, error) {
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

func (s *GameService) GetPlannedGames(userID int) (int, error) {
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

func (s *GameService) GetDroppedGames(userID int) (int, error) {
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

func (s *GameService) GetFlex(
	userID int,
	fields []string,
	where []models.WhereQuery,
	order []models.Sort,
	limit int,
	offset int,
) ([]models.UserGameResponse, error) {
	const op = "services.games.GetFlex"

	db := s.storage.DB.Model(&models.Game{})
	if userID != 0 {
		if userID <= 0 {
			return nil, fmt.Errorf("%s: userID is required", op)
		}

		db = db.Select("games.*, user_games.priority, user_games.status").
			Joins("JOIN user_games ON user_games.game_id = games.id and user_games.user_id = ?", userID)
	}

	if len(fields) > 0 {
		if userID != 0 {
			db = db.Select(append(fields, "user_games.priority", "user_games.status"))
		} else {
			db = db.Select(fields)
		}
	}

	for _, wq := range where {
		if wq.Field == "" {
			continue
		}

		condition := map[string]string{
			"gt":  ">",
			"lt":  "<",
			"gte": ">=",
			"lte": "<=",
			"eq":  "=",
			"neq": "!=",
		}[strings.ToLower(wq.Condition)]

		if condition == "" {
			continue
		}

		db = db.Where(fmt.Sprintf("%s %s ?", wq.Field, condition), wq.Value)
	}

	for _, s := range order {
		if s.Field == "" {
			continue
		}

		dir := "ASC"

		if strings.ToLower(s.Direction) == "desc" {
			dir = "DESC"
		}

		db = db.Order(fmt.Sprintf("%s %s", s.Field, dir))
	}

	if limit > 0 {
		db = db.Limit(int(limit))
	}

	if offset > 0 {
		db = db.Offset(int(offset))
	}

	var res []models.UserGameResponse
	if err := db.Scan(&res).Error; err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return res, nil
}

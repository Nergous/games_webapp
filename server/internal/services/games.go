package services

import (
	"context"
	"fmt"
	"log/slog"

	"games_webapp/internal/models"
	"games_webapp/internal/storage"
	"games_webapp/internal/storage/mariadb"
)

type GameService struct {
	storage *mariadb.Storage
}

func NewGameService(s *mariadb.Storage, log *slog.Logger) *GameService {
	return &GameService{
		storage: s,
	}
}

func (s *GameService) GetAll() ([]models.Games, error) {
	// 	rows, err := s.storage.DB.Query(`SELECT * FROM games`)
	// 	if err != nil {
	// 		return nil, fmt.Errorf("error occured: %s", err.Error())
	// 	}
	// 	defer rows.Close()

	// 	var games []models.Games
	// 	var createdAtStr, updatedAtStr string

	// 	for rows.Next() {
	// 		var g models.Games
	// 		// if err := rows.Scan(&g.ID, &g.Title, &g.Image, &g.URL, &g.Status, &createdAtStr, &updatedAtStr); err != nil {
	// 			if err == sql.ErrNoRows {
	// 				return nil, storage.ErrNotFound
	// 			}
	// 			return nil, fmt.Errorf("error occured: %s", err.Error())
	// 		}
	// 		g.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr)
	// 		if err != nil {
	// 			return nil, err
	// 		}

	// 		g.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAtStr)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		games = append(games, g)
	// 	}

	return nil, nil
}

// func (s *GameService) GetByID(id int64) (models.Games, error) {
// 	var g models.Games
// 	var createdAtStr, updatedAtStr string
// 	err := s.storage.DB.QueryRow(`SELECT * FROM games WHERE id = ?`, id).Scan(
// 		&g.ID,
// 		&g.Title,
// 		&g.Image,
// 		&g.Status,
// 		&createdAtStr,
// 		&updatedAtStr,
// 	)

// 	if err != nil {
// 		if err == sql.ErrNoRows {
// 			return models.Games{}, storage.ErrNotFound
// 		}
// 		return models.Games{}, err
// 	}
// 	return g, nil

// }
func (s *GameService) Create(g *models.Games) (*models.Games, error) {
	res, err := s.storage.DB.ExecContext(
		context.Background(),
		`INSERT INTO games 
		(title, preambula, image, developer, 
		publisher, year, genre, status, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		&g.Title, &g.Preambula, &g.Image, &g.Developer,
		&g.Publisher, &g.Year, &g.Genre,
		&g.Status, &g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf(storage.ErrCreateFailed.Error()+" - %s: %s", g.Title, err.Error())
	}

	_, err = res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf(storage.ErrCreateFailed.Error()+" - %s: %s", g.Title, err.Error())
	}

	return &models.Games{}, nil
}

// func (s *GameService) CreateThroughGamesDB(gameName string) error {
// 	return nil
// }

// func (s *GameService) Update(g *models.Games) (*models.Games, error) {
// 	return &models.Games{}, nil
// }
// func (s *GameService) Delete(id int64) error {
// 	return nil
// }
// func (s *GameService) SaveImg(filename string, file io.Reader) (string, error) {
// 	return "", nil
// }

// internal/repository/repository.go
package repository

import (
	"context"
	"games_webapp/internal/models"
)

type GamesRepository interface {
	GetByID(ctx context.Context, id int) (*models.Game, error)
	GetAllPaginated(ctx context.Context, userID int, params GetAllParams) ([]models.UserGameResponse, int, error)

	GetUserGame(ctx context.Context, userID, gameID int) (*models.UserGame, error)
	GetUserGames(ctx context.Context, userID int, params GetUserGamesParams) ([]models.UserGameResponse, int, error)

	GetUserGameStatusCounts(ctx context.Context, userID int) (StatusCounts, error)

	SearchAllGames(ctx context.Context, query string) ([]models.Game, error)

	CreateWithUserGame(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error)
	AddUserGame(ctx context.Context, userGame *models.UserGame) error

	UpdateWithUserGame(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error)
	UpdateUserGameStatus(ctx context.Context, userID, gameID int, status models.GameStatus) error
	UpdateUserGamePriority(ctx context.Context, userID, gameID int, priority int) error

	Delete(ctx context.Context, id int) error
	DeleteUserGame(ctx context.Context, userID, gameID int) error
}

type GetAllParams struct {
	Search    string
	SortBy    string
	SortOrder string
	Page      int
	PageSize  int
}

type GetUserGamesParams struct {
	Status    *models.GameStatus
	Search    string
	SortBy    string
	SortOrder string
	Page      int
	PageSize  int
}

type StatusCounts struct {
	Finished int
	Playing  int
	Planned  int
	Dropped  int
}

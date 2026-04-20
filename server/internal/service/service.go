// internal/service/service.go
package service

import (
	"context"
	"games_webapp/internal/models"
	"games_webapp/internal/repository"
)

type GameService interface {
	GetByID(ctx context.Context, id int) (*models.Game, error)
	GetAll(ctx context.Context, userID int, params repository.GetAllParams) ([]models.UserGameResponse, int, error)
	SearchAllGames(ctx context.Context, query string) ([]models.Game, error)

	GetUserGame(ctx context.Context, userID, gameID int) (*models.UserGame, error)
	GetUserGames(ctx context.Context, userID int, params repository.GetUserGamesParams) ([]models.UserGameResponse, int, error)
	GetGameStats(ctx context.Context, userID int) (repository.StatusCounts, error)

	Create(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error)
	AddUserGame(ctx context.Context, userID, gameID int) error

	Update(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error)
	UpdateStatus(ctx context.Context, userID, gameID int, status models.GameStatus) error
	UpdatePriority(ctx context.Context, userID, gameID int, priority int) error

	Delete(ctx context.Context, gameID int) error
	DeleteUserGame(ctx context.Context, userID, gameID int) error
}

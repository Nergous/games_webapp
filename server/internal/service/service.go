// internal/service/service.go
package service

import (
	"context"
	"games_webapp/internal/models"
	"games_webapp/internal/repository"
)

// ImportGameRequest is one entry in a batch import: either Name or URL must be
// non-empty (URL wins when both are provided and a slug can be extracted).
type ImportGameRequest struct {
	Name string
	URL  string
}

// ImportError describes a single failed entry in an import batch. Name echoes
// whatever the caller submitted (URL preferred, else Name) so it can be
// matched against the request.
type ImportError struct {
	Name string
	Err  string
}

// ImportResult aggregates the outcome of a batch import. Success and Errors
// are disjoint: each input ends up in exactly one of them.
type ImportResult struct {
	Success []*models.Game
	Errors  []ImportError
}

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

	BatchImportFromIGDB(ctx context.Context, userID int, requests []ImportGameRequest) (*ImportResult, error)
}

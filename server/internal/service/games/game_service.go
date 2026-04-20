// internal/service/games/game_service.go
package games

import (
	"context"
	"errors"
	"strings"

	"games_webapp/internal/models"
	"games_webapp/internal/repository"

	g_errors "games_webapp/internal/errors"
)

type GameService struct {
	repo repository.GamesRepository
}

func NewGameService(r repository.GamesRepository) *GameService {
	return &GameService{
		repo: r,
	}
}

func (s *GameService) GetByID(ctx context.Context, id int) (*models.Game, error) {
	const op = "services.games.GetByID"

	if id <= 0 {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID)
	}

	result, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, g_errors.New(op, g_errors.CodeNotFound, g_errors.GameNotFound)
		}
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return result, nil
}

func (s *GameService) GetAll(ctx context.Context, userID int, params repository.GetAllParams) ([]models.UserGameResponse, int, error) {
	const op = "services.games.GetAll"

	if userID <= 0 {
		return nil, 0, g_errors.New(op, g_errors.CodeUnauthorized, g_errors.MissingUserID)
	}

	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 10
	} else if params.PageSize > 100 {
		params.PageSize = 100
	}

	params.Search = strings.TrimSpace(params.Search)
	if strings.ToLower(params.SortOrder) != "desc" {
		params.SortOrder = "asc"
	}

	results, total, err := s.repo.GetAllPaginated(ctx, userID, params)
	if err != nil {
		return nil, 0, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return results, total, nil
}

func (s *GameService) SearchAllGames(ctx context.Context, query string) ([]models.Game, error) {
	const op = "services.games.SearchAllGames"

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.EmptyQuery)
	}

	results, err := s.repo.SearchAllGames(ctx, query)
	if err != nil {
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return results, nil
}

func (s *GameService) GetUserGame(ctx context.Context, userID, gameID int) (*models.UserGame, error) {
	const op = "services.games.GetUserGame"

	if userID <= 0 {
		return nil, g_errors.New(op, g_errors.CodeUnauthorized, g_errors.MissingUserID)
	}
	if gameID <= 0 {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID)
	}

	result, err := s.repo.GetUserGame(ctx, userID, gameID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, g_errors.New(op, g_errors.CodeNotFound, g_errors.GameNotFound)
		}
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return result, nil
}

func (s *GameService) GetUserGames(ctx context.Context, userID int, params repository.GetUserGamesParams) ([]models.UserGameResponse, int, error) {
	const op = "services.games.GetUserGames"

	if userID <= 0 {
		return nil, 0, g_errors.New(op, g_errors.CodeUnauthorized, g_errors.MissingUserID)
	}

	if params.Status != nil && !params.Status.IsValid() {
		return nil, 0, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidStatus)
	}

	params.Search = strings.TrimSpace(params.Search)
	if strings.ToLower(params.SortOrder) != "desc" {
		params.SortOrder = "asc"
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 10
	} else if params.PageSize > 100 {
		params.PageSize = 100
	}

	results, total, err := s.repo.GetUserGames(ctx, userID, params)
	if err != nil {
		return nil, 0, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return results, total, nil
}

func (s *GameService) GetGameStats(ctx context.Context, userID int) (repository.StatusCounts, error) {
	const op = "services.games.GetGameStats"

	if userID <= 0 {
		return repository.StatusCounts{}, g_errors.New(op, g_errors.CodeUnauthorized, g_errors.MissingUserID)
	}

	counts, err := s.repo.GetUserGameStatusCounts(ctx, userID)
	if err != nil {
		return repository.StatusCounts{}, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return counts, nil
}

func (s *GameService) Create(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error) {
	const op = "services.Games.Create"

	if strings.TrimSpace(game.Title) == "" {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidGameTitle)
	}
	if strings.TrimSpace(game.URL) == "" {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidURL)
	}
	if userGame.Priority < 0 || userGame.Priority > 10 {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidPriority)
	}
	if userGame.Status != "" && !userGame.Status.IsValid() {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidInput)
	}
	if userGame.Status == "" {
		userGame.Status = models.StatusPlanned
	}

	result, err := s.repo.CreateWithUserGame(ctx, game, userGame)
	if err != nil {
		if errors.Is(err, repository.ErrAlreadyExists) {
			return nil, g_errors.New(op, g_errors.CodeConflict, g_errors.GameAlreadyExists)
		}
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return result, nil
}

func (s *GameService) AddUserGame(ctx context.Context, userID, gameID int) error {
	const op = "services.games.AddUserGame"

	if userID <= 0 {
		return g_errors.New(op, g_errors.CodeUnauthorized, g_errors.MissingUserID)
	}
	if gameID <= 0 {
		return g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID)
	}

	// проверяем что игра существует
	if _, err := s.repo.GetByID(ctx, gameID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return g_errors.New(op, g_errors.CodeNotFound, g_errors.GameNotFound)
		}
		return g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	userGame := &models.UserGame{
		UserID:   userID,
		GameID:   gameID,
		Priority: 0,
		Status:   models.StatusPlanned,
	}

	if err := s.repo.AddUserGame(ctx, userGame); err != nil {
		if errors.Is(err, repository.ErrAlreadyExists) {
			return g_errors.New(op, g_errors.CodeConflict, g_errors.GameAlreadyExists)
		}
		return g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return nil
}

func (s *GameService) Update(ctx context.Context, game *models.Game, userGame *models.UserGame) (*models.Game, error) {
	const op = "services.games.Update"

	if game.ID <= 0 {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID)
	}
	if strings.TrimSpace(game.Title) == "" {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidGameTitle)
	}
	if strings.TrimSpace(game.URL) == "" {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidURL)
	}
	if userGame.Priority < 0 || userGame.Priority > 10 {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidPriority)
	}
	if userGame.Status != "" && !userGame.Status.IsValid() {
		return nil, g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidStatus)
	}

	result, err := s.repo.UpdateWithUserGame(ctx, game, userGame)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, g_errors.New(op, g_errors.CodeNotFound, g_errors.GameNotFound)
		}
		if errors.Is(err, repository.ErrAlreadyExists) {
			return nil, g_errors.New(op, g_errors.CodeConflict, g_errors.GameAlreadyExists)
		}
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return result, nil
}

func (s *GameService) UpdateStatus(ctx context.Context, userID, gameID int, status models.GameStatus) error {
	const op = "services.games.UpdateStatus"

	if userID <= 0 {
		return g_errors.New(op, g_errors.CodeUnauthorized, g_errors.MissingUserID)
	}
	if gameID <= 0 {
		return g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID)
	}
	if !status.IsValid() {
		return g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidStatus)
	}

	if err := s.repo.UpdateUserGameStatus(ctx, userID, gameID, status); err != nil {
		if errors.Is(err, repository.ErrNoRowsAffected) {
			return g_errors.New(op, g_errors.CodeNotFound, g_errors.GameNotFound)
		}
		return g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return nil
}

func (s *GameService) UpdatePriority(ctx context.Context, userID, gameID, priority int) error {
	const op = "services.games.UpdatePriority"

	if userID <= 0 {
		return g_errors.New(op, g_errors.CodeUnauthorized, g_errors.MissingUserID)
	}
	if gameID <= 0 {
		return g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID)
	}
	if priority < 0 || priority > 10 {
		return g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidPriority)
	}

	if err := s.repo.UpdateUserGamePriority(ctx, userID, gameID, priority); err != nil {
		if errors.Is(err, repository.ErrNoRowsAffected) {
			return g_errors.New(op, g_errors.CodeNotFound, g_errors.GameNotFound)
		}
		return g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return nil
}

func (s *GameService) Delete(ctx context.Context, gameID int) error {
	const op = "services.games.Delete"

	if gameID <= 0 {
		return g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID)
	}

	if err := s.repo.Delete(ctx, gameID); err != nil {
		if errors.Is(err, repository.ErrNoRowsAffected) {
			return g_errors.New(op, g_errors.CodeNotFound, g_errors.GameNotFound)
		}
		return g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return nil
}

func (s *GameService) DeleteUserGame(ctx context.Context, userID, gameID int) error {
	const op = "services.games.DeleteUserGame"

	if userID <= 0 {
		return g_errors.New(op, g_errors.CodeUnauthorized, g_errors.MissingUserID)
	}
	if gameID <= 0 {
		return g_errors.New(op, g_errors.CodeInvalidInput, g_errors.InvalidGameID)
	}

	if err := s.repo.DeleteUserGame(ctx, userID, gameID); err != nil {
		if errors.Is(err, repository.ErrNoRowsAffected) {
			return g_errors.New(op, g_errors.CodeNotFound, g_errors.GameNotFound)
		}
		return g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return nil
}

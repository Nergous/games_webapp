// Package repository defines parameter/result types and sentinel errors
// shared between the persistence layer and its consumers (services). The
// repository interface itself is declared on the consumer side — see
// gamesRepo in internal/service/games/game_service.go.
package repository

import "games_webapp/internal/models"

// GetAllParams bounds and orders a global game listing.
type GetAllParams struct {
	Search    string
	SortBy    string
	SortOrder string
	Page      int
	PageSize  int
}

// GetUserGamesParams bounds and orders a per-user game listing.
type GetUserGamesParams struct {
	Status    *models.GameStatus
	Search    string
	SortBy    string
	SortOrder string
	Page      int
	PageSize  int
}

// StatusCounts summarizes how many of each GameStatus a user owns.
type StatusCounts struct {
	Finished int
	Playing  int
	Planned  int
	Dropped  int
}

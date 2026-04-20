// internal/models/user_games.go
package models

type GameStatus string

const (
	StatusPlanned  GameStatus = "planned"
	StatusPlaying  GameStatus = "playing"
	StatusFinished GameStatus = "finished"
	StatusDropped  GameStatus = "dropped"
)

func (s GameStatus) IsValid() bool {
	switch s {
	case StatusPlanned, StatusPlaying, StatusFinished, StatusDropped:
		return true
	}
	return false
}

type UserGame struct {
	ID       int        `json:"id"`
	UserID   int        `json:"user_id"`
	GameID   int        `json:"game_id"`
	Priority int        `json:"priority"`
	Status   GameStatus `json:"status"`
}

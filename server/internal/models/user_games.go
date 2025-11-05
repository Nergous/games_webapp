package models

type GameStatus string

const (
	StatusPlanned  GameStatus = "planned"
	StatusPlaying  GameStatus = "playing"
	StatusFinished GameStatus = "finished"
	StatusDropped  GameStatus = "dropped"
)

type UserGames struct {
	ID       int64      `json:"id" gorm:"primary_key"`
	UserID   int64      `json:"user_id"`
	GameID   int64      `json:"game_id"`
	Priority int        `json:"priority"`
	Status   GameStatus `json:"status" gorm:"type:varchar(20);default:'planned'"`
}

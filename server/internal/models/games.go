package models

import (
	"time"
)

type Game struct {
	ID        int64  `json:"id" gorm:"primary_key"`
	Title     string `json:"title"`
	Preambula string `json:"preambula"`
	Image     string `json:"image"`
	Developer string `json:"developer"`
	Publisher string `json:"publisher"`
	Year      string `json:"year"`
	Genre     string `json:"genre"`
	Creator   int64  `json:"creator"`

	URL       string     `json:"url"`
	CreatedAt *time.Time `json:"created_at" gorm:"type:timestamp"`
	UpdatedAt *time.Time `json:"updated_at" gorm:"type:timestamp"`
}

type UserGameResponse struct {
	Game
	Priority int        `json:"priority"`
	Status   GameStatus `json:"status"`
}

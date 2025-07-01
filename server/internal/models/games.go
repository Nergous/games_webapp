package models

import (
	"time"
)

type GameStatus string

const (
	StatusPlanned  GameStatus = "planned"
	StatusPlaying  GameStatus = "playing"
	StatusFinished GameStatus = "finished"
)

type Game struct {
	ID        int64      `json:"id" gorm:"primary_key"`
	Title     string     `json:"title"`
	Preambula string     `json:"preambula"`
	Image     string     `json:"image"`
	Developer string     `json:"developer"`
	Publisher string     `json:"publisher"`
	Year      string     `json:"year"`
	Genre     string     `json:"genre"`
	Status    GameStatus `json:"status" gorm:"type:varchar(20);default:'planned'"`
	URL       string     `json:"url"`
	Priority  int        `json:"priority"`
	CreatedAt *time.Time `json:"created_at" gorm:"type:timestamp"`
	UpdatedAt *time.Time `json:"updated_at" gorm:"type:timestamp"`
}

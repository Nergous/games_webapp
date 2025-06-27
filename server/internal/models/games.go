package models

import "time"

type Games struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Preambula string    `json:"preambula"`
	Image     string    `json:"image"`
	Developer string    `json:"developer"`
	Publisher string    `json:"publisher"`
	Year      string    `json:"year"`
	Genre     string    `json:"genre"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

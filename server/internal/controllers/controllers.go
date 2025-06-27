package controllers

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrBadRequest    = errors.New("bad request")
	ErrTooManyGames  = errors.New("too many games")
	ErrGetGames      = errors.New("failed to get games")
	ErrGameWiki      = errors.New("failed to get game wiki")
	ErrParsing       = errors.New("failed to parse document")
	ErrPartialCreate = errors.New("partial failure in create")
	ErrExists        = errors.New("already exists")
	ErrCreate        = errors.New("failed to create")
	ErrUpdate        = errors.New("failed to update")
	ErrDelete        = errors.New("failed to delete")
	ErrEncoding      = errors.New("failed to encode")
)

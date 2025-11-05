package storage

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrExists       = errors.New("already exists")
	ErrCreateFailed = errors.New("failed to create")
	ErrUpdateFailed = errors.New("failed to update")
	ErrDeleteFailed = errors.New("failed to delete")
)

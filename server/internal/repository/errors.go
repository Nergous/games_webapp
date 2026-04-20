// internal/repository/errors.go
package repository

import "errors"

// Sentinel errors returns by repository
// Service layers reads them via errors.Is and wraps into g_errors
var (
	// ErrNotFound - not found (sql.ErrNoRows or analogs)
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists - unique key conflict (MySQL 1062)
	ErrAlreadyExists = errors.New("already exists")

	// ErrNoRowsAffected - UPDATE/DELETE affected 0 rows
	ErrNoRowsAffected = errors.New("no rows affected")
)

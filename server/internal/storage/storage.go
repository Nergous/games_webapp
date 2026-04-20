// internal/storage/storage.go
package storage

import (
	"context"
	"database/sql"
)

// Storage defines a database abstraction layer.
//
// Expected:
//   - Implementations must provide connection lifecycle management.
//   - Implementations must support migrations.
//
// Behavior:
//   - Close releases underlying resources.
//   - Migrate creates required schema if not exists.
//   - MigrateRefresh resets database state for testing.
//
// Response:
//   - error is returned if underlying DB operation fails.
type Storage interface {
	Close() error
	Migrate() error
	MigrateRefresh(ctx context.Context) error
}

type DBProvider interface {
	Storage
	GetDB() *sql.DB
}

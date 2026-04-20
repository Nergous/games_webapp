// internal/storage/factory.go
package storage

import (
	"fmt"

	"games_webapp/internal/config"
	"games_webapp/internal/storage/mariadb"
	// "games_webapp/internal/storage/postgres"
)

func New(cfg config.Database) (Storage, error) {
	switch cfg.Driver {
	case "mysql":
		return mariadb.New(cfg)
	// case "postgres":
	//     return postgres.New(cfg)
	default:
		return nil, fmt.Errorf("unsupported driver: %s", cfg.Driver)
	}
}

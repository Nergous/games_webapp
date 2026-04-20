// cmd/migrate/main.go
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"games_webapp/internal/config"
	"games_webapp/internal/storage/mariadb"
)

const (
	cmdUp      = "up"
	cmdRefresh = "refresh"
)

func main() {
	configPath := flag.String("config", "", "path to config yaml file")
	cmd := flag.String("cmd", cmdUp, `migration command: "up" — apply, "refresh" — drop+apply (dev only)`)
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	if *configPath == "" {
		log.Error("--config flag is required")
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	storage, err := mariadb.New(cfg.Database)
	if err != nil {
		log.Error("failed to connect to database",
			slog.String("host", cfg.Database.Host),
			slog.Int("port", cfg.Database.Port),
			slog.String("dbname", cfg.Database.DBName),
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			log.Error("failed to close database", slog.String("error", err.Error()))
		}
	}()

	log.Info("connected to database",
		slog.String("host", cfg.Database.Host),
		slog.Int("port", cfg.Database.Port),
		slog.String("dbname", cfg.Database.DBName),
	)

	switch *cmd {
	case cmdUp:
		if err := storage.Migrate(); err != nil {
			log.Error("migration failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		log.Info("migration applied successfully")

	case cmdRefresh:
		log.Warn("running refresh — all data will be lost")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := storage.MigrateRefresh(ctx); err != nil {
			log.Error("refresh failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		log.Info("refresh applied successfully")

	default:
		log.Error("unknown command",
			slog.String("cmd", *cmd),
			slog.String("allowed", "up, refresh"),
		)
		os.Exit(1)
	}
}

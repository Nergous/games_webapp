package main

import (
	"fmt"
	"games_webapp/internal/config"
	"games_webapp/internal/routes"
	"games_webapp/internal/storage/mariadb"
	"log/slog"
	"net/http"
	"os"
)

const (
	envLocal = "local"
	envProd  = "prod"
)

func main() {
	cfg := config.MustLoad()

	fmt.Println(cfg)

	log := setupLogger(cfg.Env)

	log.Info("starting server", slog.String("env", cfg.Env))

	storage, err := mariadb.New(cfg.Database)

	if err != nil {
		log.Error("failed to create database", slog.String("error", err.Error()))
		panic("db-err")
	}

	defer storage.Close()

	err = storage.CreateGamesTable()
	if err != nil {
		log.Error("failed to create table", slog.String("error", err.Error()))
		panic("table-err")
	}

	log.Info("database init")

	r := routes.SetupRouter(log, *storage)

	log.Info("routes init")

	server := &http.Server{
		Addr:         cfg.Address,
		Handler:      r,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	log.Info("starting server", slog.String("address", cfg.Address))

	if err := server.ListenAndServe(); err != nil {
		log.Error("server failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger
	switch env {
	case envLocal:
		log = slog.New(
			slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	case envProd:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		)
	}
	return log
}

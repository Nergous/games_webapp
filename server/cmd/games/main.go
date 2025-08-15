package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"games_webapp/internal/config"
	"games_webapp/internal/middleware"
	"games_webapp/internal/routes"
	"games_webapp/internal/storage/mariadb"
	"games_webapp/internal/storage/uploads"

	_ "games_webapp/internal/controllers"

	ssogrpc "games_webapp/internal/clients/sso/grpc"
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

	ssoClient, err := ssogrpc.New(
		context.Background(),
		log,
		cfg.Clients.SSO.Address,
		cfg.Clients.SSO.Timeout,
		cfg.Clients.SSO.RetriesCount,
	)
	if err != nil {
		log.Error("failed to create sso client", slog.String("error", err.Error()))
		panic("sso-err")
	}

	authMiddleware := middleware.NewAuthMiddleware(ssoClient)

	storage, err := mariadb.New(cfg.Database)
	if err != nil {
		log.Error("failed to create database", slog.String("error", err.Error()))
		panic("db-err")
	}

	uploadsStorage, err := uploads.NewUploads(cfg.UploadsPath)
	if err != nil {
		log.Error("failed to create uploads storage", slog.String("error", err.Error()))
		panic("uploads-err")
	}

	log.Info("storage init")

	defer func() {
		if err := storage.Close(); err != nil {
			log.Error("failed to close database", slog.String("error", err.Error()))
		}
	}()

	err = storage.Migrate()
	if err != nil {
		log.Error("migration", slog.String("error", err.Error()))
		panic("table-err")
	}

	log.Info("database init")

	r := routes.SetupRouter(log, storage, uploadsStorage, authMiddleware, ssoClient, cfg.Cors)

	log.Info("routes init")

	server := &http.Server{
		Addr:         cfg.Address,
		Handler:      r,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	serverErrors := make(chan error, 1)

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info("starting server", slog.String("address", cfg.Address))
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		log.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)

	case sig := <-shutdown:

		log.Info("shutting down", slog.String("signal", sig.String()))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Error("graceful shutdown error", slog.String("error", err.Error()))
			if err := server.Close(); err != nil {
				log.Error("force shutdown error", slog.String("error", err.Error()))
			}
		}
		close(shutdown)
		close(serverErrors)
	}
	log.Info("server stopped")
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

// @title Game WebApp API
// @version 1.0
// @description API для управления библиотекой игр пользователей
// @contact.name Nergous
// @contact.email Nergous6@yandex.ru

// @host https://games.nergous.ru
// @BasePath /api

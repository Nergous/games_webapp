// cmd/games/main.go
package main

import (
	"context"
	"errors"
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

	ssogrpc "games_webapp/internal/client/sso/grpc"
)

const (
	envLocal = "local"
	envProd  = "prod"
)

func main() {
	cfg := config.MustLoad()

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
		os.Exit(1)
	}

	authMiddleware := middleware.NewAuthMiddleware(ssoClient, cfg.AppID, log)

	storage, err := mariadb.New(cfg.Database)
	if err != nil {
		log.Error("failed to create database", slog.String("error", err.Error()))
		os.Exit(1)
	}

	uploadsStorage, err := uploads.NewUploads(cfg.UploadsPath)
	if err != nil {
		log.Error("failed to create uploads storage", slog.String("error", err.Error()))
		os.Exit(1)
	}

	log.Info("storage init")

	defer func() {
		if err := storage.Close(); err != nil {
			log.Error("failed to close database", slog.String("error", err.Error()))
		}
	}()

	log.Info("database init")

	r := routes.SetupRouter(log, storage, uploadsStorage, authMiddleware, ssoClient, cfg)

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
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
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
	}
	log.Info("server stopped")
}

func setupLogger(env string) *slog.Logger {
	switch env {
	case envLocal:
		return slog.New(
			slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	case envProd:
		return slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		)
	default:
		return slog.New(
			slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
		)
	}

}

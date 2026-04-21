// cmd/games/main.go
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	igdbclient "games_webapp/internal/client/igdb"
	ssogrpc "games_webapp/internal/client/sso/grpc"
	"games_webapp/internal/config"
	authcontroller "games_webapp/internal/controller/auth"
	gamescontroller "games_webapp/internal/controller/games"
	gamesmiddleware "games_webapp/internal/middleware"
	"games_webapp/internal/routes"
	gameservice "games_webapp/internal/service/games"
	"games_webapp/internal/storage/mariadb"
	"games_webapp/internal/storage/uploads"
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
		cfg.Clients.SSO.Insecure,
	)
	if err != nil {
		log.Error("failed to create sso client", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() {
		if err := ssoClient.Close(); err != nil {
			log.Error("failed to close sso client", slog.String("error", err.Error()))
		}
	}()

	authMiddleware := gamesmiddleware.NewAuthMiddleware(ssoClient, cfg.AppID, log)

	storage, err := mariadb.New(cfg.Database)
	if err != nil {
		log.Error("failed to create database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			log.Error("failed to close database", slog.String("error", err.Error()))
		}
	}()

	uploadsStorage, err := uploads.NewUploads(cfg.Uploads.Path, cfg.Uploads.DownloadTimeout)
	if err != nil {
		log.Error("failed to create uploads storage", slog.String("error", err.Error()))
		os.Exit(1)
	}

	log.Info("storage init")

	// Composition root: wire services and controllers here so the router
	// package stays a pure routing function with no transitive imports of
	// storage/service/client packages.
	gameRepo := mariadb.NewGamesRepository(storage.GetDB())
	igdbCl := igdbclient.New(cfg.TwitchAPI, cfg.TwitchAuthAPI, cfg.TwitchClientId, cfg.TwitchClientSecret)
	gameSvc := gameservice.NewGameService(gameRepo, igdbCl, uploadsStorage)
	gameSvc.SetIGDBSettings(gameservice.IGDBSettings{
		MaxWorkers:         cfg.IGDB.MaxWorkers,
		MaxGamesPerRequest: cfg.IGDB.MaxGamesPerRequest,
		BatchTimeout:       cfg.IGDB.BatchTimeout,
	})

	handlers := routes.Handlers{
		Game:           gamescontroller.NewGameController(gameSvc, log, uploadsStorage, cfg.Uploads.MaxUploadBytes),
		IGDBGame:       gamescontroller.NewIGDBGamesController(gameSvc, log),
		Auth:           authcontroller.NewAuthController(log, ssoClient, uploadsStorage, cfg.AppID, cfg.Uploads.MaxUploadBytes),
		Tokens:         authcontroller.NewTokensController(log, ssoClient),
		UserInfo:       authcontroller.NewUserInfoController(log, ssoClient, cfg.AppID),
		AuthMiddleware: authMiddleware.ValidateToken,
		CorsOrigins:    cfg.Cors,
	}

	r := routes.SetupRouter(log, handlers)

	log.Info("routes init")

	// appCtx is cancelled on shutdown. It is threaded into every incoming
	// request via http.Server.BaseContext so long-running handlers (e.g. the
	// IGDB batch import) abort promptly when the process is stopping.
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	server := &http.Server{
		Addr:         cfg.Address,
		Handler:      r,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
		IdleTimeout:  cfg.IdleTimeout,
		BaseContext:  func(net.Listener) context.Context { return appCtx },
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

		// Must outlast the longest handler (IGDB batch is 30s) plus a small
		// grace window so in-flight work finishes rather than being truncated.
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Error("graceful shutdown error", slog.String("error", err.Error()))
			cancelApp() // abort in-flight request contexts before forcing close
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

// internal/routes/router.go
package routes

import (
	"encoding/json"
	"log/slog"
	"net/http"

	igdbclient "games_webapp/internal/client/igdb"
	"games_webapp/internal/config"
	authcontroller "games_webapp/internal/controller/auth"
	gamescontroller "games_webapp/internal/controller/games"
	gamesmiddleware "games_webapp/internal/middleware"
	"games_webapp/internal/service/games"
	"games_webapp/internal/storage"
	"games_webapp/internal/storage/mariadb"
	"games_webapp/internal/storage/uploads"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	ssogrpc "games_webapp/internal/client/sso/grpc"
)

func SetupRouter(
	log *slog.Logger,
	store storage.DBProvider,
	uploadsStorage *uploads.Uploads,
	authMiddleware *gamesmiddleware.AuthMiddleware,
	ssoClient *ssogrpc.Client,
	cfg *config.Config,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.Cors,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	gameRepo := mariadb.NewGamesRepository(store.GetDB())
	gameService := games.NewGameService(gameRepo)
	gameController := gamescontroller.NewGameController(gameService, log, uploadsStorage)
	igdbCl := igdbclient.New(cfg.TwitchAPI, cfg.TwitchAuthAPI, cfg.TwitchClientId, cfg.TwitchClientSecret)
	igdbGameController := gamescontroller.NewIGDBGamesController(gameService, log, uploadsStorage, igdbCl)

	authController := authcontroller.NewAuthController(log, ssoClient, uploadsStorage, cfg.AppID)
	tokensController := authcontroller.NewTokensController(log, ssoClient)
	userInfoController := authcontroller.NewUserInfoController(log, ssoClient, cfg.AppID)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", healthHandler)

		// Auth (public)
		r.Post("/register", authController.Register)
		r.Post("/login", authController.Login)
		r.Post("/logout", authController.Logout)
		r.Post("/refresh", tokensController.Refresh)

		// Protected routes

		r.Group(func(r chi.Router) {
			r.Use(authMiddleware.ValidateToken)

			// User profile
			r.Get("/me", userInfoController.GetUserInfo)

			// Admin: user management
			r.Route("/users", func(r chi.Router) {
				r.Get("/", userInfoController.GetUsers)
				r.Put("/{id}", userInfoController.UpdateUser)
				r.Delete("/{id}", userInfoController.DeleteUser)
			})

			// Games
			r.Route("/games", func(r chi.Router) {
				r.Get("/", gameController.GetAll)
				r.Get("/search", gameController.SearchAllGames)
				r.Post("/", gameController.Create)
				r.Post("/twitch", igdbGameController.CreateMultiGamesIGDB)

				// Current user's game list
				r.Get("/user", gameController.GetUserGames)
				r.Get("/user/stats", gameController.GetGameStats)

				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", gameController.GetByID)
					r.Get("/user-game", gameController.GetUserGame)
					r.Post("/add", gameController.AddUserGame)
					r.Put("/", gameController.Update)
					r.Put("/status", gameController.UpdateStatus)
					r.Put("/priority", gameController.UpdatePriority)
					r.Delete("/", gameController.Delete)
					r.Delete("/user-game", gameController.DeleteUserGame)
				})

			})
		})
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

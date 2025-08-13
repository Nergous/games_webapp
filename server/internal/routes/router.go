package routes

import (
	"log/slog"

	"games_webapp/internal/controllers"
	games_middleware "games_webapp/internal/middleware"
	"games_webapp/internal/services"
	"games_webapp/internal/storage/mariadb"
	"games_webapp/internal/storage/uploads"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	_ "games_webapp/docs"

	httpSwagger "github.com/swaggo/http-swagger"

	ssogrpc "games_webapp/internal/clients/sso/grpc"
)

func SetupRouter(
	log *slog.Logger,
	storage *mariadb.Storage,
	uploads *uploads.Uploads,
	authMiddleware *games_middleware.AuthMiddleware,
	ssoClient *ssogrpc.Client,
) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	gameService := services.NewGameService(storage, log)
	gameController := controllers.NewGameController(gameService, log, uploads)

	authController := controllers.NewAuthController(log, ssoClient, uploads)

	r.Route("/api", func(r chi.Router) {
		r.Post("/register", authController.Register)
		r.Post("/login", authController.Login)
		r.Route("/games", func(r chi.Router) {
			r.Get("/", gameController.GetAll)
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.ValidateToken)
				r.Get("/user/info", authController.GetUserInfo)
				r.Get("/user/stats", gameController.GetGameStats)
				r.Get("/user", gameController.GetUserGames)
				r.Get("/search", gameController.SearchAllGames)
				r.Post("/", gameController.Create)
				r.Post("/multi", gameController.CreateMultiGamesDB)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", gameController.GetByID)
					r.Put("/", gameController.Update)
					r.Put("/status", gameController.UpdateStatus)
					r.Put("/priority", gameController.UpdatePriority)
					r.Delete("/", gameController.Delete)
				})
			})
		})
	})

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("http://localhost:8082/swagger/doc.json"),
	))
	return r
}

package routes

import (
	"log/slog"

	"games_webapp/internal/controllers"
	"games_webapp/internal/services"
	"games_webapp/internal/storage/mariadb"
	"games_webapp/internal/storage/uploads"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func SetupRouter(log *slog.Logger, storage *mariadb.Storage, uploads *uploads.Uploads) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)

	gameService := services.NewGameService(storage, log)
	gameController := controllers.NewGameController(gameService, log, uploads)

	r.Route("/api/games", func(r chi.Router) {
		r.Get("/", gameController.GetAll)
		r.Post("/", gameController.Create)
		r.Post("/multi", gameController.CreateMultiGamesDB)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", gameController.GetByID)
			r.Put("/", gameController.Update)
			r.Delete("/", gameController.Delete)
		})
	})

	return r
}

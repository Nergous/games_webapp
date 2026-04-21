// internal/routes/router.go
package routes

import (
	"encoding/json"
	"log/slog"
	"net/http"

	authcontroller "games_webapp/internal/controller/auth"
	gamescontroller "games_webapp/internal/controller/games"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Handlers bundles the already-constructed HTTP controllers and the auth
// middleware. The composition root (cmd/games) owns construction; the router
// owns only URL-to-handler mapping. This keeps SetupRouter a pure routing
// function and removes its transitive dependency on the storage, IGDB client,
// uploads, SSO, and service packages.
type Handlers struct {
	Game     *gamescontroller.GameController
	IGDBGame *gamescontroller.IGDBGamesController
	Auth     *authcontroller.AuthController
	Tokens   *authcontroller.TokensController
	UserInfo *authcontroller.UserInfoController

	// AuthMiddleware is applied to the protected route group.
	AuthMiddleware func(http.Handler) http.Handler

	// CorsOrigins is passed through to the chi cors middleware.
	CorsOrigins []string
}

func SetupRouter(log *slog.Logger, h Handlers) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   h.CorsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", healthHandler)

		// Auth (public)
		r.Post("/register", h.Auth.Register)
		r.Post("/login", h.Auth.Login)
		r.Post("/logout", h.Auth.Logout)
		r.Post("/refresh", h.Tokens.Refresh)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(h.AuthMiddleware)

			// User profile
			r.Get("/me", h.UserInfo.GetUserInfo)

			// Admin: user management
			r.Route("/users", func(r chi.Router) {
				r.Get("/", h.UserInfo.GetUsers)
				r.Put("/{id}", h.UserInfo.UpdateUser)
				r.Delete("/{id}", h.UserInfo.DeleteUser)
			})

			// Games
			r.Route("/games", func(r chi.Router) {
				r.Get("/", h.Game.GetAll)
				r.Get("/search", h.Game.SearchAllGames)
				r.Post("/", h.Game.Create)
				r.Post("/twitch", h.IGDBGame.CreateMultiGamesIGDB)

				// Current user's game list
				r.Get("/user", h.Game.GetUserGames)
				r.Get("/user/stats", h.Game.GetGameStats)

				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", h.Game.GetByID)
					r.Get("/user-game", h.Game.GetUserGame)
					r.Post("/add", h.Game.AddUserGame)
					r.Put("/", h.Game.Update)
					r.Put("/status", h.Game.UpdateStatus)
					r.Put("/priority", h.Game.UpdatePriority)
					r.Delete("/", h.Game.Delete)
					r.Delete("/user-game", h.Game.DeleteUserGame)
				})
			})
		})
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// Health check body is fire-and-forget; an encode failure after headers
	// are flushed cannot be signaled to the client.
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

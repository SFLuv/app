package router

import (
	"net/http"

	"github.com/SFLuv/app/backend/handlers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	m "github.com/SFLuv/app/backend/utils/middleware"
)

func New(s *handlers.BotService, a *handlers.AccountService, p *handlers.AppService) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(m.AuthMiddleware)

	AddBotRoutes(r, s)
	AddUserRoutes(r, p)
	AddAdminRoutes(r, p)
	AddWalletRoutes(r, p)
	AddLocationRoutes(r, p)

	r.Post("/account", a.AddAccount)
	r.Get("/account", a.GetAccount)

	return r
}

func AddBotRoutes(r *chi.Mux, s *handlers.BotService) {
	r.Post("/events", s.NewEvent)
	r.Post("/events/{event_id}/codes", s.NewCodesRequest)
	r.Get("/events", s.GetCodesRequest)

	r.Post("/redeem", s.Redeem)
}

func AddUserRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/users", withAuth(s.AddUser))
	r.Get("/users", withAuth(s.GetUserAuthed))
	r.Put("/users", withAuth(s.UpdateUserInfo))
}

func AddAdminRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Get("/admin/users", withAuth(s.GetUsers))
	r.Put("/admin/users", withAuth(s.UpdateUserRole))
}

func AddWalletRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Get("/wallets", withAuth(s.GetWalletsByUser))
	r.Post("/wallets", withAuth(s.AddWallet))
}

func AddLocationRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/locations", s.AddLocation)
	r.Get("/locations/{id}", s.GetLocation)
	r.Get("/locations", s.GetLocations)
	r.Get("/locations/user", s.GetLocationsByUser)
	r.Put("/locations/{id}", s.UpdateLocation)
}

func withAuth(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := r.Context().Value("userDid").(string)
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		handlerFunc(w, r)
	}
}

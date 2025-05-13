package router

import (
	"github.com/faucet-portal/backend/handlers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func New(s *handlers.BotService, a *handlers.AccountService) *chi.Mux {
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

	r.Post("/events", s.NewEvent)
	r.Get("/events", s.GetCodes)

	r.Post("/admins", s.NewAdmin)

	r.Post("/redeem", s.Redeem)

	r.Post("/account", a.AddAccount)
	r.Get("/account", a.GetAccount)

	return r
}

package router

import (
	"github.com/faucet-portal/backend/handlers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func New(s *handlers.BotService) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)

	r.Post("/events", s.NewEvent)
	r.Get("/events", s.GetCodes)
	r.Post("/redeem", s.Redeem)

	return r
}

package router

import (
	"net/http"

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

	AddBotRoutes(r, s)

	r.Post("/account", a.AddAccount)
	r.Get("/account", a.GetAccount)

	return r
}

func AddBotRoutes(r *chi.Mux, s *handlers.BotService) {
	r.Post("/events", s.NewEvent)
	r.Post("/events/{event_id}/codes", s.NewCodesRequest)
	r.Get("/events", s.GetCodesRequest)

	r.Post("/redeem", s.Redeem)

	// TODO SANCHEZ: add route for merchants
}

func AddMerchantRoutes(r *chi.Mux, s *handlers.MerchantService) {
	r.Post("/merchants", s.AddMerchant)
	r.Get("/merchants/{name}", s.GetMerchant)
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

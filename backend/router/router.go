package router

import (
	"net/http"
	"os"

	"github.com/SFLuv/app/backend/handlers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	m "github.com/SFLuv/app/backend/utils/middleware"
)

func New(s *handlers.BotService, p *handlers.AppService) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Access-Token", "X-Admin-Key"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(m.AuthMiddleware)

	AddBotRoutes(r, s, p)
	AddUserRoutes(r, p)
	AddAdminRoutes(r, p)
	AddWalletRoutes(r, p)
	AddLocationRoutes(r, p)
	AddContactRoutes(r, p)
	AddPonderRoutes(r, p)

	return r
}

func AddBotRoutes(r *chi.Mux, s *handlers.BotService, a *handlers.AppService) {
	r.Post("/events", withAdmin(s.NewEvent, a))
	r.Post("/events/{event_id}/codes", withAdmin(s.NewCodesRequest, a))
	r.Get("/events/{event}", withAdmin(s.GetCodesRequest, a))
	r.Delete("/events/{event}", withAdmin(s.DeleteEvent, a))
	r.Get("/events", withAdmin(s.GetEvents, a))
	r.Post("/redeem", s.Redeem)
}

func AddUserRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/users", withAuth(s.AddUser))
	r.Get("/users", withAuth(s.GetUserAuthed))
	r.Put("/users", withAuth(s.UpdateUserInfo))
}

func AddAdminRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Get("/admin/users", withAuth(s.GetUsers))
	r.Get("/admin/locations", withAdmin(s.GetAuthedLocations, s))
	r.Put("/admin/users", withAuth(s.UpdateUserRole))
	r.Put("/admin/locations", withAdmin(s.UpdateLocationApproval, s))
}

func AddWalletRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Get("/wallets", withAuth(s.GetWalletsByUser))
	r.Post("/wallets", withAuth(s.AddWallet))
	r.Put("/wallets", withAuth(s.UpdateWallet))
}

func AddLocationRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/locations", withAuth(s.AddLocation))
	r.Get("/locations/{id}", s.GetLocation)
	r.Get("/locations", s.GetLocations)
	r.Get("/locations/user", withAuth(s.GetLocationsByUser))
	r.Put("/locations", withAuth(s.UpdateLocation))
}

func AddContactRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/contacts", withAuth(s.NewContact))
	r.Get("/contacts", withAuth(s.GetContacts))
	r.Put("/contacts", withAuth(s.UpdateContact))
	r.Delete("/contacts", withAuth(s.DeleteContact))
}

func AddPonderRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/ponder", withAuth(s.AddPonderMerchantSubscription))
	r.Get("/ponder", withAuth(s.GetPonderSubscriptions))
	r.Delete("/ponder", withAuth(s.DeletePonderMerchantSubscription))
	r.Get("/ponder/callback", s.PonderPingCallback)
	r.Post("/ponder/callback", s.PonderHookHandler)
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

func withAdmin(handlerFunc http.HandlerFunc, s *handlers.AppService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqKey := r.Header.Get("X-Admin-Key")
		envKey := os.Getenv("ADMIN_KEY")
		if reqKey == envKey && envKey != "" {
			handlerFunc(w, r)
			return
		}

		id, ok := r.Context().Value("userDid").(string)
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		isAdmin := s.IsAdmin(r.Context(), id)
		if !isAdmin {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		handlerFunc(w, r)
	}
}

package router

import (
	"context"
	"net/http"
	"os"

	"github.com/SFLuv/app/backend/handlers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	m "github.com/SFLuv/app/backend/utils/middleware"
)

func New(s *handlers.BotService, a *handlers.AppService, p *handlers.PonderService) *chi.Mux {
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

	AddBotRoutes(r, s, a)
	AddUserRoutes(r, a)
	AddAdminRoutes(r, a)
	AddAffiliateRoutes(r, s, a)
	AddWorkflowRoutes(r, s, a)
	AddWalletRoutes(r, a)
	AddLocationRoutes(r, a)
	AddContactRoutes(r, a)
	AddPonderRoutes(r, a, p)
	AddW9Routes(r, a)

	return r
}

func AddBotRoutes(r *chi.Mux, s *handlers.BotService, a *handlers.AppService) {
	r.Post("/events", withAdmin(s.NewEvent, a))
	r.Post("/events/{event_id}/codes", withAdmin(s.NewCodesRequest, a))
	r.Get("/events/{event}", withAdmin(s.GetCodesRequest, a))
	r.Delete("/events/{event}", withAdmin(s.DeleteEvent, a))
	r.Get("/events", withAdmin(s.GetEvents, a))
	r.Post("/redeem", s.Redeem)
	r.Post("/drain", withAdmin(s.Drain, a))
	r.Get("/balance", withAdmin(s.RemainingBalance, a))
}

func AddUserRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/users", withAuth(s.AddUser))
	r.Get("/users", withAuth(s.GetUserAuthed))
	r.Put("/users", withAuth(s.UpdateUserInfo))
	r.Put("/paypaleth", withAuth(s.UpdateUserPayPalEth))
}

func AddAdminRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Get("/admin/users", withAuth(s.GetUsers))
	r.Get("/admin/locations", withAdmin(s.GetAuthedLocations, s))
	r.Put("/admin/users", withAuth(s.UpdateUserRole))
	r.Put("/admin/locations", withAdmin(s.UpdateLocationApproval, s))
	r.Get("/admin/affiliates", withAdmin(s.GetAffiliates, s))
	r.Put("/admin/affiliates", withAdmin(s.UpdateAffiliate, s))
}

func AddAffiliateRoutes(r *chi.Mux, s *handlers.BotService, a *handlers.AppService) {
	r.Post("/affiliates/request", withAuth(a.RequestAffiliateStatus))
	r.Put("/affiliates/logo", withAffiliate(a.UpdateAffiliateLogo, a))
	r.Get("/affiliates/balance", withAffiliate(s.AffiliateBalance, a))
	r.Post("/affiliates/events", withAffiliate(s.AffiliateNewEvent, a))
	r.Get("/affiliates/events", withAffiliate(s.AffiliateGetEvents, a))
	r.Get("/affiliates/events/{event}", withAffiliate(s.AffiliateGetCodes, a))
	r.Delete("/affiliates/events/{event}", withAffiliate(s.AffiliateDeleteEvent, a))
	r.Get("/affiliates/{user_id}", withAffiliate(a.GetAffiliate, a))
}

func AddWorkflowRoutes(r *chi.Mux, s *handlers.BotService, a *handlers.AppService) {
	r.Post("/proposers/request", withAuth(a.RequestProposerStatus))
	r.Post("/improvers/request", withAuth(a.RequestImproverStatus))

	r.Get("/proposers/workflow-templates", withProposer(a.GetProposerWorkflowTemplates, a))
	r.Post("/proposers/workflow-templates", withProposer(a.CreateProposerWorkflowTemplate, a))
	r.Post("/proposers/workflows", withProposer(a.CreateWorkflow, a))
	r.Get("/proposers/workflows", withProposer(a.GetProposerWorkflows, a))
	r.Get("/proposers/workflows/{workflow_id}", withProposer(a.GetProposerWorkflow, a))
	r.Delete("/proposers/workflows/{workflow_id}", withProposer(a.DeleteProposerWorkflow, a))
	r.Post("/proposers/workflow-deletion-proposals", withProposer(a.ProposeWorkflowDeletion, a))

	r.Get("/improvers/workflows", withImprover(a.GetImproverWorkflows, a))
	r.Post("/improvers/workflows/{workflow_id}/steps/{step_id}/claim", withImprover(a.ClaimWorkflowStep, a))
	r.Post("/improvers/workflows/{workflow_id}/steps/{step_id}/start", withImprover(a.StartWorkflowStep, a))
	r.Post("/improvers/workflows/{workflow_id}/steps/{step_id}/complete", withImprover(a.CompleteWorkflowStep, a))

	r.Get("/admin/proposers", withAdmin(a.GetProposers, a))
	r.Put("/admin/proposers", withAdmin(a.UpdateProposer, a))
	r.Get("/admin/improvers", withAdmin(a.GetImprovers, a))
	r.Put("/admin/improvers", withAdmin(a.UpdateImprover, a))
	r.Get("/admin/issuers", withAdmin(a.GetIssuers, a))
	r.Put("/admin/issuers", withAdmin(a.UpdateIssuerScopes, a))
	r.Post("/admin/workflow-templates/default", withAdmin(a.CreateDefaultWorkflowTemplate, a))
	r.Post("/admin/workflows/{workflow_id}/force-approve", withAdmin(a.AdminForceApproveWorkflow, a))

	r.Get("/voters/workflows", withVoter(a.GetVoterWorkflows, a))
	r.Get("/voters/workflow-deletion-proposals", withVoter(a.GetVoterWorkflowDeletionProposals, a))
	r.Get("/workflows/active", withAuth(a.GetActiveWorkflows))
	r.Get("/workflows/{workflow_id}", withAuth(a.GetWorkflow))
	r.Post("/workflows/{workflow_id}/votes", withVoter(a.VoteWorkflow, a))
	r.Post("/workflow-deletion-proposals/{proposal_id}/votes", withVoter(a.VoteWorkflowDeletionProposal, a))

	r.Get("/issuers/scopes", withIssuer(a.GetMyIssuerScopes, a))
	r.Post("/issuers/credentials", withIssuer(a.IssueCredential, a))
	r.Delete("/issuers/credentials", withIssuer(a.RevokeCredential, a))
	r.Get("/issuers/credentials/{user_id}", withIssuer(a.GetIssuerUserCredentials, a))
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

func AddPonderRoutes(r *chi.Mux, s *handlers.AppService, p *handlers.PonderService) {
	r.Post("/ponder", withAuth(s.AddPonderMerchantSubscription))
	r.Get("/ponder", withAuth(s.GetPonderSubscriptions))
	r.Delete("/ponder", withAuth(s.DeletePonderMerchantSubscription))
	r.Get("/ponder/callback", s.PonderPingCallback)
	r.Post("/ponder/callback", s.PonderHookHandler)
	r.Get("/transactions", p.GetTransactionHistory)
	r.Get("/transactions/balance", withAuth(p.GetBalanceAtTimestamp))
}

func AddW9Routes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/w9/submit", s.SubmitW9)
	r.Post("/w9/webhook", s.SubmitW9Webhook)
	r.Post("/w9/transaction", withAdmin(s.RecordW9Transaction, s))
	r.Post("/w9/check", withAdmin(s.CheckW9Compliance, s))
	r.Get("/admin/w9/pending", withAdmin(s.GetPendingW9Submissions, s))
	r.Put("/admin/w9/approve", withAdmin(s.ApproveW9Submission, s))
	r.Put("/admin/w9/reject", withAdmin(s.RejectW9Submission, s))
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
			if _, ok := r.Context().Value("userDid").(string); !ok {
				adminId := s.GetFirstAdminId(r.Context())
				if adminId != "" {
					ctx := context.WithValue(r.Context(), "userDid", adminId)
					r = r.WithContext(ctx)
				}
			}
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

func withAffiliate(handlerFunc http.HandlerFunc, s *handlers.AppService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := r.Context().Value("userDid").(string)
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		if s.IsAdmin(r.Context(), id) {
			handlerFunc(w, r)
			return
		}

		isAffiliate := s.IsAffiliate(r.Context(), id)
		if !isAffiliate {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		handlerFunc(w, r)
	}
}

func withProposer(handlerFunc http.HandlerFunc, s *handlers.AppService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := r.Context().Value("userDid").(string)
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		if s.IsAdmin(r.Context(), id) {
			handlerFunc(w, r)
			return
		}

		isProposer := s.IsProposer(r.Context(), id)
		if !isProposer {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		handlerFunc(w, r)
	}
}

func withImprover(handlerFunc http.HandlerFunc, s *handlers.AppService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := r.Context().Value("userDid").(string)
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		if s.IsAdmin(r.Context(), id) {
			handlerFunc(w, r)
			return
		}

		isImprover := s.IsImprover(r.Context(), id)
		if !isImprover {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		handlerFunc(w, r)
	}
}

func withVoter(handlerFunc http.HandlerFunc, s *handlers.AppService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := r.Context().Value("userDid").(string)
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		if s.IsAdmin(r.Context(), id) {
			handlerFunc(w, r)
			return
		}

		isVoter := s.IsVoter(r.Context(), id)
		if !isVoter {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		handlerFunc(w, r)
	}
}

func withIssuer(handlerFunc http.HandlerFunc, s *handlers.AppService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := r.Context().Value("userDid").(string)
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		if s.IsAdmin(r.Context(), id) {
			handlerFunc(w, r)
			return
		}

		isIssuer := s.IsIssuer(r.Context(), id)
		if !isIssuer {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		handlerFunc(w, r)
	}
}

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
	AddUnwrapRoutes(r, a)

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
	r.Put("/users/primary-wallet", withAuth(s.UpdateUserPrimaryWallet))
	r.Put("/paypaleth", withAuth(s.UpdateUserPayPalEth))
	r.Get("/users/verified-emails", withAuth(s.GetUserVerifiedEmails))
	r.Post("/users/verified-emails", withAuth(s.RequestUserEmailVerification))
	r.Post("/users/verified-emails/{email_id}/resend", withAuth(s.ResendUserEmailVerification))
	r.Post("/users/verified-emails/verify", s.VerifyUserEmailToken)
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
	r.Post("/issuers/request", withAuth(a.RequestIssuerStatus))
	r.Post("/supervisors/request", withAuth(a.RequestSupervisorStatus))
	r.Get("/supervisors/approved", withAuth(a.GetApprovedSupervisors))
	r.Get("/credentials/types", withAuth(a.GetCredentialTypes))
	r.Get("/issuers/users/by-address/{address}", withIssuer(a.GetUserByAddress, a))

	r.Get("/proposers/workflow-templates", withProposer(a.GetProposerWorkflowTemplates, a))
	r.Post("/proposers/workflow-templates", withProposer(a.CreateProposerWorkflowTemplate, a))
	r.Delete("/proposers/workflow-templates/{template_id}", withProposer(a.DeleteProposerWorkflowTemplate, a))
	r.Post("/proposers/workflows", withProposer(a.CreateWorkflow, a))
	r.Get("/proposers/workflows", withProposer(a.GetProposerWorkflows, a))
	r.Get("/proposers/workflow-deletion-proposals", withProposer(a.GetProposerWorkflowDeletionProposals, a))
	r.Get("/proposers/workflows/{workflow_id}", withProposer(a.GetProposerWorkflow, a))
	r.Post("/proposers/workflows/{workflow_id}/edit-proposals", withProposer(a.ProposeWorkflowEdit, a))
	r.Delete("/proposers/workflows/{workflow_id}", withProposer(a.DeleteProposerWorkflow, a))
	r.Post("/proposers/workflow-deletion-proposals", withProposer(a.ProposeWorkflowDeletion, a))

	r.Get("/improvers/workflows", withImprover(a.GetImproverWorkflows, a))
	r.Get("/improvers/unpaid-workflows", withImprover(a.GetImproverUnpaidWorkflows, a))
	r.Put("/improvers/primary-rewards-account", withImprover(a.UpdateImproverPrimaryRewardsAccount, a))
	r.Get("/improvers/credential-requests", withImprover(a.GetImproverCredentialRequests, a))
	r.Post("/improvers/credential-requests", withImprover(a.CreateImproverCredentialRequest, a))
	r.Get("/improvers/workflows/absence-periods", withImprover(a.GetImproverAbsencePeriods, a))
	r.Post("/improvers/workflows/absence-periods", withImprover(a.CreateImproverAbsencePeriod, a))
	r.Put("/improvers/workflows/absence-periods/{absence_id}", withImprover(a.UpdateImproverAbsencePeriod, a))
	r.Delete("/improvers/workflows/absence-periods/{absence_id}", withImprover(a.DeleteImproverAbsencePeriod, a))
	r.Post("/improvers/workflow-series/unclaim", withImprover(a.UnclaimImproverWorkflowSeries, a))
	r.Post("/improvers/workflows/{workflow_id}/steps/{step_id}/claim", withImprover(a.ClaimWorkflowStep, a))
	r.Post("/improvers/workflows/{workflow_id}/steps/{step_id}/start", withImprover(a.StartWorkflowStep, a))
	r.Post("/improvers/workflows/{workflow_id}/steps/{step_id}/photos", withImprover(a.UploadWorkflowStepPhoto, a))
	r.Post("/improvers/workflows/{workflow_id}/steps/{step_id}/complete", withImprover(a.CompleteWorkflowStep, a))
	r.Post("/improvers/workflows/{workflow_id}/steps/{step_id}/payout-request", withImprover(a.RequestWorkflowStepPayoutRetry, a))

	r.Get("/supervisors/workflows", withSupervisor(a.GetSupervisorWorkflows, a))
	r.Post("/supervisors/workflows/export", withSupervisor(a.ExportSupervisorWorkflowData, a))
	r.Put("/supervisors/primary-rewards-account", withSupervisor(a.UpdateSupervisorPrimaryRewardsAccount, a))

	r.Get("/admin/proposers", withAdmin(a.GetProposers, a))
	r.Put("/admin/proposers", withAdmin(a.UpdateProposer, a))
	r.Get("/admin/improvers", withAdmin(a.GetImprovers, a))
	r.Put("/admin/improvers", withAdmin(a.UpdateImprover, a))
	r.Get("/admin/supervisors", withAdmin(a.GetSupervisors, a))
	r.Put("/admin/supervisors", withAdmin(a.UpdateSupervisor, a))
	r.Get("/admin/issuers", withAdmin(a.GetIssuers, a))
	r.Put("/admin/issuers", withAdmin(a.UpdateIssuerScopes, a))
	r.Get("/admin/issuer-requests", withAdmin(a.GetIssuerRequests, a))
	r.Put("/admin/issuer-requests", withAdmin(a.UpdateIssuerRequest, a))
	r.Get("/admin/credential-types", withAdmin(a.GetAdminCredentialTypes, a))
	r.Post("/admin/credential-types", withAdmin(a.CreateAdminCredentialType, a))
	r.Put("/admin/credential-types/{value}", withAdmin(a.UpdateAdminCredentialType, a))
	r.Delete("/admin/credential-types/{value}", withAdmin(a.DeleteAdminCredentialType, a))
	r.Post("/admin/workflow-templates/default", withAdmin(a.CreateDefaultWorkflowTemplate, a))
	r.Get("/admin/workflows", withAdmin(a.GetAdminWorkflows, a))
	r.Get("/admin/workflow-series/{series_id}/claimants", withAdmin(a.GetAdminWorkflowSeriesClaimants, a))
	r.Post("/admin/workflow-series/{series_id}/revoke-claim", withAdmin(a.RevokeAdminWorkflowSeriesImproverClaim, a))
	r.Post("/admin/workflows/{workflow_id}/force-approve", withAdmin(a.AdminForceApproveWorkflow, a))
	r.Post("/admin/workflow-edit-proposals/{proposal_id}/force-approve", withAdmin(a.AdminForceApproveWorkflowEditProposal, a))
	r.Post("/admin/workflow-deletion-proposals/{proposal_id}/force-approve", withAdmin(a.AdminForceApproveWorkflowDeletionProposal, a))
	r.Post("/admin/workflows/{workflow_id}/payout-lock-resolution", withAdmin(a.ResolveAdminWorkflowPayoutLock, a))

	r.Get("/voters/workflows", withVoter(a.GetVoterWorkflows, a))
	r.Get("/voters/workflows/{workflow_id}", withVoter(a.GetVoterWorkflow, a))
	r.Get("/voters/workflow-edit-proposals", withVoter(a.GetVoterWorkflowEditProposals, a))
	r.Get("/voters/workflow-deletion-proposals", withVoter(a.GetVoterWorkflowDeletionProposals, a))
	r.Post("/voters/workflow-deletion-proposals", withVoter(a.ProposeWorkflowDeletion, a))
	r.Get("/workflows/active", withAuth(a.GetActiveWorkflows))
	r.Get("/workflows/{workflow_id}", withAuth(a.GetWorkflow))
	r.Get("/workflow-photos/public/{photo_id}", a.GetPublicWorkflowPhoto)
	r.Get("/workflow-photos/{photo_id}", withAuth(a.GetWorkflowPhoto))
	r.Post("/workflows/{workflow_id}/votes", withVoter(a.VoteWorkflow, a))
	r.Post("/workflow-edit-proposals/{proposal_id}/votes", withVoter(a.VoteWorkflowEditProposal, a))
	r.Post("/workflow-deletion-proposals/{proposal_id}/votes", withVoter(a.VoteWorkflowDeletionProposal, a))

	r.Get("/issuers/scopes", withIssuer(a.GetMyIssuerScopes, a))
	r.Get("/issuers/credential-requests", withIssuer(a.GetIssuerCredentialRequests, a))
	r.Post("/issuers/credential-requests/{request_id}/decision", withIssuer(a.DecideIssuerCredentialRequest, a))
	r.Post("/issuers/credentials", withIssuer(a.IssueCredential, a))
	r.Delete("/issuers/credentials", withIssuer(a.RevokeCredential, a))
	r.Get("/issuers/credentials/{user_id}", withIssuer(a.GetIssuerUserCredentials, a))
}

func AddWalletRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Get("/wallets", withAuth(s.GetWalletsByUser))
	r.Get("/wallets/lookup/{address}", withAuth(s.LookupWalletOwnerByAddress))
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
	r.Post("/transactions/memo", withAuth(p.UpsertTransactionMemo))
	r.Get("/transactions/balance", withAuth(p.GetBalanceAtTimestamp))
}

func AddW9Routes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/w9/submit", s.SubmitW9)
	r.Post("/w9/transaction", withAdmin(s.RecordW9Transaction, s))
	r.Post("/w9/check", withAdmin(s.CheckW9Compliance, s))
	r.Get("/admin/w9/pending", withAdmin(s.GetPendingW9Submissions, s))
	r.Put("/admin/w9/approve", withAdmin(s.ApproveW9Submission, s))
	r.Put("/admin/w9/reject", withAdmin(s.RejectW9Submission, s))
}

func AddUnwrapRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/unwrap/eligibility", withAuth(s.CheckUnwrapEligibility))
	r.Post("/unwrap/record", withAuth(s.RecordUnwrap))
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

func withSupervisor(handlerFunc http.HandlerFunc, s *handlers.AppService) http.HandlerFunc {
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

		isSupervisor := s.IsSupervisor(r.Context(), id)
		if !isSupervisor {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		handlerFunc(w, r)
	}
}

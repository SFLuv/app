package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/structs"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	m "github.com/SFLuv/app/backend/utils/middleware"
)

func isProduction() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("IN_PRODUCTION")), "true")
}

// prankForwardingMiddleware swaps the authenticated user id in the request
// context for a "prankee" user id when a local-dev prank has been set for the
// caller, so the rest of the stack — role guards, data lookups, everything —
// treats the request as if the prankee made it. This is what lets a developer
// see exactly what another user sees.
//
// Safety: this is only mounted when !isProduction() (see New). As a second,
// independent guard it re-checks IN_PRODUCTION on every request and no-ops in
// production regardless of how it was wired, and it only forwards when the
// pranks table exists AND holds a matching row — a table created solely by the
// local dev CLI. It never fails a request: a missing table or any db error is
// treated as "no prank" (handled in ResolvePrankTarget).
func prankForwardingMiddleware(a *handlers.AppService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isProduction() {
				next.ServeHTTP(w, r)
				return
			}

			prankerUserID, ok := r.Context().Value("userDid").(string)
			if !ok || prankerUserID == "" {
				next.ServeHTTP(w, r)
				return
			}

			if prankeeUserID, forwarded := a.ResolvePrankTarget(r.Context(), prankerUserID); forwarded {
				r = r.WithContext(context.WithValue(r.Context(), "userDid", prankeeUserID))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseOrigins(value string) []string {
	entries := strings.Split(value, ",")
	origins := make([]string, 0, len(entries))
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			origins = append(origins, parsed.Scheme+"://"+parsed.Host)
			continue
		}
		origins = append(origins, trimmed)
	}
	return origins
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}

	for _, addition := range additions {
		trimmed := strings.TrimSpace(addition)
		if trimmed == "" {
			continue
		}
		if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			trimmed = parsed.Scheme + "://" + parsed.Host
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		values = append(values, trimmed)
		seen[trimmed] = struct{}{}
	}

	return values
}

func allowedOrigins() []string {
	if configured := parseOrigins(os.Getenv("CORS_ALLOWED_ORIGINS")); len(configured) > 0 {
		return configured
	}

	origins := []string{}
	if appBaseURL := strings.TrimSpace(os.Getenv("APP_BASE_URL")); appBaseURL != "" {
		origins = appendUnique(origins, appBaseURL)
	}

	if !isProduction() {
		origins = appendUnique(
			origins,
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"https://localhost:3000",
			"https://127.0.0.1:3000",
		)
	}

	return origins
}

func New(s *handlers.BotService, a *handlers.AppService, p *handlers.PonderService, mcpRegister func(chi.Router)) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins(),
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Access-Token", "X-Admin-Key", "X-SFLUV-Client-Platform", "X-SFLUV-Client-Version", "X-SFLUV-Client-Build", "X-SFLUV-Client-Installation"},
		ExposedHeaders:   []string{"Link", "X-SFLUV-Auth-Reason"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(m.AuthMiddleware)

	// Local-dev "prank" forwarding: lets a developer act as another user to see
	// exactly what that user sees. It is wired ONLY when not in production, and
	// even then only takes effect when a pranks table has been hand-populated by
	// the local dev CLI (./scripts/dev-up/dev-up.sh menu) — the middleware itself creates
	// nothing. Two independent gates (this !isProduction() check AND the manual
	// db write) must both be true for any forwarding to happen, so an accidental
	// production build cannot be exploited without direct database access.
	if !isProduction() {
		r.Use(prankForwardingMiddleware(a))
	}

	// Read-only until a merchant has listed their shop. Mounted after prank
	// forwarding so a developer pranking into a merchant sees the gate that
	// merchant sees, which is the only way to look at it without an account.
	r.Use(merchantOnboardingGate(a))

	// Admin MCP endpoint + its OAuth authorization server. It authenticates
	// itself (OAuth access token bound to a SFLUV user DID + live admin check)
	// independently of the header-based AuthMiddleware above, so
	// unauthenticated/non-admin callers never reach a tool.
	if mcpRegister != nil {
		mcpRegister(r)
	}

	AddBotRoutes(r, s, a)
	AddVolunteerEventRoutes(r, s)
	AddVolunteerListRoutes(r, a)
	AddPartnerRoutes(r, a)
	AddOrganizationRoutes(r, a, s)
	AddClientConfigRoutes(r, a)
	AddUserRoutes(r, a)
	AddAdminRoutes(r, a)
	AddAffiliateRoutes(r, s, a)
	AddWorkflowRoutes(r, s, a)
	AddWalletRoutes(r, a)
	AddLocationRoutes(r, a)
	AddContactRoutes(r, a)
	AddMerchantModeRoutes(r, a)
	AddPonderRoutes(r, a, p)
	AddW9Routes(r, a)
	AddUnwrapRoutes(r, a)

	return r
}

func AddOrganizationRoutes(r *chi.Mux, a *handlers.AppService, s *handlers.BotService) {
	// Membership-scoped org management. Fine-grained authorization (member vs
	// admin vs superadmin) is enforced inside each handler via requireOrgRole,
	// always resolved live from organization_members.
	r.Get("/organizations/mine", withActiveAuth(a.GetMyOrganization, a))
	r.Put("/organizations/mine/name", withActiveAuth(a.UpdateMyOrganizationName, a))
	r.Put("/organizations/mine/logo", withActiveAuth(a.UpdateMyOrganizationLogo, a))
	r.Post("/organizations/mine/invites", withActiveAuth(a.InviteOrganizationMember, a))
	r.Delete("/organizations/mine/members/{user_id}", withActiveAuth(a.RemoveOrganizationMember, a))
	r.Put("/organizations/mine/members/role", withActiveAuth(a.SetOrganizationMemberRole, a))
	r.Post("/organizations/mine/transfer-superadmin", withActiveAuth(a.TransferOrganizationSuperadmin, a))
	r.Post("/organizations/mine/leave", withActiveAuth(a.LeaveOrganization, a))
	r.Post("/organizations/mine/roles/request", withActiveAuth(a.RequestOrganizationRole, a))
	r.Post("/organizations/join", withActiveAuth(a.JoinOrganization, a))

	// Platform-admin org controls.
	r.Get("/admin/organizations", withAdmin(a.AdminListOrganizations, a))
	r.Put("/admin/organizations/superadmin", withAdmin(a.AdminSetOrganizationSuperadmin, a))
	r.Put("/admin/organizations/roles", withAdmin(a.AdminSetOrganizationRoleStatus, a))
	r.Put("/admin/organizations/issuer-scopes", withAdmin(a.AdminSetOrganizationIssuerScopes, a))
	r.Get("/admin/organizations/{id}/events", withAdmin(s.AdminGetOrganizationEvents, a))
}

func AddClientConfigRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Get("/config", s.GetClientConfig)
	r.Get("/client-version", s.GetClientVersion)
}

// AddVolunteerEventRoutes mounts the public volunteer portal API consumed by
// the mobile app and the marketing site.
//
// These routes are deliberately NOT under the /events prefix: that whole tree
// is admin-guarded, and hanging public reads inside it would put the
// public/private boundary one refactor away from flipping in either direction.
// Auth here is optional — the auth middleware passes through when no valid
// token is present, and handlers enrich the response (the viewer block) only
// when a caller is identified.
func AddVolunteerEventRoutes(r *chi.Mux, s *handlers.BotService) {
	r.Get("/volunteer-events", s.GetVolunteerEvents)
	r.Get("/volunteer-events/organizers", s.GetVolunteerEventOrganizers)
	r.Get("/volunteer-events/photos/{photo_id}", s.GetVolunteerEventPhoto)
	r.Get("/volunteer-events/{id}", s.GetVolunteerEvent)
	r.Get("/organizers/{id}/logo", s.GetOrganizerLogo)

	// One signup path for both clients: auth is optional. Authenticated callers
	// get their identity from their profile; anonymous callers supply it.
	r.Post("/volunteer-events/{id}/signup", s.SignUpForVolunteerEvent)
	r.Delete("/volunteer-events/{id}/signup", withAuth(s.CancelVolunteerEventSignup))
	r.Get("/volunteer-events/mine", withAuth(s.GetMyVolunteerEvents))
}

// AddVolunteerListRoutes mounts the volunteer email list token flows and the
// per-user reminder preferences.
//
// The token GETs are deliberately read-only and the mutations are POST: mail
// clients and corporate link scanners prefetch URLs in email, so a GET that
// mutated would unsubscribe — or silently complete a double opt-in for —
// people who never clicked.
func AddVolunteerListRoutes(r *chi.Mux, a *handlers.AppService) {
	// Inline blast images must be fetchable by an email client, which cannot
	// present credentials. Ids are unguessable UUIDs.
	r.Get("/volunteer-events/blast-images/{image_id}", a.GetEventBlastImage)

	// Signup confirmation for portal (anonymous) signups. Read-only GET, mutating
	// POST — same prefetch-safety rule as the email-list tokens.
	r.Get("/volunteer-events/signup/confirm", a.GetVolunteerSignupConfirmState)
	r.Post("/volunteer-events/signup/confirm", a.PostVolunteerSignupConfirm)

	r.Get("/volunteer-email-list/confirm", a.GetVolunteerListTokenState)
	r.Post("/volunteer-email-list/confirm", a.PostVolunteerListTokenAction)
	r.Get("/volunteer-email-list/unsubscribe", a.GetVolunteerListTokenState)
	r.Post("/volunteer-email-list/unsubscribe", a.PostVolunteerListTokenAction)

	// Cover photos staged before their event exists. Any signed-in user may
	// stage one — it is parked under their own name and invisible until an
	// event they are entitled to create claims it, so the authorisation that
	// matters stays on event creation.
	r.Post("/volunteer-events/staged-photos", withAuth(a.StageVolunteerEventPhoto))
	r.Delete("/volunteer-events/staged-photos/{photo_id}", withAuth(a.DeleteStagedVolunteerEventPhoto))

	r.Get("/volunteer-events/reminder-preferences", withAuth(a.GetVolunteerReminderPreferences))
	r.Put("/volunteer-events/reminder-preferences", withAuth(a.SetVolunteerReminderPreferences))
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
	r.Post("/recovery/balance", s.RecoveryPreview)
	r.Post("/recovery/claim", withAuth(s.RecoveryClaim))
}

func AddUserRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/users", withAuth(s.AddUser))
	r.Get("/users/bootstrap", withAuth(s.GetUserBootstrap))
	r.Get("/users/policy-status", withAuth(s.GetUserPolicyStatus))
	r.Post("/users/policies/accept", withAuth(s.AcceptUserPolicies))
	r.Get("/users", withActiveAuth(s.GetUserAuthed, s))
	r.Put("/users", withActiveAuth(s.UpdateUserInfo, s))
	r.Put("/users/primary-wallet", withActiveAuth(s.UpdateUserPrimaryWallet, s))
	r.Get("/users/account-type/revert-eligibility", withActiveAuth(s.GetMerchantRevertEligibility, s))
	r.Put("/users/account-type", withActiveAuth(s.UpdateOwnAccountType, s))
	r.Post("/users/web-merchant-prompt-seen", withActiveAuth(s.MarkWebMerchantPromptSeen, s))
	r.Put("/paypaleth", withActiveAuth(s.UpdateUserPayPalEth, s))
	r.Post("/users/oauth/apple", withAuth(s.StoreAppleOAuthCredential))
	r.Get("/users/delete-account/preview", withActiveAuth(s.GetDeleteAccountPreview, s))
	r.Post("/users/delete-account", withActiveAuth(s.DeleteAccount, s))
	r.Post("/users/delete-account/cancel", withAuth(s.CancelDeleteAccount))
	r.Get("/users/delete-account/status", withAuth(s.GetDeleteAccountStatus))
	r.Get("/users/verified-emails", withActiveAuth(s.GetUserVerifiedEmails, s))
	r.Post("/users/verified-emails", withActiveAuth(s.RequestUserEmailVerification, s))
	r.Post("/users/verified-emails/{email_id}/resend", withActiveAuth(s.ResendUserEmailVerification, s))
	r.Post("/users/verified-emails/verify", s.VerifyUserEmailToken)
}

func AddAdminRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Get("/admin/users", withAdmin(s.GetUsers, s))
	r.Get("/admin/users/email-list.csv", withAdmin(s.ExportUserEmailList, s))
	r.Post("/admin/users/delete-account/purge", withAdmin(s.PurgeDeletedAccountsManual, s))
	r.Get("/admin/locations", withAdmin(s.GetAuthedLocations, s))
	r.Put("/admin/users", withActiveAuth(s.UpdateUserRole, s))
	r.Put("/admin/users/account-type", withAdmin(s.UpdateUserAccountType, s))
	r.Put("/admin/locations", withAdmin(s.UpdateLocationApproval, s))
	r.Put("/admin/locations/{id}", withAdmin(s.AdminUpdateLocation, s))
	r.Put("/admin/locations/{id}/google-place", withAdmin(s.AdminUpdateLocationGooglePlace, s))
	r.Get("/admin/affiliates", withAdmin(s.GetAffiliates, s))
	r.Put("/admin/affiliates", withAdmin(s.UpdateAffiliate, s))

	// Volunteer event management. Admin-created events are approved on creation,
	// so these mint codes and reserve the faucet allocation immediately.
	r.Get("/admin/volunteer-events", withAdmin(s.AdminListVolunteerEvents, s))
	r.Post("/admin/volunteer-events", withAdmin(s.AdminCreateVolunteerEvent, s))
	r.Put("/admin/volunteer-events/{id}", withAdmin(s.AdminUpdateVolunteerEvent, s))
	r.Post("/admin/volunteer-events/{id}/approve", withAdmin(s.AdminApproveVolunteerEvent, s))
	r.Post("/admin/volunteer-events/{id}/edit/approve", withAdmin(s.AdminApproveVolunteerEventEdit, s))
	r.Post("/admin/volunteer-events/{id}/edit/reject", withAdmin(s.AdminRejectVolunteerEventEdit, s))
	r.Post("/admin/volunteer-events/{id}/reject", withAdmin(s.AdminRejectVolunteerEvent, s))
	r.Get("/admin/volunteer-events/{id}/codes.csv", withAdmin(s.AdminDownloadVolunteerEventCodes, s))
	// JSON for the printable-card export, CSV for a spreadsheet. Same codes,
	// same organization scoping — see loadVolunteerEventCodes.
	r.Get("/admin/volunteer-events/{id}/codes", withAdmin(s.AdminGetVolunteerEventCodes, s))
	r.Post("/admin/volunteer-events/{id}/cancel", withAdmin(s.AdminCancelVolunteerEvent, s))
	r.Post("/admin/volunteer-events/{id}/blast", withAdmin(s.AdminSendEventBlast, s))
	r.Post("/admin/volunteer-events/{id}/blast/preview", withAdmin(s.PreviewEventBlast, s))
	r.Post("/admin/volunteer-events/{id}/blast/images", withAdmin(s.UploadEventBlastImage, s))
	r.Post("/admin/volunteer-events/{id}/photos", withAdmin(s.AdminUploadVolunteerEventPhoto, s))
	r.Delete("/admin/volunteer-events/photos/{photo_id}", withAdmin(s.AdminDeleteVolunteerEventPhoto, s))

	// Partner carousel shown on the public marketing site.
	r.Get("/admin/partners", withAdmin(s.AdminGetPartners, s))
	r.Post("/admin/partners", withAdmin(s.AdminCreatePartner, s))
	r.Put("/admin/partners/order", withAdmin(s.AdminReorderPartners, s))
	r.Put("/admin/partners/{id}", withAdmin(s.AdminUpdatePartner, s))
	r.Delete("/admin/partners/{id}", withAdmin(s.AdminDeletePartner, s))
	r.Post("/admin/partners/{id}/logo", withAdmin(s.AdminUploadPartnerLogo, s))
}

// AddPartnerRoutes exposes the partner carousel publicly. Same data every
// visitor already sees rendered on sfluv.org, so it needs no auth — and the
// marketing site fetches it server-side at build/ISR time.
func AddPartnerRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Get("/partners", s.GetPublicPartners)
	r.Get("/partners/{id}/logo", s.GetPartnerLogo)
}

func AddAffiliateRoutes(r *chi.Mux, s *handlers.BotService, a *handlers.AppService) {
	r.Post("/affiliates/request", withActiveAuth(a.RequestAffiliateStatus, a))
	r.Put("/affiliates/logo", withAffiliate(a.UpdateAffiliateLogo, a))
	// Volunteer events are request-only for affiliates: approval is what commits
	// faucet funds, so an affiliate can never mint codes on their own.
	r.Post("/affiliates/volunteer-events", withAffiliate(a.AffiliateRequestVolunteerEvent, a))
	r.Get("/affiliates/volunteer-events", withAffiliate(a.AffiliateListVolunteerEvents, a))
	// Organizers still need their codes to print, even though they can no longer
	// create events unilaterally. Scoped to their own organization inside the
	// handler — these codes are bearer tokens for faucet funds.
	r.Get("/affiliates/volunteer-events/{id}/codes.csv", withAffiliate(a.AffiliateDownloadVolunteerEventCodes, a))
	r.Get("/affiliates/volunteer-events/{id}/codes", withAffiliate(a.AffiliateGetVolunteerEventCodes, a))
	r.Put("/affiliates/volunteer-events/{id}", withAffiliate(a.AffiliateUpdateVolunteerEvent, a))
	r.Post("/affiliates/volunteer-events/{id}/blast", withAffiliate(a.AffiliateSendEventBlast, a))
	r.Post("/affiliates/volunteer-events/{id}/blast/preview", withAffiliate(a.PreviewEventBlast, a))
	r.Post("/affiliates/volunteer-events/{id}/blast/images", withAffiliate(a.UploadEventBlastImage, a))

	r.Get("/affiliates/{user_id}", withAffiliate(a.GetAffiliate, a))
}

func AddWorkflowRoutes(r *chi.Mux, s *handlers.BotService, a *handlers.AppService) {
	r.Post("/proposers/request", withActiveAuth(a.RequestProposerStatus, a))
	r.Post("/improvers/request", withActiveAuth(a.RequestImproverStatus, a))
	r.Post("/issuers/request", withActiveAuth(a.RequestIssuerStatus, a))
	r.Post("/supervisors/request", withActiveAuth(a.RequestSupervisorStatus, a))
	r.Get("/supervisors/approved", withActiveAuth(a.GetApprovedSupervisors, a))
	r.Get("/credentials/types", withActiveAuth(a.GetCredentialTypes, a))
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
	// The feed is no longer improver-only. It filters on assignment anyway
	// (assigned_improver_id / manager_improver_id), so a non-improver simply
	// gets no workflow rows — the role gate was access control, not
	// correctness. Opening it is what lets a volunteer see that they owe a W-9.
	//
	// The old paths stay as aliases so no shipped client breaks.
	r.Get("/notifications", withActiveAuth(a.GetImproverNotifications, a))
	r.Post("/notifications/seen", withActiveAuth(a.MarkImproverNotificationsSeen, a))
	r.Get("/improvers/notifications", withActiveAuth(a.GetImproverNotifications, a))
	r.Post("/improvers/notifications/seen", withActiveAuth(a.MarkImproverNotificationsSeen, a))
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
	// Per-user issuer credential scopes are owned by organizations now
	// (organization_issuer_scopes + syncMemberIssuerScopesTx). The legacy
	// full-replace writer (SetIssuerScopes) blind-deletes by issuer_id, which
	// would wipe org-derived rows, so its route is retired — issuance settings
	// are managed via PUT /admin/organizations/issuer-scopes.
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
	r.Get("/workflows/active", withActiveAuth(a.GetActiveWorkflows, a))
	r.Get("/workflows/{workflow_id}", withActiveAuth(a.GetWorkflow, a))
	r.Get("/workflow-photos/public/{photo_id}", a.GetPublicWorkflowPhoto)
	r.Get("/workflow-photos/{photo_id}", withActiveAuth(a.GetWorkflowPhoto, a))
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
	r.Get("/wallets", withActiveAuth(s.GetWalletsByUser, s))
	r.Get("/wallets/lookup/{address}", withActiveAuth(s.LookupWalletOwnerByAddress, s))
	r.Post("/wallets", withActiveAuth(s.AddWallet, s))
	r.Put("/wallets", withActiveAuth(s.UpdateWallet, s))
}

func AddLocationRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/locations", withActiveAuth(s.AddLocation, s))
	r.Get("/locations/{id}", s.GetLocation)
	r.Get("/locations", s.GetLocations)
	r.Get("/locations/user", withActiveAuth(s.GetLocationsByUser, s))
	r.Put("/locations", withActiveAuth(s.UpdateLocation, s))
	r.Delete("/locations/{id}", withActiveAuth(s.CancelLocationApplication, s))
	r.Put("/locations/{id}/wallet-settings", withActiveAuth(s.UpdateLocationWalletSettings, s))
	// Unhooking a wallet is always a swap: the picker needs to know which of the
	// merchant's wallets are free, and the replacement is one atomic call.
	r.Get("/locations/{id}/assignable-wallets", withActiveAuth(s.GetAssignableWallets, s))
	r.Put("/locations/{id}/wallets/{role}", withActiveAuth(s.ReplaceLocationWallet, s))
	r.Put("/locations/{id}/google-place", withActiveAuth(s.UpdateLocationGooglePlace, s))
	r.Put("/locations/{id}/hours", withActiveAuth(s.UpdateLocationHours, s))

	// The map is public on the app, the mobile client and the marketing site,
	// so the icon it draws has to be fetchable without a token. Writes stay
	// behind ownership (or admin) checks in the handler.
	r.Get("/locations/{id}/icon", s.GetLocationIcon)
	r.Post("/locations/{id}/icon", withActiveAuth(s.UploadLocationIcon, s))
	r.Delete("/locations/{id}/icon", withActiveAuth(s.DeleteLocationIcon, s))

	// The storefront photo is public on the same three surfaces, under the same
	// ownership rules for writes.
	r.Get("/locations/{id}/photo", s.GetLocationPhoto)
	r.Post("/locations/{id}/photo", withActiveAuth(s.UploadLocationPhoto, s))
	r.Delete("/locations/{id}/photo", withActiveAuth(s.DeleteLocationPhoto, s))
}

func AddContactRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/contacts", withActiveAuth(s.NewContact, s))
	r.Get("/contacts", withActiveAuth(s.GetContacts, s))
	r.Put("/contacts", withActiveAuth(s.UpdateContact, s))
	r.Delete("/contacts", withActiveAuth(s.DeleteContact, s))
}

func AddMerchantModeRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Get("/merchant-mode/status", withActiveAuth(s.GetMerchantModeStatus, s))
	r.Get("/merchant-mode/today", withActiveAuth(s.GetMerchantToday, s))
	r.Get("/merchant-mode/devices", withActiveAuth(s.ListMerchantModeDevices, s))
	// Shops this merchant can put a device to work at — backs the picker on
	// enable and the location toggle on the till.
	r.Get("/merchant-mode/locations", withActiveAuth(s.ListMerchantModeLocations, s))
	r.Patch("/merchant-mode/devices/{device_id}", withActiveAuth(s.UpdateMerchantModeDevice, s))
	r.Post("/merchant-mode/pin", withActiveAuth(s.SetMerchantModePIN, s))
	r.Post("/merchant-mode/pin/help", withActiveAuth(s.RequestMerchantModePINHelp, s))
	r.Post("/merchant-mode/enable", withActiveAuth(s.EnableMerchantMode, s))
	r.Post("/merchant-mode/disable", withActiveAuth(s.DisableMerchantMode, s))
}

func AddPonderRoutes(r *chi.Mux, s *handlers.AppService, p *handlers.PonderService) {
	r.Post("/ponder", withActiveAuth(s.AddPonderMerchantSubscription, s))
	r.Get("/ponder", withActiveAuth(s.GetPonderSubscriptions, s))
	r.Delete("/ponder", withActiveAuth(s.DeletePonderMerchantSubscription, s))
	r.Get("/ponder/push", withActiveAuth(s.GetPonderPushSubscriptions, s))
	r.Put("/ponder/push", withActiveAuth(s.SyncPonderPushSubscriptions, s))
	r.Delete("/ponder/push", withActiveAuth(s.DeletePonderPushSubscription, s))
	r.Get("/ponder/callback", s.PonderPingCallback)
	r.Post("/ponder/callback", s.PonderHookHandler)
	r.Get("/transactions", p.GetTransactionHistory)
	r.Post("/transactions/memo", withActiveAuth(p.UpsertTransactionMemo, s))
	r.Get("/transactions/balance", withActiveAuth(p.GetBalanceAtTimestamp, s))
	r.Get("/admin/analytics/dashboard", withAdmin(p.GetAdminAnalyticsDashboard, s))
}

func AddW9Routes(r *chi.Mux, s *handlers.AppService) {
	// The old surface is gone. POST /w9/submit was unauthenticated, so anyone
	// could file a submission for any wallet with any email — and that email
	// then received the approval notice. There is no approve/reject here either:
	// the vendor validates the form, so an admin eyeballing a wallet address
	// added delay without adding assurance.
	r.Get("/w9/status", withActiveAuth(s.GetW9Status, s))
	r.Post("/w9/start", withActiveAuth(s.StartW9, s))
	r.Post("/w9/tier/{tier}/ack", withActiveAuth(s.AcknowledgeW9Tier, s))

	// The vendor's Form W-9 Status Change callback.
	//
	// Outside withAuth because there is no session behind a machine-to-machine
	// delivery, but not unauthenticated: the handler refuses anything whose
	// HMAC does not verify before it reads a byte of the body. A provider that
	// does not sign its callbacks makes this a 404.
	//
	// Completion is still discovered by the maintenance sweep as well. That is
	// the backstop for a delivery lost past all nine of the vendor's retries,
	// which nothing else would ever tell us about.
	r.Post("/w9/webhook/taxbandits", s.ReceiveW9Webhook)

	// Where the vendor drops somebody after they submit the form. Public and
	// carries no identifiers: it is reached in whatever browser they had open,
	// and the URL can end up in history or a referrer.
	r.Get("/w9/complete", s.ServeW9CompletePage)

	// The local stand-in for the vendor's hosted form, mounted only when the
	// fake provider is selected. It is what makes an end-to-end run possible on
	// a laptop, deep link included.
	if handlers.FakeW9FormEnabled() {
		r.Get("/w9/fake/form/{request_id}", s.ServeFakeW9Form)
		r.Post("/w9/fake/form/{request_id}", s.ServeFakeW9Form)
		// Lets the reset script clear what the stand-in is holding in memory,
		// which no amount of deleting database rows can reach.
		r.Post("/w9/fake/forget", withAdmin(s.ForgetFakeW9, s))
	}

	r.Post("/admin/w9/precheck", withAdmin(s.PrecheckW9ForRecipient, s))
	r.Get("/admin/w9/overview", withAdmin(s.GetW9AdminOverview, s))
	// Year-end 1099-NEC data. Reachable now so it is not discovered in January.
	r.Get("/admin/w9/1099", withAdmin(s.Get1099Report, s))
	r.Post("/admin/w9/{user_id}/clear", withAdmin(s.ClearW9Filing, s))
	r.Post("/admin/w9/{user_id}/resend", withAdmin(s.ResendW9Request, s))
}

func AddUnwrapRoutes(r *chi.Mux, s *handlers.AppService) {
	r.Post("/unwrap/eligibility", withActiveAuth(s.CheckUnwrapEligibility, s))
	r.Post("/unwrap/record", withActiveAuth(s.RecordUnwrap, s))
}

func withAuth(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Value("userDid").(string); !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		handlerFunc(w, r)
	}
}

func userDidFromContext(r *http.Request) (string, bool) {
	id, ok := r.Context().Value("userDid").(string)
	if !ok {
		return "", false
	}

	return id, true
}

func writePolicyRequired(w http.ResponseWriter) {
	w.Header().Set("X-SFLUV-Auth-Reason", structs.AuthReasonPrivacyPolicyRequired)
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(structs.AuthReasonPrivacyPolicyRequired))
}

// merchantOnboardingChecker is the single question the read-only gate asks.
//
// It is an interface rather than *handlers.AppService because the gated state
// cannot be produced from real data: every merchant account in production
// finished onboarding before the gate existed, so the tests have to construct
// somebody who is behind it.
type merchantOnboardingChecker interface {
	MerchantOnboardingRequired(ctx context.Context, userId string) bool
}

func writeMerchantOnboardingRequired(w http.ResponseWriter) {
	w.Header().Set("X-SFLUV-Auth-Reason", structs.AuthReasonMerchantOnboardingRequired)
	// JSON, unlike the privacy-policy refusal above it, because GetUserBootstrap
	// re-dispatches handlers through an httptest recorder and drops any body
	// that does not parse — a plain-text reason reaches that client as a bare
	// status code with nothing to explain it.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"reason": structs.AuthReasonMerchantOnboardingRequired,
		"error":  "List your business to finish setting up your merchant account.",
	})
}

// merchantOnboardingOpenRoutes are the writes a gated merchant must still be
// able to make. Each one is either the act that clears the gate or the way out
// of the app; anything else is a hole in it, so the list stays this short.
//
// POST /users and the policy acceptance are what write account_type in the
// first place, and the delete-account pair is what stops the gate from being a
// trap. Keyed by method as well as path because the difference between
// POST /locations and PUT /locations is the difference between listing your
// first shop and editing one — only the first belongs here.
var merchantOnboardingOpenRoutes = map[string]struct{}{
	"POST /locations":                   {},
	"POST /users":                       {},
	"POST /users/policies/accept":       {},
	"POST /users/delete-account":        {},
	"POST /users/delete-account/cancel": {},
	// Sign-in mirrors the Apple credential the deletion path later needs to
	// revoke with Apple. Refusing it would cost somebody a clean deletion for a
	// write they never asked to make.
	"POST /users/oauth/apple": {},
	// Registering a wallet is part of arriving, not of trading.
	//
	// The web client registers its Privy wallet on EVERY sign-in — _initWallets
	// posts any wallet the backend has no row for, and rethrows if that fails,
	// which makes _userLogin log the person straight back out. Gating it made
	// the gate unsatisfiable: a new merchant was signed out mid-signup, could
	// never reach the onboarding form, and so could never clear the gate. They
	// could not even delete the account, because that screen needs a session
	// they never got.
	//
	// A wallet row is a record of an address the person already controls. It
	// moves nothing, and a merchant needs one before a location can be paid
	// into anyway.
	"POST /wallets": {},
	// The companion write to POST /wallets, and gated it fails the same way:
	// the client designates its primary wallet on every sign-in and throws the
	// person out when the call refuses. Choosing which of your own registered
	// wallets is primary moves nothing either — it is part of arriving.
	"PUT /users/primary-wallet": {},
	// Saying "actually, this is a personal account" is the other way out of the
	// gate, and the one somebody who picked merchant by mistake needs. It is
	// refused on its own terms when a listing exists, so opening it here cannot
	// let anybody escape a gate they are genuinely behind.
	"PUT /users/account-type": {},
}

// merchantOnboardingGateAllows scopes the gate by method, not by a list of
// endpoints. An allowlist of routes is a list somebody forgets to extend, and
// the way that fails is a merchant quietly able to act.
func merchantOnboardingGateAllows(method string, path string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}

	trimmed := strings.TrimSuffix(path, "/")
	if trimmed == "" {
		trimmed = "/"
	}
	if _, open := merchantOnboardingOpenRoutes[method+" "+trimmed]; open {
		return true
	}

	// A prefix rather than a path: the client reads its configuration before it
	// knows anything at all, including whether it is gated.
	return trimmed == "/config" || strings.HasPrefix(trimmed, "/config/")
}

// merchantOnboardingGate holds a merchant who has not listed a shop yet to
// reads. They are meant to be able to open the app and look around — the map
// is most of what SFLUV is — so this refuses actions rather than access.
//
// It is mounted on the mux rather than folded into requireAcceptedAuthedUser
// because the withAuth-only routes mutate too, and the gate has to mean the
// same thing everywhere. A request with no identified caller passes through
// untouched: whatever authorizes those is not this.
func merchantOnboardingGate(check merchantOnboardingChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if check == nil || merchantOnboardingGateAllows(r.Method, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			id, ok := userDidFromContext(r)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			if !check.MerchantOnboardingRequired(r.Context(), id) {
				next.ServeHTTP(w, r)
				return
			}

			writeMerchantOnboardingRequired(w)
		})
	}
}

func requireAcceptedAuthedUser(w http.ResponseWriter, r *http.Request, s *handlers.AppService) (string, bool) {
	id, ok := userDidFromContext(r)
	if !ok {
		w.WriteHeader(http.StatusForbidden)
		return "", false
	}

	if !s.UserIsActive(r.Context(), id) {
		w.WriteHeader(http.StatusForbidden)
		return "", false
	}

	if !s.UserHasAcceptedPrivacyPolicy(r.Context(), id) {
		writePolicyRequired(w)
		return "", false
	}

	return id, true
}

func withActiveAuth(handlerFunc http.HandlerFunc, s *handlers.AppService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := requireAcceptedAuthedUser(w, r, s)
		if !ok {
			return
		}
		s.RecordAnalyticsUserActivity(r.Context(), id, r)

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

		id, ok := requireAcceptedAuthedUser(w, r, s)
		if !ok {
			return
		}
		isAdmin := s.IsAdmin(r.Context(), id)
		if !isAdmin {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		s.RecordAnalyticsUserActivity(r.Context(), id, r)

		handlerFunc(w, r)
	}
}

func withAffiliate(handlerFunc http.HandlerFunc, s *handlers.AppService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := requireAcceptedAuthedUser(w, r, s)
		if !ok {
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
		id, ok := requireAcceptedAuthedUser(w, r, s)
		if !ok {
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
		id, ok := requireAcceptedAuthedUser(w, r, s)
		if !ok {
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
		id, ok := requireAcceptedAuthedUser(w, r, s)
		if !ok {
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
		id, ok := requireAcceptedAuthedUser(w, r, s)
		if !ok {
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
		id, ok := requireAcceptedAuthedUser(w, r, s)
		if !ok {
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

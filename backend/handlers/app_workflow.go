package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math/big"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"

	_ "image/gif"
	_ "image/png"
)

func userCanViewWorkflowNotifyEmails(workflow *structs.Workflow, userID string) bool {
	if workflow == nil || userID == "" {
		return false
	}
	if workflow.ProposerId == userID {
		return true
	}
	if workflow.SupervisorUserId != nil && *workflow.SupervisorUserId == userID {
		return true
	}
	return workflow.ManagerImproverId != nil && *workflow.ManagerImproverId == userID
}

func redactWorkflowItemNotifyEmailsForUser(workflow *structs.Workflow, userID string) {
	if workflow == nil {
		return
	}
	if userCanViewWorkflowNotifyEmails(workflow, userID) {
		return
	}
	for stepIdx := range workflow.Steps {
		for itemIdx := range workflow.Steps[stepIdx].WorkItems {
			for optionIdx := range workflow.Steps[stepIdx].WorkItems[itemIdx].DropdownOptions {
				option := &workflow.Steps[stepIdx].WorkItems[itemIdx].DropdownOptions[optionIdx]
				notifyEmailCount := option.NotifyEmailCount
				if len(option.NotifyEmails) > notifyEmailCount {
					notifyEmailCount = len(option.NotifyEmails)
				}
				option.NotifyEmailCount = notifyEmailCount
				option.NotifyEmails = nil
			}
		}
	}
}

func redactWorkflowItemNotifyEmailsForUserInList(workflows []*structs.Workflow, userID string) {
	for _, workflow := range workflows {
		redactWorkflowItemNotifyEmailsForUser(workflow, userID)
	}
}

func userCanViewWorkflowSubmissionData(workflow *structs.Workflow, userID string, isAdmin bool) bool {
	if workflow == nil || userID == "" {
		return false
	}
	if isAdmin {
		return true
	}
	if workflow.SupervisorUserId != nil && *workflow.SupervisorUserId == userID {
		return true
	}
	for _, step := range workflow.Steps {
		if step.AssignedImproverId != nil && *step.AssignedImproverId == userID {
			return true
		}
		if step.Submission != nil && step.Submission.ImproverId == userID {
			return true
		}
	}
	return false
}

func redactWorkflowSubmissionDataForUser(workflow *structs.Workflow, userID string, isAdmin bool) {
	if workflow == nil {
		return
	}
	if userCanViewWorkflowSubmissionData(workflow, userID, isAdmin) {
		return
	}
	for stepIdx := range workflow.Steps {
		workflow.Steps[stepIdx].Submission = nil
	}
}

func redactWorkflowSubmissionDataForUserInList(workflows []*structs.Workflow, userID string, isAdmin bool) {
	for _, workflow := range workflows {
		redactWorkflowSubmissionDataForUser(workflow, userID, isAdmin)
	}
}

func canUserViewWorkflowSupervisorData(workflow *structs.Workflow, userID string) bool {
	if workflow == nil || userID == "" {
		return false
	}
	if workflow.SupervisorUserId == nil {
		return false
	}
	return strings.TrimSpace(*workflow.SupervisorUserId) == strings.TrimSpace(userID)
}

const (
	workflowPayoutErrorNoImproverAssigned = "Workflow payout failed: no assigned improver."
	workflowPayoutErrorNoPayoutWallet     = "Workflow payout failed: payout wallet is not configured."
	workflowPayoutErrorInsufficientFaucet = "Workflow payout failed: insufficient faucet balance."
	workflowPayoutErrorTransferFailed     = "Workflow payout failed: transfer failed. Please retry."
	workflowPayoutErrorProcessingFailed   = "Workflow payout failed: processing error."
	workflowPayoutErrorUnknown            = "Workflow payout failed. Please retry."
)

func normalizeWorkflowPayoutErrorForClient(raw *string) *string {
	if raw == nil {
		return nil
	}
	message := strings.TrimSpace(*raw)
	if message == "" {
		return nil
	}

	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "no assigned improver"), strings.Contains(lower, "no improver is assigned"):
		normalized := workflowPayoutErrorNoImproverAssigned
		return &normalized
	case strings.Contains(lower, "no payout wallet"), strings.Contains(lower, "wallet is not configured"):
		normalized := workflowPayoutErrorNoPayoutWallet
		return &normalized
	case strings.Contains(lower, "insufficient faucet balance"):
		normalized := workflowPayoutErrorInsufficientFaucet
		return &normalized
	case strings.Contains(lower, "transfer failed"):
		normalized := workflowPayoutErrorTransferFailed
		return &normalized
	case strings.Contains(lower, "processing error"), strings.Contains(lower, "state update failed"):
		normalized := workflowPayoutErrorProcessingFailed
		return &normalized
	default:
		normalized := workflowPayoutErrorUnknown
		return &normalized
	}
}

func normalizeWorkflowPayoutErrorsForClient(workflow *structs.Workflow) {
	if workflow == nil {
		return
	}
	workflow.SupervisorPayoutError = normalizeWorkflowPayoutErrorForClient(workflow.SupervisorPayoutError)
	for idx := range workflow.Steps {
		workflow.Steps[idx].PayoutError = normalizeWorkflowPayoutErrorForClient(workflow.Steps[idx].PayoutError)
	}
}

func sanitizeWorkflowForUserWithOptions(workflow *structs.Workflow, userID string, isAdmin bool, includeSupervisorData bool) {
	redactWorkflowItemNotifyEmailsForUser(workflow, userID)
	redactWorkflowSubmissionDataForUser(workflow, userID, isAdmin)
	normalizeWorkflowPayoutErrorsForClient(workflow)
	if !includeSupervisorData {
		workflow.SupervisorDataFields = nil
	}
}

func sanitizeWorkflowForUser(workflow *structs.Workflow, userID string, isAdmin bool) {
	sanitizeWorkflowForUserWithOptions(workflow, userID, isAdmin, false)
}

func sanitizeWorkflowListForUser(workflows []*structs.Workflow, userID string, isAdmin bool) {
	redactWorkflowItemNotifyEmailsForUserInList(workflows, userID)
	redactWorkflowSubmissionDataForUserInList(workflows, userID, isAdmin)
	for _, workflow := range workflows {
		normalizeWorkflowPayoutErrorsForClient(workflow)
		workflow.SupervisorDataFields = nil
	}
}

func (a *AppService) refreshWorkflowStartAvailabilityAndNotify(ctx context.Context) error {
	refreshResult, err := a.db.RefreshWorkflowStartAvailability(ctx)
	if err != nil {
		return err
	}
	for _, notification := range refreshResult.AvailabilityNotifications {
		a.sendWorkflowStepAvailableEmail(notification)
	}
	a.sendWorkflowSeriesFundingShortfallEmails(ctx, refreshResult.SeriesFundingChecks)
	return nil
}

func (a *AppService) RequestProposerStatus(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading proposer request body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.ProposerRequest
	err = json.Unmarshal(body, &req)
	if err != nil || strings.TrimSpace(req.Organization) == "" || strings.TrimSpace(req.Email) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	proposer, err := a.db.UpsertProposerRequest(r.Context(), *userDid, req.Organization, req.Email)
	if err != nil {
		if err.Error() == "proposer already approved" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "verified") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		a.logger.Logf("error upserting proposer request for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.sendRoleRequestEmail(
		"PROPOSER_ADMIN_EMAIL",
		"New Proposer Request",
		"A user has requested proposer status.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:140px;">User</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Organization</td>
    <td style="padding:12px 0; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Notification Email</td>
    <td style="padding:12px 0; font-size:13px; color:#111827;">%s</td>
  </tr>
</table>`, proposer.UserId, proposer.Organization, proposer.Email),
	)

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(proposer)
}

func (a *AppService) GetProposers(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 {
		count = 20
	}
	proposers, err := a.db.GetProposers(r.Context(), search, page, count)
	if err != nil {
		a.logger.Logf("error getting proposers: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(proposers)
}

func (a *AppService) UpdateProposer(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update proposer body: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.ProposerUpdateRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	proposer, err := a.db.UpdateProposer(r.Context(), &req)
	if err != nil {
		a.logger.Logf("error updating proposer %s: %s", req.UserId, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(proposer)
}

func (a *AppService) RequestImproverStatus(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading improver request body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.ImproverRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	improver, err := a.db.UpsertImproverRequest(r.Context(), *userDid, &req)
	if err != nil {
		if err.Error() == "improver already approved" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "verified") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		a.logger.Logf("error upserting improver request for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.sendRoleRequestEmail(
		"IMPROVER_ADMIN_EMAIL",
		"New Improver Request",
		"A user has requested improver status.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:140px;">User</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">First Name</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Last Name</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Email</td>
    <td style="padding:12px 0; font-size:13px; color:#111827;">%s</td>
  </tr>
</table>`, improver.UserId, improver.FirstName, improver.LastName, improver.Email),
	)

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(improver)
}

func (a *AppService) GetImprovers(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 {
		count = 20
	}
	improvers, err := a.db.GetImprovers(r.Context(), search, page, count)
	if err != nil {
		a.logger.Logf("error getting improvers: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(improvers)
}

func (a *AppService) UpdateImprover(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update improver body: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.ImproverUpdateRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	improver, err := a.db.UpdateImprover(r.Context(), &req)
	if err != nil {
		a.logger.Logf("error updating improver %s: %s", req.UserId, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(improver)
}

func (a *AppService) UpdateImproverPrimaryRewardsAccount(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading improver primary rewards account body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := structs.PrimaryRewardsAccountUpdateRequest{}
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	improver, err := a.db.UpdateImproverPrimaryRewardsAccount(r.Context(), *userDid, req.PrimaryRewardsAccount)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "valid ethereum address") || strings.Contains(errMsg, "approved") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not found") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error updating improver primary rewards account for user %s: %s", *userDid, errMsg)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(improver)
}

func (a *AppService) RequestSupervisorStatus(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading supervisor request body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.SupervisorRequest
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Organization) == "" || strings.TrimSpace(req.Email) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	supervisor, err := a.db.UpsertSupervisorRequest(r.Context(), *userDid, req.Organization, req.Email)
	if err != nil {
		if err.Error() == "supervisor already approved" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "verified") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		a.logger.Logf("error upserting supervisor request for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.sendRoleRequestEmail(
		"SUPERVISOR_ADMIN_EMAIL",
		"New Supervisor Request",
		"A user has requested supervisor status.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:140px;">User</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Organization</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Notification Email</td>
    <td style="padding:12px 0; font-size:13px; color:#111827;">%s</td>
  </tr>
</table>`, supervisor.UserId, supervisor.Organization, supervisor.Email),
	)

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(supervisor)
}

func (a *AppService) GetSupervisors(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 {
		count = 20
	}
	supervisors, err := a.db.GetSupervisors(r.Context(), search, page, count)
	if err != nil {
		a.logger.Logf("error getting supervisors: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(supervisors)
}

func (a *AppService) UpdateSupervisor(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update supervisor body: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.SupervisorUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	supervisor, err := a.db.UpdateSupervisor(r.Context(), &req)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error updating supervisor %s: %s", req.UserId, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(supervisor)
}

func (a *AppService) UpdateSupervisorPrimaryRewardsAccount(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading supervisor primary rewards account body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := structs.PrimaryRewardsAccountUpdateRequest{}
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	supervisor, err := a.db.UpdateSupervisorPrimaryRewardsAccount(r.Context(), *userDid, req.PrimaryRewardsAccount)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "valid ethereum address") || strings.Contains(errMsg, "approved") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not found") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error updating supervisor primary rewards account for user %s: %s", *userDid, errMsg)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(supervisor)
}

func (a *AppService) GetApprovedSupervisors(w http.ResponseWriter, r *http.Request) {
	supervisors, err := a.db.GetApprovedSupervisors(r.Context())
	if err != nil {
		a.logger.Logf("error getting approved supervisors: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(supervisors)
}

func (a *AppService) GetProposerWorkflowTemplates(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	templates, err := a.db.GetWorkflowTemplatesForProposer(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting workflow templates for proposer %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(templates)
}

func (a *AppService) CreateProposerWorkflowTemplate(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading workflow template request body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.WorkflowTemplateCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.TemplateTitle) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	req.Recurrence = strings.TrimSpace(req.Recurrence)
	switch req.Recurrence {
	case "one_time", "daily", "weekly", "monthly":
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	startAt, err := parseWorkflowStartAt(req.StartAt)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	template, err := a.db.CreateWorkflowTemplate(r.Context(), *userDid, &req, startAt, false)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "duplicate") || strings.Contains(errMsg, "unknown") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error creating workflow template for proposer %s: %s", *userDid, errMsg)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(template)
}

func (a *AppService) CreateDefaultWorkflowTemplate(w http.ResponseWriter, r *http.Request) {
	adminId := utils.GetDid(r)
	if adminId == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading default workflow template request body for admin %s: %s", *adminId, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.WorkflowTemplateCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.TemplateTitle) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	req.Recurrence = strings.TrimSpace(req.Recurrence)
	switch req.Recurrence {
	case "one_time", "daily", "weekly", "monthly":
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	startAt, err := parseWorkflowStartAt(req.StartAt)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	template, err := a.db.CreateWorkflowTemplate(r.Context(), *adminId, &req, startAt, true)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "duplicate") || strings.Contains(errMsg, "unknown") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error creating default workflow template for admin %s: %s", *adminId, errMsg)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(template)
}

func (a *AppService) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading workflow request body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.WorkflowCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	recurrence := strings.TrimSpace(req.Recurrence)
	switch recurrence {
	case "one_time", "daily", "weekly", "monthly":
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	req.Recurrence = recurrence

	startAt, err := parseWorkflowStartAt(req.StartAt)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	workflow, err := a.db.CreateWorkflow(r.Context(), *userDid, &req, startAt)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not approved") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "duplicate") || strings.Contains(errMsg, "unknown") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error creating workflow for proposer %s: %s", *userDid, errMsg)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if workflow.Status == "approved" {
		go a.sendWorkflowProposalOutcomeEmailByWorkflow(context.Background(), workflow.Id)
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) GetProposerWorkflows(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	workflows, err := a.db.GetWorkflowsByProposer(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting workflows for proposer %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sanitizeWorkflowListForUser(workflows, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflows)
}

func (a *AppService) GetProposerWorkflow(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	workflowId := strings.TrimSpace(r.PathValue("workflow_id"))
	if workflowId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.refreshWorkflowStartAvailabilityAndNotify(r.Context()); err != nil {
		a.logger.Logf("error refreshing workflow availability before proposer workflow detail %s for user %s: %s", workflowId, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	workflow, err := a.db.GetWorkflowByIDAndProposer(r.Context(), workflowId, *userDid)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error getting workflow %s for proposer %s: %s", workflowId, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sanitizeWorkflowForUser(workflow, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) DeleteProposerWorkflow(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	workflowId := strings.TrimSpace(r.PathValue("workflow_id"))
	if workflowId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := a.db.DeleteWorkflowByProposer(r.Context(), workflowId, *userDid)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "cannot be archived") || strings.Contains(err.Error(), "already archived") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		a.logger.Logf("error archiving workflow %s for proposer %s: %s", workflowId, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *AppService) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)
	includeSupervisorData := false
	includeSupervisorDataRaw := strings.TrimSpace(r.URL.Query().Get("include_supervisor_data"))
	if includeSupervisorDataRaw != "" {
		parsed, parseErr := strconv.ParseBool(includeSupervisorDataRaw)
		if parseErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("invalid include_supervisor_data value"))
			return
		}
		includeSupervisorData = parsed
	}

	workflowId := strings.TrimSpace(r.PathValue("workflow_id"))
	if workflowId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.refreshWorkflowStartAvailabilityAndNotify(r.Context()); err != nil {
		a.logger.Logf("error refreshing workflow availability before workflow detail %s for user %s: %s", workflowId, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	workflow, err := a.db.GetWorkflowByID(r.Context(), workflowId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error getting workflow %s: %s", workflowId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	allowSupervisorData := includeSupervisorData && canUserViewWorkflowSupervisorData(workflow, *userDid)
	sanitizeWorkflowForUserWithOptions(workflow, *userDid, isAdmin, allowSupervisorData)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) GetImproverWorkflows(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	refreshResult, err := a.db.RefreshWorkflowStartAvailability(r.Context())
	if err != nil {
		a.logger.Logf("error refreshing workflow availability for improver %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for _, notification := range refreshResult.AvailabilityNotifications {
		a.sendWorkflowStepAvailableEmail(notification)
	}
	a.sendWorkflowSeriesFundingShortfallEmails(r.Context(), refreshResult.SeriesFundingChecks)

	activeCredentials, err := a.db.GetActiveCredentialTypesForUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error loading active credentials for improver %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	activeSet := map[string]struct{}{}
	for _, credential := range activeCredentials {
		activeSet[credential] = struct{}{}
	}

	workflows, err := a.db.GetImproverWorkflows(r.Context())
	if err != nil {
		a.logger.Logf("error loading improver workflows for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	filtered := make([]*structs.Workflow, 0, len(workflows))
	for _, workflow := range workflows {
		roleById := map[string]structs.WorkflowRole{}
		for _, role := range workflow.Roles {
			roleById[role.Id] = role
		}

		isManager := workflow.ManagerImproverId != nil && *workflow.ManagerImproverId == *userDid
		isRelevant := false
		if isManager {
			isRelevant = true
		}
		if workflow.ManagerRequired && workflow.ManagerImproverId == nil && workflow.ManagerRoleId != nil {
			if managerRole, ok := roleById[*workflow.ManagerRoleId]; ok {
				hasAllManagerCredentials := true
				for _, required := range managerRole.RequiredCredentials {
					if _, has := activeSet[required]; !has {
						hasAllManagerCredentials = false
						break
					}
				}
				if hasAllManagerCredentials {
					isRelevant = true
				}
			}
		}
		for _, step := range workflow.Steps {
			if step.AssignedImproverId != nil && *step.AssignedImproverId == *userDid {
				isRelevant = true
				continue
			}

			if step.AssignedImproverId != nil {
				continue
			}
			if step.Status != "available" && step.Status != "locked" {
				continue
			}
			if step.RoleId == nil {
				continue
			}

			role, ok := roleById[*step.RoleId]
			if !ok {
				continue
			}

			missingRequiredCredential := false
			for _, required := range role.RequiredCredentials {
				if _, has := activeSet[required]; !has {
					missingRequiredCredential = true
					break
				}
			}
			if !missingRequiredCredential {
				isRelevant = true
			}
		}

		if isRelevant {
			sanitizeWorkflowForUser(workflow, *userDid, isAdmin)
			filtered = append(filtered, workflow)
		}
	}

	feed := structs.ImproverWorkflowFeed{
		ActiveCredentials: activeCredentials,
		Workflows:         filtered,
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(feed)
}

func (a *AppService) GetManagedWorkflows(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	workflows, err := a.db.GetManagedWorkflowsByImprover(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error loading managed workflows for improver %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sanitizeWorkflowListForUser(workflows, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflows)
}

func parseSupervisorDateQueryValue(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		utc := parsed.UTC()
		return &utc, nil
	}
	if parsed, err := time.ParseInLocation("2006-01-02", value, time.UTC); err == nil {
		utc := parsed.UTC()
		return &utc, nil
	}
	return nil, fmt.Errorf("invalid date value")
}

func (a *AppService) GetSupervisorWorkflows(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	search := r.URL.Query().Get("search")
	statusFilter := r.URL.Query().Get("status")
	sortBy := r.URL.Query().Get("sort_by")
	sortDirection := r.URL.Query().Get("sort_direction")
	dateField := r.URL.Query().Get("date_field")
	page, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	count, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("count")))

	dateFrom, err := parseSupervisorDateQueryValue(r.URL.Query().Get("date_from"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid date_from"))
		return
	}
	dateTo, err := parseSupervisorDateQueryValue(r.URL.Query().Get("date_to"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid date_to"))
		return
	}

	response, err := a.db.GetSupervisorWorkflows(
		r.Context(),
		*userDid,
		search,
		statusFilter,
		sortBy,
		sortDirection,
		dateField,
		dateFrom,
		dateTo,
		page,
		count,
	)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error loading supervisor workflows for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (a *AppService) GetImproverUnpaidWorkflows(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	workflows, err := a.db.GetImproverUnpaidWorkflows(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error loading unpaid workflows for improver %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, workflow := range workflows {
		a.processWorkflowSeriesPayouts(r.Context(), workflow.Id)
	}

	refreshed, err := a.db.GetImproverUnpaidWorkflows(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error refreshing unpaid workflows for improver %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sanitizeWorkflowListForUser(refreshed, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(refreshed)
}

func (a *AppService) RequestWorkflowStepPayoutRetry(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	workflowID := strings.TrimSpace(r.PathValue("workflow_id"))
	stepID := strings.TrimSpace(r.PathValue("step_id"))
	if workflowID == "" || stepID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.db.RequestWorkflowStepPayoutRetry(r.Context(), workflowID, stepID, *userDid); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no failed step payout found") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error requesting workflow step payout retry for workflow %s step %s improver %s: %s", workflowID, stepID, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.processWorkflowSeriesPayouts(r.Context(), workflowID)

	workflow, err := a.db.GetWorkflowByID(r.Context(), workflowID)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading workflow %s after payout retry request: %s", workflowID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sanitizeWorkflowForUser(workflow, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) RequestWorkflowManagerPayoutRetry(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	workflowID := strings.TrimSpace(r.PathValue("workflow_id"))
	if workflowID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.db.RequestWorkflowManagerPayoutRetry(r.Context(), workflowID, *userDid); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no failed manager payout found") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error requesting workflow manager payout retry for workflow %s improver %s: %s", workflowID, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.processWorkflowSeriesPayouts(r.Context(), workflowID)

	workflow, err := a.db.GetWorkflowByID(r.Context(), workflowID)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading workflow %s after manager payout retry request: %s", workflowID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sanitizeWorkflowForUser(workflow, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) ClaimWorkflowManager(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	workflowID := strings.TrimSpace(r.PathValue("workflow_id"))
	if workflowID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	workflow, err := a.db.ClaimWorkflowManager(r.Context(), workflowID, *userDid)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "missing required credentials") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "already claimed") || strings.Contains(errMsg, "already assigned") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not enabled") ||
			strings.Contains(errMsg, "not available") ||
			strings.Contains(errMsg, "no credential requirements") ||
			strings.Contains(errMsg, "unknown credential type") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error claiming workflow manager on workflow %s for improver %s: %s", workflowID, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sanitizeWorkflowForUser(workflow, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) DownloadManagedWorkflowCSV(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	workflowID := strings.TrimSpace(r.PathValue("workflow_id"))
	if workflowID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	isManager, err := a.db.IsWorkflowManagedByImprover(r.Context(), workflowID, *userDid)
	if err != nil {
		a.logger.Logf("error checking manager authorization for workflow csv %s improver %s: %s", workflowID, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !isManager {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	workflow, err := a.db.GetWorkflowByID(r.Context(), workflowID)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading managed workflow %s for csv export: %s", workflowID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	photoFileNamesByID := map[string]string{}
	photoFileNameCounts := map[string]int{}
	photoExports, err := a.db.GetWorkflowSubmissionPhotoExports(r.Context(), workflowID)
	if err != nil {
		a.logger.Logf("error loading managed workflow %s photos for csv export: %s", workflowID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	for _, photoExport := range photoExports {
		archiveFileName := buildManagedWorkflowPhotoArchiveName(workflowID, photoExport, photoFileNameCounts)
		photoFileNamesByID[photoExport.Photo.Id] = archiveFileName
	}

	csvData, err := buildManagedWorkflowCSV(workflow, photoFileNamesByID)
	if err != nil {
		a.logger.Logf("error building csv export for managed workflow %s: %s", workflowID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("workflow_%s_export.csv", workflowID)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(csvData)
}

var workflowPhotoFieldSlugger = regexp.MustCompile(`[^a-z0-9]+`)

func slugifyWorkflowPhotoField(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = workflowPhotoFieldSlugger.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "field"
	}
	return value
}

func workflowPhotoFileExtension(contentType string, fileName string) string {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(fileName)))
	if ext != "" {
		return ext
	}
	contentType = strings.TrimSpace(contentType)
	if contentType != "" {
		if inferred, err := mime.ExtensionsByType(contentType); err == nil && len(inferred) > 0 {
			ext = strings.ToLower(strings.TrimSpace(inferred[0]))
			if ext != "" {
				return ext
			}
		}
	}
	return ".bin"
}

func buildManagedWorkflowPhotoArchiveName(
	workflowID string,
	photoExport *structs.WorkflowSubmissionPhotoExport,
	fileNameCounts map[string]int,
) string {
	ext := workflowPhotoFileExtension(photoExport.Photo.ContentType, photoExport.Photo.FileName)
	fieldSlug := slugifyWorkflowPhotoField(photoExport.ItemTitle)
	baseName := fmt.Sprintf("%s_step%02d_%s_%s", workflowID, photoExport.StepOrder, fieldSlug, photoExport.Photo.Id)
	archiveFileName := baseName + ext
	if seenCount := fileNameCounts[archiveFileName]; seenCount > 0 {
		archiveFileName = fmt.Sprintf("%s_%d%s", baseName, seenCount+1, ext)
	}
	fileNameCounts[archiveFileName]++
	return archiveFileName
}

func (a *AppService) DownloadManagedWorkflowPhotosZip(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	workflowID := strings.TrimSpace(r.PathValue("workflow_id"))
	if workflowID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	isManager, err := a.db.IsWorkflowManagedByImprover(r.Context(), workflowID, *userDid)
	if err != nil {
		a.logger.Logf("error checking manager authorization for workflow photo export %s improver %s: %s", workflowID, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !isManager {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	photos, err := a.db.GetWorkflowSubmissionPhotoExports(r.Context(), workflowID)
	if err != nil {
		a.logger.Logf("error loading workflow photo export rows for workflow %s: %s", workflowID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)
	fileNameCounts := map[string]int{}
	for _, photoExport := range photos {
		archiveFileName := buildManagedWorkflowPhotoArchiveName(workflowID, photoExport, fileNameCounts)

		entryWriter, entryErr := zipWriter.Create(archiveFileName)
		if entryErr != nil {
			a.logger.Logf("error creating photo zip entry for workflow %s photo %s: %s", workflowID, photoExport.Photo.Id, entryErr)
			continue
		}
		if _, writeErr := entryWriter.Write(photoExport.Photo.PhotoData); writeErr != nil {
			a.logger.Logf("error writing photo zip entry for workflow %s photo %s: %s", workflowID, photoExport.Photo.Id, writeErr)
		}
	}
	if err := zipWriter.Close(); err != nil {
		a.logger.Logf("error finalizing photo zip for workflow %s: %s", workflowID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("workflow_%s_photos.zip", workflowID)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(zipBuffer.Bytes())
}

var supervisorExportTokenSanitizer = regexp.MustCompile(`[^A-Z0-9]+`)
var supervisorCSVHeaderSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

func sanitizeSupervisorExportToken(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = supervisorExportTokenSanitizer.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "UNTITLED"
	}
	return value
}

func sanitizeSupervisorCSVHeaderToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = supervisorCSVHeaderSanitizer.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "column"
	}
	return value
}

func buildSupervisorWorkflowPhotoArchiveName(
	photoExport *structs.WorkflowSubmissionPhotoExport,
	defaultWorkflowStartAt int64,
	fileNameCounts map[string]int,
) string {
	workflowToken := sanitizeSupervisorExportToken(photoExport.WorkflowTitle)
	itemToken := sanitizeSupervisorExportToken(photoExport.ItemTitle)
	timestampUnix := defaultWorkflowStartAt
	if photoExport.WorkflowStartAt > 0 {
		timestampUnix = photoExport.WorkflowStartAt
	}
	baseName := fmt.Sprintf("%s_%d_%d_%s", workflowToken, timestampUnix, photoExport.StepOrder, itemToken)
	archiveName := baseName + ".jpeg"
	if seenCount := fileNameCounts[archiveName]; seenCount > 0 {
		archiveName = fmt.Sprintf("%s_%d.jpeg", baseName, seenCount+1)
	}
	fileNameCounts[archiveName]++
	return archiveName
}

func uniqueSupervisorCSVHeader(base string, seen map[string]int) string {
	if seen == nil {
		return base
	}
	count := seen[base]
	seen[base] = count + 1
	if count == 0 {
		return base
	}
	return base + strings.Repeat("_duplicate", count)
}

func (a *AppService) collectSupervisorWorkflowIDsForExport(
	ctx context.Context,
	supervisorID string,
	req *structs.SupervisorWorkflowExportRequest,
) ([]string, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	seen := map[string]struct{}{}
	ids := []string{}
	for _, workflowID := range req.WorkflowIds {
		workflowID = strings.TrimSpace(workflowID)
		if workflowID == "" {
			continue
		}
		if _, exists := seen[workflowID]; exists {
			continue
		}
		seen[workflowID] = struct{}{}
		ids = append(ids, workflowID)
	}
	if len(ids) > 0 {
		return ids, nil
	}

	dateFrom, err := parseSupervisorDateQueryValue(req.DateFrom)
	if err != nil {
		return nil, fmt.Errorf("invalid date_from")
	}
	dateTo, err := parseSupervisorDateQueryValue(req.DateTo)
	if err != nil {
		return nil, fmt.Errorf("invalid date_to")
	}

	page := 0
	pageSize := 200
	total := -1
	for total < 0 || len(ids) < total {
		result, err := a.db.GetSupervisorWorkflows(
			ctx,
			supervisorID,
			"",
			"",
			"created_at",
			"desc",
			req.DateField,
			dateFrom,
			dateTo,
			page,
			pageSize,
		)
		if err != nil {
			return nil, err
		}
		if total < 0 {
			total = result.Total
		}
		if len(result.Items) == 0 {
			break
		}
		for _, item := range result.Items {
			if _, exists := seen[item.Id]; exists {
				continue
			}
			seen[item.Id] = struct{}{}
			ids = append(ids, item.Id)
		}
		page++
	}
	return ids, nil
}

type supervisorExportColumn struct {
	Key    string
	Header string
}

func deriveSupervisorCSVStepStatus(step structs.WorkflowStep) string {
	if step.Submission != nil {
		if step.Submission.StepNotPossible {
			return "failed"
		}
		return "complete"
	}

	switch strings.ToLower(strings.TrimSpace(step.Status)) {
	case "in_progress":
		return "active"
	case "completed", "paid_out":
		return "complete"
	default:
		// locked + available (and any unknown status) are treated as not yet submitted.
		return "pending"
	}
}

func buildSupervisorWorkflowCSV(
	workflows []*structs.Workflow,
	improverEmails map[string]string,
	photoFileNamesByID map[string]string,
) ([]byte, error) {
	if len(workflows) == 0 {
		return nil, fmt.Errorf("no workflows provided")
	}

	dynamicColumns := map[string]supervisorExportColumn{}
	usedHeaders := map[string]int{}
	metadataColumns := map[string]supervisorExportColumn{}
	for _, workflow := range workflows {
		for _, metadataField := range workflow.SupervisorDataFields {
			header := strings.TrimSpace(metadataField.Key)
			if header == "" {
				continue
			}
			key := strings.ToLower(header)
			if _, exists := metadataColumns[key]; exists {
				continue
			}
			metadataColumns[key] = supervisorExportColumn{
				Key:    key,
				Header: header,
			}
		}
		for _, step := range workflow.Steps {
			for _, item := range step.WorkItems {
				title := strings.TrimSpace(item.Title)
				if title == "" {
					title = "Untitled Item"
				}
				titleKey := strings.ToLower(title)
				if item.RequiresDropdown {
					key := titleKey + "|dropdown"
					if _, exists := dynamicColumns[key]; !exists {
						baseHeader := sanitizeSupervisorCSVHeaderToken(title + "_dropdown")
						dynamicColumns[key] = supervisorExportColumn{
							Key:    key,
							Header: uniqueSupervisorCSVHeader(baseHeader, usedHeaders),
						}
					}
				}
				if item.RequiresWrittenResponse {
					key := titleKey + "|written"
					if _, exists := dynamicColumns[key]; !exists {
						baseHeader := sanitizeSupervisorCSVHeaderToken(title + "_written")
						dynamicColumns[key] = supervisorExportColumn{
							Key:    key,
							Header: uniqueSupervisorCSVHeader(baseHeader, usedHeaders),
						}
					}
				}
				if item.RequiresPhoto {
					key := titleKey + "|photos"
					if _, exists := dynamicColumns[key]; !exists {
						baseHeader := sanitizeSupervisorCSVHeaderToken(title + "_photos")
						dynamicColumns[key] = supervisorExportColumn{
							Key:    key,
							Header: uniqueSupervisorCSVHeader(baseHeader, usedHeaders),
						}
					}
				}
				if item.RequiresDropdown {
					for _, option := range item.DropdownOptions {
						if option.RequiresWrittenResponse {
							key := titleKey + "|written"
							if _, exists := dynamicColumns[key]; !exists {
								baseHeader := sanitizeSupervisorCSVHeaderToken(title + "_written")
								dynamicColumns[key] = supervisorExportColumn{
									Key:    key,
									Header: uniqueSupervisorCSVHeader(baseHeader, usedHeaders),
								}
							}
							break
						}
					}
				}
			}
		}
	}

	dynamicOrder := make([]supervisorExportColumn, 0, len(dynamicColumns))
	for _, column := range dynamicColumns {
		dynamicOrder = append(dynamicOrder, column)
	}
	sort.Slice(dynamicOrder, func(i, j int) bool {
		return dynamicOrder[i].Header < dynamicOrder[j].Header
	})
	metadataOrder := make([]supervisorExportColumn, 0, len(metadataColumns))
	for _, column := range metadataColumns {
		metadataOrder = append(metadataOrder, column)
	}
	sort.Slice(metadataOrder, func(i, j int) bool {
		return strings.ToLower(metadataOrder[i].Header) < strings.ToLower(metadataOrder[j].Header)
	})

	headers := []string{
		"id",
		"series_id",
		"title",
		"start_timestamp",
		"completed_timestamp",
		"step_number",
		"status",
		"improver_email",
	}
	for _, column := range metadataOrder {
		headers = append(headers, column.Header)
	}
	for _, column := range dynamicOrder {
		headers = append(headers, column.Header)
	}

	var csvBuffer bytes.Buffer
	writer := csv.NewWriter(&csvBuffer)
	if err := writer.Write(headers); err != nil {
		return nil, err
	}

	for _, workflow := range workflows {
		steps := append([]structs.WorkflowStep{}, workflow.Steps...)
		sort.SliceStable(steps, func(i, j int) bool {
			return steps[i].StepOrder < steps[j].StepOrder
		})

		for _, step := range steps {
			responseByItem := map[string]structs.WorkflowStepItemResponse{}
			if step.Submission != nil {
				for _, response := range step.Submission.ItemResponses {
					responseByItem[response.ItemId] = response
				}
			}
			statusValue := deriveSupervisorCSVStepStatus(step)

			improverID := ""
			if step.AssignedImproverId != nil {
				improverID = strings.TrimSpace(*step.AssignedImproverId)
			}
			if improverID == "" && step.Submission != nil {
				improverID = strings.TrimSpace(step.Submission.ImproverId)
			}
			improverEmail := strings.TrimSpace(improverEmails[improverID])
			if improverEmail == "" {
				improverEmail = improverID
			}

			completedAt := ""
			if step.CompletedAt != nil {
				completedAt = strconv.FormatInt(*step.CompletedAt, 10)
			}
			if completedAt == "" && step.Submission != nil {
				completedAt = strconv.FormatInt(step.Submission.SubmittedAt, 10)
			}

			row := []string{
				workflow.Id,
				workflow.SeriesId,
				workflow.Title,
				strconv.FormatInt(workflow.StartAt, 10),
				completedAt,
				strconv.Itoa(step.StepOrder),
				statusValue,
				improverEmail,
			}
			metadataValuesByKey := map[string]string{}
			for _, metadataField := range workflow.SupervisorDataFields {
				key := strings.ToLower(strings.TrimSpace(metadataField.Key))
				if key == "" {
					continue
				}
				metadataValuesByKey[key] = strings.TrimSpace(metadataField.Value)
			}
			for _, column := range metadataOrder {
				row = append(row, metadataValuesByKey[column.Key])
			}

			valuesByKey := map[string]string{}
			for _, item := range step.WorkItems {
				title := strings.TrimSpace(item.Title)
				if title == "" {
					title = "Untitled Item"
				}
				titleKey := strings.ToLower(title)

				response, hasResponse := responseByItem[item.Id]
				if !hasResponse {
					continue
				}

				if item.RequiresDropdown && response.DropdownValue != nil {
					valuesByKey[titleKey+"|dropdown"] = strings.TrimSpace(*response.DropdownValue)
				}
				if (item.RequiresWrittenResponse || item.RequiresDropdown) && response.WrittenResponse != nil {
					valuesByKey[titleKey+"|written"] = strings.TrimSpace(*response.WrittenResponse)
				}
				if item.RequiresPhoto {
					photoIDs := append([]string{}, response.PhotoIDs...)
					if len(photoIDs) == 0 && len(response.PhotoURLs) > 0 {
						photoIDs = append(photoIDs, response.PhotoURLs...)
					}
					photoNames := make([]string, 0, len(photoIDs))
					for _, photoID := range photoIDs {
						photoID = strings.TrimSpace(photoID)
						if photoID == "" {
							continue
						}
						if name := strings.TrimSpace(photoFileNamesByID[photoID]); name != "" {
							photoNames = append(photoNames, name)
							continue
						}
						photoNames = append(photoNames, photoID)
					}
					valuesByKey[titleKey+"|photos"] = strings.Join(photoNames, ";")
				}
			}

			for _, column := range dynamicOrder {
				row = append(row, valuesByKey[column.Key])
			}

			if err := writer.Write(row); err != nil {
				return nil, err
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return csvBuffer.Bytes(), nil
}

func (a *AppService) ExportSupervisorWorkflowData(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading supervisor workflow export body for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := structs.SupervisorWorkflowExportRequest{}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	workflowIDs, err := a.collectSupervisorWorkflowIDsForExport(r.Context(), *userDid, &req)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error selecting supervisor workflows for export by %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if len(workflowIDs) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	workflows := make([]*structs.Workflow, 0, len(workflowIDs))
	improverIDs := map[string]struct{}{}
	for _, workflowID := range workflowIDs {
		workflow, err := a.db.GetWorkflowByID(r.Context(), workflowID)
		if err != nil {
			if err == pgx.ErrNoRows {
				continue
			}
			a.logger.Logf("error loading workflow %s for supervisor export by %s: %s", workflowID, *userDid, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if workflow.SupervisorUserId == nil || strings.TrimSpace(*workflow.SupervisorUserId) != *userDid {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		workflows = append(workflows, workflow)
		for _, step := range workflow.Steps {
			if step.AssignedImproverId != nil && strings.TrimSpace(*step.AssignedImproverId) != "" {
				improverIDs[strings.TrimSpace(*step.AssignedImproverId)] = struct{}{}
			}
			if step.Submission != nil && strings.TrimSpace(step.Submission.ImproverId) != "" {
				improverIDs[strings.TrimSpace(step.Submission.ImproverId)] = struct{}{}
			}
		}
	}
	if len(workflows) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	sort.SliceStable(workflows, func(i, j int) bool {
		return workflows[i].StartAt < workflows[j].StartAt
	})

	improverIDList := make([]string, 0, len(improverIDs))
	for improverID := range improverIDs {
		improverIDList = append(improverIDList, improverID)
	}
	improverEmails, err := a.db.GetUserEmailsByIDs(r.Context(), improverIDList)
	if err != nil {
		a.logger.Logf("error loading improver emails for supervisor export by %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)
	fileNameCounts := map[string]int{}
	photoFileNamesByID := map[string]string{}
	for _, workflow := range workflows {
		photos, err := a.db.GetWorkflowSubmissionPhotoExports(r.Context(), workflow.Id)
		if err != nil {
			a.logger.Logf("error loading supervisor photo exports for workflow %s user %s: %s", workflow.Id, *userDid, err)
			continue
		}
		for _, photoExport := range photos {
			img, _, decodeErr := image.Decode(bytes.NewReader(photoExport.Photo.PhotoData))
			if decodeErr != nil {
				continue
			}

			var jpegBuffer bytes.Buffer
			if err := jpeg.Encode(&jpegBuffer, img, &jpeg.Options{Quality: 90}); err != nil {
				continue
			}

			archiveName := buildSupervisorWorkflowPhotoArchiveName(photoExport, workflow.StartAt, fileNameCounts)

			entryWriter, entryErr := zipWriter.Create(filepath.Join("photos", archiveName))
			if entryErr != nil {
				continue
			}
			if _, writeErr := entryWriter.Write(jpegBuffer.Bytes()); writeErr != nil {
				continue
			}
			photoFileNamesByID[photoExport.Photo.Id] = archiveName
		}
	}

	csvData, err := buildSupervisorWorkflowCSV(workflows, improverEmails, photoFileNamesByID)
	if err != nil {
		a.logger.Logf("error building supervisor workflow csv for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	csvEntry, err := zipWriter.Create("supervisor_workflows.csv")
	if err != nil {
		a.logger.Logf("error creating supervisor export csv zip entry for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if _, err := csvEntry.Write(csvData); err != nil {
		a.logger.Logf("error writing supervisor export csv for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := zipWriter.Close(); err != nil {
		a.logger.Logf("error finalizing supervisor workflow export zip for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"supervisor_workflow_export.zip\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(zipBuffer.Bytes())
}

func buildManagedWorkflowCSV(workflow *structs.Workflow, photoFileNamesByID map[string]string) ([]byte, error) {
	if workflow == nil {
		return nil, fmt.Errorf("workflow is required")
	}

	var csvBuffer bytes.Buffer
	writer := csv.NewWriter(&csvBuffer)
	headers := []string{
		"workflow_id",
		"workflow_title",
		"series_id",
		"workflow_status",
		"recurrence",
		"start_at",
		"manager_improver_id",
		"manager_bounty",
		"step_order",
		"step_id",
		"step_title",
		"step_status",
		"step_assigned_improver_id",
		"step_bounty",
		"item_order",
		"item_id",
		"item_title",
		"item_optional",
		"submitted_at",
		"submitted_by",
		"dropdown_value",
		"written_response",
		"photo_ids",
	}
	if err := writer.Write(headers); err != nil {
		return nil, err
	}

	steps := append([]structs.WorkflowStep{}, workflow.Steps...)
	sort.SliceStable(steps, func(i int, j int) bool {
		return steps[i].StepOrder < steps[j].StepOrder
	})

	managerImproverID := ""
	if workflow.ManagerImproverId != nil {
		managerImproverID = *workflow.ManagerImproverId
	}

	for _, step := range steps {
		items := append([]structs.WorkflowWorkItem{}, step.WorkItems...)
		sort.SliceStable(items, func(i int, j int) bool {
			return items[i].ItemOrder < items[j].ItemOrder
		})

		responseByItem := map[string]structs.WorkflowStepItemResponse{}
		submittedAt := ""
		submittedBy := ""
		if step.Submission != nil {
			submittedAt = time.Unix(step.Submission.SubmittedAt, 0).UTC().Format(time.RFC3339)
			submittedBy = step.Submission.ImproverId
			for _, response := range step.Submission.ItemResponses {
				responseByItem[response.ItemId] = response
			}
		}

		if len(items) == 0 {
			row := []string{
				workflow.Id,
				workflow.Title,
				workflow.SeriesId,
				workflow.Status,
				workflow.Recurrence,
				time.Unix(workflow.StartAt, 0).UTC().Format(time.RFC3339),
				managerImproverID,
				strconv.FormatUint(workflow.ManagerBounty, 10),
				strconv.Itoa(step.StepOrder),
				step.Id,
				step.Title,
				step.Status,
				valueOrEmpty(step.AssignedImproverId),
				strconv.FormatUint(step.Bounty, 10),
				"",
				"",
				"",
				"",
				submittedAt,
				submittedBy,
				"",
				"",
				"",
			}
			if err := writer.Write(row); err != nil {
				return nil, err
			}
			continue
		}

		for _, item := range items {
			response, hasResponse := responseByItem[item.Id]
			dropdownValue := ""
			writtenResponse := ""
			photoIDs := []string{}
			if hasResponse {
				if response.DropdownValue != nil {
					dropdownValue = *response.DropdownValue
				}
				if response.WrittenResponse != nil {
					writtenResponse = *response.WrittenResponse
				}
				photoIDs = append(photoIDs, response.PhotoIDs...)
				if len(photoIDs) == 0 && len(response.PhotoURLs) > 0 {
					photoIDs = append(photoIDs, response.PhotoURLs...)
				}
			}
			photoNames := make([]string, 0, len(photoIDs))
			for _, photoID := range photoIDs {
				photoID = strings.TrimSpace(photoID)
				if photoID == "" {
					continue
				}
				if name := strings.TrimSpace(photoFileNamesByID[photoID]); name != "" {
					photoNames = append(photoNames, name)
					continue
				}
				photoNames = append(photoNames, photoID)
			}

			row := []string{
				workflow.Id,
				workflow.Title,
				workflow.SeriesId,
				workflow.Status,
				workflow.Recurrence,
				time.Unix(workflow.StartAt, 0).UTC().Format(time.RFC3339),
				managerImproverID,
				strconv.FormatUint(workflow.ManagerBounty, 10),
				strconv.Itoa(step.StepOrder),
				step.Id,
				step.Title,
				step.Status,
				valueOrEmpty(step.AssignedImproverId),
				strconv.FormatUint(step.Bounty, 10),
				strconv.Itoa(item.ItemOrder),
				item.Id,
				item.Title,
				strconv.FormatBool(item.Optional),
				submittedAt,
				submittedBy,
				dropdownValue,
				writtenResponse,
				strings.Join(photoNames, ";"),
			}
			if err := writer.Write(row); err != nil {
				return nil, err
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return csvBuffer.Bytes(), nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (a *AppService) GetWorkflowPhoto(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	photoID := strings.TrimSpace(r.PathValue("photo_id"))
	if photoID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	photo, err := a.db.GetWorkflowSubmissionPhotoBlobByID(r.Context(), photoID)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading workflow photo %s: %s", photoID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	isAdmin := a.IsAdmin(r.Context(), *userDid)
	canAccess := isAdmin
	if !canAccess {
		workflow, wfErr := a.db.GetWorkflowByID(r.Context(), photo.WorkflowId)
		if wfErr != nil && wfErr != pgx.ErrNoRows {
			a.logger.Logf("error checking workflow photo access for photo %s user %s: %s", photoID, *userDid, wfErr)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if wfErr == nil {
			canAccess = userCanViewWorkflowSubmissionData(workflow, *userDid, isAdmin)
		}
	}

	if !canAccess {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	contentType := strings.TrimSpace(photo.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	filename := strings.TrimSpace(filepath.Base(photo.FileName))
	if filename == "" {
		filename = photo.Id + workflowPhotoFileExtension(contentType, "")
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(photo.PhotoData)
}

func (a *AppService) GetImproverAbsencePeriods(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	periods, err := a.db.GetImproverAbsencePeriods(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error loading improver absence periods for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(periods)
}

func (a *AppService) CreateImproverAbsencePeriod(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading improver absence request body for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.ImproverAbsencePeriodCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	absentFrom, err := parseAbsenceBoundary(req.AbsentFrom, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid absent_from"))
		return
	}
	absentUntil, err := parseAbsenceBoundary(req.AbsentUntil, true)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid absent_until"))
		return
	}

	result, err := a.db.CreateImproverAbsencePeriod(
		r.Context(),
		*userDid,
		req.SeriesId,
		req.StepOrder,
		absentFrom,
		absentUntil,
	)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "must be") || strings.Contains(errMsg, "overlapping") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "no claimed recurring workpiece") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error creating improver absence period for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(result)
}

func (a *AppService) UpdateImproverAbsencePeriod(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	absenceID := strings.TrimSpace(r.PathValue("absence_id"))
	if absenceID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading improver absence update request body for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.ImproverAbsencePeriodUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	absentFrom, err := parseAbsenceBoundary(req.AbsentFrom, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid absent_from"))
		return
	}
	absentUntil, err := parseAbsenceBoundary(req.AbsentUntil, true)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid absent_until"))
		return
	}

	result, err := a.db.UpdateImproverAbsencePeriod(
		r.Context(),
		*userDid,
		absenceID,
		absentFrom,
		absentUntil,
	)
	if err != nil {
		errMsg := err.Error()
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "must be") || strings.Contains(errMsg, "overlapping") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "another improver has already claimed") || strings.Contains(errMsg, "no claimed recurring workpiece") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error updating improver absence period %s for %s: %s", absenceID, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

func (a *AppService) DeleteImproverAbsencePeriod(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	absenceID := strings.TrimSpace(r.PathValue("absence_id"))
	if absenceID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	result, err := a.db.DeleteImproverAbsencePeriod(r.Context(), *userDid, absenceID)
	if err != nil {
		errMsg := err.Error()
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.Contains(errMsg, "required") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "another improver has already claimed") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error deleting improver absence period %s for %s: %s", absenceID, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

func (a *AppService) UnclaimImproverWorkflowSeries(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading improver workflow series unclaim body for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.ImproverWorkflowSeriesUnclaimRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	result, err := a.db.UnclaimImproverWorkflowSeriesStep(r.Context(), *userDid, req.SeriesId, req.StepOrder)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "greater than zero") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "no claimed recurring workpiece") || strings.Contains(errMsg, "no claimable recurring assignments") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error unclaiming workflow series %s step %d for improver %s: %s", req.SeriesId, req.StepOrder, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

func (a *AppService) ClaimWorkflowStep(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	if refreshResult, err := a.db.RefreshWorkflowStartAvailability(r.Context()); err == nil {
		for _, notification := range refreshResult.AvailabilityNotifications {
			a.sendWorkflowStepAvailableEmail(notification)
		}
		a.sendWorkflowSeriesFundingShortfallEmails(r.Context(), refreshResult.SeriesFundingChecks)
	} else {
		a.logger.Logf("error refreshing workflow start availability before claim for improver %s: %s", *userDid, err)
	}

	workflowId := strings.TrimSpace(r.PathValue("workflow_id"))
	stepId := strings.TrimSpace(r.PathValue("step_id"))
	if workflowId == "" || stepId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	workflow, availabilityNotification, err := a.db.ClaimWorkflowStep(r.Context(), workflowId, stepId, *userDid)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "missing required credentials") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "already assigned") || strings.Contains(errMsg, "already claimed") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "absence period") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not claimable") ||
			strings.Contains(errMsg, "not available") ||
			strings.Contains(errMsg, "missing a role") ||
			strings.Contains(errMsg, "does not belong") ||
			strings.Contains(errMsg, "no credential requirements") ||
			strings.Contains(errMsg, "unknown credential type") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error claiming workflow step %s on workflow %s for improver %s: %s", stepId, workflowId, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if availabilityNotification != nil {
		a.sendWorkflowStepAvailableEmail(*availabilityNotification)
	}
	sanitizeWorkflowForUser(workflow, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) StartWorkflowStep(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	if refreshResult, err := a.db.RefreshWorkflowStartAvailability(r.Context()); err == nil {
		for _, notification := range refreshResult.AvailabilityNotifications {
			a.sendWorkflowStepAvailableEmail(notification)
		}
		a.sendWorkflowSeriesFundingShortfallEmails(r.Context(), refreshResult.SeriesFundingChecks)
	} else {
		a.logger.Logf("error refreshing workflow start availability before start for improver %s: %s", *userDid, err)
	}

	workflowId := strings.TrimSpace(r.PathValue("workflow_id"))
	stepId := strings.TrimSpace(r.PathValue("step_id"))
	if workflowId == "" || stepId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	workflow, err := a.db.StartWorkflowStep(r.Context(), workflowId, stepId, *userDid)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not assigned") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not available yet") || strings.Contains(errMsg, "not active") || strings.Contains(errMsg, "already been completed") || strings.Contains(errMsg, "does not belong") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error starting workflow step %s on workflow %s for improver %s: %s", stepId, workflowId, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sanitizeWorkflowForUser(workflow, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) CompleteWorkflowStep(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	if refreshResult, err := a.db.RefreshWorkflowStartAvailability(r.Context()); err == nil {
		for _, notification := range refreshResult.AvailabilityNotifications {
			a.sendWorkflowStepAvailableEmail(notification)
		}
		a.sendWorkflowSeriesFundingShortfallEmails(r.Context(), refreshResult.SeriesFundingChecks)
	} else {
		a.logger.Logf("error refreshing workflow start availability before complete for improver %s: %s", *userDid, err)
	}

	workflowId := strings.TrimSpace(r.PathValue("workflow_id"))
	stepId := strings.TrimSpace(r.PathValue("step_id"))
	if workflowId == "" || stepId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading workflow step completion body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := structs.WorkflowStepCompleteRequest{}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	result, err := a.db.CompleteWorkflowStep(r.Context(), workflowId, stepId, *userDid, req.StepNotPossible, req.StepNotPossibleDetails, req.Items)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "not assigned") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "requires") || strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "missing") || strings.Contains(errMsg, "duplicate") || strings.Contains(errMsg, "not available yet") || strings.Contains(errMsg, "already been completed") || strings.Contains(errMsg, "does not belong") || strings.Contains(errMsg, "not active") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error completing workflow step %s on workflow %s for improver %s: %s", stepId, workflowId, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, notification := range result.AvailabilityNotifications {
		a.sendWorkflowStepAvailableEmail(notification)
	}
	for _, notification := range result.DropdownNotifications {
		a.sendWorkflowDropdownAlertEmail(notification)
	}

	a.processWorkflowSeriesPayouts(r.Context(), workflowId)

	workflow, err := a.db.GetWorkflowByID(r.Context(), workflowId)
	if err != nil {
		a.logger.Logf("error loading workflow %s after step completion: %s", workflowId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sanitizeWorkflowForUser(workflow, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) GetVoterWorkflows(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	a.expireStaleWorkflowProposalsAndNotify(r.Context())

	workflows, err := a.db.GetVoterWorkflows(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting voter workflows for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for idx, workflow := range workflows {
		if workflow.Status != "pending" {
			continue
		}

		allowApproval, err := a.workflowApprovalAllowed(r.Context(), workflow)
		if err != nil {
			a.logger.Logf("error checking faucet allocation for workflow %s vote evaluation: %s", workflow.Id, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		evaluatedWorkflow, err := a.db.EvaluateWorkflowVoteStateWithApproval(r.Context(), workflow.Id, allowApproval)
		if err != nil {
			a.logger.Logf("error evaluating workflow vote state %s for voter %s: %s", workflow.Id, *userDid, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if workflow.Status == "pending" && evaluatedWorkflow.Status != "pending" {
			a.sendWorkflowProposalOutcomeEmailByWorkflow(r.Context(), evaluatedWorkflow.Id)
		}
		votes, err := a.db.GetWorkflowVotesForUser(r.Context(), workflow.Id, *userDid)
		if err == nil {
			evaluatedWorkflow.Votes = *votes
		}
		workflows[idx] = evaluatedWorkflow
	}
	sanitizeWorkflowListForUser(workflows, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflows)
}

func (a *AppService) VoteWorkflow(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	isAdmin := a.IsAdmin(r.Context(), *userDid)

	workflowId := strings.TrimSpace(r.PathValue("workflow_id"))
	if workflowId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req structs.WorkflowVoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	req.Decision = strings.ToLower(strings.TrimSpace(req.Decision))
	if req.Decision != "approve" && req.Decision != "deny" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	a.expireStaleWorkflowProposalsAndNotify(r.Context())

	workflow, err := a.db.GetWorkflowForApproval(r.Context(), workflowId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading workflow %s for voting: %s", workflowId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if workflow.Status != "pending" {
		sanitizeWorkflowForUser(workflow, *userDid, isAdmin)
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(workflow)
		return
	}

	_, err = a.db.RecordWorkflowVote(r.Context(), workflowId, *userDid, req.Decision, req.Comment)
	if err != nil {
		a.logger.Logf("error recording workflow vote for workflow %s voter %s: %s", workflowId, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	allowApproval, err := a.workflowApprovalAllowed(r.Context(), workflow)
	if err != nil {
		a.logger.Logf("error checking faucet allocation for workflow %s vote: %s", workflowId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	updatedWorkflow, err := a.db.EvaluateWorkflowVoteStateWithApproval(r.Context(), workflowId, allowApproval)
	if err != nil {
		a.logger.Logf("error evaluating workflow vote state for workflow %s: %s", workflowId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	votes, err := a.db.GetWorkflowVotesForUser(r.Context(), workflowId, *userDid)
	if err == nil {
		updatedWorkflow.Votes = *votes
	}

	if workflow.Status == "pending" && updatedWorkflow.Status != "pending" {
		a.sendWorkflowProposalOutcomeEmailByWorkflow(r.Context(), updatedWorkflow.Id)
	}
	sanitizeWorkflowForUser(updatedWorkflow, *userDid, isAdmin)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(updatedWorkflow)
}

func (a *AppService) AdminForceApproveWorkflow(w http.ResponseWriter, r *http.Request) {
	adminId := utils.GetDid(r)
	if adminId == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	workflowId := strings.TrimSpace(r.PathValue("workflow_id"))
	if workflowId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	a.expireStaleWorkflowProposalsAndNotify(r.Context())

	workflow, err := a.db.GetWorkflowForApproval(r.Context(), workflowId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading workflow %s for admin force approve: %s", workflowId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if workflow.Status != "pending" {
		sanitizeWorkflowForUser(workflow, *adminId, true)
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(workflow)
		return
	}

	allowApproval, err := a.workflowApprovalAllowed(r.Context(), workflow)
	if err != nil {
		a.logger.Logf("error checking faucet allocation for admin force approval on workflow %s: %s", workflowId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !allowApproval {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "insufficient faucet balance to approve workflow",
		})
		return
	}

	if err := a.db.ForceApproveWorkflowAsAdmin(r.Context(), workflowId, *adminId); err != nil {
		a.logger.Logf("error force approving workflow %s by admin %s: %s", workflowId, *adminId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	updatedWorkflow, err := a.db.GetWorkflowByID(r.Context(), workflowId)
	if err != nil {
		a.logger.Logf("error loading workflow %s after admin force approve: %s", workflowId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.sendWorkflowProposalOutcomeEmailByWorkflow(r.Context(), updatedWorkflow.Id)
	sanitizeWorkflowForUser(updatedWorkflow, *adminId, true)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(updatedWorkflow)
}

func (a *AppService) ResolveAdminWorkflowPayoutLock(w http.ResponseWriter, r *http.Request) {
	adminId := utils.GetDid(r)
	if adminId == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	workflowId := strings.TrimSpace(r.PathValue("workflow_id"))
	if workflowId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading admin workflow payout resolution body for workflow %s: %s", workflowId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := structs.AdminWorkflowPayoutResolutionRequest{}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	if err := a.db.ResolveWorkflowPayoutLockByAdmin(r.Context(), *adminId, workflowId, &req); err != nil {
		errMsg := err.Error()
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "not allowed") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not currently locked") ||
			strings.Contains(errMsg, "already marked paid out") ||
			strings.Contains(errMsg, "requires completed") ||
			strings.Contains(errMsg, "not applicable") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf(
			"error resolving admin payout lock for workflow %s by admin %s target %s step %s action %s: %s",
			workflowId,
			*adminId,
			strings.TrimSpace(req.TargetType),
			strings.TrimSpace(req.StepId),
			strings.TrimSpace(req.Action),
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if strings.EqualFold(strings.TrimSpace(req.Action), "mark_paid_out") {
		if _, err := a.db.FinalizeWorkflowPaidOutIfSettled(r.Context(), workflowId); err != nil {
			a.logger.Logf("error finalizing workflow %s after admin payout resolution: %s", workflowId, err)
		}
	}

	workflow, err := a.db.GetWorkflowByID(r.Context(), workflowId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading workflow %s after admin payout resolution: %s", workflowId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	sanitizeWorkflowForUser(workflow, *adminId, true)

	a.logger.Logf(
		"admin payout resolution applied: admin=%s workflow=%s target=%s step=%s action=%s",
		*adminId,
		workflowId,
		strings.TrimSpace(req.TargetType),
		strings.TrimSpace(req.StepId),
		strings.TrimSpace(req.Action),
	)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) GetActiveWorkflows(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	canView := a.IsAdmin(r.Context(), *userDid) || a.IsProposer(r.Context(), *userDid) || a.IsImprover(r.Context(), *userDid) || a.IsVoter(r.Context(), *userDid) || a.IsSupervisor(r.Context(), *userDid)
	if !canView {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	workflows, err := a.db.GetActiveWorkflows(r.Context())
	if err != nil {
		a.logger.Logf("error loading active workflows for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflows)
}

func (a *AppService) GetAdminWorkflows(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	count, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("count")))
	if count <= 0 {
		count = 20
	}
	includeArchived := false
	includeArchivedRaw := strings.TrimSpace(r.URL.Query().Get("include_archived"))
	if includeArchivedRaw != "" {
		parsed, parseErr := strconv.ParseBool(includeArchivedRaw)
		if parseErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("invalid include_archived value"))
			return
		}
		includeArchived = parsed
	}

	response, err := a.db.GetAdminWorkflows(r.Context(), search, page, count, includeArchived)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "invalid") || strings.Contains(errMsg, "required") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error loading admin workflows: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (a *AppService) GetAdminWorkflowSeriesClaimants(w http.ResponseWriter, r *http.Request) {
	seriesId := strings.TrimSpace(r.PathValue("series_id"))
	if seriesId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	claimants, err := a.db.GetWorkflowSeriesClaimants(r.Context(), seriesId)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error loading workflow series claimants for %s: %s", seriesId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(claimants)
}

func (a *AppService) RevokeAdminWorkflowSeriesImproverClaim(w http.ResponseWriter, r *http.Request) {
	seriesId := strings.TrimSpace(r.PathValue("series_id"))
	if seriesId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading admin workflow series revoke body for %s: %s", seriesId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.WorkflowSeriesClaimRevokeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	result, err := a.db.AdminRevokeWorkflowSeriesImproverClaims(r.Context(), seriesId, req.ImproverUserId)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "no claimed workflow assignments") || strings.Contains(errMsg, "no claimable assignments") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error revoking workflow series claim for %s improver %s: %s", seriesId, req.ImproverUserId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

func (a *AppService) ProposeWorkflowDeletion(w http.ResponseWriter, r *http.Request) {
	requesterId := utils.GetDid(r)
	if requesterId == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if !a.IsAdmin(r.Context(), *requesterId) && !a.IsProposer(r.Context(), *requesterId) && !a.IsVoter(r.Context(), *requesterId) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading workflow deletion proposal body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.WorkflowDeletionProposalCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	proposal, err := a.db.CreateWorkflowDeletionProposal(r.Context(), *requesterId, &req)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") ||
			strings.Contains(errMsg, "invalid") ||
			strings.Contains(errMsg, "active") ||
			strings.Contains(errMsg, "already exists") ||
			strings.Contains(errMsg, "not allowed") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not approved") || strings.Contains(errMsg, "not authorized") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not found") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error creating workflow deletion proposal for requester %s: %s", *requesterId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(proposal)
}

func (a *AppService) GetVoterWorkflowDeletionProposals(w http.ResponseWriter, r *http.Request) {
	voterId := utils.GetDid(r)
	if voterId == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	proposals, err := a.db.GetWorkflowDeletionProposalsForVoter(r.Context(), *voterId)
	if err != nil {
		a.logger.Logf("error loading workflow deletion proposals for voter %s: %s", *voterId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(proposals)
}

func (a *AppService) VoteWorkflowDeletionProposal(w http.ResponseWriter, r *http.Request) {
	voterId := utils.GetDid(r)
	if voterId == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	proposalId := strings.TrimSpace(r.PathValue("proposal_id"))
	if proposalId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading workflow deletion vote body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.WorkflowDeletionProposalVoteRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	req.Decision = strings.ToLower(strings.TrimSpace(req.Decision))
	if req.Decision != "approve" && req.Decision != "deny" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	proposal, err := a.db.GetWorkflowDeletionProposalByIDForUser(r.Context(), proposalId, voterId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error loading workflow deletion proposal %s for vote: %s", proposalId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if proposal.Status != "pending" {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(proposal)
		return
	}

	if _, err := a.db.RecordWorkflowDeletionVote(r.Context(), proposalId, *voterId, req.Decision, req.Comment); err != nil {
		a.logger.Logf("error recording workflow deletion vote %s by %s: %s", proposalId, *voterId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	updatedProposal, err := a.db.EvaluateWorkflowDeletionVoteState(r.Context(), proposalId)
	if err != nil {
		a.logger.Logf("error evaluating workflow deletion vote state %s: %s", proposalId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	proposalWithVotes, err := a.db.GetWorkflowDeletionProposalByIDForUser(r.Context(), proposalId, voterId)
	if err == nil {
		updatedProposal.Votes = proposalWithVotes.Votes
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(updatedProposal)
}

func (a *AppService) GetIssuers(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 {
		count = 20
	}
	issuers, err := a.db.GetIssuersWithScopes(r.Context(), search, page, count)
	if err != nil {
		a.logger.Logf("error getting issuers: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(issuers)
}

func (a *AppService) UpdateIssuerScopes(w http.ResponseWriter, r *http.Request) {
	adminId := utils.GetDid(r)
	if adminId == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading issuer scope update body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.IssuerScopeUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	issuer, err := a.db.SetIssuerScopes(r.Context(), *adminId, &req)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not found") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error updating issuer scopes for %s: %s", req.UserId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(issuer)
}

func (a *AppService) GetCredentialTypes(w http.ResponseWriter, r *http.Request) {
	credentialTypes, err := a.db.GetGlobalCredentialTypes(r.Context())
	if err != nil {
		a.logger.Logf("error getting credential types: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(credentialTypes)
}

func (a *AppService) GetImproverCredentialRequests(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	requests, err := a.db.GetCredentialRequestsByUser(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting credential requests for improver %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(requests)
}

func (a *AppService) CreateImproverCredentialRequest(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading improver credential request body for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.CredentialRequestCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	created, err := a.db.CreateCredentialRequest(r.Context(), *userDid, req.CredentialType)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "already active") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not found") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error creating improver credential request for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.sendCredentialRequestEmails(r.Context(), *created)

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(created)
}

func (a *AppService) GetIssuerCredentialRequests(w http.ResponseWriter, r *http.Request) {
	issuerId := utils.GetDid(r)
	if issuerId == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 {
		count = 20
	}

	requests, err := a.db.GetCredentialRequestsForIssuer(r.Context(), *issuerId, search, page, count)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "role required") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error getting credential requests for issuer %s: %s", *issuerId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(requests)
}

func (a *AppService) DeleteProposerWorkflowTemplate(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	templateId := strings.TrimSpace(r.PathValue("template_id"))
	if templateId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	isAdmin := a.IsAdmin(r.Context(), *userDid)
	if err := a.db.DeleteWorkflowTemplate(r.Context(), templateId, *userDid, isAdmin); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) DecideIssuerCredentialRequest(w http.ResponseWriter, r *http.Request) {
	issuerId := utils.GetDid(r)
	if issuerId == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	requestId := strings.TrimSpace(r.PathValue("request_id"))
	if requestId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading issuer credential request decision body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.CredentialRequestDecisionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	statusInput := strings.TrimSpace(req.Status)
	if statusInput == "" {
		statusInput = strings.TrimSpace(req.Decision)
	}

	updated, err := a.db.ResolveCredentialRequest(r.Context(), *issuerId, requestId, statusInput)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid decision") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "role required") || strings.Contains(errMsg, "not allowed") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not found") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "pending credential request already exists") {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error resolving credential request %s by issuer %s: %s", requestId, *issuerId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(updated)
}

func (a *AppService) GetMyIssuerScopes(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	isAdmin := a.IsAdmin(r.Context(), *userDid)
	isIssuer := isAdmin || a.IsIssuer(r.Context(), *userDid)
	allowed := []string{}
	if isAdmin {
		allTypes, err := a.db.GetGlobalCredentialTypes(r.Context())
		if err != nil {
			a.logger.Logf("error getting credential types for admin %s: %s", *userDid, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, t := range allTypes {
			allowed = append(allowed, t.Value)
		}
	} else {
		scopes, err := a.db.GetIssuerScopeCredentials(r.Context(), *userDid)
		if err != nil {
			a.logger.Logf("error getting issuer scopes for %s: %s", *userDid, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		allowed = scopes
	}

	resp := structs.IssuerWithScopes{
		UserId:             *userDid,
		IsIssuer:           isIssuer,
		AllowedCredentials: allowed,
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *AppService) IssueCredential(w http.ResponseWriter, r *http.Request) {
	issuerId := utils.GetDid(r)
	if issuerId == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading issue credential body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.CredentialIssueRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	credential, err := a.db.IssueCredential(r.Context(), *issuerId, &req)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not allowed") || strings.Contains(errMsg, "role required") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not found") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error issuing credential %s to %s by %s: %s", req.CredentialType, req.UserId, *issuerId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(credential)
}

func (a *AppService) RevokeCredential(w http.ResponseWriter, r *http.Request) {
	issuerId := utils.GetDid(r)
	if issuerId == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading revoke credential body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.CredentialIssueRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = a.db.RevokeCredential(r.Context(), *issuerId, &req)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not allowed") || strings.Contains(errMsg, "role required") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(errMsg))
			return
		}
		if strings.Contains(errMsg, "not found") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error revoking credential %s from %s by %s: %s", req.CredentialType, req.UserId, *issuerId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *AppService) GetIssuerUserCredentials(w http.ResponseWriter, r *http.Request) {
	userId := strings.TrimSpace(r.PathValue("user_id"))
	if userId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	credentials, err := a.db.GetUserCredentials(r.Context(), userId)
	if err != nil {
		a.logger.Logf("error getting credentials for user %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(credentials)
}

func (a *AppService) workflowApprovalAllowed(ctx context.Context, workflow *structs.Workflow) (bool, error) {
	if workflow == nil {
		return false, fmt.Errorf("workflow is required")
	}
	if a.bot == nil {
		a.logger.Logf("workflow vote approval checks are disabled because bot service is not configured")
		return false, nil
	}

	unallocatedTokens, err := a.bot.unallocatedBalanceTokens(ctx)
	if err != nil {
		return false, err
	}
	weeklyRequirement := new(big.Int).SetUint64(workflow.WeeklyBountyRequirement)
	return unallocatedTokens.Cmp(weeklyRequirement) >= 0, nil
}

func (a *AppService) expireStaleWorkflowProposalsAndNotify(ctx context.Context) {
	expiredNotices, err := a.db.ExpireStaleWorkflowProposals(ctx)
	if err != nil {
		a.logger.Logf("error expiring stale workflow proposals: %s", err)
		return
	}

	for _, notice := range expiredNotices {
		a.sendWorkflowProposalExpiredEmail(notice)
	}
}

func (a *AppService) sendWorkflowProposalOutcomeEmailByWorkflow(ctx context.Context, workflowId string) {
	notification, err := a.db.GetWorkflowProposalOutcomeNotification(ctx, workflowId)
	if err != nil {
		if strings.Contains(err.Error(), "not finalized") {
			return
		}
		a.logger.Logf("error building workflow proposal outcome notification for %s: %s", workflowId, err)
		return
	}
	a.sendWorkflowProposalOutcomeEmail(*notification)
}

func (a *AppService) sendWorkflowProposalOutcomeEmail(notification structs.WorkflowProposalOutcomeNotification) {
	toEmail := strings.TrimSpace(notification.ProposerEmail)
	if toEmail == "" {
		return
	}

	emailSender := utils.NewEmailSender()
	if emailSender == nil {
		return
	}

	fromDomain := os.Getenv("MAILGUN_DOMAIN")
	fromEmail := "no_reply@sfluv.org"
	if fromDomain != "" {
		fromEmail = "no_reply@" + fromDomain
	}

	outcomeLabel := "approved"
	subtitle := "Your workflow proposal has been approved."
	title := "Workflow Proposal Approved"
	if notification.Decision == "rejected" {
		outcomeLabel = "rejected"
		subtitle = "Your workflow proposal has been rejected."
		title = "Workflow Proposal Rejected"
	}
	htmlContent := utils.BuildStyledEmail(
		title,
		subtitle,
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:160px;">Workflow</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Workflow ID</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Outcome</td>
    <td style="padding:12px 0; font-size:13px; color:#111827; font-weight:600;">%s</td>
  </tr>
</table>`, notification.WorkflowTitle, notification.WorkflowId, strings.ToUpper(outcomeLabel)),
	)

	if err := emailSender.SendEmail(toEmail, "Proposer", title, htmlContent, fromEmail, "SFLuv Workflows"); err != nil {
		a.logger.Logf("error sending workflow proposal outcome email for %s: %s", notification.WorkflowId, err)
	}
}

func (a *AppService) sendWorkflowProposalExpiredEmail(notification structs.WorkflowProposalExpiryNotice) {
	toEmail := strings.TrimSpace(notification.ProposerEmail)
	if toEmail == "" {
		return
	}

	emailSender := utils.NewEmailSender()
	if emailSender == nil {
		return
	}

	fromDomain := os.Getenv("MAILGUN_DOMAIN")
	fromEmail := "no_reply@sfluv.org"
	if fromDomain != "" {
		fromEmail = "no_reply@" + fromDomain
	}

	title := "Workflow Proposal Expired"
	htmlContent := utils.BuildStyledEmail(
		title,
		"Your workflow proposal expired before reaching a voting decision.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:160px;">Workflow</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Workflow ID</td>
    <td style="padding:12px 0; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
</table>`, notification.WorkflowTitle, notification.WorkflowId),
	)

	if err := emailSender.SendEmail(toEmail, "Proposer", title, htmlContent, fromEmail, "SFLuv Workflows"); err != nil {
		a.logger.Logf("error sending workflow proposal expiry email for %s: %s", notification.WorkflowId, err)
	}
}

func (a *AppService) sendRoleRequestEmail(envKey string, title string, subtitle string, details string) {
	adminEmail := strings.TrimSpace(os.Getenv(envKey))
	if adminEmail == "" {
		adminEmail = strings.TrimSpace(os.Getenv("AFFILIATE_ADMIN_EMAIL"))
	}
	if adminEmail == "" {
		return
	}

	emailSender := utils.NewEmailSender()
	if emailSender == nil {
		return
	}

	fromDomain := os.Getenv("MAILGUN_DOMAIN")
	fromEmail := "no_reply@sfluv.org"
	if fromDomain != "" {
		fromEmail = "no_reply@" + fromDomain
	}

	htmlContent := utils.BuildStyledEmail(title, subtitle, details)
	if err := emailSender.SendEmail(adminEmail, "Admin", title, htmlContent, fromEmail, "SFLuv Workflows"); err != nil {
		a.logger.Logf("error sending %s email: %s", title, err.Error())
	}
}

func (a *AppService) sendCredentialRequestEmails(ctx context.Context, request structs.CredentialRequest) {
	recipients, err := a.db.GetIssuersAllowedForCredential(ctx, request.CredentialType)
	if err != nil {
		a.logger.Logf("error loading issuer recipients for credential request %s: %s", request.Id, err)
		return
	}
	if len(recipients) == 0 {
		return
	}

	emailSender := utils.NewEmailSender()
	if emailSender == nil {
		return
	}

	fromDomain := os.Getenv("MAILGUN_DOMAIN")
	fromEmail := "no_reply@sfluv.org"
	if fromDomain != "" {
		fromEmail = "no_reply@" + fromDomain
	}

	credentialLabel := request.CredentialType
	types, err := a.db.GetGlobalCredentialTypes(ctx)
	if err == nil {
		for _, ct := range types {
			if ct.Value == request.CredentialType {
				credentialLabel = ct.Label
				break
			}
		}
	}

	title := "New Credential Request"
	htmlContent := utils.BuildStyledEmail(
		title,
		"A user requested a credential your issuer account can grant.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:170px;">Requester</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Requester Email</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Credential</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s (%s)</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Request ID</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Requester User ID</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Requested At (UTC)</td>
    <td style="padding:12px 0; font-size:13px; color:#111827;">%s</td>
  </tr>
</table>`,
			request.RequesterName,
			request.RequesterEmail,
			credentialLabel,
			request.CredentialType,
			request.Id,
			request.UserId,
			request.RequestedAt.UTC().Format(time.RFC3339),
		),
	)

	sentEmails := map[string]struct{}{}
	for _, recipient := range recipients {
		toEmail := strings.TrimSpace(recipient.Email)
		if toEmail == "" {
			continue
		}
		if _, exists := sentEmails[toEmail]; exists {
			continue
		}
		sentEmails[toEmail] = struct{}{}

		recipientName := strings.TrimSpace(recipient.Name)
		if recipientName == "" {
			recipientName = "Issuer"
		}

		if err := emailSender.SendEmail(toEmail, recipientName, title, htmlContent, fromEmail, "SFLuv Workflows"); err != nil {
			a.logger.Logf("error sending credential request email %s to issuer %s: %s", request.Id, recipient.UserId, err.Error())
		}
	}
}

func (a *AppService) sendWorkflowSeriesFundingShortfallEmails(ctx context.Context, checks []structs.WorkflowSeriesStartFundingCheck) {
	if len(checks) == 0 {
		return
	}
	if a.bot == nil {
		a.logger.Logf("workflow series funding checks skipped: bot service is not configured")
		return
	}

	unallocatedTokens, err := a.bot.unallocatedBalanceTokens(ctx)
	if err != nil {
		a.logger.Logf("error getting unallocated faucet balance for workflow series funding checks: %s", err)
		return
	}

	for _, check := range checks {
		requiredTokens := new(big.Int).SetUint64(check.TotalBounty)
		if unallocatedTokens.Cmp(requiredTokens) >= 0 {
			continue
		}

		shortfallTokens := new(big.Int).Sub(requiredTokens, unallocatedTokens)
		a.sendRoleRequestEmail(
			"WORKFLOW_ADMIN_EMAIL",
			"Workflow Series Funding Shortfall",
			"A series workflow reached start time with insufficient unallocated faucet balance.",
			fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:170px;">Workflow</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Workflow ID</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Series ID</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Recurrence</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Start At (UTC)</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Required Unallocated</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s SFLuv</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Current Unallocated</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s SFLuv</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Amount Needed</td>
    <td style="padding:12px 0; font-size:13px; color:#111827; font-weight:600;">%s SFLuv</td>
  </tr>
</table>`, check.WorkflowTitle, check.WorkflowId, check.SeriesId, check.Recurrence, time.Unix(check.StartAt, 0).UTC().Format(time.RFC3339), requiredTokens.String(), unallocatedTokens.String(), shortfallTokens.String()),
		)
	}
}

type workflowPayoutTarget struct {
	WorkflowId    string
	WorkflowTitle string
	SeriesId      string
	StepId        string
	StepTitle     string
	StepOrder     int
	IsManager     bool
	ImproverId    string
	Amount        uint64
}

func collectWorkflowPayoutTargets(workflow *structs.Workflow) []workflowPayoutTarget {
	if workflow == nil || workflow.Status != "completed" {
		return []workflowPayoutTarget{}
	}

	steps := append([]structs.WorkflowStep{}, workflow.Steps...)
	sort.SliceStable(steps, func(i int, j int) bool {
		return steps[i].StepOrder < steps[j].StepOrder
	})

	targets := make([]workflowPayoutTarget, 0, len(steps)+1)
	for _, step := range steps {
		if step.Status != "completed" {
			continue
		}
		if step.PayoutInProgress {
			continue
		}
		manualRetryRequired := step.PayoutError != nil && strings.TrimSpace(*step.PayoutError) != ""
		if manualRetryRequired && step.RetryRequestedAt == nil {
			continue
		}
		improverId := ""
		if step.AssignedImproverId != nil {
			improverId = strings.TrimSpace(*step.AssignedImproverId)
		}
		targets = append(targets, workflowPayoutTarget{
			WorkflowId:    workflow.Id,
			WorkflowTitle: workflow.Title,
			SeriesId:      workflow.SeriesId,
			StepId:        step.Id,
			StepTitle:     step.Title,
			StepOrder:     step.StepOrder,
			IsManager:     false,
			ImproverId:    improverId,
			Amount:        step.Bounty,
		})
	}

	managerRetryRequired := workflow.ManagerPayoutError != nil && strings.TrimSpace(*workflow.ManagerPayoutError) != ""
	if workflow.ManagerBounty > 0 &&
		workflow.ManagerImproverId != nil &&
		workflow.ManagerPaidOutAt == nil &&
		!workflow.ManagerPayoutInProgress &&
		(!managerRetryRequired || workflow.ManagerRetryRequestedAt != nil) {
		targets = append(targets, workflowPayoutTarget{
			WorkflowId:    workflow.Id,
			WorkflowTitle: workflow.Title,
			SeriesId:      workflow.SeriesId,
			IsManager:     true,
			ImproverId:    strings.TrimSpace(*workflow.ManagerImproverId),
			Amount:        workflow.ManagerBounty,
		})
	}

	return targets
}

func (a *AppService) attemptWorkflowPayoutTransfer(ctx context.Context, amount uint64, walletAddress string) (*big.Int, *big.Int, bool, error) {
	neededTokens := new(big.Int).SetUint64(amount)

	if a.bot == nil || a.bot.bot == nil {
		return nil, neededTokens, false, fmt.Errorf("bot service is not configured")
	}

	faucetBalanceWei, err := a.bot.bot.Balance()
	if err != nil {
		return nil, neededTokens, false, fmt.Errorf("error checking faucet balance: %s", err)
	}

	multiplier, err := getTokenMultiplier()
	if err != nil {
		return nil, neededTokens, false, fmt.Errorf("error reading token decimals: %s", err)
	}

	currentTokens := new(big.Int).Div(faucetBalanceWei, multiplier)
	if currentTokens.Cmp(neededTokens) < 0 {
		return currentTokens, neededTokens, true, fmt.Errorf("insufficient faucet balance for workflow payout")
	}

	if err := a.bot.bot.Send(amount, walletAddress); err != nil {
		errLower := strings.ToLower(err.Error())
		isInsufficient := strings.Contains(errLower, "insufficient")
		return currentTokens, neededTokens, isInsufficient, err
	}

	return currentTokens, neededTokens, false, nil
}

func (a *AppService) sendWorkflowPayoutErrorEmail(
	ctx context.Context,
	target workflowPayoutTarget,
	walletAddress string,
	errorMessage string,
	currentBalance *big.Int,
	neededBalance *big.Int,
	isInsufficient bool,
) {
	errorMessage = strings.TrimSpace(errorMessage)
	if errorMessage == "" {
		errorMessage = "Unknown workflow payout error"
	}

	improverName := "Improver"
	improverEmail := ""
	if target.ImproverId != "" {
		if improver, err := a.db.GetImproverByUser(ctx, target.ImproverId); err == nil && improver != nil {
			fullName := strings.TrimSpace(improver.FirstName + " " + improver.LastName)
			if fullName != "" {
				improverName = fullName
			}
			improverEmail = strings.TrimSpace(improver.Email)
		}
	}

	targetLabel := "Workflow Step"
	targetDetails := fmt.Sprintf("Step %d: %s", target.StepOrder, target.StepTitle)
	if target.IsManager {
		targetLabel = "Workflow Manager"
		targetDetails = "Workflow manager completion payout"
	}

	extraRows := ""
	if isInsufficient && currentBalance != nil && neededBalance != nil {
		shortfall := new(big.Int).Sub(new(big.Int).Set(neededBalance), currentBalance)
		if shortfall.Sign() < 0 {
			shortfall = big.NewInt(0)
		}
		extraRows = fmt.Sprintf(`
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Current Faucet Balance</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s SFLuv</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Needed For This Payout</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s SFLuv</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Amount Needed</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; font-weight:600;">%s SFLuv</td>
  </tr>`, currentBalance.String(), neededBalance.String(), shortfall.String())
	}

	a.sendRoleRequestEmail(
		"WORKFLOW_ADMIN_EMAIL",
		"Workflow Payout Error",
		"A workflow payout attempt failed.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:180px;">Workflow</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Workflow ID</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Series ID</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Target</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Target Details</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Improver</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s (%s)</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Improver Email</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Payout Wallet</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Payout Amount</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%d SFLuv</td>
  </tr>%s
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Error</td>
    <td style="padding:12px 0; font-size:13px; color:#111827; white-space:pre-wrap;">%s</td>
  </tr>
</table>`,
			target.WorkflowTitle,
			target.WorkflowId,
			target.SeriesId,
			targetLabel,
			targetDetails,
			improverName,
			target.ImproverId,
			improverEmail,
			strings.TrimSpace(walletAddress),
			target.Amount,
			extraRows,
			errorMessage,
		),
	)
}

func (a *AppService) processWorkflowSeriesPayouts(ctx context.Context, triggerWorkflowID string) {
	triggerWorkflowID = strings.TrimSpace(triggerWorkflowID)
	if triggerWorkflowID == "" {
		return
	}

	if a.bot == nil || a.bot.bot == nil {
		a.logger.Logf("workflow payout processing skipped for %s: bot service is not configured", triggerWorkflowID)
		return
	}

	workflowIDs, err := a.db.GetWorkflowSeriesOrderedIDs(ctx, triggerWorkflowID)
	if err != nil {
		a.logger.Logf("error loading workflow series order for payout processing %s: %s", triggerWorkflowID, err)
		return
	}

	for _, workflowID := range workflowIDs {
		workflow, err := a.db.GetWorkflowByID(ctx, workflowID)
		if err != nil {
			a.logger.Logf("error loading workflow %s during payout processing: %s", workflowID, err)
			return
		}

		switch workflow.Status {
		case "deleted", "rejected", "expired", "paid_out":
			continue
		case "completed":
			targets := collectWorkflowPayoutTargets(workflow)
			for _, target := range targets {
				if target.Amount == 0 {
					var markErr error
					if target.IsManager {
						_, markErr = a.db.MarkWorkflowManagerPaidOut(ctx, target.WorkflowId)
					} else {
						_, markErr = a.db.MarkWorkflowStepPaidOut(ctx, target.WorkflowId, target.StepId)
					}
					if markErr != nil {
						a.logger.Logf("error auto-settling zero-value workflow payout target workflow %s step %s manager %t: %s", target.WorkflowId, target.StepId, target.IsManager, markErr)
						return
					}
					continue
				}

				if target.IsManager {
					claimed, claimErr := a.db.ClaimWorkflowManagerPayoutAttempt(ctx, target.WorkflowId)
					if claimErr != nil {
						a.logger.Logf("error claiming manager payout attempt lock for workflow %s: %s", target.WorkflowId, claimErr)
						return
					}
					if !claimed {
						continue
					}
				} else {
					claimed, claimErr := a.db.ClaimWorkflowStepPayoutAttempt(ctx, target.WorkflowId, target.StepId)
					if claimErr != nil {
						a.logger.Logf("error claiming step payout attempt lock for workflow %s step %s: %s", target.WorkflowId, target.StepId, claimErr)
						return
					}
					if !claimed {
						continue
					}
				}

				walletAddress := ""
				if target.ImproverId == "" {
					errMsg := workflowPayoutErrorNoImproverAssigned
					if target.IsManager {
						if dbErr := a.db.MarkWorkflowManagerPayoutFailed(ctx, target.WorkflowId, errMsg); dbErr != nil {
							a.logger.Logf("error recording manager payout assignment failure for workflow %s: %s", target.WorkflowId, dbErr)
						}
					} else {
						if dbErr := a.db.MarkWorkflowStepPayoutFailed(ctx, target.WorkflowId, target.StepId, errMsg); dbErr != nil {
							a.logger.Logf("error recording step payout assignment failure for workflow %s step %s: %s", target.WorkflowId, target.StepId, dbErr)
						}
					}
					a.sendWorkflowPayoutErrorEmail(ctx, target, walletAddress, errMsg, nil, nil, false)
					return
				}

				walletAddress, err = a.db.GetPreferredWorkflowPayoutAddressForUser(ctx, target.ImproverId, target.IsManager)
				if err != nil {
					errMsg := workflowPayoutErrorNoPayoutWallet
					if target.IsManager {
						if dbErr := a.db.MarkWorkflowManagerPayoutFailed(ctx, target.WorkflowId, errMsg); dbErr != nil {
							a.logger.Logf("error recording manager payout wallet failure for workflow %s: %s", target.WorkflowId, dbErr)
						}
					} else {
						if dbErr := a.db.MarkWorkflowStepPayoutFailed(ctx, target.WorkflowId, target.StepId, errMsg); dbErr != nil {
							a.logger.Logf("error recording step payout wallet failure for workflow %s step %s: %s", target.WorkflowId, target.StepId, dbErr)
						}
					}
					a.sendWorkflowPayoutErrorEmail(ctx, target, walletAddress, fmt.Sprintf("%s Detail: %s", errMsg, err), nil, nil, false)
					return
				}

				currentBalance, neededBalance, insufficient, transferErr := a.attemptWorkflowPayoutTransfer(ctx, target.Amount, walletAddress)
				if transferErr != nil {
					errMsg := workflowPayoutErrorTransferFailed
					if insufficient {
						errMsg = workflowPayoutErrorInsufficientFaucet
					}
					if target.IsManager {
						if dbErr := a.db.MarkWorkflowManagerPayoutFailed(ctx, target.WorkflowId, errMsg); dbErr != nil {
							a.logger.Logf("error recording manager payout transfer failure for workflow %s: %s", target.WorkflowId, dbErr)
						}
					} else {
						if dbErr := a.db.MarkWorkflowStepPayoutFailed(ctx, target.WorkflowId, target.StepId, errMsg); dbErr != nil {
							a.logger.Logf("error recording step payout transfer failure for workflow %s step %s: %s", target.WorkflowId, target.StepId, dbErr)
						}
					}
					a.sendWorkflowPayoutErrorEmail(ctx, target, walletAddress, fmt.Sprintf("%s Detail: %s", errMsg, transferErr), currentBalance, neededBalance, insufficient)
					return
				}

				if target.IsManager {
					if _, err := a.db.MarkWorkflowManagerPaidOut(ctx, target.WorkflowId); err != nil {
						errMsg := fmt.Sprintf("workflow payout post-transfer state update failed: %s", err)
						a.logger.Logf("error marking manager payout complete for workflow %s: %s", target.WorkflowId, err)
						a.sendWorkflowPayoutErrorEmail(ctx, target, walletAddress, errMsg, nil, nil, false)
						return
					}
				} else {
					if _, err := a.db.MarkWorkflowStepPaidOut(ctx, target.WorkflowId, target.StepId); err != nil {
						errMsg := fmt.Sprintf("workflow payout post-transfer state update failed: %s", err)
						a.logger.Logf("error marking step payout complete for workflow %s step %s: %s", target.WorkflowId, target.StepId, err)
						a.sendWorkflowPayoutErrorEmail(ctx, target, walletAddress, errMsg, nil, nil, false)
						return
					}
				}
			}

			settled, err := a.db.FinalizeWorkflowPaidOutIfSettled(ctx, workflowID)
			if err != nil {
				a.logger.Logf("error finalizing workflow paid_out status for workflow %s: %s", workflowID, err)
				return
			}
			if !settled {
				return
			}
		default:
			// Stop processing here so later workflows in the same series remain held
			// until all earlier workflows are fully finished and paid.
			return
		}
	}
}

func (a *AppService) sendWorkflowStepAvailableEmail(notification structs.WorkflowStepAvailabilityNotification) {
	toEmail := strings.TrimSpace(notification.Email)
	if toEmail == "" {
		return
	}

	emailSender := utils.NewEmailSender()
	if emailSender == nil {
		return
	}

	fromDomain := os.Getenv("MAILGUN_DOMAIN")
	fromEmail := "no_reply@sfluv.org"
	if fromDomain != "" {
		fromEmail = "no_reply@" + fromDomain
	}

	recipientName := strings.TrimSpace(notification.Name)
	if recipientName == "" {
		recipientName = "Improver"
	}

	title := "Workflow Step Available"
	htmlContent := utils.BuildStyledEmail(
		title,
		"A step assigned to you is now ready for completion.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:140px;">Workflow</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Step</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Workflow ID</td>
    <td style="padding:12px 0; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
</table>`, notification.WorkflowTitle, notification.StepTitle, notification.WorkflowId),
	)

	if err := emailSender.SendEmail(toEmail, recipientName, title, htmlContent, fromEmail, "SFLuv Workflows"); err != nil {
		a.logger.Logf("error sending step-available email for workflow %s step %s user %s: %s", notification.WorkflowId, notification.StepId, notification.UserId, err.Error())
	}
}

func (a *AppService) sendWorkflowDropdownAlertEmail(notification structs.WorkflowDropdownNotification) {
	if len(notification.Emails) == 0 {
		return
	}

	emailSender := utils.NewEmailSender()
	if emailSender == nil {
		return
	}

	fromDomain := os.Getenv("MAILGUN_DOMAIN")
	fromEmail := "no_reply@sfluv.org"
	if fromDomain != "" {
		fromEmail = "no_reply@" + fromDomain
	}

	title := "Workflow Dropdown Alert"
	htmlContent := utils.BuildStyledEmail(
		title,
		"A watched dropdown response was submitted.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:140px;">Workflow</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Step</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Item</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Response</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Workflow ID</td>
    <td style="padding:12px 0; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
</table>`, notification.WorkflowTitle, notification.StepTitle, notification.ItemTitle, notification.DropdownValue, notification.WorkflowId),
	)

	for _, toEmail := range notification.Emails {
		toEmail = strings.TrimSpace(toEmail)
		if toEmail == "" {
			continue
		}
		if err := emailSender.SendEmail(toEmail, "Workflow Watcher", title, htmlContent, fromEmail, "SFLuv Workflows"); err != nil {
			a.logger.Logf("error sending dropdown-alert email for workflow %s step %s item %s to %s: %s", notification.WorkflowId, notification.StepId, notification.ItemId, toEmail, err.Error())
		}
	}
}

func parseWorkflowStartAt(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("start_at is required")
	}

	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), nil
	}

	// Legacy fallback for naive values: interpret as UTC to avoid server-local timezone drift.
	if parsed, err := time.ParseInLocation("2006-01-02T15:04", value, time.UTC); err == nil {
		return parsed.UTC(), nil
	}

	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.UTC); err == nil {
		return parsed.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("invalid start_at")
}

func parseAbsenceBoundary(value string, isEnd bool) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if isEnd {
			return time.Time{}, fmt.Errorf("absent_until is required")
		}
		return time.Time{}, fmt.Errorf("absent_from is required")
	}

	if parsed, err := time.ParseInLocation("2006-01-02", value, time.UTC); err == nil {
		if isEnd {
			parsed = parsed.Add(24 * time.Hour)
		}
		return parsed.UTC(), nil
	}

	return parseWorkflowStartAt(value)
}

func (a *AppService) IsProposer(ctx context.Context, id string) bool {
	isProposer, err := a.db.IsProposer(ctx, id)
	if err != nil {
		a.logger.Logf("error getting proposer state for user %s: %s", id, err)
		return false
	}
	return isProposer
}

func (a *AppService) IsImprover(ctx context.Context, id string) bool {
	isImprover, err := a.db.IsImprover(ctx, id)
	if err != nil {
		a.logger.Logf("error getting improver state for user %s: %s", id, err)
		return false
	}
	return isImprover
}

func (a *AppService) IsVoter(ctx context.Context, id string) bool {
	isVoter, err := a.db.IsVoter(ctx, id)
	if err != nil {
		a.logger.Logf("error getting voter state for user %s: %s", id, err)
		return false
	}
	return isVoter
}

func (a *AppService) IsIssuer(ctx context.Context, id string) bool {
	isIssuer, err := a.db.IsIssuer(ctx, id)
	if err != nil {
		a.logger.Logf("error getting issuer state for user %s: %s", id, err)
		return false
	}
	return isIssuer
}

func (a *AppService) IsSupervisor(ctx context.Context, id string) bool {
	isSupervisor, err := a.db.IsSupervisor(ctx, id)
	if err != nil {
		a.logger.Logf("error getting supervisor state for user %s: %s", id, err)
		return false
	}
	return isSupervisor
}

func (a *AppService) RequestIssuerStatus(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading issuer request body for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.IssuerRequest
	err = json.Unmarshal(body, &req)
	if err != nil || strings.TrimSpace(req.Organization) == "" || strings.TrimSpace(req.Email) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	issuer, err := a.db.UpsertIssuerRequest(r.Context(), *userDid, req.Organization, req.Email)
	if err != nil {
		if err.Error() == "issuer already approved" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "verified") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		a.logger.Logf("error upserting issuer request for user %s: %s", *userDid, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.sendRoleRequestEmail(
		"ISSUER_ADMIN_EMAIL",
		"New Issuer Request",
		"A user has requested issuer status.",
		fmt.Sprintf(`
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;">
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280; width:140px;">User</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827; word-break:break-all;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#6b7280;">Organization</td>
    <td style="padding:12px 0; border-bottom:1px solid #e5e7eb; font-size:13px; color:#111827;">%s</td>
  </tr>
  <tr>
    <td style="padding:12px 0; font-size:13px; color:#6b7280;">Notification Email</td>
    <td style="padding:12px 0; font-size:13px; color:#111827;">%s</td>
  </tr>
</table>`, issuer.UserId, issuer.Organization, issuer.Email),
	)

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(issuer)
}

func (a *AppService) GetIssuerRequests(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count <= 0 {
		count = 20
	}
	issuers, err := a.db.GetIssuerRequests(r.Context(), search, page, count)
	if err != nil {
		a.logger.Logf("error getting issuer requests: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(issuers)
}

func (a *AppService) UpdateIssuerRequest(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading update issuer request body: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.IssuerUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	issuer, err := a.db.UpdateIssuerRequest(r.Context(), &req)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "invalid") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error updating issuer request %s: %s", req.UserId, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(issuer)
}

func (a *AppService) GetAdminCredentialTypes(w http.ResponseWriter, r *http.Request) {
	types, err := a.db.GetGlobalCredentialTypes(r.Context())
	if err != nil {
		a.logger.Logf("error getting credential types: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(types)
}

func (a *AppService) CreateAdminCredentialType(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.logger.Logf("error reading create credential type body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var req structs.GlobalCredentialTypeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	t, err := a.db.CreateGlobalCredentialType(r.Context(), req.Value, req.Label)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "required") || strings.Contains(errMsg, "already exists") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(errMsg))
			return
		}
		a.logger.Logf("error creating credential type %s: %s", req.Value, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(t)
}

func (a *AppService) DeleteAdminCredentialType(w http.ResponseWriter, r *http.Request) {
	value := strings.TrimSpace(r.PathValue("value"))
	if value == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := a.db.DeleteGlobalCredentialType(r.Context(), value); err != nil {
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "in use") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		a.logger.Logf("error deleting credential type %s: %s", value, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *AppService) GetUserByAddress(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimSpace(r.PathValue("address"))
	if address == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user, err := a.db.GetUserByAddress(r.Context(), address)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("user not found"))
			return
		}
		a.logger.Logf("error looking up user by address %s: %s", address, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"user_id": user.Id,
		"address": user.PayPalEth,
	})
}

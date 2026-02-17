package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

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
	if err != nil || strings.TrimSpace(req.Organization) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	proposer, err := a.db.UpsertProposerRequest(r.Context(), *userDid, req.Organization)
	if err != nil {
		if err.Error() == "proposer already approved" {
			w.WriteHeader(http.StatusConflict)
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
</table>`, proposer.UserId, proposer.Organization),
	)

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(proposer)
}

func (a *AppService) GetProposers(w http.ResponseWriter, r *http.Request) {
	proposers, err := a.db.GetProposers(r.Context())
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
		if strings.Contains(err.Error(), "required") {
			w.WriteHeader(http.StatusBadRequest)
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
	improvers, err := a.db.GetImprovers(r.Context())
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

func (a *AppService) GetProposerBalance(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	balance, err := a.db.GetProposerBalance(r.Context(), *userDid)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		a.logger.Logf("error getting proposer balance for user %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(balance)
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

	if strings.TrimSpace(req.TemplateTitle) == "" || strings.TrimSpace(req.TemplateDescription) == "" {
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

	if strings.TrimSpace(req.TemplateTitle) == "" || strings.TrimSpace(req.TemplateDescription) == "" {
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

	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Description) == "" {
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
		if strings.Contains(errMsg, "insufficient proposer balance") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("insufficient proposer balance"))
			return
		}
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

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) GetProposerWorkflows(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	workflows, err := a.db.GetWorkflowsByProposer(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting workflows for proposer %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflows)
}

func (a *AppService) GetProposerWorkflow(w http.ResponseWriter, r *http.Request) {
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
		if strings.Contains(err.Error(), "cannot be deleted") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		a.logger.Logf("error deleting workflow %s for proposer %s: %s", workflowId, *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *AppService) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowId := strings.TrimSpace(r.PathValue("workflow_id"))
	if workflowId == "" {
		w.WriteHeader(http.StatusBadRequest)
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

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) GetImproverWorkflows(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

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

		isRelevant := false
		hasAssignment := false
		for _, step := range workflow.Steps {
			if step.AssignedImproverId != nil && *step.AssignedImproverId == *userDid {
				isRelevant = true
				hasAssignment = true
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

		if !hasAssignment {
			for idx := range workflow.Steps {
				workflow.Steps[idx].Submission = nil
			}
		}

		if isRelevant {
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

func (a *AppService) ClaimWorkflowStep(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

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
		if strings.Contains(errMsg, "not claimable") || strings.Contains(errMsg, "not available") || strings.Contains(errMsg, "missing a role") || strings.Contains(errMsg, "does not belong") {
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

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) StartWorkflowStep(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

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

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) CompleteWorkflowStep(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

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

	result, err := a.db.CompleteWorkflowStep(r.Context(), workflowId, stepId, *userDid, req.Items)
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

	workflow, err := a.db.GetWorkflowByID(r.Context(), workflowId)
	if err != nil {
		a.logger.Logf("error loading workflow %s after step completion: %s", workflowId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflow)
}

func (a *AppService) GetVoterWorkflows(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	workflows, err := a.db.GetVoterWorkflows(r.Context(), *userDid)
	if err != nil {
		a.logger.Logf("error getting voter workflows for %s: %s", *userDid, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflows)
}

func (a *AppService) GetIssuers(w http.ResponseWriter, r *http.Request) {
	issuers, err := a.db.GetIssuersWithScopes(r.Context())
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
		allowed = []string{
			string(structs.CredentialDPWCertified),
			string(structs.CredentialSFLUVVerifier),
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
</table>`, check.WorkflowTitle, check.WorkflowId, check.SeriesId, check.Recurrence, check.StartAt.UTC().Format(time.RFC3339), requiredTokens.String(), unallocatedTokens.String(), shortfallTokens.String()),
		)
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

	if parsed, err := time.ParseInLocation("2006-01-02T15:04", value, time.Local); err == nil {
		return parsed.UTC(), nil
	}

	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local); err == nil {
		return parsed.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("invalid start_at")
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

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"

	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
)

func (s *BotService) VoteWorkflow(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	req.Decision = strings.ToLower(strings.TrimSpace(req.Decision))
	if req.Decision != "approve" && req.Decision != "deny" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	workflow, err := s.appDb.GetWorkflowForApproval(r.Context(), workflowId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if workflow.Status != "pending" {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(workflow)
		return
	}

	_, err = s.appDb.RecordWorkflowVote(r.Context(), workflowId, *userDid, req.Decision, req.Comment)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	unallocatedTokens, err := s.unallocatedBalanceTokens(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	weeklyRequirement := new(big.Int).SetUint64(workflow.WeeklyBountyRequirement)
	allowApproval := unallocatedTokens.Cmp(weeklyRequirement) >= 0

	updatedWorkflow, err := s.appDb.EvaluateWorkflowVoteStateWithApproval(r.Context(), workflowId, allowApproval)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	votes, err := s.appDb.GetWorkflowVotesForUser(r.Context(), workflowId, *userDid)
	if err == nil {
		updatedWorkflow.Votes = *votes
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(updatedWorkflow)
}

func (s *BotService) GetVoterWorkflows(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	workflows, err := s.appDb.GetVoterWorkflows(r.Context(), *userDid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for idx, workflow := range workflows {
		if workflow.Status != "pending" {
			continue
		}

		unallocatedTokens, err := s.unallocatedBalanceTokens(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		weeklyRequirement := new(big.Int).SetUint64(workflow.WeeklyBountyRequirement)
		allowApproval := unallocatedTokens.Cmp(weeklyRequirement) >= 0

		evaluatedWorkflow, err := s.appDb.EvaluateWorkflowVoteStateWithApproval(r.Context(), workflow.Id, allowApproval)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		votes, err := s.appDb.GetWorkflowVotesForUser(r.Context(), workflow.Id, *userDid)
		if err == nil {
			evaluatedWorkflow.Votes = *votes
		}
		workflows[idx] = evaluatedWorkflow
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(workflows)
}

func (s *BotService) AdminForceApproveWorkflow(w http.ResponseWriter, r *http.Request) {
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

	workflow, err := s.appDb.GetWorkflowForApproval(r.Context(), workflowId)
	if err != nil {
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if workflow.Status != "pending" {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(workflow)
		return
	}

	unallocatedTokens, err := s.unallocatedBalanceTokens(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	weeklyRequirement := new(big.Int).SetUint64(workflow.WeeklyBountyRequirement)
	if unallocatedTokens.Cmp(weeklyRequirement) < 0 {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "insufficient faucet balance to approve workflow",
		})
		return
	}

	if err := s.appDb.ForceApproveWorkflowAsAdmin(r.Context(), workflowId, *adminId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	updatedWorkflow, err := s.appDb.GetWorkflowByID(r.Context(), workflowId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(updatedWorkflow)
}

func (s *BotService) totalAllocatedBalance(ctx context.Context) (uint64, error) {
	eventAllocated, err := s.db.AllocatedBalance(ctx)
	if err != nil {
		return 0, err
	}

	workflowAllocated := uint64(0)
	if s.appDb != nil {
		workflowAllocated, err = s.appDb.AllocatedWorkflowBalance(ctx)
		if err != nil {
			return 0, err
		}
	}

	return eventAllocated + workflowAllocated, nil
}

func getTokenMultiplier() (*big.Int, error) {
	multiplier := strings.TrimSpace(os.Getenv("TOKEN_DECIMALS"))
	if multiplier == "" {
		return nil, fmt.Errorf("invalid token decimals in .env")
	}

	value, ok := new(big.Int).SetString(multiplier, 10)
	if !ok || value.Sign() <= 0 {
		return nil, fmt.Errorf("invalid token decimals in .env")
	}
	return value, nil
}

func (s *BotService) unallocatedBalanceWei(ctx context.Context) (*big.Int, error) {
	faucetBalance, err := s.bot.Balance()
	if err != nil {
		return nil, err
	}

	allocatedBalance, err := s.totalAllocatedBalance(ctx)
	if err != nil {
		return nil, err
	}

	multiplier, err := getTokenMultiplier()
	if err != nil {
		return nil, err
	}

	allocatedWei := new(big.Int).SetUint64(allocatedBalance)
	allocatedWei.Mul(allocatedWei, multiplier)

	return new(big.Int).Sub(faucetBalance, allocatedWei), nil
}

func (s *BotService) unallocatedBalanceTokens(ctx context.Context) (*big.Int, error) {
	unallocatedWei, err := s.unallocatedBalanceWei(ctx)
	if err != nil {
		return nil, err
	}

	multiplier, err := getTokenMultiplier()
	if err != nil {
		return nil, err
	}

	return new(big.Int).Div(unallocatedWei, multiplier), nil
}

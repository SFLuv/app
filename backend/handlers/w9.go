package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
)

type W9Service struct {
	appDb    *db.AppDB
	ponderDb *db.PonderDB
	logger   *logger.LogCloser
}

func NewW9Service(appDb *db.AppDB, ponderDb *db.PonderDB, logger *logger.LogCloser) *W9Service {
	return &W9Service{appDb: appDb, ponderDb: ponderDb, logger: logger}
}

func (w *W9Service) adminAddresses() []string {
	return utils.ParseAddressList(os.Getenv("PAID_ADMIN_ADDRESSES"))
}

func (w *W9Service) CheckCompliance(ctx context.Context, fromAddress string, toAddress string, amount *big.Int) (*structs.W9CheckResponse, error) {
	if w.appDb == nil || w.ponderDb == nil {
		return nil, fmt.Errorf("w9 service not configured")
	}
	adminAddresses := w.adminAddresses()
	if !utils.IsAddressInList(fromAddress, adminAddresses) {
		return &structs.W9CheckResponse{Allowed: true}, nil
	}

	year, _, _ := utils.CurrentYearBounds()
	totalStr, err := w.ponderDb.GetPaidTotalForWalletYear(ctx, toAddress, year, adminAddresses)
	if err != nil {
		return nil, err
	}

	total := new(big.Int)
	if totalStr == "" {
		total = big.NewInt(0)
	} else {
		parsed, ok := new(big.Int).SetString(totalStr, 10)
		if !ok {
			return nil, fmt.Errorf("invalid total value %s", totalStr)
		}
		total = parsed
	}

	if amount == nil {
		amount = big.NewInt(0)
	}

	newTotal := new(big.Int).Add(total, amount)

	limit, err := utils.W9Threshold()
	if err != nil {
		return nil, err
	}

	existing, err := w.appDb.GetW9WalletEarning(ctx, toAddress, year)
	if err != nil {
		return nil, err
	}

	var w9Required bool
	var w9RequiredAt *time.Time
	if existing != nil {
		w9Required = existing.W9Required
		w9RequiredAt = existing.W9RequiredAt
	}

	shouldNotify := false
	now := time.Now().UTC()
	if !w9Required && total.Cmp(limit) >= 0 {
		w9Required = true
		w9RequiredAt = &now
		shouldNotify = true
	}

	if !w9Required && total.Cmp(limit) < 0 && newTotal.Cmp(limit) >= 0 {
		w9Required = true
		w9RequiredAt = &now
		shouldNotify = true
	}

	userId, err := w.appDb.GetUserIdByWalletAddress(ctx, toAddress)
	if err != nil {
		return nil, err
	}

	earning := &structs.W9WalletEarning{
		WalletAddress:  utils.NormalizeAddress(toAddress),
		Year:           year,
		AmountReceived: total.String(),
		UserId:         userId,
		W9Required:     w9Required,
		W9RequiredAt:   w9RequiredAt,
	}

	err = w.appDb.UpsertW9WalletEarning(ctx, earning)
	if err != nil {
		return nil, err
	}

	submission, err := w.appDb.GetW9SubmissionByWalletYear(ctx, toAddress, year)
	if err != nil {
		return nil, err
	}

	approved := false
	pending := false
	if submission != nil && submission.PendingApproval {
		pending = true
	}
	if submission != nil && !submission.PendingApproval && submission.ApprovedAt != nil {
		approved = true
	}

	allowed := !(newTotal.Cmp(limit) > 0 && !approved)

	if shouldNotify {
		w.notifyAdminThreshold(toAddress, total.String(), newTotal.String(), limit.String(), year)
	}

	resp := &structs.W9CheckResponse{
		Allowed:      allowed,
		CurrentTotal: total.String(),
		NewTotal:     newTotal.String(),
		Limit:        limit.String(),
		Year:         year,
	}
	if submission != nil && submission.Email != "" {
		resp.Email = submission.Email
	} else if userId != nil {
		email, err := w.appDb.GetUserContactEmail(ctx, *userId)
		if err != nil {
			return nil, err
		}
		if email != nil {
			resp.Email = *email
		}
	}
	if !allowed {
		if pending {
			resp.Reason = "w9_pending"
		} else {
			resp.Reason = "w9_required"
			resp.W9URL = os.Getenv("W9_SUBMISSION_URL")
		}
	}
	return resp, nil
}

func (w *W9Service) ProcessPaidTransfer(ctx context.Context, fromAddress string, toAddress string, amount string, hash string, timestamp int64) (*structs.W9WalletEarning, error) {
	if w.appDb == nil || w.ponderDb == nil {
		return nil, fmt.Errorf("w9 service not configured")
	}

	adminAddresses := w.adminAddresses()
	if !utils.IsAddressInList(fromAddress, adminAddresses) {
		return nil, nil
	}

	year := time.Unix(timestamp, 0).UTC().Year()
	if timestamp == 0 {
		year = time.Now().UTC().Year()
	}

	totalStr, err := w.ponderDb.GetPaidTotalForWalletYear(ctx, toAddress, year, adminAddresses)
	if err != nil {
		return nil, err
	}

	total := new(big.Int)
	if totalStr == "" {
		total = big.NewInt(0)
	} else {
		parsed, ok := new(big.Int).SetString(totalStr, 10)
		if !ok {
			return nil, fmt.Errorf("invalid total value %s", totalStr)
		}
		total = parsed
	}

	limit, err := utils.W9Threshold()
	if err != nil {
		return nil, err
	}

	existing, err := w.appDb.GetW9WalletEarning(ctx, toAddress, year)
	if err != nil {
		return nil, err
	}

	var w9Required bool
	var w9RequiredAt *time.Time
	if existing != nil {
		w9Required = existing.W9Required
		w9RequiredAt = existing.W9RequiredAt
	}

	shouldNotify := false
	now := time.Now().UTC()
	if !w9Required && total.Cmp(limit) >= 0 {
		w9Required = true
		w9RequiredAt = &now
		shouldNotify = true
	}

	userId, err := w.appDb.GetUserIdByWalletAddress(ctx, toAddress)
	if err != nil {
		return nil, err
	}

	var lastHash *string
	if hash != "" {
		h := hash
		lastHash = &h
	}

	var lastTimestamp *int
	if timestamp != 0 {
		ts := int(timestamp)
		lastTimestamp = &ts
	}

	earning := &structs.W9WalletEarning{
		WalletAddress:   utils.NormalizeAddress(toAddress),
		Year:            year,
		AmountReceived:  total.String(),
		UserId:          userId,
		W9Required:      w9Required,
		W9RequiredAt:    w9RequiredAt,
		LastTxHash:      lastHash,
		LastTxTimestamp: lastTimestamp,
	}

	err = w.appDb.UpsertW9WalletEarning(ctx, earning)
	if err != nil {
		return nil, err
	}

	if shouldNotify {
		w.notifyAdminThreshold(toAddress, total.String(), total.String(), limit.String(), year)
	}

	return earning, nil
}

func (w *W9Service) notifyAdminThreshold(wallet string, currentTotal string, newTotal string, limit string, year int) {
	sender := utils.NewEmailSender()
	if sender == nil {
		if w.logger != nil {
			w.logger.Logf("w9 admin email not sent; mailgun not configured")
		}
		return
	}

	adminEmail := os.Getenv("W9_ADMIN_EMAIL")
	if adminEmail == "" {
		adminEmail = "admin@sfluv.com"
	}

	subject := fmt.Sprintf("W9 required for wallet %s", wallet)
	body := fmt.Sprintf(
		"Wallet %s reached the W9 threshold in %d.<br/><br/>Current total: %s<br/>New total: %s<br/>Limit: %s",
		wallet,
		year,
		currentTotal,
		newTotal,
		limit,
	)

	err := sender.SendEmail(
		adminEmail,
		"SFLuv Admin",
		subject,
		body,
		"no_reply@sfluv.org",
		"SFLuv Admin",
	)
	if err != nil && w.logger != nil {
		w.logger.Logf("error sending w9 admin email: %s", err)
	}
}

func (a *AppService) SubmitW9(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req structs.W9SubmitRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.WalletAddress == "" || req.Email == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	year := time.Now().UTC().Year()
	if req.Year != nil {
		year = *req.Year
	}

	submission := &structs.W9Submission{
		WalletAddress: utils.NormalizeAddress(req.WalletAddress),
		Year:          year,
		Email:         req.Email,
	}

	stored, err := a.db.UpsertW9Submission(r.Context(), submission)
	if err != nil {
		a.logger.Logf("error upserting w9 submission: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"submission": stored,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write(bytes)
}

func (a *AppService) GetPendingW9Submissions(w http.ResponseWriter, r *http.Request) {
	submissions, err := a.db.GetPendingW9Submissions(r.Context())
	if err != nil {
		a.logger.Logf("error getting pending w9 submissions: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := structs.W9PendingResponse{
		Submissions: submissions,
	}

	bytes, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

func (a *AppService) ApproveW9Submission(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req structs.W9ApprovalRequest
	err = json.Unmarshal(body, &req)
	if err != nil || req.Id == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	approver := utils.GetDid(r)
	approvedBy := ""
	if approver != nil {
		approvedBy = *approver
	}

	submission, err := a.db.ApproveW9Submission(r.Context(), req.Id, approvedBy)
	if err != nil {
		a.logger.Logf("error approving w9 submission: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sender := utils.NewEmailSender()
	if sender != nil {
		subject := "W9 approved - restriction removed"
		body := fmt.Sprintf(
			"Your W9 has been approved for wallet %s. The $600 restriction has been removed.",
			submission.WalletAddress,
		)
		err = sender.SendEmail(
			submission.Email,
			"SFLuv User",
			subject,
			body,
			"no_reply@sfluv.org",
			"SFLuv Admin",
		)
		if err != nil {
			a.logger.Logf("error sending w9 approval email: %s", err)
		}
	}

	resp := map[string]any{
		"submission": submission,
	}
	bytes, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

func (a *AppService) CheckW9Compliance(w http.ResponseWriter, r *http.Request) {
	if a.w9 == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req structs.W9CheckRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.FromAddress == "" || req.ToAddress == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	amount := new(big.Int)
	if req.Amount != "" {
		parsed, ok := amount.SetString(req.Amount, 10)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		amount = parsed
	}

	resp, err := a.w9.CheckCompliance(r.Context(), req.FromAddress, req.ToAddress, amount)
	if err != nil {
		a.logger.Logf("error checking w9 compliance: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	bytes, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !resp.Allowed {
		w.WriteHeader(http.StatusForbidden)
		w.Write(bytes)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

func (a *AppService) RecordW9Transaction(w http.ResponseWriter, r *http.Request) {
	if a.w9 == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req structs.W9TransactionRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.FromAddress == "" || req.ToAddress == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, err = a.w9.ProcessPaidTransfer(r.Context(), req.FromAddress, req.ToAddress, req.Amount, req.Hash, req.Timestamp)
	if err != nil {
		a.logger.Logf("error processing w9 transaction: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

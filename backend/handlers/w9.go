package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/jackc/pgx/v5"
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

	now := time.Now().UTC()
	if !w9Required && total.Cmp(limit) >= 0 {
		w9Required = true
		w9RequiredAt = &now
	}

	if !w9Required && total.Cmp(limit) < 0 && newTotal.Cmp(limit) >= 0 {
		w9Required = true
		w9RequiredAt = &now
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
	recipientEmail, err := w.resolveRecipientEmail(ctx, userId, submission)
	if err != nil {
		return nil, err
	}

	resp := &structs.W9CheckResponse{
		Allowed:      allowed,
		CurrentTotal: total.String(),
		NewTotal:     newTotal.String(),
		Limit:        limit.String(),
		Year:         year,
	}
	if recipientEmail != "" {
		resp.Email = recipientEmail
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

	now := time.Now().UTC()
	if !w9Required && total.Cmp(limit) >= 0 {
		w9Required = true
		w9RequiredAt = &now
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

	return earning, nil
}

func (w *W9Service) resolveRecipientEmail(ctx context.Context, userId *string, submission *structs.W9Submission) (string, error) {
	if submission != nil && strings.TrimSpace(submission.Email) != "" {
		return strings.TrimSpace(submission.Email), nil
	}
	if userId == nil {
		return "", nil
	}
	email, err := w.appDb.GetUserContactEmail(ctx, *userId)
	if err != nil {
		return "", err
	}
	if email == nil {
		return "", nil
	}
	return strings.TrimSpace(*email), nil
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

	stored, ok := a.submitW9(r.Context(), w, &req, "webapp")
	if !ok {
		return
	}

	a.writeW9SubmissionResponse(w, stored)
}

func (a *AppService) GetPendingW9Submissions(w http.ResponseWriter, r *http.Request) {
	submissions, err := a.db.GetPendingW9Submissions(r.Context())
	if err != nil {
		a.logger.Logf("error getting pending w9 submissions: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, submission := range submissions {
		userId, err := a.db.GetUserIdByWalletAddress(r.Context(), submission.WalletAddress)
		if err != nil {
			a.logger.Logf("error getting user by wallet for w9 submission %d: %s", submission.Id, err)
			continue
		}
		if userId == nil {
			continue
		}
		email, err := a.db.GetUserContactEmail(r.Context(), *userId)
		if err != nil {
			a.logger.Logf("error getting user contact email for w9 submission %d: %s", submission.Id, err)
			continue
		}
		if email != nil && *email != "" {
			submission.UserContactEmail = email
		}
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
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error":"w9_not_pending"}`))
			return
		}
		a.logger.Logf("error approving w9 submission: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	a.sendW9ApprovedUserEmail(r.Context(), submission)

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

func (a *AppService) RejectW9Submission(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req structs.W9RejectRequest
	err = json.Unmarshal(body, &req)
	if err != nil || req.Id == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	rejector := utils.GetDid(r)
	rejectedBy := ""
	if rejector != nil {
		rejectedBy = *rejector
	}

	submission, err := a.db.RejectW9Submission(r.Context(), req.Id, rejectedBy, req.Reason)
	if err != nil {
		a.logger.Logf("error rejecting w9 submission: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
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

func (a *AppService) SubmitW9Webhook(w http.ResponseWriter, r *http.Request) {
	secret := strings.TrimSpace(os.Getenv("W9_WEBHOOK_SECRET"))
	if secret == "" {
		a.logger.Logf("w9 webhook rejected; W9_WEBHOOK_SECRET is not configured")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	key := r.Header.Get("X-W9-Secret")
	if key == "" {
		key = r.Header.Get("X-W9-Key")
	}
	if key != secret {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	var req structs.W9SubmitRequest
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		req.WalletAddress = r.FormValue("wallet")
		if req.WalletAddress == "" {
			req.WalletAddress = r.FormValue("wallet_address")
		}
		req.Email = r.FormValue("email")
		if yearStr := r.FormValue("year"); yearStr != "" {
			if parsed, err := strconv.Atoi(yearStr); err == nil {
				req.Year = &parsed
			}
		}
	}

	stored, ok := a.submitW9(r.Context(), w, &req, "wordpress")
	if !ok {
		return
	}

	a.writeW9SubmissionResponse(w, stored)
}

func (a *AppService) submitW9(ctx context.Context, w http.ResponseWriter, req *structs.W9SubmitRequest, source string) (*structs.W9Submission, bool) {
	if req.WalletAddress == "" || req.Email == "" {
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}

	year := time.Now().UTC().Year()
	if req.Year != nil {
		year = *req.Year
	}

	existing, err := a.db.GetW9SubmissionByWalletYear(ctx, req.WalletAddress, year)
	if err != nil {
		a.logger.Logf("error checking existing w9 submission: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}
	if existing != nil && existing.PendingApproval {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"w9_pending"}`))
		return nil, false
	}
	if existing != nil && existing.ApprovedAt != nil && existing.RejectedAt == nil {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"w9_approved"}`))
		return nil, false
	}

	submission := &structs.W9Submission{
		WalletAddress: utils.NormalizeAddress(req.WalletAddress),
		Year:          year,
		Email:         req.Email,
	}

	stored, err := a.db.UpsertW9Submission(ctx, submission)
	if err != nil {
		a.logger.Logf("error upserting w9 submission: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}
	a.sendW9AdminAlertEmail(stored, source)
	return stored, true
}

func (a *AppService) sendW9ApprovedUserEmail(ctx context.Context, submission *structs.W9Submission) {
	sender := utils.NewEmailSender()
	if sender == nil {
		a.logger.Logf("w9 user approval email not sent; mailgun not configured")
		return
	}

	recipientEmail := strings.TrimSpace(submission.Email)
	if recipientEmail == "" {
		userId, err := a.db.GetUserIdByWalletAddress(ctx, submission.WalletAddress)
		if err != nil {
			a.logger.Logf("error resolving user for w9 approval email: %s", err)
		} else if userId != nil {
			email, err := a.db.GetUserContactEmail(ctx, *userId)
			if err != nil {
				a.logger.Logf("error resolving user contact email for w9 approval email: %s", err)
			} else if email != nil {
				recipientEmail = strings.TrimSpace(*email)
			}
		}
	}

	if recipientEmail == "" {
		a.logger.Logf("w9 user approval email not sent; no recipient email for wallet %s", submission.WalletAddress)
		return
	}

	subject := "W9 approved - restriction removed"
	body := fmt.Sprintf(
		"Your W9 has been approved for wallet %s. The $600 restriction has been removed.",
		submission.WalletAddress,
	)

	err := sender.SendEmail(
		recipientEmail,
		"SFLuv User",
		subject,
		body,
		w9NoReplyFromEmail(),
		"SFLuv Admin",
	)
	if err != nil {
		a.logger.Logf("error sending w9 approval email: %s", err)
	}
}

func (a *AppService) sendW9AdminAlertEmail(submission *structs.W9Submission, source string) {
	sender := utils.NewEmailSender()
	if sender == nil {
		a.logger.Logf("w9 admin email not sent; mailgun not configured")
		return
	}

	adminEmail := strings.TrimSpace(os.Getenv("W9_ADMIN_EMAIL"))
	if adminEmail == "" {
		adminEmail = "admin@sfluv.org"
	}

	subject := fmt.Sprintf("W9 required for wallet %s", submission.WalletAddress)
	contact := strings.TrimSpace(submission.Email)
	if contact == "" {
		contact = submission.WalletAddress
	}

	sourceName := strings.TrimSpace(source)
	if sourceName == "" {
		sourceName = "webapp"
	}

	submittedAtUTC := submission.SubmittedAt.UTC().Format(time.RFC3339)
	body := fmt.Sprintf(
		"%s has reached the 1099 limit and needs a W9 submission form. Please send them a form using <a href=\"https://app.getw9.tax/subscriber\">https://app.getw9.tax/subscriber</a>.<br/><br/>Wallet: %s<br/>Email: %s<br/>Year: %d<br/>Source: %s<br/>Submitted at (UTC): %s",
		contact,
		submission.WalletAddress,
		submission.Email,
		submission.Year,
		sourceName,
		submittedAtUTC,
	)
	if sourceName == "wordpress" {
		body = fmt.Sprintf(
			"A request for a W9 form has been submitted from the Wordpress site.<br/><br/>Wallet: %s<br/>Email: %s<br/>Year: %d<br/>Source: %s<br/>Submitted at (UTC): %s",
			submission.WalletAddress,
			submission.Email,
			submission.Year,
			sourceName,
			submittedAtUTC,
		)
	}

	err := sender.SendEmail(
		adminEmail,
		"SFLuv Admin",
		subject,
		body,
		w9NoReplyFromEmail(),
		"SFLuv Admin",
	)
	if err != nil {
		a.logger.Logf("error sending w9 admin alert email: %s", err)
	}
}

func w9NoReplyFromEmail() string {
	fromDomain := strings.TrimSpace(os.Getenv("MAILGUN_DOMAIN"))
	if fromDomain == "" {
		return "no_reply@sfluv.org"
	}
	return "no_reply@" + fromDomain
}

func (a *AppService) writeW9SubmissionResponse(w http.ResponseWriter, stored *structs.W9Submission) {
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

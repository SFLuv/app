package handlers

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/utils"
	"github.com/ethereum/go-ethereum/common"
)

func (s *BotService) recoveryClaimExpiry() time.Duration {
	mins := 30
	if v := strings.TrimSpace(os.Getenv("RECOVERY_CLAIM_EXPIRY_MINUTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			mins = n
		}
	}
	return time.Duration(mins) * time.Minute
}

// StartRecoveryReconciler runs a background loop that periodically reconciles
// recovery payouts: claims whose stored tx hash never confirmed (past the expiry
// window) are freed back to unclaimed so the holder can claim again.
func (s *BotService) StartRecoveryReconciler(ctx context.Context) {
	if s == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			if err := s.ReconcileRecoveryClaims(runCtx); err != nil {
				fmt.Printf("error reconciling recovery claims: %s\n", err)
			}
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// ReconcileRecoveryClaims re-checks claimed payouts past the expiry window. Only
// claims whose transaction is definitively NOT on-chain are reset (to avoid ever
// double-sending a payout that actually landed).
func (s *BotService) ReconcileRecoveryClaims(ctx context.Context) error {
	if s == nil || s.db == nil || s.bot == nil {
		return nil
	}
	before := time.Now().Add(-s.recoveryClaimExpiry())
	claims, err := s.db.RecoveryClaimsToReconcile(ctx, before)
	if err != nil {
		return err
	}
	for _, c := range claims {
		amount, ok := new(big.Int).SetString(strings.TrimSpace(c.Amount), 10)
		if !ok {
			continue
		}
		res, err := s.bot.VerifyTransferBaseUnits(ctx, c.ClaimTxHash, c.ClaimedBy, amount)
		if err != nil {
			fmt.Printf("recovery reconcile: error verifying tx %s for %s: %s\n", c.ClaimTxHash, c.Address, err)
			continue
		}
		if res != nil && res.Found {
			// Transaction is on-chain (confirmed or pending); leave the claim.
			continue
		}
		reset, err := s.db.ResetRecoveryClaim(ctx, c.Address, c.ClaimTxHash)
		if err != nil {
			fmt.Printf("recovery reconcile: error resetting %s: %s\n", c.Address, err)
			continue
		}
		if reset {
			fmt.Printf("recovery reconcile: reset %s to unclaimed (tx %s never confirmed)\n", c.Address, c.ClaimTxHash)
		}
	}
	return nil
}

// RecoveryPreview returns the claimable balance for a sigAuth-verified account
// without claiming it. It is public (logged-out): the sigAuth signature itself
// authenticates a read of the caller's own pre-migration balance, which the
// recovery page shows before prompting the user to log in.
func (s *BotService) RecoveryPreview(w http.ResponseWriter, r *http.Request) {
	body := EnsureBody(w, r)
	if body == nil {
		return
	}
	var req *structs.RecoveryClaimRequest
	if !EnsureUnmarshal(w, &req, body) {
		return
	}
	if req == nil ||
		strings.TrimSpace(req.SigAuthAccount) == "" ||
		strings.TrimSpace(req.SigAuthExpiry) == "" ||
		strings.TrimSpace(req.SigAuthSignature) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	verifyCtx, cancelVerify := context.WithTimeout(r.Context(), 20*time.Second)
	account, err := VerifySigAuth(verifyCtx, s.readRPCURL, SigAuthParams{
		Account:   req.SigAuthAccount,
		Expiry:    req.SigAuthExpiry,
		Signature: req.SigAuthSignature,
		Redirect:  req.SigAuthRedirect,
	})
	cancelVerify()
	if err != nil {
		writeJSON(w, http.StatusForbidden, structs.RecoveryPreviewResponse{Message: "signature verification failed"})
		return
	}

	lookupCtx, cancelLookup := context.WithTimeout(r.Context(), 8*time.Second)
	rb, err := s.db.GetRecoveryBalance(lookupCtx, account)
	cancelLookup()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if rb == nil {
		writeJSON(w, http.StatusOK, structs.RecoveryPreviewResponse{Account: account, Amount: "0", Message: "no recovery balance for this account"})
		return
	}
	writeJSON(w, http.StatusOK, structs.RecoveryPreviewResponse{
		Account: account,
		Amount:  rb.Amount,
		Claimed: rb.ClaimStatus == structs.RecoveryStatusClaimed,
	})
}

// RecoveryClaim lets an authenticated user claim a non-auto-migrated balance
// (typically a Citizen Wallet account) by proving control of the previous
// account via the CW sigAuth parameter set. On success the recorded balance is
// sent from the faucet to the caller's primary wallet.
//
// The balance is atomically reserved (unclaimed -> claiming) BEFORE any payout,
// so it can only be sent once; the claim tx hash is stored so an unconfirmed
// payout can be reconciled/recovered after the transaction expiry period.
func (s *BotService) RecoveryClaim(w http.ResponseWriter, r *http.Request) {
	userDid := utils.GetDid(r)
	if userDid == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body := EnsureBody(w, r)
	if body == nil {
		return
	}
	var req *structs.RecoveryClaimRequest
	if !EnsureUnmarshal(w, &req, body) {
		return
	}
	if req == nil ||
		strings.TrimSpace(req.SigAuthAccount) == "" ||
		strings.TrimSpace(req.SigAuthExpiry) == "" ||
		strings.TrimSpace(req.SigAuthSignature) == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Recipient is the authenticated user's primary wallet.
	userCtx, cancelUser := context.WithTimeout(r.Context(), 8*time.Second)
	user, err := s.appDb.GetUserById(userCtx, *userDid)
	cancelUser()
	if err != nil || user == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	recipient := strings.TrimSpace(user.PrimaryWalletAddress)
	if !common.IsHexAddress(recipient) {
		writeJSON(w, http.StatusBadRequest, structs.RecoveryClaimResponse{Message: "no primary wallet set for this account"})
		return
	}

	// Verify the sigAuth parameters and resolve the previous account address.
	verifyCtx, cancelVerify := context.WithTimeout(r.Context(), 20*time.Second)
	account, err := VerifySigAuth(verifyCtx, s.readRPCURL, SigAuthParams{
		Account:   req.SigAuthAccount,
		Expiry:    req.SigAuthExpiry,
		Signature: req.SigAuthSignature,
		Redirect:  req.SigAuthRedirect,
	})
	cancelVerify()
	if err != nil {
		writeJSON(w, http.StatusForbidden, structs.RecoveryClaimResponse{Message: "signature verification failed"})
		return
	}

	lookupCtx, cancelLookup := context.WithTimeout(r.Context(), 8*time.Second)
	rb, err := s.db.GetRecoveryBalance(lookupCtx, account)
	cancelLookup()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if rb == nil {
		writeJSON(w, http.StatusNotFound, structs.RecoveryClaimResponse{Message: "no recovery balance for this account"})
		return
	}
	if rb.ClaimStatus == structs.RecoveryStatusClaimed {
		writeJSON(w, http.StatusConflict, structs.RecoveryClaimResponse{
			Claimed: true, Amount: rb.Amount, TxHash: rb.ClaimTxHash, Message: "balance already claimed",
		})
		return
	}

	// Atomically reserve the balance (unclaimed -> claiming) before sending.
	beginCtx, cancelBegin := context.WithTimeout(r.Context(), 8*time.Second)
	amountStr, err := s.db.BeginRecoveryClaim(beginCtx, account, recipient, *userDid)
	cancelBegin()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if amountStr == "" {
		writeJSON(w, http.StatusConflict, structs.RecoveryClaimResponse{Message: "balance is already being claimed"})
		return
	}

	amount, ok := new(big.Int).SetString(amountStr, 10)
	if !ok || amount.Sign() <= 0 {
		// Nothing to send; close the record out so it cannot be re-reserved.
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 8*time.Second)
		_ = s.db.CompleteRecoveryClaim(closeCtx, account, "", s.chainID())
		cancelClose()
		writeJSON(w, http.StatusOK, structs.RecoveryClaimResponse{
			Claimed: true, Amount: amountStr, Recipient: recipient, Message: "no positive balance to recover",
		})
		return
	}

	// Send the exact base-unit balance from the faucet.
	txHash, sendErr := s.bot.SubmitTransferBaseUnits(amount, recipient)
	if sendErr != nil {
		// Pre-broadcast failures are revertable; return the balance to unclaimed
		// so the user can retry. A non-revertable error may have broadcast, so the
		// balance stays 'claiming' for reconciliation rather than being freed.
		if bot.ShouldRevertRedemption(sendErr) {
			revertCtx, cancelRevert := context.WithTimeout(context.Background(), 8*time.Second)
			_ = s.db.RevertRecoveryClaim(revertCtx, account)
			cancelRevert()
		}
		fmt.Printf("error sending recovery payout for account %s to %s: %s\n", account, recipient, sendErr)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Persist the claim + tx hash. The payout already broadcast, so never revert
	// here even if bookkeeping fails — the stored hash enables reconciliation.
	completeCtx, cancelComplete := context.WithTimeout(context.Background(), 8*time.Second)
	completeErr := s.db.CompleteRecoveryClaim(completeCtx, account, txHash, s.chainID())
	cancelComplete()
	if completeErr != nil {
		fmt.Printf("recovery payout for %s sent (tx %s) but failed to mark claimed: %s\n", account, txHash, completeErr)
	}

	writeJSON(w, http.StatusOK, structs.RecoveryClaimResponse{
		Claimed: true, Amount: amountStr, TxHash: txHash, Recipient: recipient,
	})
}

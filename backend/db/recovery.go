package db

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// GetRecoveryBalance returns the recovery record for a (lowercased) address, or
// nil if there is none.
func (s *BotDB) GetRecoveryBalance(ctx context.Context, address string) (*structs.RecoveryBalance, error) {
	address = strings.ToLower(strings.TrimSpace(address))
	row := s.db.QueryRow(ctx, `
		SELECT address, chain_id, amount::text, claim_status,
		       COALESCE(claimed_by, ''), COALESCE(claim_tx_hash, ''), claimed_at
		FROM recovery_balances
		WHERE address = $1;
	`, address)

	var rb structs.RecoveryBalance
	var claimedAt *time.Time
	err := row.Scan(&rb.Address, &rb.ChainID, &rb.Amount, &rb.ClaimStatus, &rb.ClaimedBy, &rb.ClaimTxHash, &claimedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error reading recovery balance for %s: %w", address, err)
	}
	rb.ClaimedAt = claimedAt
	return &rb, nil
}

// BeginRecoveryClaim atomically transitions an address from unclaimed to
// claiming and returns the base-unit amount to send. It must succeed before any
// payout: the WHERE clause guarantees only one claimer can reserve a balance,
// preventing double sends. Returns ("", nil) when there is no unclaimed balance
// (already claimed/claiming, or none recorded).
func (s *BotDB) BeginRecoveryClaim(ctx context.Context, address, recipient, userID string) (string, error) {
	address = strings.ToLower(strings.TrimSpace(address))
	row := s.db.QueryRow(ctx, `
		UPDATE recovery_balances
		SET claim_status = 'claiming',
		    claimed_by = $2,
		    claimed_by_user_id = $3,
		    updated_at = NOW()
		WHERE address = $1 AND claim_status = 'unclaimed'
		RETURNING amount::text;
	`, address, strings.ToLower(strings.TrimSpace(recipient)), strings.TrimSpace(userID))

	var amount string
	err := row.Scan(&amount)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("error beginning recovery claim for %s: %w", address, err)
	}
	return amount, nil
}

// CompleteRecoveryClaim records a successful (broadcast) payout: marks the
// balance claimed and stores the tx hash so an unconfirmed transaction can be
// re-checked and recovered after the expiry period.
func (s *BotDB) CompleteRecoveryClaim(ctx context.Context, address, txHash string, chainID int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE recovery_balances
		SET claim_status = 'claimed',
		    claim_tx_hash = $2,
		    claim_tx_chain_id = $3,
		    claimed_at = NOW(),
		    updated_at = NOW()
		WHERE address = $1;
	`, strings.ToLower(strings.TrimSpace(address)), strings.TrimSpace(txHash), chainID)
	if err != nil {
		return fmt.Errorf("error completing recovery claim for %s: %w", address, err)
	}
	return nil
}

// RevertRecoveryClaim returns a claiming balance to unclaimed after a payout
// failed before broadcast, so the user can retry.
func (s *BotDB) RevertRecoveryClaim(ctx context.Context, address string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE recovery_balances
		SET claim_status = 'unclaimed',
		    claimed_by = NULL,
		    claimed_by_user_id = NULL,
		    updated_at = NOW()
		WHERE address = $1 AND claim_status = 'claiming';
	`, strings.ToLower(strings.TrimSpace(address)))
	if err != nil {
		return fmt.Errorf("error reverting recovery claim for %s: %w", address, err)
	}
	return nil
}

// recoveryAllocatedWholeUnits returns the not-yet-claimed recovery balances as a
// whole-token count (rounded up), so they can be folded into the faucet's
// allocated/reserved total and never double-spent by workflow payouts. Both
// unclaimed and in-flight (claiming) balances are reserved.
func (s *BotDB) recoveryAllocatedWholeUnits(ctx context.Context) (uint64, error) {
	exists, err := s.tableExists(ctx, "recovery_balances")
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}

	row := s.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0)::text
		FROM recovery_balances
		WHERE claim_status <> 'claimed';
	`)
	var sumText string
	if err := row.Scan(&sumText); err != nil {
		return 0, fmt.Errorf("error summing unclaimed recovery balances: %w", err)
	}
	baseSum, ok := new(big.Int).SetString(strings.TrimSpace(sumText), 10)
	if !ok || baseSum.Sign() <= 0 {
		return 0, nil
	}

	scale, err := tokenMultiplierFromEnv()
	if err != nil {
		return 0, err
	}
	// ceil(baseSum / scale)
	whole := new(big.Int).Add(baseSum, new(big.Int).Sub(scale, big.NewInt(1)))
	whole.Div(whole, scale)
	return whole.Uint64(), nil
}

// RecoveryClaimsToReconcile returns claimed balances whose payout tx was
// recorded before `before` (i.e. past the expiry window), so the reconciler can
// re-check whether the transaction ever confirmed.
func (s *BotDB) RecoveryClaimsToReconcile(ctx context.Context, before time.Time) ([]*structs.RecoveryBalance, error) {
	exists, err := s.tableExists(ctx, "recovery_balances")
	if err != nil || !exists {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT address, chain_id, amount::text, claim_status,
		       COALESCE(claimed_by, ''), COALESCE(claim_tx_hash, ''), claimed_at
		FROM recovery_balances
		WHERE claim_status = 'claimed'
		  AND COALESCE(claim_tx_hash, '') <> ''
		  AND claimed_at IS NOT NULL
		  AND claimed_at < $1
		ORDER BY claimed_at ASC
		LIMIT 200;
	`, before)
	if err != nil {
		return nil, fmt.Errorf("error querying recovery claims to reconcile: %w", err)
	}
	defer rows.Close()

	claims := make([]*structs.RecoveryBalance, 0)
	for rows.Next() {
		rb := &structs.RecoveryBalance{}
		var claimedAt *time.Time
		if err := rows.Scan(&rb.Address, &rb.ChainID, &rb.Amount, &rb.ClaimStatus, &rb.ClaimedBy, &rb.ClaimTxHash, &claimedAt); err != nil {
			return nil, fmt.Errorf("error scanning recovery claim to reconcile: %w", err)
		}
		rb.ClaimedAt = claimedAt
		claims = append(claims, rb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading recovery claims to reconcile: %w", err)
	}
	return claims, nil
}

// ResetRecoveryClaim frees a claimed balance back to unclaimed so it can be
// re-claimed, but only if it is still claimed with the same tx hash that was
// verified as never-confirmed (guards against racing a concurrent re-claim).
func (s *BotDB) ResetRecoveryClaim(ctx context.Context, address, expectedTxHash string) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE recovery_balances
		SET claim_status = 'unclaimed',
		    claimed_by = NULL,
		    claimed_by_user_id = NULL,
		    claim_tx_hash = NULL,
		    claim_tx_chain_id = NULL,
		    claimed_at = NULL,
		    updated_at = NOW()
		WHERE address = $1 AND claim_status = 'claimed' AND claim_tx_hash = $2;
	`, strings.ToLower(strings.TrimSpace(address)), strings.TrimSpace(expectedTxHash))
	if err != nil {
		return false, fmt.Errorf("error resetting recovery claim for %s: %w", address, err)
	}
	return tag.RowsAffected() > 0, nil
}

func (s *BotDB) tableExists(ctx context.Context, name string) (bool, error) {
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL;`, "public."+name).Scan(&exists); err != nil {
		return false, fmt.Errorf("error checking table %s: %w", name, err)
	}
	return exists, nil
}

func tokenMultiplierFromEnv() (*big.Int, error) {
	raw := strings.TrimSpace(os.Getenv("TOKEN_DECIMALS"))
	if raw == "" {
		return nil, fmt.Errorf("TOKEN_DECIMALS not set")
	}
	scale, ok := new(big.Int).SetString(raw, 10)
	if !ok || scale.Sign() <= 0 {
		return nil, fmt.Errorf("invalid TOKEN_DECIMALS value %q", raw)
	}
	return scale, nil
}

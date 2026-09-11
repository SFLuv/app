package db

import (
	"context"
	"fmt"
	"strings"
)

// ClaimTransferNotification records that this transfer is being notified about,
// and reports whether this caller is the one that got to do it.
//
// The incoming-transfer hook had no memory before this. It mailed every
// subscriber for every callback it received, so a redelivery re-notified — and
// a Ponder re-index, which replays the chain from its start block, would have
// mailed every user about every payment they have ever received. Migration 1.54
// seeded the table with every transfer indexed up to that point precisely so
// that the first re-index after it finds them all claimed.
//
// The insert IS the claim: whoever inserts the row sends the mail. Doing it in
// one statement rather than a read-then-write means two simultaneous deliveries
// of the same transfer cannot both decide they are first.
//
// The key deliberately omits the chain. The hook payload carries no chain id,
// so the handler substitutes the active one, and keying on a value the sender
// never sent would make dedup depend on a guess. Transaction hashes do not
// collide across chains; the four columns below identify a transfer on their
// own. chain_id is stored for the record, not for matching.
func (a *AppDB) ClaimTransferNotification(ctx context.Context, chainID int64, txHash string, to string, from string, amount string) (bool, error) {
	txHash = strings.ToLower(strings.TrimSpace(txHash))
	to = strings.ToLower(strings.TrimSpace(to))
	from = strings.ToLower(strings.TrimSpace(from))
	amount = strings.TrimSpace(amount)

	// A transfer we cannot identify cannot be deduplicated. Claiming it would
	// write a junk row that silently swallows the next unidentifiable one, so
	// it is allowed through and the notification decision is left as it was.
	if txHash == "" || to == "" {
		return true, nil
	}

	var chain any
	if chainID > 0 {
		chain = chainID
	}

	tag, err := a.db.Exec(ctx, `
		INSERT INTO ponder_notified_transfers
			(tx_hash, to_address, from_address, amount, chain_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tx_hash, to_address, from_address, amount) DO NOTHING;
	`, txHash, to, from, amount, chain)
	if err != nil {
		return false, fmt.Errorf("error claiming transfer notification for %s: %w", txHash, err)
	}
	return tag.RowsAffected() > 0, nil
}

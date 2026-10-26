package handlers

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/protocol"
	"github.com/SFLuv/app/backend/structs"
	"github.com/ethereum/go-ethereum/common"
)

// How the faucet confirmation sweep reads the chain.
const (
	// The ledger row is written before the send, so its time is a lower bound
	// on the transfer. A few blocks earlier absorbs clock drift between this
	// server and the chain.
	faucetSearchClockMargin = 30
	// A faucet operation is only submitted once the one before it has finished
	// its wait, and that wait is bounded well under this. A transfer this long
	// after a payout was recorded therefore came from a later operation.
	faucetLaterOperationGap = 10 * time.Minute
	// Claims this recent may belong to a send that is still recording its own
	// hash, so they are not adopted for anyone else.
	faucetOrphanMinAge = 5 * time.Minute
)

// FaucetPayoutSearch is what the chain says about one unresolved payout.
type FaucetPayoutSearch struct {
	// TxHash is the transfer that carried it, empty if none was found.
	TxHash string
	// LaterOperationLanded means a faucet operation submitted after this payout
	// has been mined. Operations take their nonces in order, so from then on
	// this payout has either landed or never will.
	LaterOperationLanded bool
	// SearchedUntil is the timestamp of the newest block searched.
	SearchedUntil time.Time
}

// FaucetSafeHeldAt reports whether the faucet Safe was the faucet at that
// moment. Payouts from before the Safe were sent by the old key and cannot be
// found by searching the Safe's transfers.
func (p *ProtocolService) FaucetSafeHeldAt(ctx context.Context, at time.Time) (bool, error) {
	safe, err := p.SafeFor(ctx, structs.ProtocolAccountFaucet)
	if err != nil {
		return false, nil
	}
	windows, err := p.db.ProtocolRoleWindows(ctx, structs.ProtocolRoleFaucet, p.chainID)
	if err != nil {
		return false, err
	}
	return windows.HeldAt(safe.Hex(), at), nil
}

// FindFaucetPayout looks for the transfer an unresolved payout should have made
// and claims it, so no other payout can take credit for the same money.
//
// A transfer the dispatcher already claimed is adopted only when no payout
// recorded it — the case where the caller stopped waiting before the transfer
// was seen.
func (p *ProtocolService) FindFaucetPayout(ctx context.Context, recipient common.Address, amount *big.Int, recordedAt time.Time) (FaucetPayoutSearch, error) {
	result := FaucetPayoutSearch{}
	if err := p.Available(); err != nil {
		return result, err
	}
	safe, err := p.SafeFor(ctx, structs.ProtocolAccountFaucet)
	if err != nil {
		return result, err
	}

	head, err := p.client.ChainHead(ctx)
	if err != nil {
		return result, err
	}
	start, err := p.client.BlockAtOrBefore(ctx, recordedAt)
	if err != nil {
		return result, err
	}
	if start > faucetSearchClockMargin {
		start -= faucetSearchClockMargin
	}

	logs, err := p.client.TransfersFrom(ctx, p.token, safe, start, head.Number)
	if err != nil {
		return result, err
	}
	result.SearchedUntil = head.Time

	// Any transfer at or past this block came from a later operation.
	laterAt := recordedAt.Add(faucetLaterOperationGap)
	if head.Time.After(laterAt) {
		laterBlock, err := p.client.BlockAtOrBefore(ctx, laterAt)
		if err != nil {
			return result, err
		}
		for _, entry := range logs {
			if !entry.Removed && entry.BlockNumber > laterBlock {
				result.LaterOperationLanded = true
				break
			}
		}
	}

	orphanBefore := head.Number
	if blocks := uint64(faucetOrphanMinAge / time.Second); orphanBefore > blocks {
		orphanBefore -= blocks
	}

	signerAddress := ""
	if signer, err := p.db.GetActiveProtocolSigner(ctx, structs.ProtocolAccountFaucet); err == nil && signer != nil {
		signerAddress = signer.EOAAddress
	}

	for _, candidate := range protocol.MatchingTransfers(logs, recipient, amount) {
		claimed, err := p.db.ClaimProtocolTransaction(ctx, db.ProtocolTransaction{
			TxHash:        candidate.TxHash,
			LogIndex:      candidate.Index,
			ChainID:       p.chainID,
			AccountKey:    structs.ProtocolAccountFaucet,
			SenderAddress: safe.Hex(),
			SignerAddress: signerAddress,
			Purpose:       db.ProtocolTxPurposePayout,
			Recipient:     recipient.Hex(),
			AmountBase:    amount,
			BlockNumber:   candidate.Block,
		})
		if err != nil {
			return result, err
		}
		if claimed {
			result.TxHash = candidate.TxHash
			return result, nil
		}
		if candidate.Block > orphanBefore {
			continue
		}
		orphaned, err := p.db.ProtocolPayoutClaimOrphaned(ctx, candidate.TxHash, recipient.Hex(), amount.String())
		if err != nil {
			return result, err
		}
		if orphaned {
			result.TxHash = strings.ToLower(candidate.TxHash)
			return result, nil
		}
	}
	return result, nil
}

// describeSearch is the line logged and stored when a payout is concluded.
func (s FaucetPayoutSearch) describeSearch(recordedAt time.Time) string {
	return fmt.Sprintf("no faucet transfer found between %s and %s, and a later faucet operation has landed",
		recordedAt.UTC().Format(time.RFC3339), s.SearchedUntil.UTC().Format(time.RFC3339))
}

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
	// How long the paymaster's signature on an operation lasts: it signs every
	// one valid from two minutes before sponsorship until seven days after
	// (read from paymasterAndData on this deployment's payouts). Past that the
	// EntryPoint refuses it, so an operation not included by then never will
	// be. A payout with no operation written down is only concluded lost once
	// this has passed for any operation its attempt could have made.
	faucetSponsorshipWindow = 7 * 24 * time.Hour
	faucetSponsorshipMargin = 24 * time.Hour
	// Claims this recent may belong to a send that is still recording its own
	// hash, so they are not adopted for anyone else.
	faucetOrphanMinAge = 5 * time.Minute
)

// FaucetPayoutSearch is what the chain says about one unresolved payout.
type FaucetPayoutSearch struct {
	// TxHash is the transfer that carried it, empty if none was found.
	TxHash string
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

const (
	// faucetOperationConfirmations is how deep a block must be before anything
	// is concluded from it. About half a minute on Celo: enough that a reorg
	// cannot take the evidence back, and that every node behind the public RPC
	// has the block.
	faucetOperationConfirmations = 30
	// faucetInclusionSearchMargin reaches back before an attempt began when
	// looking for the block that included it. The paymaster's window opens two
	// minutes before sponsorship, and this server's clock may disagree with the
	// chain's.
	faucetInclusionSearchMargin = 10 * time.Minute
)

// JournaledOperation is an operation as it was written down before it was
// sent.
type JournaledOperation struct {
	Hash   string
	Sender string
	Nonce  *big.Int
	// ValidUntil is the end of the paymaster's window. Nil when it could not
	// be read, and then an operation that has not landed is never concluded
	// lost.
	ValidUntil *time.Time
	// Since is when the attempt began; the operation cannot have been
	// included before it.
	Since time.Time
}

// FaucetOperationSearch is what the chain says about one journaled operation.
type FaucetOperationSearch struct {
	// Included: the EntryPoint has used the operation's nonce, so it ran, in
	// TxHash at Block.
	Included bool
	TxHash   string
	Block    uint64
	// Sent: it carried this payout's transfer, now claimed for the payout.
	// Included without Sent means it ran and sent nothing — reverted, or the
	// transfer inside it failed.
	Sent bool
	// Expired: never included, and the paymaster's window has closed, so it
	// never can be.
	Expired bool
}

// NeverSent reports whether the search proves no money went out.
func (s FaucetOperationSearch) NeverSent() bool {
	return (s.Included && !s.Sent) || s.Expired
}

// Describe is the line logged and stored when a payout is concluded from it.
func (s FaucetOperationSearch) Describe(opHash string) string {
	switch {
	case s.Included && !s.Sent:
		return fmt.Sprintf("operation %s ran in %s at block %d without this payout's transfer; nothing was sent", opHash, s.TxHash, s.Block)
	case s.Expired:
		return fmt.Sprintf("operation %s was never included and its paymaster window has closed; it can no longer land", opHash)
	case s.Sent:
		return fmt.Sprintf("operation %s sent this payout in %s at block %d", opHash, s.TxHash, s.Block)
	default:
		return fmt.Sprintf("operation %s has not been included yet", opHash)
	}
}

// FindFaucetOperation asks the chain what became of one operation, and claims
// its transfer for this payout if it made one.
//
// Every answer is read at a block faucetOperationConfirmations deep. Included
// comes from the EntryPoint's nonce for the operation's own key, then the block
// that included it, the handleOps transaction in that block that lists it, and
// that transaction's receipt — all of a transaction's logs or none, so a
// receipt without the transfer means nothing was sent. Expired comes from the
// paymaster's own validUntil, past which the EntryPoint refuses the operation.
// There is no inference from timing or from other operations: on this
// deployment each operation has a nonce key of its own, so a later one landing
// says nothing about an earlier one.
func (p *ProtocolService) FindFaucetOperation(ctx context.Context, op JournaledOperation, recipient common.Address, amount *big.Int) (FaucetOperationSearch, error) {
	result := FaucetOperationSearch{}
	if err := p.Available(); err != nil {
		return result, err
	}
	if !common.IsHexAddress(op.Sender) {
		return result, fmt.Errorf("operation sender %q is not an address", op.Sender)
	}
	if op.Nonce == nil || op.Nonce.Sign() < 0 {
		return result, fmt.Errorf("operation %s has no nonce recorded", op.Hash)
	}
	sender := common.HexToAddress(op.Sender)

	head, err := p.client.ChainHead(ctx)
	if err != nil {
		return result, err
	}
	if head.Number <= faucetOperationConfirmations {
		return result, nil
	}
	confirmed, err := p.client.HeadAt(ctx, head.Number-faucetOperationConfirmations)
	if err != nil {
		return result, err
	}

	included, err := p.client.OperationIncludedAt(ctx, sender, op.Nonce, confirmed.Number)
	if err != nil {
		return result, err
	}
	if !included {
		// A block after validUntil cannot include it, so not included by a
		// confirmed block past that point means never.
		if op.ValidUntil != nil && confirmed.Time.After(*op.ValidUntil) {
			result.Expired = true
		}
		return result, nil
	}

	from, err := p.client.BlockAtOrBefore(ctx, op.Since.Add(-faucetInclusionSearchMargin))
	if err != nil {
		return result, err
	}
	block, err := p.client.OperationInclusionBlock(ctx, sender, op.Nonce, from, confirmed.Number)
	if err != nil {
		return result, err
	}
	txHash, found, err := p.client.OperationTxIn(ctx, block, sender, op.Nonce)
	if err != nil {
		return result, err
	}
	if !found {
		// Included in this block by the nonce, but no handleOps here lists
		// it: submitted some other way. Not something to guess about.
		return result, fmt.Errorf("operation %s was included at block %d but no handleOps there lists it", op.Hash, block)
	}
	logs, found, err := p.client.TransfersInReceipt(ctx, txHash, p.token, sender)
	if err != nil {
		return result, err
	}
	if !found {
		return result, fmt.Errorf("no receipt for %s yet", txHash)
	}

	result.Included, result.TxHash, result.Block = true, txHash, block
	candidates := protocol.MatchingTransfers(logs, recipient, amount)
	if len(candidates) == 0 {
		return result, nil
	}

	signerAddress := ""
	if signer, err := p.db.GetActiveProtocolSigner(ctx, structs.ProtocolAccountFaucet); err == nil && signer != nil {
		signerAddress = signer.EOAAddress
	}
	for _, candidate := range candidates {
		claimed, err := p.db.ClaimProtocolTransaction(ctx, db.ProtocolTransaction{
			TxHash:        candidate.TxHash,
			LogIndex:      candidate.Index,
			ChainID:       p.chainID,
			AccountKey:    structs.ProtocolAccountFaucet,
			SenderAddress: sender.Hex(),
			SignerAddress: signerAddress,
			OperationHash: op.Hash,
			Purpose:       db.ProtocolTxPurposePayout,
			Recipient:     recipient.Hex(),
			AmountBase:    amount,
			BlockNumber:   candidate.Block,
		})
		if err != nil {
			return result, err
		}
		if !claimed {
			// Claimed already, most likely by the dispatcher for this same
			// payout before its caller stopped waiting. Taken only if no
			// payout holds it.
			if claimed, err = p.db.ProtocolPayoutClaimOrphaned(ctx, candidate.TxHash, recipient.Hex(), amount.String()); err != nil {
				return result, err
			}
		}
		if claimed {
			result.Sent = true
			return result, nil
		}
	}
	// The operation made this transfer, and another payout holds it. Neither
	// paid nor provably unpaid, so nothing is concluded.
	return result, fmt.Errorf("the transfer in %s is already held by another payout", txHash)
}

// describeSearch is the line logged and stored when a payout is concluded.
func (s FaucetPayoutSearch) describeSearch(recordedAt time.Time) string {
	return fmt.Sprintf("no faucet transfer found between %s and %s, and the sponsorship of any operation it could have gone out as has expired",
		recordedAt.UTC().Format(time.RFC3339), s.SearchedUntil.UTC().Format(time.RFC3339))
}

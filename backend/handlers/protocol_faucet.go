package handlers

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/protocol"
	"github.com/SFLuv/app/backend/structs"
	"github.com/ethereum/go-ethereum/common"
)

// Paying out of the faucet Safe, one queue deep.
//
// The problem this solves is an event: thirty people present QR codes inside a
// couple of minutes, and every payout is a sponsored operation that takes
// several seconds to settle. Done one at a time that is minutes of queue, and
// the callers at the back time out.
//
// Two things fix it together. Queued payouts are batched — everything waiting
// while an operation settles goes out as a single operation carrying many
// transfers, so thirty redemptions are two operations rather than thirty. And
// waiting is on a channel rather than a mutex, so a caller that gives up gets
// an answer that says whether its money was ever sent.
//
// That last distinction is the one with a cost attached. A payout abandoned
// before submission must release its redemption code, or the volunteer is never
// paid and the code is spent. A payout abandoned after submission must not,
// because the money may yet land and releasing the code would let the same
// reward be claimed twice.

const (
	// How many transfers go in one operation.
	//
	// Bounded by gas rather than by taste: the paymaster sponsors a limited
	// amount per operation, and a batch that exceeds it fails as a whole,
	// taking everybody in it down together. Twenty-five ERC-20 transfers is
	// comfortably inside what the engine will sponsor and turns any plausible
	// event burst into one or two operations.
	maxFaucetBatch = 25
	// How long a request may sit unqueued before the caller is told the system
	// is saturated. Generous, because the alternative for a redemption is a
	// failure the volunteer sees.
	faucetQueueDepth = 256
)

// faucetSend is one payout waiting its turn.
type faucetSend struct {
	to     common.Address
	amount *big.Int
	result chan faucetSendResult
	// submitted flips the moment this request is part of an operation that has
	// gone to the engine. After that its money may be on its way, and the
	// redemption behind it must not be released.
	submitted atomic.Bool
}

type faucetSendResult struct {
	txHash string
	err    error
}

// faucetDispatcher owns the queue and the single goroutine that drains it.
type faucetDispatcher struct {
	queue chan *faucetSend
	once  sync.Once
}

// SendFromFaucet transfers SFLUV out of the faucet Safe and returns the hash of
// the transaction that carried it.
//
// A transaction hash, not the operation hash. The payout ledger stores a value
// that is later looked up as a receipt and searched for a Transfer log; the
// engine answers with a user-operation hash, which is not a transaction and —
// on this deployment — resolves to one through no event at all, because the
// CommunityModule emits none. So the transfer is found the only way available,
// by watching the token for the log this operation was supposed to produce, and
// claimed exclusively so that two identical payouts cannot report the same one.
func (p *ProtocolService) SendFromFaucet(ctx context.Context, amountBase *big.Int, to common.Address) (string, error) {
	if err := p.Available(); err != nil {
		// Nothing was sent, so the code behind this may be presented again.
		return "", bot.Releasable(err)
	}
	if amountBase == nil || amountBase.Sign() <= 0 {
		return "", bot.Releasable(fmt.Errorf("refusing to send a non-positive amount"))
	}

	request := &faucetSend{
		to:     to,
		amount: new(big.Int).Set(amountBase),
		result: make(chan faucetSendResult, 1),
	}

	p.startFaucetDispatcher()

	// Queueing respects the caller's deadline. A redemption that gives up here
	// was never submitted, by construction: it is still in this function.
	select {
	case p.faucet.queue <- request:
	case <-ctx.Done():
		return "", bot.Releasable(fmt.Errorf("faucet queue did not accept the payout in time: %w", ctx.Err()))
	}

	select {
	case result := <-request.result:
		return result.txHash, result.err
	case <-ctx.Done():
		// Whether this is releasable depends on how far the request got, which
		// is the whole reason the flag exists. Unsubmitted: hand the code back.
		// Submitted: the money may be in flight and the code must stay spent.
		if request.submitted.Load() {
			return "", fmt.Errorf("faucet payout was submitted but not confirmed before the caller gave up: %w", ctx.Err())
		}
		return "", bot.Releasable(fmt.Errorf("faucet payout was still queued when the caller gave up: %w", ctx.Err()))
	}
}

func (p *ProtocolService) startFaucetDispatcher() {
	p.faucet.once.Do(func() {
		p.faucet.queue = make(chan *faucetSend, faucetQueueDepth)
		go p.runFaucetDispatcher()
	})
}

// runFaucetDispatcher drains the queue, batching whatever is waiting.
//
// One goroutine, so operations are submitted strictly one at a time. That is
// not a performance compromise but a requirement: the engine assigns the
// operation's nonce when it sponsors it, so two operations sponsored at the
// same moment can be handed the same nonce and one of them is rejected. The
// batching is what buys the throughput instead.
func (p *ProtocolService) runFaucetDispatcher() {
	for first := range p.faucet.queue {
		batch := []*faucetSend{first}
		// Everything already waiting joins this operation. Non-blocking, so a
		// quiet faucet sends one transfer immediately rather than waiting to
		// see whether company arrives.
		for len(batch) < maxFaucetBatch {
			select {
			case next := <-p.faucet.queue:
				batch = append(batch, next)
			default:
			}
			if len(p.faucet.queue) == 0 {
				break
			}
		}
		p.dispatchFaucetBatch(batch)
	}
}

func (p *ProtocolService) dispatchFaucetBatch(batch []*faucetSend) {
	// Its own budget, independent of any caller's. A caller that has given up
	// must not abandon an operation mid-flight for everybody else in the batch.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	finish := func(err error) {
		for _, request := range batch {
			request.result <- faucetSendResult{err: err}
		}
	}

	safe, err := p.SafeFor(ctx, structs.ProtocolAccountFaucet)
	if err != nil {
		finish(bot.Releasable(ErrFaucetNotMigrated))
		return
	}
	signerAddress := ""
	if signer, err := p.db.GetActiveProtocolSigner(ctx, structs.ProtocolAccountFaucet); err == nil && signer != nil {
		signerAddress = signer.EOAAddress
	}

	// Noted before submitting. A log search that started lower could match an
	// identical earlier payout; starting here cannot.
	fromBlock, err := p.client.BlockNumber(ctx)
	if err != nil {
		finish(bot.Releasable(fmt.Errorf("could not read the head block before a faucet send: %w", err)))
		return
	}

	callData, err := p.faucetBatchCallData(batch)
	if err != nil {
		finish(bot.Releasable(err))
		return
	}

	// Past this point the operation is going out, so nothing below is
	// releasable: the money may land whatever the caller does.
	for _, request := range batch {
		request.submitted.Store(true)
	}

	// Sponsoring, signing and submitting are held against rotations and
	// sweeps, which take the same lock.
	//
	// A swapOwner landing between the sponsor and the send would leave this
	// operation signed by an address that is no longer an owner, and it would
	// fail validation for a reason nothing in the error explains. The lock is
	// taken inside the dispatcher rather than around the whole batch so that
	// waiting for the transfers to land — the slow part — blocks nothing.
	//
	// It is also why the signer is read inside executeRaw: a batch that queued
	// behind a rotation must sign with the key that rotation installed, not the
	// one that was current when it was queued.
	p.mu.Lock()
	opHash, err := p.executeRaw(ctx, structs.ProtocolAccountFaucet, callData)
	p.mu.Unlock()
	if err != nil {
		// The submission itself failed, so nothing was accepted and the codes
		// can go back. This is the one post-marking case that is still safe to
		// release, because the engine refused it outright.
		for _, request := range batch {
			request.submitted.Store(false)
		}
		finish(bot.Releasable(fmt.Errorf("faucet operation was not accepted: %w", err)))
		return
	}

	if len(batch) > 1 {
		p.logf("protocol faucet: operation %s carries %d payouts", opHash, len(batch))
	}

	// Each payout claims its own transfer out of the operation. A batch lands
	// as one transaction with one Transfer log per payout, which is why the
	// claim is keyed on the log index and not just the hash.
	for _, request := range batch {
		txHash, err := p.client.AwaitTransfer(ctx, p.token, safe, request.to, request.amount, fromBlock,
			func(candidate string, block uint64, logIndex uint) (bool, error) {
				return p.db.ClaimProtocolTransaction(ctx, db.ProtocolTransaction{
					TxHash:        candidate,
					LogIndex:      logIndex,
					ChainID:       p.chainID,
					AccountKey:    structs.ProtocolAccountFaucet,
					SenderAddress: safe.Hex(),
					SignerAddress: signerAddress,
					OperationHash: opHash,
					Purpose:       db.ProtocolTxPurposePayout,
					Recipient:     request.to.Hex(),
					AmountBase:    request.amount,
					BlockNumber:   block,
				})
			})
		if err != nil {
			request.result <- faucetSendResult{err: fmt.Errorf(
				"faucet operation %s was submitted but this payout's transfer was not observed: %w", opHash, err)}
			continue
		}
		request.result <- faucetSendResult{txHash: txHash}
	}
}

// faucetBatchCallData is one transfer on its own, or many through MultiSend.
//
// A single payout stays on the plain path — an ordinary CALL from the Safe to
// the token — so the common case gains no delegatecall and no helper contract
// from a feature that exists for bursts.
func (p *ProtocolService) faucetBatchCallData(batch []*faucetSend) ([]byte, error) {
	if len(batch) == 1 {
		transfer, err := protocol.TransferCallData(batch[0].to, batch[0].amount)
		if err != nil {
			return nil, err
		}
		return protocol.SafeModuleCallData(p.token, big.NewInt(0), transfer)
	}

	calls := make([]protocol.BatchedCall, 0, len(batch))
	for _, request := range batch {
		transfer, err := protocol.TransferCallData(request.to, request.amount)
		if err != nil {
			return nil, err
		}
		calls = append(calls, protocol.BatchedCall{To: p.token, Value: big.NewInt(0), Data: transfer})
	}
	return protocol.BatchCallData(calls)
}

// executeRaw submits a prepared callData as the account, sponsored.
//
// Separate from Execute because a batch's callData is built by the caller: it
// is a delegatecall into MultiSend rather than a single call, and Execute's job
// of wrapping one inner call does not apply.
func (p *ProtocolService) executeRaw(ctx context.Context, accountKey string, callData []byte) (string, error) {
	account, err := p.db.GetProtocolAccount(ctx, accountKey)
	if err != nil {
		return "", err
	}
	if account == nil || !common.IsHexAddress(account.SafeAddress) {
		return "", fmt.Errorf("protocol account %s has no Safe to execute from", accountKey)
	}
	signer, _, err := p.signerFor(ctx, accountKey)
	if err != nil {
		return "", err
	}

	safe := common.HexToAddress(account.SafeAddress)
	exists, err := p.client.AccountExists(ctx, safe)
	if err != nil {
		return "", err
	}

	op := protocol.NewUserOperation(safe)
	op.CallData = callData
	if !exists {
		if op.InitCode, err = p.client.InitCode(
			common.HexToAddress(account.Signer.EOAAddress),
			big.NewInt(int64(account.OwnerIndex)),
		); err != nil {
			return "", err
		}
	}

	sponsored, err := p.client.Sponsor(ctx, op)
	if err != nil {
		return "", err
	}
	hash, err := p.client.Hash(ctx, sponsored)
	if err != nil {
		return "", err
	}
	if err := protocol.Sign(sponsored, hash, signer); err != nil {
		return "", err
	}
	return p.client.Send(ctx, sponsored, nil)
}

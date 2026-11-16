package handlers

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
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
// Two things fix it together. Payouts go to the engine in waves, up to ten at
// once, each its own operation, which the engine carries together in one
// transaction; the next wave goes as soon as the engine has taken this one, so
// thirty redemptions are three quick waves rather than thirty sends one after
// another. And waiting is on a channel rather than a mutex, so a caller that
// gives up gets an answer that says whether its money was ever sent.
//
// That last distinction is the one with a cost attached. A payout abandoned
// before submission must release its redemption code, or the volunteer is never
// paid and the code is spent. A payout abandoned after submission must not,
// because the money may yet land and releasing the code would let the same
// reward be claimed twice.

const (
	// How many transfers go in one operation when batching is on.
	//
	// Batches go through SFLuvBatcher (protocol/batcher.go), and only while
	// faucetBatcherReady says so, checked again before every batch: the
	// batcher is configured and is exactly the audited contract, the
	// paymaster would sponsor a call to it, and the Safe's allowance covers
	// the batch. Otherwise each payout goes out as its own operation.
	//
	// That date is why the checks are not optional. Batching went in through
	// Safe's MultiSendCallOnly, which the paymaster had never whitelisted, and
	// four perk-card payouts batched together reverted on chain with "AA38
	// contract not whitelisted" and left their codes spent. It could not have
	// worked even whitelisted: the CommunityModule executes every call as a
	// CALL, so MultiSend's loop would have run as itself, holding nothing.
	//
	// Twenty-five keeps one operation's gas well inside what a sponsored
	// operation carries (a 100-recipient batch is about 3M).
	maxFaucetBatch = 25
	// How many payouts go to the engine at once. Its single queue worker
	// carries up to ten operations in a pass (citizenwallet/engine,
	// internal/queue/queue.go batchSize), so a wave of ten is one handleOps
	// transaction when nothing else is queued, and never more than one pass of
	// ours waiting. The engine has no rate limiter; what it does have is a
	// twelve-second limit on how long it holds a send open, and an operation
	// still queued when that runs out is sent anyway. Keeping ours to a pass at
	// a time keeps every send well inside it.
	faucetWaveSize = 10
	// How many payouts may be waiting for their transfers at once, across
	// waves. Each wave polls the read RPC every three seconds while it waits,
	// so this bounds that load: six waves of ten.
	maxFaucetInFlight = 60
	// How long a request may sit unqueued before the caller is told the system
	// is saturated. Generous, because the alternative for a redemption is a
	// failure the volunteer sees.
	faucetQueueDepth = 256
)

// faucetBatcherAddress is the SFLuvBatcher deployment named by
// FAUCET_BATCHER_ADDRESS, or the zero address, which keeps batching off.
func faucetBatcherAddress() common.Address {
	raw := strings.TrimSpace(os.Getenv("FAUCET_BATCHER_ADDRESS"))
	if !common.IsHexAddress(raw) {
		return common.Address{}
	}
	return common.HexToAddress(raw)
}

// MaxFaucetBatch is maxFaucetBatch, for the boot log.
const MaxFaucetBatch = maxFaucetBatch

// FaucetBatcherConfigured reports whether FAUCET_BATCHER_ADDRESS names one.
func (p *ProtocolService) FaucetBatcherConfigured() bool {
	return faucetBatcherAddress() != (common.Address{})
}

// FaucetBatchingReady is faucetBatcherReady for the smallest batch: nil when a
// batch would go through the batcher now.
func (p *ProtocolService) FaucetBatchingReady(ctx context.Context) error {
	if err := p.Available(); err != nil {
		return err
	}
	safe, err := p.SafeFor(ctx, structs.ProtocolAccountFaucet)
	if err != nil {
		return err
	}
	return p.faucetBatcherReady(ctx, safe, big.NewInt(1))
}

// faucetWaveLimit is how many payouts go out together: as many as one engine
// pass carries, or a full batch when a batcher is configured.
func faucetWaveLimit() int {
	if faucetBatcherAddress() == (common.Address{}) {
		return faucetWaveSize
	}
	return maxFaucetBatch
}

// checkFaucetBatcher confirms the configured batcher is SFLuvBatcher, built for
// this token, by its runtime code hash and its token(). It is what the faucet
// Safe approves without a limit, so an address that is anything else — a typo,
// or a contract planted to drain the faucet — is never approved or used.
func (p *ProtocolService) checkFaucetBatcher(ctx context.Context) (common.Address, error) {
	batcher := faucetBatcherAddress()
	if batcher == (common.Address{}) {
		return batcher, fmt.Errorf("FAUCET_BATCHER_ADDRESS is not set")
	}
	codeHash, err := p.client.BatcherCodeHashAt(ctx, batcher)
	if err != nil {
		return batcher, err
	}
	if codeHash != protocol.BatcherCodeHash {
		return batcher, fmt.Errorf("the code at %s (hash %s) is not SFLuvBatcher (hash %s)",
			batcher.Hex(), codeHash.Hex(), protocol.BatcherCodeHash.Hex())
	}
	token, err := p.client.BatcherToken(ctx, batcher)
	if err != nil {
		return batcher, err
	}
	if token != p.token {
		return batcher, fmt.Errorf("the batcher at %s moves %s, not %s", batcher.Hex(), token.Hex(), p.token.Hex())
	}
	return batcher, nil
}

// faucetBatcherReady reports why a batch of total cannot go through the
// batcher right now, or nil when it can.
func (p *ProtocolService) faucetBatcherReady(ctx context.Context, safe common.Address, total *big.Int) error {
	batcher, err := p.checkFaucetBatcher(ctx)
	if err != nil {
		return err
	}
	sponsored, err := p.client.PaymasterSponsors(ctx, safe, batcher)
	if err != nil {
		return err
	}
	if !sponsored {
		return fmt.Errorf("the paymaster does not sponsor calls to the batcher %s; it is not on the whitelist", batcher.Hex())
	}
	allowance, err := p.client.Allowance(ctx, p.token, safe, batcher)
	if err != nil {
		return err
	}
	if allowance.Cmp(total) < 0 {
		return fmt.Errorf("the faucet's allowance to the batcher is %s, under the batch's %s", allowance, total)
	}
	return nil
}

// EnsureFaucetBatcherAllowance approves the batcher from the faucet Safe,
// once, so batches can be paid. A no-op without a batcher, when the allowance
// is already in place, or when the batcher is not the audited contract.
//
// The approval has no limit, which is safe for this contract alone: it pulls
// only from whoever calls it, so the allowance can be spent by nobody but the
// faucet Safe, calling it itself.
func (p *ProtocolService) EnsureFaucetBatcherAllowance(ctx context.Context) error {
	if faucetBatcherAddress() == (common.Address{}) {
		return nil
	}
	if err := p.Available(); err != nil {
		return err
	}
	batcher, err := p.checkFaucetBatcher(ctx)
	if err != nil {
		return err
	}
	safe, err := p.SafeFor(ctx, structs.ProtocolAccountFaucet)
	if err != nil {
		return err
	}
	half := new(big.Int).Rsh(protocol.MaxAllowance, 1)
	allowance, err := p.client.Allowance(ctx, p.token, safe, batcher)
	if err != nil {
		return err
	}
	if allowance.Cmp(half) >= 0 {
		return nil
	}

	approve, err := protocol.ApproveCallData(batcher, protocol.MaxAllowance)
	if err != nil {
		return err
	}
	callData, err := protocol.SafeModuleCallData(p.token, big.NewInt(0), approve)
	if err != nil {
		return err
	}
	p.mu.Lock()
	opHash, err := p.executeRaw(ctx, structs.ProtocolAccountFaucet, callData, nil)
	p.mu.Unlock()
	if err != nil {
		return fmt.Errorf("could not send the batcher approval: %w", err)
	}
	p.logf("protocol faucet: approving batcher %s from the faucet Safe in operation %s", batcher.Hex(), opHash)

	for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline); {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
		if allowance, err := p.client.Allowance(ctx, p.token, safe, batcher); err == nil && allowance.Cmp(half) >= 0 {
			p.logf("protocol faucet: batcher %s approved; batches go through it once the paymaster sponsors it", batcher.Hex())
			return nil
		}
	}
	return fmt.Errorf("the batcher approval in operation %s has not landed yet", opHash)
}

// A payout's lifecycle. It leaves sendQueued exactly once, by compare-and-swap,
// so the caller giving up and the dispatcher sending it can never both win:
//
//	queued -> cancelled   the caller gave up first. Never sent; its code goes back.
//	queued -> submitted   the dispatcher took it. Its money may be on its way.
//
// A cancelled request is still queued, and the dispatcher drops it when it
// gets there. Without that, a payout whose code had already been
// handed back would be paid later anyway, and the volunteer could redeem the
// same code a second time.
const (
	sendQueued int32 = iota
	sendCancelled
	sendSubmitted
)

// faucetSend is one payout waiting its turn.
type faucetSend struct {
	to     common.Address
	amount *big.Int
	// journal is handed the operation before it is sent. May be nil.
	journal bot.OperationJournal
	result  chan faucetSendResult
	state   atomic.Int32
}

// cancel claims the request for the caller. False means the dispatcher already
// took it.
func (s *faucetSend) cancel() bool {
	return s.state.CompareAndSwap(sendQueued, sendCancelled)
}

// take claims the request for the dispatcher. False means the caller already
// gave up on it.
func (s *faucetSend) take() bool {
	return s.state.CompareAndSwap(sendQueued, sendSubmitted)
}

type faucetSendResult struct {
	txHash string
	err    error
}

// faucetDispatcher owns the queue and the goroutine that turns it into waves.
type faucetDispatcher struct {
	queue chan *faucetSend
	// settled hands each wave back to the dispatcher once every payout in it
	// has been answered, freeing its recipients and its room.
	settled chan []*faucetSend
	once    sync.Once
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
//
// journal, when not nil, is handed the operation after it is signed and before
// it is sent, so the payout behind it can be checked against the chain however
// this call ends — even if the process ends with it.
func (p *ProtocolService) SendFromFaucet(ctx context.Context, amountBase *big.Int, to common.Address, journal bot.OperationJournal) (string, error) {
	if err := p.Available(); err != nil {
		// Nothing was sent, so the code behind this may be presented again.
		return "", bot.Releasable(err)
	}
	if amountBase == nil || amountBase.Sign() <= 0 {
		return "", bot.Releasable(fmt.Errorf("refusing to send a non-positive amount"))
	}

	request := &faucetSend{
		to:      to,
		amount:  new(big.Int).Set(amountBase),
		journal: journal,
		result:  make(chan faucetSendResult, 1),
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
		// Whether this is releasable depends on how far the request got, and
		// the answer has to be decided by one atomic step, not read: a check
		// followed by a return leaves a gap in which the dispatcher can take
		// the request and send it. Winning the cancel means it will never be
		// sent, so the code can go back. Losing it means the dispatcher took it
		// first, the money may be in flight, and the code must stay spent.
		if request.cancel() {
			return "", bot.Releasable(fmt.Errorf("faucet payout was still queued when the caller gave up: %w", ctx.Err()))
		}
		return "", fmt.Errorf("faucet payout was submitted but not confirmed before the caller gave up: %w", ctx.Err())
	}
}

func (p *ProtocolService) startFaucetDispatcher() {
	p.faucet.once.Do(func() {
		p.faucet.queue = make(chan *faucetSend, faucetQueueDepth)
		// One report per wave, and there are never more waves in flight than
		// payouts, so this never fills.
		p.faucet.settled = make(chan []*faucetSend, maxFaucetInFlight)
		go p.runFaucetDispatcher()
	})
}

// runFaucetDispatcher turns the queue into waves.
//
// A wave is up to faucetWaveLimit payouts, all to different people, submitted
// at the same moment as separate operations. The engine's one queue worker
// gathers whatever arrives within a quarter of a second and carries it in one
// handleOps transaction, ten at a time, so a wave of ten is one of its passes.
// The next wave goes as soon as the engine has answered every send in this one
// — when the transactions are broadcast, not when they land — so at most one
// pass's worth of ours is ever waiting in its queue. Waiting for the transfers
// to land happens in the background, for up to maxFaucetInFlight payouts.
//
// Nobody has two payouts in flight at once. A payout is matched to the Transfer
// it made by recipient and amount, so two identical payouts in flight together
// could each claim the other's; a payout to someone already being paid waits
// until theirs has settled.
//
// This used to send one operation at a time, on the belief that two operations
// sponsored together could be handed the same nonce. They are not:
// pm_ooSponsorUserOperation gives every operation a nonce key of its own at
// sequence zero (pkg/common/nonce.go) — 24 random bytes cut to 100 bits, with
// the second's timestamp ORed into the low ones, which leaves 76 bits that
// differ between two operations sponsored in the same second. The faucet's
// operations on chain each carry a different one. The nonce is set at
// sponsorship and is covered by both the paymaster's signature and the
// owner's, so nothing after that can change it.
func (p *ProtocolService) runFaucetDispatcher() {
	var held []*faucetSend              // received, not yet in a wave
	paying := map[common.Address]bool{} // recipients with a payout in flight
	inFlight := 0

	receive := func(request *faucetSend) { held = append(held, request) }
	release := func(settled []*faucetSend) {
		inFlight -= len(settled)
		for _, request := range settled {
			delete(paying, request.to)
		}
	}

	for {
		// Wait for anything that could let a payout go: a new one, or a wave
		// settling, which frees its recipients and its room.
		select {
		case request := <-p.faucet.queue:
			receive(request)
		case settled := <-p.faucet.settled:
			release(settled)
		}
	drain:
		for {
			select {
			case request := <-p.faucet.queue:
				receive(request)
			case settled := <-p.faucet.settled:
				release(settled)
			default:
				break drain
			}
		}

		for inFlight < maxFaucetInFlight {
			limit := min(faucetWaveLimit(), maxFaucetInFlight-inFlight)
			wave := make([]*faucetSend, 0, limit)
			var rest []*faucetSend
			inWave := map[common.Address]bool{}
			for _, request := range held {
				if len(wave) < limit && !paying[request.to] && !inWave[request.to] {
					wave = append(wave, request)
					inWave[request.to] = true
					continue
				}
				rest = append(rest, request)
			}
			held = rest
			if len(wave) == 0 {
				break
			}

			// Claim each request for this wave. Any the caller has already
			// given up on are dropped here — their codes were handed back, so
			// sending them now would pay a reward that can be claimed again.
			// From a successful take on, the caller can no longer cancel, so
			// nothing taken is ever released by the caller; only a definite
			// refusal hands codes back.
			wave = takeFaucetBatch(wave)
			if len(wave) == 0 {
				continue
			}
			for _, request := range wave {
				paying[request.to] = true
			}
			inFlight += len(wave)
			p.dispatchFaucetWave(wave)
		}
	}
}

// faucetSubmission is one operation the engine accepted, and the payouts it
// carries.
type faucetSubmission struct {
	requests    []*faucetSend
	opHash      string
	signed      bot.SubmittedOperation
	submittedAt time.Time
}

// dispatchFaucetWave submits a wave, every payout in it already taken, and
// returns once the engine has answered each send. Waiting for the transfers
// carries on in the background and hands the wave back on p.faucet.settled
// when it is done; so does every early exit here.
func (p *ProtocolService) dispatchFaucetWave(wave []*faucetSend) {
	// Its own budget, independent of any caller's. A caller that has given up
	// must not abandon a send mid-flight for everybody else in the wave.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	finish := func(err error) {
		for _, request := range wave {
			request.result <- faucetSendResult{err: err}
		}
		p.faucet.settled <- wave
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

	// The operations the wave goes out as: one through the batcher if it can
	// take the whole wave now — checked here, before every wave, not trusted
	// from some earlier moment — and otherwise one per payout.
	var operations [][]*faucetSend
	if len(wave) > 1 && faucetBatcherAddress() != (common.Address{}) {
		total := new(big.Int)
		for _, request := range wave {
			total.Add(total, request.amount)
		}
		if err := p.faucetBatcherReady(ctx, safe, total); err != nil {
			p.logf("protocol faucet: sending %d payouts as separate operations: %s", len(wave), err)
		} else {
			operations = [][]*faucetSend{wave}
		}
	}
	if operations == nil {
		operations = make([][]*faucetSend, len(wave))
		for i, request := range wave {
			operations[i] = []*faucetSend{request}
		}
	}

	// All at once, so they reach the engine inside one of its quarter-second
	// gathers. Sponsoring, signing and submitting are held against rotations
	// and sweeps, which take the write lock: a swapOwner landing between a
	// sponsor and its send would leave the operation signed by an address that
	// is no longer an owner, and it would fail validation for a reason nothing
	// in the error explains. Only the submitting is held — waiting for the
	// transfers to land, the slow part, blocks nothing. The signer is read
	// inside executeRaw, so a wave that queued behind a rotation signs with the
	// key that rotation installed.
	submitted := make([]*faucetSubmission, len(operations))
	var wg sync.WaitGroup
	p.mu.RLock()
	for i, requests := range operations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			submitted[i] = p.submitFaucetOperation(ctx, requests)
		}()
	}
	wg.Wait()
	p.mu.RUnlock()

	go p.settleFaucetWave(safe, signerAddress, fromBlock, wave, submitted)
}

// submitFaucetOperation sends one operation carrying every payout in requests.
// When it is not accepted it answers them itself and returns nil. Called with
// the read lock held.
func (p *ProtocolService) submitFaucetOperation(ctx context.Context, requests []*faucetSend) *faucetSubmission {
	finish := func(err error) *faucetSubmission {
		for _, request := range requests {
			request.result <- faucetSendResult{err: err}
		}
		return nil
	}

	callData, err := p.faucetBatchCallData(requests)
	if err != nil {
		return finish(bot.Releasable(err))
	}

	// Every payout in the operation writes it down before it goes. If any of
	// them cannot, nothing is sent: the operation would be one that payout could
	// never be checked against.
	submission := &faucetSubmission{requests: requests, submittedAt: time.Now()}
	journal := func(op *protocol.UserOperation, hash [32]byte) error {
		submission.signed = bot.SubmittedOperation{
			Hash:   common.Hash(hash).Hex(),
			Sender: op.Sender.Hex(),
			Nonce:  new(big.Int).Set(op.Nonce),
		}
		if validUntil, ok := protocol.ValidUntil(op.PaymasterAndData); ok {
			submission.signed.ValidUntil = &validUntil
		}
		for _, request := range requests {
			if request.journal == nil {
				continue
			}
			if err := request.journal(submission.signed); err != nil {
				return fmt.Errorf("could not record operation %s before sending it: %w", submission.signed.Hash, err)
			}
		}
		return nil
	}

	opHash, err := p.executeRaw(ctx, structs.ProtocolAccountFaucet, callData, journal)
	if err != nil {
		var uncertain *uncertainSendError
		if errors.As(err, &uncertain) {
			// The operation went to the engine and the answer was lost, or the
			// engine answered after queueing it: it may have been accepted and
			// may still land. Releasing these codes could pay every reward in
			// it twice, so they stay spent, and the operation hash is logged so
			// an operator can trace it.
			p.logf("protocol faucet: operation %s for %d payouts may or may not have been accepted: %s",
				uncertain.userOpHash, len(requests), uncertain.err)
			return finish(fmt.Errorf("faucet operation %s was submitted but its acceptance is unknown: %w",
				uncertain.userOpHash, uncertain.err))
		}
		// Failed before submission, or refused outright by the engine, so
		// nothing was accepted and the codes can go back.
		return finish(bot.Releasable(fmt.Errorf("faucet operation was not accepted: %w", err)))
	}
	if len(requests) > 1 {
		p.logf("protocol faucet: operation %s carries %d payouts", opHash, len(requests))
	}
	submission.opHash = opHash
	return submission
}

// settleFaucetWave waits for the transfers of every operation in a wave the
// engine accepted, answers each payout, and then hands the wave back so its
// recipients can be paid again.
//
// One search for the whole wave, under one deadline. Each payout claims its own
// transfer out of it — keyed on the log index, since a batch is one transaction
// with a Transfer log per payout — and each operation's transaction is pinned
// on its own, since the engine may carry the wave in one transaction or split
// it across passes. Searching payout by payout used to give each its own wait,
// so an operation that never landed held the queue for minutes.
func (p *ProtocolService) settleFaucetWave(safe common.Address, signerAddress string, fromBlock uint64, wave []*faucetSend, submitted []*faucetSubmission) {
	defer func() { p.faucet.settled <- wave }()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	type payout struct {
		request    *faucetSend
		submission *faucetSubmission
	}
	var payouts []payout
	var expected []protocol.ExpectedTransfer
	for i, submission := range submitted {
		if submission == nil {
			continue // refused, and answered when it was
		}
		for _, request := range submission.requests {
			payouts = append(payouts, payout{request: request, submission: submission})
			expected = append(expected, protocol.ExpectedTransfer{To: request.to, Amount: request.amount, Operation: i})
		}
	}
	if len(payouts) == 0 {
		return
	}

	hashes, errs := p.client.AwaitTransfers(ctx, p.token, safe, expected, fromBlock,
		func(i int, candidate string, block uint64, logIndex uint) (bool, error) {
			return p.db.ClaimProtocolTransaction(ctx, db.ProtocolTransaction{
				TxHash:        candidate,
				LogIndex:      logIndex,
				ChainID:       p.chainID,
				AccountKey:    structs.ProtocolAccountFaucet,
				SenderAddress: safe.Hex(),
				SignerAddress: signerAddress,
				OperationHash: payouts[i].submission.opHash,
				Purpose:       db.ProtocolTxPurposePayout,
				Recipient:     payouts[i].request.to.Hex(),
				AmountBase:    payouts[i].request.amount,
				BlockNumber:   block,
			})
		})
	// A transfer not seen in time is not yet a failure. Before handing back
	// "may have been sent", ask the chain once whether the operation ran: one
	// that ran without this payout's transfer sent nothing, and saying so now
	// releases the code at once instead of leaving it spent until the sweep
	// gets there; one that ran with it is paid. One that has not run yet stays
	// unconfirmed — it may still land, and the sweep follows it.
	for i, payout := range payouts {
		request, submission := payout.request, payout.submission
		if errs[i] == nil {
			request.result <- faucetSendResult{txHash: hashes[i]}
			continue
		}
		if signed := submission.signed; signed.Hash != "" {
			journaled := JournaledOperation{
				Hash: signed.Hash, Sender: signed.Sender, Nonce: signed.Nonce,
				ValidUntil: signed.ValidUntil, Since: submission.submittedAt,
			}
			search, err := p.FindFaucetOperation(ctx, journaled, request.to, request.amount)
			switch {
			case err != nil:
				p.logf("protocol faucet: could not check operation %s after its transfer went unseen: %s", signed.Hash, err)
			case search.Sent:
				request.result <- faucetSendResult{txHash: search.TxHash}
				continue
			case search.NeverSent():
				request.result <- faucetSendResult{err: bot.Releasable(fmt.Errorf(
					"faucet payout was not sent: %s", search.Describe(signed.Hash)))}
				continue
			}
		}
		request.result <- faucetSendResult{err: fmt.Errorf(
			"faucet operation %s was submitted but this payout's transfer was not observed: %w", submission.opHash, errs[i])}
	}
}

// takeFaucetBatch claims every request still wanted and drops the ones whose
// callers have already given up.
func takeFaucetBatch(batch []*faucetSend) []*faucetSend {
	taken := make([]*faucetSend, 0, len(batch))
	for _, request := range batch {
		if request.take() {
			taken = append(taken, request)
		}
	}
	return taken
}

// faucetBatchCallData is one transfer on its own, or many through the batcher.
//
// A single payout stays on the plain path — an ordinary CALL from the Safe to
// the token — so the common case gains no helper contract from a feature that
// exists for bursts. A batch reaches here only after faucetBatcherReady.
func (p *ProtocolService) faucetBatchCallData(batch []*faucetSend) ([]byte, error) {
	if len(batch) == 1 {
		transfer, err := protocol.TransferCallData(batch[0].to, batch[0].amount)
		if err != nil {
			return nil, err
		}
		return protocol.SafeModuleCallData(p.token, big.NewInt(0), transfer)
	}

	recipients := make([]common.Address, len(batch))
	amounts := make([]*big.Int, len(batch))
	for i, request := range batch {
		recipients[i] = request.to
		amounts[i] = request.amount
	}
	return protocol.BatchTransferCallData(faucetBatcherAddress(), recipients, amounts)
}

// executeRaw submits a prepared callData as the account, sponsored.
//
// Separate from Execute because a batch's callData is built by the caller: it
// is a delegatecall into MultiSend rather than a single call, and Execute's job
// of wrapping one inner call does not apply.
//
// beforeSend, when not nil, sees the signed operation and its hash before the
// engine does. An error from it stops the send, and is returned as a plain
// error — nothing has left this process.
func (p *ProtocolService) executeRaw(ctx context.Context, accountKey string, callData []byte, beforeSend func(*protocol.UserOperation, [32]byte) error) (string, error) {
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
	if beforeSend != nil {
		if err := beforeSend(sponsored, hash); err != nil {
			return "", err
		}
	}
	opHash, err := p.client.Send(ctx, sponsored, nil)
	if err != nil && !protocol.IsRejected(err) {
		// Everything above failed before anything left this process. Send is
		// the one step where a failure can mean "accepted, answer lost", so
		// unless the engine said no outright, the caller must not assume the
		// operation went nowhere.
		return "", &uncertainSendError{userOpHash: common.Hash(hash).Hex(), err: err}
	}
	return opHash, err
}

// uncertainSendError is eth_sendUserOperation failing in a way that does not
// say whether the engine accepted the operation: a dropped connection, a
// timeout, a 5xx. The operation hash is the one we signed, so the operation can
// still be traced on chain.
type uncertainSendError struct {
	userOpHash string
	err        error
}

func (e *uncertainSendError) Error() string {
	return fmt.Sprintf("operation %s: %s", e.userOpHash, e.err)
}

func (e *uncertainSendError) Unwrap() error { return e.err }

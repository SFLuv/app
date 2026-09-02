package handlers

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/structs"
	"github.com/SFLuv/app/backend/w9provider"
)

// PayoutService is the one way money leaves the faucet.
//
// Before this existed there were three independent send paths and a tax check
// on one of them, so a person could be paid past the reporting threshold by any
// of the other two without anything noticing. Everything now goes through Pay,
// which either sends or holds, and records what it did either way.
//
// payout_choke_point_test.go fails the build if a new direct send appears
// outside this file.
type PayoutService struct {
	appDb    *db.AppDB
	bot      bot.IBot
	provider w9provider.Provider
	logger   *logger.LogCloser
	chainID  int64

	// notify is set after construction; the notification path needs AppService,
	// which needs this service. Nil is tolerated so tests need not wire it.
	notify *AppService
}

func NewPayoutService(appDb *db.AppDB, b bot.IBot, provider w9provider.Provider, appLogger *logger.LogCloser, chainID int64) *PayoutService {
	return &PayoutService{appDb: appDb, bot: b, provider: provider, logger: appLogger, chainID: chainID}
}

func (p *PayoutService) SetAppService(a *AppService) { p.notify = a }

// Enforcement modes.
//
// W9_ENFORCEMENT is a plain on/off switch: true withholds at the tiers, false
// computes the decision, records what it would have done, and pays anyway.
// "shadow" is that recorded-but-paid state — how the gate is proven against
// real traffic before it is allowed to hold anyone's money.
const (
	enforcementShadow  = "shadow"
	enforcementEnforce = "enforce"
)

// payoutEnforcementMode reads W9_ENFORCEMENT as a boolean. The canonical values
// are "true" (enforce) and "false" (do not withhold), but the older "enforce"
// and "shadow"/"off" strings are kept working so an existing environment does
// not silently flip when this parsing changed.
func payoutEnforcementMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("W9_ENFORCEMENT"))) {
	case "true", "enforce", "1", "yes", "on":
		return enforcementEnforce
	default:
		// Anything else — false, shadow, off, empty, or unreadable — computes
		// the decision but still pays. Defaulting to no-withholding means
		// deploying this code cannot, on its own, start holding anyone's money.
		return enforcementShadow
	}
}

// escrowWindow is how long a held payout releases automatically once the form
// is filed. After it, the money stops being reserved and becomes an
// admin-approved back payment instead.
func escrowWindow() time.Duration {
	if days, err := strconv.Atoi(strings.TrimSpace(os.Getenv("W9_ESCROW_WINDOW_DAYS"))); err == nil && days > 0 {
		return time.Duration(days) * 24 * time.Hour
	}
	return 7 * 24 * time.Hour
}

type PayoutRequest struct {
	// IdempotencyKey makes a retry safe. Same key, same payout, once.
	IdempotencyKey   string
	UserID           string
	RecipientAddress string
	AmountBase       *big.Int
	Source           string
	SourceRef        string
}

type PayoutResult struct {
	LedgerID int64
	State    string
	TxHash   string
	TaxYear  int
	Escrowed bool
	// Blocked means the payout was refused outright. Distinct from Escrowed:
	// escrowed money is the person's and waiting, refused money never moved and
	// the source — a redemption code — stays claimable.
	Blocked bool
	// Tier is the warning level this payout put the person at, if any. Drives
	// which modal they see.
	Tier       string
	AmountBase *big.Int
}

// What a payout decision can conclude.
const (
	payoutActionPay    = "pay"
	payoutActionEscrow = "escrow"
	payoutActionBlock  = "block"
)

// payoutDecision is the whole gate, extracted as a pure function so the rules
// can be tested exhaustively without a database, a chain, or a clock.
type payoutDecision struct {
	Action string
	// Tier names how close this person now is to the reporting threshold, and
	// drives which warning they see. Empty below the first tier.
	Tier   string
	Reason string
}

func (d payoutDecision) escrowed() bool { return d.Action == payoutActionEscrow }
func (d payoutDecision) blocked() bool  { return d.Action == payoutActionBlock }

// payoutThresholds are the three lines a person's annual total can cross.
type payoutThresholds struct {
	Notice  *big.Int // a polite heads-up; still paid
	Warning *big.Int // a firmer one; still paid
	Limit   *big.Int // the reporting threshold itself
}

// decidePayout answers: may we send this person this money right now, and what
// should they be told?
//
// The escalation matters more than any single rule. Someone earning steadily
// gets a friendly notice, then a firmer one, and only then does anything stop —
// so by the time money is withheld they have been warned twice while still
// being paid. Withholding is the third step, not the first.
//
//   - A cleared filing pays through, always.
//   - Someone already holding money is BLOCKED rather than held again. This is
//     what bounds escrow to a single payment: nothing accumulates, so there is
//     never a pile of held money to reconcile or an expiry to chase.
//   - Otherwise the annual total decides, counting this payment. The payment
//     that crosses the limit is held in full rather than split — one reward is
//     one thing, and half a reward cannot be explained to a volunteer.
func decidePayout(priorTotal *big.Int, amount *big.Int, thresholds payoutThresholds, hasOpenEscrow bool, filingStatus string) payoutDecision {
	if db.W9StatusClears(filingStatus) {
		return payoutDecision{Action: payoutActionPay, Reason: "w9 on file"}
	}

	// Already holding money and still no form. Paying now would put them past
	// the limit; holding again would grow an obligation nobody is tracking.
	if hasOpenEscrow {
		return payoutDecision{
			Action: payoutActionBlock,
			Tier:   db.W9TierBlocked,
			Reason: "already holding a payout for this person pending their w9",
		}
	}

	if priorTotal == nil {
		priorTotal = big.NewInt(0)
	}
	if amount == nil {
		amount = big.NewInt(0)
	}
	// An unset limit must fail open. Failing closed would stop every payout on
	// the platform the moment an env var went missing.
	if thresholds.Limit == nil || thresholds.Limit.Sign() <= 0 {
		return payoutDecision{Action: payoutActionPay, Reason: "no threshold configured"}
	}

	newTotal := new(big.Int).Add(priorTotal, amount)

	if newTotal.Cmp(thresholds.Limit) >= 0 {
		return payoutDecision{
			Action: payoutActionEscrow,
			Tier:   db.W9TierEscrowed,
			Reason: "annual total reaches the reporting threshold",
		}
	}
	if thresholds.Warning != nil && thresholds.Warning.Sign() > 0 && newTotal.Cmp(thresholds.Warning) >= 0 {
		return payoutDecision{
			Action: payoutActionPay,
			Tier:   db.W9TierWarning,
			Reason: "approaching the reporting threshold",
		}
	}
	if thresholds.Notice != nil && thresholds.Notice.Sign() > 0 && newTotal.Cmp(thresholds.Notice) >= 0 {
		return payoutDecision{
			Action: payoutActionPay,
			Tier:   db.W9TierNotice,
			Reason: "past the first warning threshold",
		}
	}
	return payoutDecision{Action: payoutActionPay, Reason: "below the reporting threshold"}
}

// Pay sends money, or holds it and says why.
//
// The sequence matters. The ledger row is written first so nothing can be paid
// without being recorded; the advisory lock is taken before the total is read
// so two concurrent payouts cannot both believe they are below the line; and
// the chain call happens after the transaction commits, because a transfer that
// takes two minutes must not hold a database lock for two minutes.
func (p *PayoutService) Pay(ctx context.Context, req PayoutRequest) (*PayoutResult, error) {
	if req.AmountBase == nil || req.AmountBase.Sign() <= 0 {
		return nil, fmt.Errorf("payout amount must be positive")
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return nil, fmt.Errorf("payout requires an idempotency key")
	}

	recipient := strings.TrimSpace(req.RecipientAddress)
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		// Redemption is unauthenticated, so an address may belong to nobody.
		// Resolve if we can; if not, the payout still goes through and is
		// attributed later, when a wallet is linked.
		if lookup, err := p.appDb.GetWalletAddressOwnerLookup(ctx, recipient); err == nil && lookup != nil {
			userID = lookup.UserID
		}
	}

	taxYear := time.Now().UTC().Year()
	row := &structs.PayoutLedgerRow{
		IdempotencyKey:        req.IdempotencyKey,
		UserID:                userID,
		RecipientAddress:      recipient,
		ChainID:               p.chainID,
		TaxYear:               taxYear,
		Source:                req.Source,
		SourceRef:             req.SourceRef,
		AmountBase:            req.AmountBase.String(),
		CountsTowardThreshold: true,
	}

	decision, created, err := p.recordAndDecide(ctx, row, req.AmountBase, userID, taxYear)
	if err != nil {
		return nil, err
	}

	// An existing row that already settled is the retry case: report what
	// happened the first time rather than paying again.
	//
	// Blocked must be carried through, not just Escrowed. A refused payout
	// leaves a cancelled row and hands the redemption code back, so the same
	// code WILL be presented again — that is the design. Omitting Blocked here
	// made that second attempt look like a plain success: the caller answered
	// HTTP 200 and sent nothing, telling a volunteer they had been paid when
	// they had not. Found by the threshold-crossing test on 2026-08-19.
	if !created && row.State != db.PayoutStatePending {
		return &PayoutResult{
			LedgerID: row.ID, State: row.State, TxHash: row.TxHash,
			TaxYear:    row.TaxYear,
			Escrowed:   row.State == db.PayoutStateEscrowed,
			Blocked:    row.State == db.PayoutStateCancelled,
			AmountBase: req.AmountBase,
		}, nil
	}

	// Refused. The ledger row is cancelled rather than left pending, so it stops
	// counting toward the annual total — a payment that never happened is not
	// income, and leaving it counted would push the person further past a line
	// they were already blocked at.
	if decision.blocked() {
		if err := p.appDb.MarkPayoutCancelled(ctx, row.ID, decision.Reason); err != nil && p.logger != nil {
			p.logger.Logf("w9: payout %d was refused but could not be cancelled: %s", row.ID, err)
		}
		p.onBlocked(ctx, userID, taxYear)
		return &PayoutResult{
			LedgerID: row.ID, State: db.PayoutStateCancelled, TaxYear: taxYear,
			Blocked: true, Tier: decision.Tier, AmountBase: req.AmountBase,
		}, nil
	}

	if decision.escrowed() {
		p.onEscrowed(ctx, userID, taxYear, req.AmountBase)
		return &PayoutResult{
			LedgerID: row.ID, State: db.PayoutStateEscrowed, TaxYear: taxYear,
			Escrowed: true, Tier: decision.Tier, AmountBase: req.AmountBase,
		}, nil
	}

	// A warning tier that still pays. Recorded before the transfer so the modal
	// is waiting by the time they look at the reward that triggered it.
	if decision.Tier != "" {
		p.onTierReached(ctx, userID, taxYear, decision.Tier)
	}

	txHash, err := p.transfer(ctx, row.ID, recipient, req.AmountBase)
	if err != nil {
		// The row counted toward the threshold from the moment it was inserted,
		// which is what stops a concurrent payout slipping past it. A send that
		// never happened must stop counting, or one failure would hold this
		// person's money for the rest of the year.
		if markErr := p.appDb.MarkPayoutFailed(ctx, row.ID, err.Error()); markErr != nil && p.logger != nil {
			p.logger.Logf("w9: payout %d failed and could not be marked failed: %s", row.ID, markErr)
		}
		return nil, err
	}
	if err := p.appDb.MarkPayoutPaid(ctx, row.ID, txHash, time.Now().UTC().Year()); err != nil {
		return nil, err
	}

	return &PayoutResult{
		LedgerID: row.ID, State: db.PayoutStatePaid, TxHash: txHash,
		TaxYear: taxYear, AmountBase: req.AmountBase,
	}, nil
}

// recordAndDecide writes the ledger row and settles the escrow question inside
// one transaction, holding the per-person lock while it does.
func (p *PayoutService) recordAndDecide(ctx context.Context, row *structs.PayoutLedgerRow, amount *big.Int, userID string, taxYear int) (payoutDecision, bool, error) {
	tx, err := p.appDb.BeginTx(ctx)
	if err != nil {
		return payoutDecision{}, false, fmt.Errorf("error starting payout transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	created, err := p.appDb.InsertPayoutIntent(ctx, tx, row)
	if err != nil {
		return payoutDecision{}, false, err
	}
	if !created && row.State != db.PayoutStatePending {
		return payoutDecision{}, false, tx.Commit(ctx)
	}

	if err := db.LockUserTaxYear(ctx, tx, userID, taxYear); err != nil {
		return payoutDecision{}, created, err
	}

	priorTotal, err := p.appDb.SumCountedPayoutsForUserYear(ctx, tx, userID, taxYear)
	if err != nil {
		return payoutDecision{}, created, err
	}
	// This payout's own row is already counted by the sum once it exists, so
	// remove it to get the true prior total.
	priorTotal = new(big.Int).Sub(priorTotal, amount)
	if priorTotal.Sign() < 0 {
		priorTotal = big.NewInt(0)
	}

	hasOpen, err := p.appDb.HasOpenEscrow(ctx, tx, userID, taxYear)
	if err != nil {
		return payoutDecision{}, created, err
	}
	filingStatus, err := p.appDb.GetW9FilingStatusTx(ctx, tx, userID, taxYear)
	if err != nil {
		return payoutDecision{}, created, err
	}

	decision := decidePayout(priorTotal, amount, w9Thresholds(), hasOpen, filingStatus)

	// An unidentified recipient cannot be asked for a form, so there is nothing
	// to wait for and withholding would only strand the money. It is paid and
	// attributed later, when a wallet is linked.
	if userID == "" && decision.Action != payoutActionPay {
		decision = payoutDecision{Action: payoutActionPay, Reason: "recipient is not linked to an account"}
	}

	// Shadow mode proves the gate against real traffic without withholding from
	// anybody: the decision is computed and recorded, and the money still goes.
	mode := payoutEnforcementMode()
	if decision.Action != payoutActionPay && mode != enforcementEnforce {
		if err := p.appDb.MarkPayoutShadowDecision(ctx, tx, row.ID, "would_"+decision.Action+":"+decision.Reason); err != nil {
			return payoutDecision{}, created, err
		}
		if p.logger != nil {
			p.logger.Logf("w9 %s: would %s %s for user %s (%s)", mode, decision.Action, row.AmountBase, userID, decision.Reason)
		}
		decision.Action = payoutActionPay
	}

	if decision.escrowed() {
		if err := p.appDb.MarkPayoutEscrowed(ctx, tx, row.ID); err != nil {
			return payoutDecision{}, created, err
		}
		if err := p.appDb.EnsureW9FilingRequestedTx(ctx, tx, userID, taxYear); err != nil {
			return payoutDecision{}, created, err
		}
		row.State = db.PayoutStateEscrowed
	}

	if err := tx.Commit(ctx); err != nil {
		return payoutDecision{}, created, fmt.Errorf("error committing payout decision: %w", err)
	}
	return decision, created, nil
}

// w9Thresholds reads the three lines from the environment so they can be moved
// without a deploy.
//
// The two warning lines are courtesies and default off-limit-relative: 400 and
// 500 against a 600 limit. A warning above the limit would never fire, so both
// are clamped below it rather than trusted blindly.
func w9Thresholds() payoutThresholds {
	limit := w9ThresholdBase()
	thresholds := payoutThresholds{
		Limit:   limit,
		Notice:  w9TierThresholdBase("W9_TIER_NOTICE_SFLUV", 400),
		Warning: w9TierThresholdBase("W9_TIER_WARNING_SFLUV", 500),
	}
	if limit != nil && limit.Sign() > 0 {
		if thresholds.Warning != nil && thresholds.Warning.Cmp(limit) >= 0 {
			thresholds.Warning = nil
		}
		if thresholds.Notice != nil && thresholds.Notice.Cmp(limit) >= 0 {
			thresholds.Notice = nil
		}
	}
	return thresholds
}

func w9TierThresholdBase(envKey string, defaultSfluv int64) *big.Int {
	sfluv := defaultSfluv
	if raw := strings.TrimSpace(os.Getenv(envKey)); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			// A zero or unreadable value disables that tier rather than
			// defaulting it back on — turning a warning off should be possible.
			return nil
		}
		sfluv = parsed
	}
	multiplier, err := getTokenMultiplier()
	if err != nil || multiplier == nil || multiplier.Sign() <= 0 {
		multiplier = big.NewInt(1_000_000)
	}
	return new(big.Int).Mul(big.NewInt(sfluv), multiplier)
}

// transfer moves the money and waits for confirmation.
//
// The hash is recorded before the wait deliberately: a wait that times out must
// leave enough behind to reconcile against the chain, rather than losing track
// of a transfer that may well have succeeded.
func (p *PayoutService) transfer(ctx context.Context, ledgerID int64, recipient string, amount *big.Int) (string, error) {
	if p.bot == nil {
		return "", fmt.Errorf("no faucet is configured")
	}

	txHash, err := p.bot.SubmitTransferBaseUnits(amount, recipient)
	if err != nil {
		return "", fmt.Errorf("error sending payout: %w", err)
	}
	if err := p.appDb.RecordPayoutTxHash(ctx, ledgerID, txHash, p.chainID); err != nil {
		return txHash, err
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for {
		result, err := p.bot.VerifyTransferBaseUnits(waitCtx, txHash, recipient, amount)
		if err == nil && result != nil && result.Found && result.Successful && !result.Pending {
			return txHash, nil
		}
		select {
		case <-waitCtx.Done():
			// Not an error the caller should retry through: the transfer may
			// have landed. The reconciler settles it from the recorded hash.
			return txHash, nil
		case <-time.After(2 * time.Second):
		}
	}
}

// w9ThresholdBase is the annual reporting threshold in token base units.
//
// Replaces the old W9_LIMIT_WEI / W9_LIMIT_SFLUV pair, whose names were both
// misleading — SFLUV has six decimals, not eighteen, and TOKEN_DECIMALS in this
// codebase is a multiplier rather than an exponent.
func w9ThresholdBase() *big.Int {
	sfluv := int64(600)
	if raw := strings.TrimSpace(os.Getenv("W9_THRESHOLD_SFLUV")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			sfluv = parsed
		}
	}
	multiplier, err := getTokenMultiplier()
	if err != nil || multiplier == nil || multiplier.Sign() <= 0 {
		multiplier = big.NewInt(1_000_000)
	}
	return new(big.Int).Mul(big.NewInt(sfluv), multiplier)
}

func taxYearNow() int { return time.Now().UTC().Year() }

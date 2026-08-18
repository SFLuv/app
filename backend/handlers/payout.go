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
// shadow computes the decision, records what it would have done, and pays
// anyway. It is how the gate is proven against real traffic before it is
// allowed to hold anyone's money — and the only reliable way to discover a
// payout path that was missed.
const (
	enforcementShadow  = "shadow"
	enforcementEnforce = "enforce"
	enforcementOff     = "off"
)

func payoutEnforcementMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("W9_ENFORCEMENT"))) {
	case enforcementEnforce:
		return enforcementEnforce
	case enforcementOff:
		return enforcementOff
	default:
		// Shadow is the default so that deploying this code cannot, on its own,
		// start withholding money from anybody.
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
	LedgerID   int64
	State      string
	TxHash     string
	TaxYear    int
	Escrowed   bool
	AmountBase *big.Int
}

// escrowDecision is the whole gate, extracted as a pure function so the rules
// can be tested exhaustively without a database, a chain, or a clock.
type escrowDecision struct {
	Escrow bool
	Reason string
}

// decideEscrow answers: may we send this person this money right now?
//
//   - A cleared filing pays through, always.
//   - Anyone already holding money keeps holding it. Letting a later payment
//     slip past held ones would be incoherent, and would let someone drain the
//     obligation by earning in small pieces.
//   - Otherwise it is the annual total that decides, counting this payment. The
//     payment that crosses the line is held in full rather than split: one
//     reward is one thing, and half a reward is impossible to explain to a
//     volunteer.
func decideEscrow(priorTotal *big.Int, amount *big.Int, threshold *big.Int, hasOpenEscrow bool, filingStatus string) escrowDecision {
	if db.W9StatusClears(filingStatus) {
		return escrowDecision{Escrow: false, Reason: "w9 on file"}
	}
	if hasOpenEscrow {
		return escrowDecision{Escrow: true, Reason: "already holding money for this person"}
	}
	if priorTotal == nil {
		priorTotal = big.NewInt(0)
	}
	if amount == nil {
		amount = big.NewInt(0)
	}
	if threshold == nil || threshold.Sign() <= 0 {
		return escrowDecision{Escrow: false, Reason: "no threshold configured"}
	}

	newTotal := new(big.Int).Add(priorTotal, amount)
	if newTotal.Cmp(threshold) >= 0 {
		return escrowDecision{Escrow: true, Reason: "annual total reaches the reporting threshold"}
	}
	return escrowDecision{Escrow: false, Reason: "below the reporting threshold"}
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
	if !created && row.State != db.PayoutStatePending {
		return &PayoutResult{
			LedgerID: row.ID, State: row.State, TxHash: row.TxHash,
			TaxYear: row.TaxYear, Escrowed: row.State == db.PayoutStateEscrowed,
			AmountBase: req.AmountBase,
		}, nil
	}

	if decision.Escrow {
		p.onEscrowed(ctx, userID, taxYear, req.AmountBase)
		return &PayoutResult{
			LedgerID: row.ID, State: db.PayoutStateEscrowed, TaxYear: taxYear,
			Escrowed: true, AmountBase: req.AmountBase,
		}, nil
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
func (p *PayoutService) recordAndDecide(ctx context.Context, row *structs.PayoutLedgerRow, amount *big.Int, userID string, taxYear int) (escrowDecision, bool, error) {
	tx, err := p.appDb.BeginTx(ctx)
	if err != nil {
		return escrowDecision{}, false, fmt.Errorf("error starting payout transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	created, err := p.appDb.InsertPayoutIntent(ctx, tx, row)
	if err != nil {
		return escrowDecision{}, false, err
	}
	if !created && row.State != db.PayoutStatePending {
		return escrowDecision{}, false, tx.Commit(ctx)
	}

	if err := db.LockUserTaxYear(ctx, tx, userID, taxYear); err != nil {
		return escrowDecision{}, created, err
	}

	priorTotal, err := p.appDb.SumCountedPayoutsForUserYear(ctx, tx, userID, taxYear)
	if err != nil {
		return escrowDecision{}, created, err
	}
	// This payout's own row is already counted by the sum once it exists, so
	// remove it to get the true prior total.
	priorTotal = new(big.Int).Sub(priorTotal, amount)
	if priorTotal.Sign() < 0 {
		priorTotal = big.NewInt(0)
	}

	hasOpen, err := p.appDb.HasOpenEscrow(ctx, tx, userID, taxYear)
	if err != nil {
		return escrowDecision{}, created, err
	}
	filingStatus, err := p.appDb.GetW9FilingStatusTx(ctx, tx, userID, taxYear)
	if err != nil {
		return escrowDecision{}, created, err
	}

	decision := decideEscrow(priorTotal, amount, w9ThresholdBase(), hasOpen, filingStatus)

	// An unidentified recipient cannot be asked for a form, so there is nothing
	// to wait for and holding the money would strand it. It is paid and
	// attributed later, when a wallet is linked.
	if decision.Escrow && userID == "" {
		decision = escrowDecision{Escrow: false, Reason: "recipient is not linked to an account"}
	}

	mode := payoutEnforcementMode()
	if decision.Escrow && mode != enforcementEnforce {
		if err := p.appDb.MarkPayoutShadowDecision(ctx, tx, row.ID, "would_escrow:"+decision.Reason); err != nil {
			return escrowDecision{}, created, err
		}
		if p.logger != nil {
			p.logger.Logf("w9 %s: would escrow %s for user %s (%s)", mode, row.AmountBase, userID, decision.Reason)
		}
		decision.Escrow = false
	}

	if decision.Escrow {
		if err := p.appDb.MarkPayoutEscrowed(ctx, tx, row.ID); err != nil {
			return escrowDecision{}, created, err
		}
		if err := p.appDb.EnsureW9FilingRequestedTx(ctx, tx, userID, taxYear); err != nil {
			return escrowDecision{}, created, err
		}
		row.State = db.PayoutStateEscrowed
	}

	if err := tx.Commit(ctx); err != nil {
		return escrowDecision{}, created, fmt.Errorf("error committing payout decision: %w", err)
	}
	return decision, created, nil
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

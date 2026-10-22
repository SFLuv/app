package bootstrap

import (
	"context"
	"math/big"
	"time"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/logger"
	"github.com/ethereum/go-ethereum/common"
)

// StartProtocolAccounts brings the backend's own Safes up at boot.
//
// Returns immediately and does the work on its own goroutine. It used to run
// inline, which put a chain round trip and two 90-second waits in front of
// every other background job: a slow engine delayed the payout sweep and
// workflow maintenance on every deploy. Nothing below is on anybody's critical
// path, so nothing below should hold up what is.
//
// Runs under the jobs leader, so one process does this however many are
// serving. Four stages, and the ones that touch the chain are off by default:
//
//  1. Register the accounts, mint a signer for any that lacks one, derive the
//     Safe addresses, and record the addresses that held each role before.
//     Always runs: it writes only to our own database, and the panel cannot
//     say anything useful until it has.
//  2. Deploy any Safe that is not on chain yet — needs
//     PROTOCOL_DEPLOY_ENABLED. Main auto-deploys, so without that gate
//     merging a pull request would put contracts on chain.
//  3. Hand the redeemer role to its Safe — same gate, same reason.
//  4. Sweep the old balances and revoke the old authority — needs
//     PROTOCOL_SWEEP_ENABLED. The irreversible half.
//
// Stages 2 to 4 are skipped outright once the wallets exist, so every later
// release — including the branches queued behind this one, which boot this same
// sequence — reaches the chain not at all.
//
// Failure is logged and the server carries on. A faucet whose Safe is not up
// keeps paying from its EOA, which is what it did yesterday, so a bad deploy of
// this degrades to the previous behaviour rather than stopping payouts.
func StartProtocolAccounts(
	ctx context.Context,
	protocolSvc *handlers.ProtocolService,
	botClient *bot.Bot,
	redeemer *handlers.RedeemerService,
	appLogger *logger.LogCloser,
) {
	if protocolSvc == nil {
		return
	}
	go runProtocolAccounts(ctx, protocolSvc, botClient, redeemer, appLogger)
}

func runProtocolAccounts(
	ctx context.Context,
	protocolSvc *handlers.ProtocolService,
	botClient *bot.Bot,
	redeemer *handlers.RedeemerService,
	appLogger *logger.LogCloser,
) {
	protocolSvc.LogConfig(ctx, appLogger)
	if err := protocolSvc.Available(); err != nil {
		return
	}

	if err := protocolSvc.EnsureAccounts(ctx); err != nil {
		appLogger.Logf("protocol accounts: could not be prepared: %s", err)
		return
	}

	// The wallets already exist, so the cutover is history. Nothing on chain
	// from here — just point the faucet at what is already there.
	if protocolSvc.CutoverComplete(ctx) {
		appLogger.Logf("protocol accounts: already migrated — nothing to deploy, sweep or hand over")
		keepFaucetAttached(ctx, protocolSvc, botClient, appLogger)
		attachRedeemer(ctx, protocolSvc, redeemer, appLogger, false)
		return
	}

	if protocolSvc.DeployEnabled() {
		if err := protocolSvc.DeployAccounts(ctx, false); err != nil {
			appLogger.Logf("protocol accounts: deployment pass failed: %s", err)
		}
	} else {
		appLogger.Logf(
			"protocol accounts: Safes are derived but NOT deployed (%s is not true). "+
				"The old EOAs are still doing the work.",
			handlers.EnvDeployEnabled,
		)
	}

	if protocolSvc.SweepEnabled() {
		if err := protocolSvc.SweepLegacyFunds(ctx, false); err != nil {
			appLogger.Logf("protocol accounts: fund sweep failed: %s", err)
		}
	}

	// The faucet only starts spending from its Safe once the Safe is deployed,
	// has a signer and holds a balance. Checked here rather than assumed from
	// the steps above, so a half-finished cutover leaves the EOA in charge
	// instead of pointing payouts at an empty account.
	keepFaucetAttached(ctx, protocolSvc, botClient, appLogger)
	attachRedeemer(ctx, protocolSvc, redeemer, appLogger, protocolSvc.DeployEnabled())
}

// attachFaucet points payouts at the faucet Safe, retrying until it can.
//
// Returns whether the faucet is now attached. Two reasons it might not be, and
// they want opposite responses:
//
//   - The cutover has not happened. Nothing to attach; the EOA is legitimately
//     paying and retrying would log forever about a state that is correct.
//   - The cutover has happened and something transient got in the way — an RPC
//     blip reading the balance, say. Every payout until this succeeds is now a
//     refusal, so this must keep trying rather than leaving the process in that
//     state until somebody redeploys. That was the original bug here: one
//     failed balance read and the faucet stayed on the EOA path for the life of
//     the process, signing with a key whose account had been emptied.
func attachFaucet(
	ctx context.Context,
	protocolSvc *handlers.ProtocolService,
	botClient *bot.Bot,
	appLogger *logger.LogCloser,
	quiet bool,
) bool {
	if botClient == nil {
		return false
	}
	logf := func(format string, args ...any) {
		if !quiet {
			appLogger.Logf(format, args...)
		}
	}

	account, err := protocolSvc.FaucetAccount(ctx)
	if err != nil || account == nil {
		logf("protocol accounts: faucet could not be read to attach: %v", err)
		return false
	}
	if account.DeployedAt == nil || account.Signer == nil || !common.IsHexAddress(account.SafeAddress) {
		logf("protocol accounts: faucet still paying from its EOA — Safe not ready")
		return false
	}

	// Before anything is routed through it, confirm the key we hold actually
	// owns the Safe. A database restored from before a rotation looks healthy
	// in every other respect.
	if err := protocolSvc.VerifyFaucetSigner(ctx); err != nil {
		// Never quiet: this one does not fix itself, and it is the failure that
		// would otherwise look like the engine misbehaving.
		appLogger.Logf("protocol accounts: REFUSING to attach the faucet: %s", err)
		return false
	}

	balance, err := protocolSvc.FaucetBalance(ctx)
	if err != nil {
		logf("protocol accounts: faucet Safe balance unreadable, retrying: %s", err)
		return false
	}
	if balance == nil || balance.Sign() == 0 {
		logf(
			"protocol accounts: faucet Safe %s is deployed but empty — nothing to pay from yet",
			account.SafeAddress,
		)
		return false
	}

	botClient.SetFaucet(
		func(ctx context.Context, amountBase *big.Int, to common.Address) (string, error) {
			return protocolSvc.SendFromFaucet(ctx, amountBase, to)
		},
		func(ctx context.Context) (*big.Int, error) {
			return protocolSvc.FaucetBalance(ctx)
		},
	)
	appLogger.Logf(
		"protocol accounts: faucet now pays from Safe %s (balance %s base units)",
		account.SafeAddress, balance.String(),
	)
	return true
}

// keepFaucetAttached retries the attach until it takes.
//
// Only worth doing once the cutover is complete, because only then is a
// detached faucet an outage: every payout refuses until this succeeds, which is
// the correct behaviour — the alternative is signing with the leaked key
// against an emptied account — but it is not a state to leave a process in.
//
// Backs off to two minutes and then stays there, and logs only the first
// failure of each attempt cycle, so an engine that is down for an hour does not
// fill the log with the same line.
func keepFaucetAttached(
	ctx context.Context,
	protocolSvc *handlers.ProtocolService,
	botClient *bot.Bot,
	appLogger *logger.LogCloser,
) {
	if attachFaucet(ctx, protocolSvc, botClient, appLogger, false) {
		return
	}
	if !protocolSvc.CutoverComplete(ctx) {
		// Pre-cutover: the EOA is paying and that is right. Nothing to retry.
		return
	}

	appLogger.Logf("protocol accounts: faucet Safe not attached — payouts are refused until it is; retrying")
	go func() {
		delay := 5 * time.Second
		const maxDelay = 2 * time.Minute
		attempts := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			attempts++
			// Quiet except for the first few tries and then every tenth, so a
			// long outage leaves a trail without burying everything else.
			quiet := attempts > 3 && attempts%10 != 0
			if attachFaucet(ctx, protocolSvc, botClient, appLogger, quiet) {
				appLogger.Logf("protocol accounts: faucet attached after %d attempt(s)", attempts)
				return
			}
			if delay < maxDelay {
				delay *= 2
				if delay > maxDelay {
					delay = maxDelay
				}
			}
		}
	}()
}

// attachRedeemer moves the granting role onto the redeemer admin's Safe and
// routes grants through it.
//
// mayHandOver is the on-chain gate. Granting a token role is a write, so it
// needs the same permission as deploying; with the gate shut the Safe may still
// hold the role from an earlier run, and that case is checked rather than
// assumed, because if it does hold it then the env key should no longer be the
// one granting.
//
// The handover itself is idempotent and returns early once the token reports
// the Safe holds the role. The revoke is gated with the sweep, because it is
// the irreversible half: after it, only the Safe can grant REDEEMER_ROLE, and
// only the database holds the key that signs for the Safe.
func attachRedeemer(
	ctx context.Context,
	protocolSvc *handlers.ProtocolService,
	redeemer *handlers.RedeemerService,
	appLogger *logger.LogCloser,
	mayHandOver bool,
) {
	if redeemer == nil || !redeemer.IsEnabled() {
		return
	}
	if _, ok := redeemer.GrantAuthorityRole(); !ok {
		return
	}

	if !mayHandOver {
		held, err := protocolSvc.RedeemerSafeHoldsAuthority(ctx, redeemer)
		if err != nil || !held {
			return
		}
		redeemer.SetRoleGranter(protocolSvc.RedeemerRoleGranter())
		appLogger.Logf("protocol accounts: REDEEMER_ROLE grants go through the redeemer admin Safe")
		return
	}

	if err := protocolSvc.HandOverRedeemerAuthority(ctx, redeemer); err != nil {
		appLogger.Logf("protocol accounts: redeemer authority not handed over, env key still granting: %s", err)
		return
	}

	// Only now is the Safe demonstrably able to do the job, so only now are
	// grants routed through it.
	redeemer.SetRoleGranter(protocolSvc.RedeemerRoleGranter())
	appLogger.Logf("protocol accounts: REDEEMER_ROLE grants now go through the redeemer admin Safe")

	if protocolSvc.SweepEnabled() {
		if err := protocolSvc.RevokeLegacyRedeemerAuthority(ctx, redeemer, false); err != nil {
			appLogger.Logf("protocol accounts: legacy redeemer authority not revoked: %s", err)
		}
	}
}

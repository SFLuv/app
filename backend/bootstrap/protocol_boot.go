package bootstrap

import (
	"context"
	"math/big"

	"github.com/SFLuv/app/backend/bot"
	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/structs"
	"github.com/ethereum/go-ethereum/common"
)

// StartProtocolAccounts brings the backend's own Safes up at boot.
//
// Runs under the jobs leader, so one process does this however many are
// serving. Three stages, and only the third can be destructive:
//
//  1. Register the accounts, mint a signer for any that lacks one, derive the
//     Safe addresses, and record the addresses that held each role before.
//     All idempotent; nothing on chain changes.
//  2. Deploy any Safe that is not on chain yet, as a sponsored operation, and
//     promote its signer once the Safe confirms it is an owner.
//  3. Sweep the old EOA balances onto the Safes — only when
//     PROTOCOL_SWEEP_ENABLED says so, because it moves real money with keys
//     that are presumed exposed, and it must not fire because a staging image
//     booted or a crashed process came round again.
//
// Failure is logged and the server carries on. A faucet whose Safe is not up
// yet keeps paying from its EOA, which is exactly what it did yesterday — so a
// bad deploy of this code degrades to the previous behaviour rather than
// stopping payouts.
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

	protocolSvc.LogConfig(ctx, appLogger)
	if err := protocolSvc.Available(); err != nil {
		return
	}

	if err := protocolSvc.EnsureAccounts(ctx); err != nil {
		appLogger.Logf("protocol accounts: could not be prepared: %s", err)
		return
	}
	if err := protocolSvc.DeployAccounts(ctx); err != nil {
		appLogger.Logf("protocol accounts: deployment pass failed: %s", err)
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
	attachFaucet(ctx, protocolSvc, botClient, appLogger)
	attachRedeemer(ctx, protocolSvc, redeemer, appLogger)
}

// attachRedeemer moves the granting role onto the redeemer admin's Safe and
// routes grants through it.
//
// The grant is attempted on every boot and is idempotent — it returns early
// once the token reports the Safe holds the role. The revoke is gated with the
// sweep, because it is the irreversible half: after it, only the Safe can grant
// REDEEMER_ROLE, and only the database holds the key that signs for the Safe.
func attachRedeemer(
	ctx context.Context,
	protocolSvc *handlers.ProtocolService,
	redeemer *handlers.RedeemerService,
	appLogger *logger.LogCloser,
) {
	if redeemer == nil || !redeemer.IsEnabled() {
		return
	}
	if _, ok := redeemer.GrantAuthorityRole(); !ok {
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

func attachFaucet(
	ctx context.Context,
	protocolSvc *handlers.ProtocolService,
	botClient *bot.Bot,
	appLogger *logger.LogCloser,
) {
	if botClient == nil {
		return
	}
	accounts, err := protocolSvc.Accounts(ctx)
	if err != nil {
		appLogger.Logf("protocol accounts: could not be read to attach the faucet: %s", err)
		return
	}
	for _, account := range accounts {
		if account.Key != structs.ProtocolAccountFaucet {
			continue
		}
		if account.DeployedAt == nil || account.Signer == nil || !common.IsHexAddress(account.SafeAddress) {
			appLogger.Logf("protocol accounts: faucet still paying from its EOA — Safe not ready")
			return
		}
		balance, err := protocolSvc.FaucetBalance(ctx)
		if err != nil {
			appLogger.Logf("protocol accounts: faucet Safe balance unreadable, leaving the EOA in charge: %s", err)
			return
		}
		if balance == nil || balance.Sign() == 0 {
			appLogger.Logf(
				"protocol accounts: faucet Safe %s is deployed but empty — the EOA keeps paying until it is funded",
				account.SafeAddress,
			)
			return
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
		return
	}
}

package handlers

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/SFLuv/app/backend/protocol"
	"github.com/SFLuv/app/backend/structs"
	"github.com/ethereum/go-ethereum/common"
)

// Moving the redeemer admin's authority onto its Safe.
//
// Sweeping the balance is only half of what the redeemer admin is. Its real
// power is a role on the token — the one that may grant REDEEMER_ROLE — and a
// role is granted to an address, so moving the money without moving the role
// would leave a funded Safe that can do nothing and an env key that still runs
// merchant onboarding.
//
// Strictly ordered, because the wrong order is unrecoverable: grant to the
// Safe, confirm from the token that it holds it, and only then take it off the
// old address. Revoking first — or revoking on the assumption that the grant
// landed — would leave nobody able to grant REDEEMER_ROLE, and no way to fix it
// from this codebase.

// HandOverRedeemerAuthority gives the redeemer admin's Safe the role it needs.
func (p *ProtocolService) HandOverRedeemerAuthority(ctx context.Context, redeemer *RedeemerService) error {
	if err := p.Available(); err != nil {
		return err
	}
	role, ok := redeemer.GrantAuthorityRole()
	if !ok {
		return fmt.Errorf("the redeemer service is disabled, so it has no authority to hand over")
	}
	safe, err := p.SafeFor(ctx, structs.ProtocolAccountRedeemerAdmin)
	if err != nil {
		return err
	}

	// Already done, by a previous boot or by hand.
	held, err := redeemer.HoldsGrantAuthority(ctx, safe)
	if err != nil {
		return err
	}
	if held {
		return nil
	}

	step := structs.ProtocolAccountRedeemerAdmin + ":role:grant"
	claimed, err := p.db.ClaimProtocolStep(ctx, step)
	if err != nil || !claimed {
		return err
	}

	hash, err := redeemer.LegacyGrantRole(ctx, role, safe)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), hash, nil)
		return err
	}

	// Confirmed from the token, not from the receipt. A successful transaction
	// that granted the wrong role would look identical here, and the next step
	// revokes on the strength of this answer.
	held, err = redeemer.HoldsGrantAuthority(ctx, safe)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), hash, nil)
		return err
	}
	if !held {
		detail := fmt.Sprintf("grant transaction %s succeeded but the token does not report %s as holding the role", hash, safe.Hex())
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, detail, hash, nil)
		return fmt.Errorf("%s", detail)
	}

	p.logf("protocol account %s: Safe %s now holds the authority to grant REDEEMER_ROLE (%s)",
		structs.ProtocolAccountRedeemerAdmin, safe.Hex(), hash)
	return p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepDone, "Safe holds the granting role", hash, nil)
}

// RevokeLegacyRedeemerAuthority strips the old address of the granting role.
//
// Gated with the sweep, and refuses outright unless the Safe is confirmed to
// hold the role first. This is the step that makes the exposed key worthless,
// and also the one that would break merchant onboarding permanently if it ran
// against an unconfirmed grant.
func (p *ProtocolService) RevokeLegacyRedeemerAuthority(ctx context.Context, redeemer *RedeemerService, force bool) error {
	if err := p.Available(); err != nil {
		return err
	}
	if !force && !p.SweepEnabled() {
		return fmt.Errorf("revoking the old redeemer authority is gated off: set %s=true", EnvSweepEnabled)
	}
	role, ok := redeemer.GrantAuthorityRole()
	if !ok {
		return fmt.Errorf("the redeemer service is disabled")
	}
	legacy, ok := redeemer.LegacyAdminAddress()
	if !ok {
		return fmt.Errorf("the redeemer service has no legacy address")
	}
	safe, err := p.SafeFor(ctx, structs.ProtocolAccountRedeemerAdmin)
	if err != nil {
		return err
	}

	held, err := redeemer.HoldsGrantAuthority(ctx, safe)
	if err != nil {
		return err
	}
	if !held {
		return fmt.Errorf(
			"refusing to revoke: the Safe %s does not hold the granting role yet, and revoking would leave nobody who does",
			safe.Hex(),
		)
	}

	step := structs.ProtocolAccountRedeemerAdmin + ":role:revoke"
	claimed, err := p.db.ClaimProtocolStep(ctx, step)
	if err != nil || !claimed {
		return err
	}

	hash, err := redeemer.LegacyRevokeRole(ctx, role, legacy)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), hash, nil)
		return err
	}

	p.logf("protocol account %s: %s no longer holds the authority to grant REDEEMER_ROLE (%s)",
		structs.ProtocolAccountRedeemerAdmin, legacy.Hex(), hash)
	return p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepDone, "legacy authority revoked", hash, nil)
}

// RedeemerRoleGranter issues a role through the redeemer admin's Safe.
func (p *ProtocolService) RedeemerRoleGranter() func(context.Context, [32]byte, common.Address) (string, error) {
	return func(ctx context.Context, role [32]byte, account common.Address) (string, error) {
		data, err := protocol.GrantRoleCallData(role, account)
		if err != nil {
			return "", err
		}
		return p.Execute(ctx, structs.ProtocolAccountRedeemerAdmin, p.token, nil, data)
	}
}

// RedeemerSafeHoldsAuthority reports whether the redeemer admin's Safe already
// holds the granting role, without trying to give it one.
//
// Asked on a boot where on-chain writes are switched off: the role may have
// been handed over by an earlier run, and if it has, grants should go through
// the Safe rather than the env key.
func (p *ProtocolService) RedeemerSafeHoldsAuthority(ctx context.Context, redeemer *RedeemerService) (bool, error) {
	if err := p.Available(); err != nil {
		return false, err
	}
	if _, ok := redeemer.GrantAuthorityRole(); !ok {
		return false, fmt.Errorf("the redeemer service is disabled")
	}
	safe, err := p.SafeFor(ctx, structs.ProtocolAccountRedeemerAdmin)
	if err != nil {
		return false, err
	}
	return redeemer.HoldsGrantAuthority(ctx, safe)
}

// ── Recovery ─────────────────────────────────────────────────────────────────

// EnvRecoveryOwner is a second owner added to every protocol Safe.
//
// The answer to the single worst failure this design has: each Safe has exactly
// one owner, and that key exists only encrypted in Postgres under a master key
// held in the environment. Lose or mistype the master key, or restore the
// database from a backup taken before the last rotation, and nobody can ever
// sign for the faucet again — the money is not stolen, it is simply beyond
// reach, permanently.
//
// A second owner removes that. It should be a key that never touches this
// system: a hardware wallet, or one kept only in a password manager. The
// threshold stays 1, so either owner can act alone — this adds a way in, not a
// second signature requirement.
//
// It also makes the other rotation possible. If the database key is ever the
// thing that leaks, the recovery owner can swapOwner away from the compromised
// signer; without it, the only key that can rotate the Safe is the one that
// needs rotating.
const EnvRecoveryOwner = "PROTOCOL_RECOVERY_OWNER"

// RecoveryOwner is the configured offline owner, if there is one.
func RecoveryOwner() (common.Address, bool) {
	raw := envValue(EnvRecoveryOwner)
	if !common.IsHexAddress(raw) {
		return common.Address{}, false
	}
	return common.HexToAddress(raw), true
}

// EnsureRecoveryOwner adds the offline owner to an account's Safe.
//
// Idempotent, and ordered so it cannot lock anybody out: the Safe is read
// first, the owner is only added if absent, and the threshold stays at 1
// throughout. Adding an owner needs the Safe to call itself, so it goes out as
// a sponsored operation signed by the key we hold — which means it has to
// happen while that key still works, not after it is lost.
func (p *ProtocolService) EnsureRecoveryOwner(ctx context.Context, accountKey string) error {
	recovery, ok := RecoveryOwner()
	if !ok {
		return nil
	}
	if err := p.Available(); err != nil {
		return err
	}

	safe, err := p.SafeFor(ctx, accountKey)
	if err != nil {
		return err
	}
	owners, err := p.client.Owners(ctx, safe)
	if err != nil {
		return err
	}
	for _, owner := range owners {
		if owner == recovery {
			return nil
		}
	}

	step := accountKey + ":recovery-owner"
	claimed, err := p.db.ClaimProtocolStep(ctx, step)
	if err != nil || !claimed {
		return err
	}

	data, err := protocol.AddOwnerCallData(recovery, big.NewInt(1))
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}
	opHash, err := p.Execute(ctx, accountKey, safe, big.NewInt(0), data)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}

	// Confirmed from the Safe rather than assumed from a submitted operation.
	// A recovery owner that was never actually added is worse than none, since
	// it would be believed in.
	deadline := time.Now().Add(90 * time.Second)
	for {
		owners, err := p.client.Owners(ctx, safe)
		if err == nil {
			for _, owner := range owners {
				if owner == recovery {
					p.logf("protocol account %s: recovery owner %s added to Safe %s",
						accountKey, recovery.Hex(), safe.Hex())
					return p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepDone,
						"recovery owner added, threshold unchanged", opHash, nil)
				}
			}
		}
		if !time.Now().Before(deadline) {
			detail := fmt.Sprintf("operation %s was submitted but %s is not an owner of %s",
				opHash, recovery.Hex(), safe.Hex())
			_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, detail, opHash, nil)
			return fmt.Errorf("%s", detail)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

package handlers

import (
	"context"
	"fmt"

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

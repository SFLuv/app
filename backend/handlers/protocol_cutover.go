package handlers

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/abi"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/protocol"
	"github.com/SFLuv/app/backend/structs"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// The cutover: from EOAs whose keys sat in the environment to Safes whose
// signers sit encrypted in the database.
//
// Three properties shape all of it:
//
//   - Every step claims itself in the ledger first, so a restart, a second
//     instance or a crash loop cannot repeat one. The steps that move money
//     need this absolutely; the ones that only derive an address are harmless
//     either way and claim anyway, so there is one pattern rather than two.
//   - Deriving and deploying are automatic. Moving funds is not: it waits for
//     PROTOCOL_SWEEP_ENABLED or a button, because a staging image booting must
//     not drain a live faucet.
//   - The old keys are presumed exposed. They are used exactly once more —
//     to move the money off themselves — and then nothing reads them again.

// legacyAccount is the environment-held identity a protocol account replaces.
type legacyAccount struct {
	accountKey string
	role       string
	keyEnv     string
	addressEnv string
}

func legacyAccounts() []legacyAccount {
	return []legacyAccount{
		{
			accountKey: structs.ProtocolAccountFaucet,
			role:       structs.ProtocolRoleFaucet,
			keyEnv:     "BOT_KEY",
			addressEnv: "BOT_ADDRESS",
		},
		{
			accountKey: structs.ProtocolAccountRedeemerAdmin,
			role:       structs.ProtocolRoleRedeemerAdmin,
			keyEnv:     "REDEEMER_ADMIN_KEY",
			addressEnv: "REDEEMER_ADMIN_ADDRESS",
		},
	}
}

// classificationAddresses are the env addresses that are only ever compared
// against, never signed with: the admin wallets payouts are attributed to, the
// faucet alias, the zapper. They get history rows so a report over last March
// still classifies March's transfers, but no Safe and no key.
func classificationAddresses() map[string][]string {
	return map[string][]string{
		structs.ProtocolRoleAdmin: append(
			splitAddressList(envValue("PAID_ADMIN_ADDRESSES")),
			envValue("ADMIN_ADDRESS"),
		),
		structs.ProtocolRoleFaucet: {
			envValue("FAUCET_ADDRESS"),
			envValue("NEXT_PUBLIC_FAUCET_ADDRESS"),
		},
		structs.ProtocolRoleZapper: {
			envValue("ZAPPER_ADDRESS"),
			envValue("NEXT_PUBLIC_ZAPPER_ADDRESS"),
		},
	}
}

func splitAddressList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// EnsureAccounts registers every protocol account, mints a signer for any that
// lacks one, derives its Safe address and records the addresses that held its
// role before it.
//
// Safe to run on every boot. Nothing here moves money or changes anything on
// chain: the Safe address is CREATE2 arithmetic, answered by a view call.
func (p *ProtocolService) EnsureAccounts(ctx context.Context) error {
	if err := p.Available(); err != nil {
		return err
	}

	if err := p.recordLegacyAddresses(ctx); err != nil {
		return err
	}

	for _, account := range protocolAccountCatalog(p.chainID) {
		if err := p.db.EnsureProtocolAccount(ctx, account); err != nil {
			return err
		}
		if err := p.ensureSigner(ctx, account); err != nil {
			return fmt.Errorf("error preparing a signer for %s: %w", account.Key, err)
		}
	}
	return nil
}

// recordLegacyAddresses gives every address we have ever used a history row
// that starts at the epoch.
//
// The epoch, not today. These addresses really did hold their roles for every
// item that already exists, and dating the window to the migration would put
// every past payout before the start of its own sender's turn — which is the
// failure this table exists to prevent, arriving by a different route.
func (p *ProtocolService) recordLegacyAddresses(ctx context.Context) error {
	epoch := time.Unix(0, 0).UTC()

	for _, legacy := range legacyAccounts() {
		address := envValue(legacy.addressEnv)
		if !common.IsHexAddress(address) {
			continue
		}
		if err := p.db.OpenProtocolAddressWindow(
			ctx, address, legacy.role, p.chainID, "", epoch,
			structs.ProtocolSourceEnv,
			fmt.Sprintf("held in %s before the Safe cutover", legacy.addressEnv),
		); err != nil {
			return err
		}
	}

	for role, addresses := range classificationAddresses() {
		for _, address := range addresses {
			if !common.IsHexAddress(address) {
				continue
			}
			if err := p.db.OpenProtocolAddressWindow(
				ctx, address, role, p.chainID, "", epoch,
				structs.ProtocolSourceEnv, "classification address from the environment",
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureSigner mints the key an account will be owned by, if it has none.
//
// Minted before the Safe exists, because the Safe's address is derived from it:
// the owner is an input to CREATE2, so the key has to come first and the
// address follows from it. The signer stays unactivated until the chain has
// confirmed it owns something.
func (p *ProtocolService) ensureSigner(ctx context.Context, account structs.ProtocolAccount) error {
	existing, err := p.db.GetActiveProtocolSigner(ctx, account.Key)
	if err == nil && existing != nil {
		// Already has a live signer. Make sure the derived address is recorded,
		// which it will not be if a previous boot died between the two writes.
		return p.recordSafeAddress(ctx, account.Key, common.HexToAddress(existing.EOAAddress), account.OwnerIndex)
	}
	// Having no signer yet is the expected state on a first boot; anything else
	// is a real read failure and must not be mistaken for it.
	if err != nil && !errors.Is(err, db.ErrNoProtocolSigner) {
		return err
	}

	// A pending signer from an interrupted first boot is reused rather than
	// replaced: minting a second key would derive a second Safe and abandon
	// the first, which may already have been funded.
	pending, err := p.pendingSigner(ctx, account.Key)
	if err != nil {
		return err
	}
	if pending != nil {
		return p.recordSafeAddress(ctx, account.Key, common.HexToAddress(pending.EOAAddress), account.OwnerIndex)
	}

	key, address, err := protocol.GenerateSigner()
	if err != nil {
		return err
	}
	sealed, err := protocol.SealSigner(p.master, account.Key, key)
	if err != nil {
		return err
	}
	if _, err := p.db.InsertProtocolSigner(ctx, account.Key, strings.ToLower(address.Hex()), sealed, ""); err != nil {
		return err
	}
	p.logf("protocol account %s: minted signer %s", account.Key, address.Hex())

	return p.recordSafeAddress(ctx, account.Key, address, account.OwnerIndex)
}

// pendingSigner is the newest minted-but-not-activated key for an account.
func (p *ProtocolService) pendingSigner(ctx context.Context, accountKey string) (*structs.ProtocolSigner, error) {
	signers, err := p.db.ListProtocolSigners(ctx, accountKey)
	if err != nil {
		return nil, err
	}
	for i := range signers {
		if signers[i].ActivatedAt == nil && signers[i].RetiredAt == nil {
			return &signers[i], nil
		}
	}
	return nil, nil
}

func (p *ProtocolService) recordSafeAddress(ctx context.Context, accountKey string, owner common.Address, index int) error {
	safe, err := p.client.DeriveAddress(ctx, owner, big.NewInt(int64(index)))
	if err != nil {
		return err
	}
	return p.db.SetProtocolAccountSafe(ctx, accountKey, strings.ToLower(safe.Hex()))
}

// ── Deployment ───────────────────────────────────────────────────────────────

// DeployAccounts deploys any registered Safe that is not on chain yet.
//
// force is the Protocol panel asking directly. Without it this needs
// PROTOCOL_DEPLOY_ENABLED, because a deploy of main would otherwise put
// contracts on chain as a side effect of a merge.
func (p *ProtocolService) DeployAccounts(ctx context.Context, force bool) error {
	if err := p.Available(); err != nil {
		return err
	}
	if !force && !p.DeployEnabled() {
		return fmt.Errorf("deploying protocol Safes is gated off: set %s=true or deploy from the Protocol panel", EnvDeployEnabled)
	}
	accounts, err := p.Accounts(ctx)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if err := p.deployAccount(ctx, account); err != nil {
			p.logf("protocol account %s: deployment failed: %s", account.Key, err)
		}
	}
	return nil
}

// deployAccount puts one Safe on chain and promotes its signer.
//
// The deploying operation carries initCode and a call the Safe makes to itself
// with no data. The call is deliberately inert: the point of the operation is
// the initCode, and anything with an effect would be an effect that only
// happens on the one boot that deployed the account.
func (p *ProtocolService) deployAccount(ctx context.Context, account structs.ProtocolAccount) error {
	if !common.IsHexAddress(account.SafeAddress) {
		return fmt.Errorf("no Safe address derived")
	}
	safe := common.HexToAddress(account.SafeAddress)

	// Already deployed and signed for: nothing to do, and this is the common
	// case on every boot after the first.
	if account.DeployedAt != nil && account.Signer != nil {
		return nil
	}

	step := account.Key + ":deploy"
	claimed, err := p.db.ClaimProtocolStep(ctx, step)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	finish := func(status, detail, hash string) error {
		return p.db.FinishProtocolStep(ctx, step, status, detail, hash, nil)
	}

	pending, err := p.pendingSigner(ctx, account.Key)
	if err != nil {
		_ = finish(structs.ProtocolStepFailed, err.Error(), "")
		return err
	}

	exists, err := p.client.AccountExists(ctx, safe)
	if err != nil {
		_ = finish(structs.ProtocolStepFailed, err.Error(), "")
		return err
	}

	opHash := ""
	if !exists {
		if pending == nil && account.Signer == nil {
			err := fmt.Errorf("no signer to deploy with")
			_ = finish(structs.ProtocolStepFailed, err.Error(), "")
			return err
		}
		signerKey := account.Key
		key, _, err := p.openSignerRow(ctx, signerKey, pending)
		if err != nil {
			_ = finish(structs.ProtocolStepFailed, err.Error(), "")
			return err
		}
		// A call to itself with no data: valid, cheap, and with nothing to undo
		// if the operation is retried.
		data, err := protocol.SafeModuleCallData(safe, big.NewInt(0), nil)
		if err != nil {
			_ = finish(structs.ProtocolStepFailed, err.Error(), "")
			return err
		}
		op := protocol.NewUserOperation(safe)
		op.CallData = data
		if op.InitCode, err = p.client.InitCode(crypto.PubkeyToAddress(key.PublicKey), big.NewInt(int64(account.OwnerIndex))); err != nil {
			_ = finish(structs.ProtocolStepFailed, err.Error(), "")
			return err
		}
		sponsored, err := p.client.Sponsor(ctx, op)
		if err != nil {
			_ = finish(structs.ProtocolStepFailed, err.Error(), "")
			return err
		}
		hash, err := p.client.Hash(ctx, sponsored)
		if err != nil {
			_ = finish(structs.ProtocolStepFailed, err.Error(), "")
			return err
		}
		if err := protocol.Sign(sponsored, hash, key); err != nil {
			_ = finish(structs.ProtocolStepFailed, err.Error(), "")
			return err
		}
		if opHash, err = p.client.Send(ctx, sponsored, nil); err != nil {
			_ = finish(structs.ProtocolStepFailed, err.Error(), "")
			return err
		}
		p.logf("protocol account %s: deployment operation %s submitted for Safe %s", account.Key, opHash, safe.Hex())

		if err := p.awaitDeployment(ctx, safe); err != nil {
			_ = finish(structs.ProtocolStepFailed, err.Error(), opHash)
			return err
		}
	}

	// Confirmed from the chain rather than assumed from a successful submission:
	// the signer is only promoted once the Safe itself says it is an owner.
	if err := p.confirmOwnership(ctx, account, pending, opHash); err != nil {
		_ = finish(structs.ProtocolStepFailed, err.Error(), opHash)
		return err
	}

	if err := p.db.MarkProtocolAccountDeployed(ctx, account.Key, opHash); err != nil {
		_ = finish(structs.ProtocolStepFailed, err.Error(), opHash)
		return err
	}

	// The Safe can hold and move money from now on, so its window opens now.
	// The old address's window stays open until the sweep actually moves the
	// balance: between these two moments either address could legitimately have
	// sent a payout, and the verifier accepting both is the accurate answer.
	if err := p.db.OpenProtocolAddressWindow(
		ctx, strings.ToLower(safe.Hex()), account.Role, p.chainID, account.Key,
		time.Now().UTC(), structs.ProtocolSourceCutover, "Safe deployed",
	); err != nil {
		_ = finish(structs.ProtocolStepFailed, err.Error(), opHash)
		return err
	}

	// The offline owner goes on as soon as the Safe exists, while the key we
	// hold still works. Added after the step above is recorded, so a failure
	// here leaves a deployed Safe rather than an un-deployed one — and it is
	// its own step, retried on the next boot.
	if err := p.EnsureRecoveryOwner(ctx, account.Key); err != nil {
		p.logf("protocol account %s: recovery owner not added: %s", account.Key, err)
	}

	p.logf("protocol account %s: Safe %s is deployed and signed for", account.Key, safe.Hex())
	return finish(structs.ProtocolStepDone, "Safe deployed and signer confirmed", opHash)
}

// awaitDeployment waits for the engine to agree the account exists.
func (p *ProtocolService) awaitDeployment(ctx context.Context, safe common.Address) error {
	deadline := time.Now().Add(90 * time.Second)
	for {
		exists, err := p.client.AccountExists(ctx, safe)
		if err == nil && exists {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("Safe %s had not appeared on chain 90s after its deployment operation", safe.Hex())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// confirmOwnership reads the Safe's owners and promotes the matching signer.
func (p *ProtocolService) confirmOwnership(
	ctx context.Context,
	account structs.ProtocolAccount,
	pending *structs.ProtocolSigner,
	opHash string,
) error {
	owners, err := p.client.Owners(ctx, common.HexToAddress(account.SafeAddress))
	if err != nil {
		return err
	}
	ownerSet := map[common.Address]bool{}
	for _, owner := range owners {
		ownerSet[owner] = true
	}

	if account.Signer != nil && ownerSet[common.HexToAddress(account.Signer.EOAAddress)] {
		return nil
	}
	if pending == nil {
		return fmt.Errorf("Safe %s is owned by none of our recorded signers", account.SafeAddress)
	}
	if !ownerSet[common.HexToAddress(pending.EOAAddress)] {
		return fmt.Errorf(
			"Safe %s does not list %s as an owner, so the signer was not promoted",
			account.SafeAddress, pending.EOAAddress,
		)
	}
	return p.db.PromoteProtocolSigner(ctx, account.Key, pending.ID, opHash, "replaced at deployment")
}

// openSignerRow decrypts either the account's active signer or a specific
// pending one.
//
// Deployment needs the pending case: the key that will own the Safe is not its
// active signer yet, because nothing is on chain to be active for.
func (p *ProtocolService) openSignerRow(
	ctx context.Context,
	accountKey string,
	pending *structs.ProtocolSigner,
) (*ecdsa.PrivateKey, common.Address, error) {
	if pending == nil {
		return p.signerFor(ctx, accountKey)
	}
	sealed, err := p.db.GetSealedProtocolSigner(ctx, pending.ID)
	if err != nil {
		return nil, common.Address{}, err
	}
	key, err := protocol.OpenSigner(p.master, accountKey, sealed.SealedKey)
	if err != nil {
		return nil, common.Address{}, err
	}
	return key, common.HexToAddress(sealed.EOAAddress), nil
}

// ── The sweep ────────────────────────────────────────────────────────────────

// SweepLegacyFunds moves what the old EOAs hold onto the new Safes.
//
// Gated, irreversible, and signed with keys that are presumed to be in someone
// else's hands — which is the reason it is worth doing at all, and the reason
// it is done once and recorded. Order matters: the token goes first because the
// EOA needs its remaining CELO to pay for that transfer, and the CELO sweep
// then leaves itself enough to pay for its own.
func (p *ProtocolService) SweepLegacyFunds(ctx context.Context, force bool) error {
	if err := p.Available(); err != nil {
		return err
	}
	if !force && !p.SweepEnabled() {
		return fmt.Errorf("the fund sweep is gated off: set %s=true or run it from the Protocol panel", EnvSweepEnabled)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, legacy := range legacyAccounts() {
		if err := p.sweepOne(ctx, legacy); err != nil {
			p.logf("protocol sweep %s: %s", legacy.accountKey, err)
		}
	}
	return nil
}

func (p *ProtocolService) sweepOne(ctx context.Context, legacy legacyAccount) error {
	rawKey := envValue(legacy.keyEnv)
	if rawKey == "" || rawKey == "x" {
		return fmt.Errorf("no %s configured, so there is nothing to sweep from", legacy.keyEnv)
	}
	oldKey, err := crypto.HexToECDSA(strings.TrimPrefix(rawKey, "0x"))
	if err != nil {
		return fmt.Errorf("%s is not a usable key: %w", legacy.keyEnv, err)
	}
	oldAddress := crypto.PubkeyToAddress(oldKey.PublicKey)

	safe, err := p.SafeFor(ctx, legacy.accountKey)
	if err != nil {
		return err
	}
	account, err := p.db.GetProtocolAccount(ctx, legacy.accountKey)
	if err != nil {
		return err
	}
	if account == nil || account.DeployedAt == nil {
		return fmt.Errorf("Safe is not deployed yet, so sweeping into it would strand the funds")
	}

	if err := p.sweepToken(ctx, legacy, oldKey, oldAddress, safe); err != nil {
		return err
	}
	if err := p.sweepNative(ctx, legacy, oldKey, oldAddress, safe); err != nil {
		return err
	}

	// The old address stops being this role's holder at the moment its balance
	// left. Recorded with the real instant rather than NOW() at some later
	// bookkeeping pass, because the window boundary is what every historical
	// check reads.
	return p.db.CloseProtocolAddressWindow(ctx, strings.ToLower(oldAddress.Hex()), legacy.role, p.chainID, time.Now().UTC())
}

func (p *ProtocolService) sweepToken(
	ctx context.Context,
	legacy legacyAccount,
	oldKey *ecdsa.PrivateKey,
	oldAddress, safe common.Address,
) error {
	step := legacy.accountKey + ":sweep:token"
	claimed, err := p.db.ClaimProtocolStep(ctx, step)
	if err != nil || !claimed {
		return err
	}

	balance, err := p.client.TokenBalance(ctx, p.token, oldAddress)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}
	if balance.Sign() == 0 {
		return p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepSkipped, "no token balance to move", "", balance)
	}

	client, err := p.client.ReadClient(ctx)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}
	defer client.Close()

	chainID, err := client.ChainID(ctx)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}
	contract, err := abi.NewSFLUVv2(p.token, client)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}
	transactor, err := bind.NewKeyedTransactorWithChainID(oldKey, chainID)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}

	tx, err := contract.Transfer(&bind.TransactOpts{
		From:    oldAddress,
		Signer:  transactor.Signer,
		Context: ctx,
	}, safe, balance)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", balance)
		return fmt.Errorf("error sweeping the token balance of %s: %w", oldAddress.Hex(), err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	receipt, err := bind.WaitMined(waitCtx, client, tx)
	if err != nil || receipt == nil || receipt.Status != types.ReceiptStatusSuccessful {
		detail := fmt.Sprintf("token sweep %s did not confirm", tx.Hash().Hex())
		if err != nil {
			detail = fmt.Sprintf("%s: %s", detail, err)
		}
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, detail, tx.Hash().Hex(), balance)
		return fmt.Errorf("%s", detail)
	}

	// Recorded in the transaction map like any other operation of ours, so a
	// check on this transfer later is answered by lookup rather than by
	// deducing which address was the faucet at the time — which, for a transfer
	// that happens exactly at the cutover boundary, is the hardest case for the
	// window arithmetic to get right.
	if _, err := p.db.ClaimProtocolTransaction(ctx, db.ProtocolTransaction{
		TxHash:        tx.Hash().Hex(),
		ChainID:       p.chainID,
		AccountKey:    legacy.accountKey,
		SenderAddress: oldAddress.Hex(),
		Purpose:       db.ProtocolTxPurposeSweep,
		Recipient:     safe.Hex(),
		AmountBase:    balance,
		BlockNumber:   receipt.BlockNumber.Uint64(),
	}); err != nil {
		p.logf("protocol sweep %s: token transfer recorded on chain but not in the transaction map: %s", legacy.accountKey, err)
	}

	p.logf("protocol sweep %s: moved %s token base units from %s to %s (%s)",
		legacy.accountKey, balance.String(), oldAddress.Hex(), safe.Hex(), tx.Hash().Hex())
	return p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepDone, "token balance moved", tx.Hash().Hex(), balance)
}

// EnvCeloSweepTo is where the old accounts' leftover CELO goes.
//
// Not the Safes. They never need gas — every operation they make is sponsored —
// so CELO inside one is dead weight, and worse than dead if the paymaster turns
// out not to sponsor a value transfer: getting it back out would need the
// direct execTransaction path, which is not built. A wallet we already control
// is the destination, and leaving this unset skips the step rather than
// guessing at one.
const EnvCeloSweepTo = "PROTOCOL_CELO_SWEEP_TO"

// sweepNative moves the gas balance off the old account, leaving behind exactly
// enough to pay for doing so.
//
// A transfer of the whole balance cannot be signed: the transaction's own fee
// has to come out of it. So the fee is computed, subtracted, and the remainder
// sent — and if the remainder is not positive the step records that there was
// nothing worth moving rather than failing.
func (p *ProtocolService) sweepNative(
	ctx context.Context,
	legacy legacyAccount,
	oldKey *ecdsa.PrivateKey,
	oldAddress, safe common.Address,
) error {
	step := legacy.accountKey + ":sweep:native"

	destination := envValue(EnvCeloSweepTo)
	if !common.IsHexAddress(destination) {
		// Claimed first so the skip is recorded once rather than reconsidered
		// on every boot.
		claimed, err := p.db.ClaimProtocolStep(ctx, step)
		if err != nil || !claimed {
			return err
		}
		return p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepSkipped,
			fmt.Sprintf("no %s configured, so the gas balance was left where it is", EnvCeloSweepTo), "", nil)
	}
	to := common.HexToAddress(destination)

	claimed, err := p.db.ClaimProtocolStep(ctx, step)
	if err != nil || !claimed {
		return err
	}

	client, err := p.client.ReadClient(ctx)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}
	defer client.Close()

	balance, err := client.BalanceAt(ctx, oldAddress, nil)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}
	chainID, err := client.ChainID(ctx)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}
	tipCap, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		tipCap = big.NewInt(0)
	}
	head, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}
	baseFee := big.NewInt(0)
	if head.BaseFee != nil {
		baseFee = head.BaseFee
	}
	// Twice the base fee plus the tip is the usual headroom for a base fee that
	// rises between signing and inclusion.
	feeCap := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), tipCap)
	if feeCap.Sign() == 0 {
		suggested, err := client.SuggestGasPrice(ctx)
		if err != nil {
			_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
			return err
		}
		feeCap = suggested
	}

	const transferGas = uint64(21_000)
	reserve := new(big.Int).Mul(feeCap, new(big.Int).SetUint64(transferGas))
	value := new(big.Int).Sub(balance, reserve)
	if value.Sign() <= 0 {
		detail := fmt.Sprintf("balance %s does not cover its own transfer fee of %s", balance.String(), reserve.String())
		return p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepSkipped, detail, "", balance)
	}

	nonce, err := client.PendingNonceAt(ctx, oldAddress)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}

	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: tipCap,
		GasFeeCap: feeCap,
		Gas:       transferGas,
		To:        &to,
		Value:     value,
	})
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), oldKey)
	if err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), "", nil)
		return err
	}
	if err := client.SendTransaction(ctx, signed); err != nil {
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, err.Error(), signed.Hash().Hex(), value)
		return fmt.Errorf("error sweeping the gas balance of %s: %w", oldAddress.Hex(), err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	receipt, err := bind.WaitMined(waitCtx, client, signed)
	if err != nil || receipt == nil || receipt.Status != types.ReceiptStatusSuccessful {
		detail := fmt.Sprintf("gas sweep %s did not confirm", signed.Hash().Hex())
		_ = p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepFailed, detail, signed.Hash().Hex(), value)
		return fmt.Errorf("%s", detail)
	}

	p.logf("protocol sweep %s: moved %s wei of gas from %s to %s (%s)",
		legacy.accountKey, value.String(), oldAddress.Hex(), to.Hex(), signed.Hash().Hex())
	return p.db.FinishProtocolStep(ctx, step, structs.ProtocolStepDone, "gas balance moved", signed.Hash().Hex(), value)
}

func (p *ProtocolService) logf(format string, args ...any) {
	if p == nil || p.logger == nil {
		return
	}
	p.logger.Logf(format, args...)
}

package handlers

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SFLuv/app/backend/clientconfig"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/protocol"
	"github.com/SFLuv/app/backend/structs"
	"github.com/ethereum/go-ethereum/common"
)

// ProtocolService owns the backend's own Safes: deriving them, deploying them,
// spending from them, and rotating the keys that sign for them.
//
// Every write it makes to the chain is a sponsored user operation, so none of
// these accounts ever needs a gas balance. The one exception is the cutover
// sweep, which spends from the old EOAs and therefore pays its own gas — see
// protocol_cutover.go.
type ProtocolService struct {
	db     *db.AppDB
	logger *logger.LogCloser
	client *protocol.Client
	config *clientconfig.Config

	chainID int
	token   common.Address

	// master is the key that decrypts signer keys. Held in memory for the
	// process lifetime because every payout needs it; never logged, never
	// returned, and absent rather than empty when unconfigured.
	master []byte

	// disabled records why the service cannot act, so every caller can say the
	// same thing instead of each inventing its own phrasing for "not wired".
	disabled error

	// faucet is the payout queue. One goroutine drains it, batching whatever
	// is waiting, so a burst of redemptions is a couple of operations rather
	// than one per person.
	faucet faucetDispatcher

	// afterPanelStep runs once a Protocol-panel deploy or sweep succeeds, so
	// the running process re-reads the cutover state and points the faucet at
	// its Safe. Set at wiring; nil means nothing to do.
	afterPanelStep func(step string)

	mu sync.Mutex
}

// Panel steps passed to the afterPanelStep hook.
const (
	PanelStepDeploy = "deploy"
	PanelStepSweep  = "sweep"
)

// SetAfterPanelStep installs what to do after a panel deploy or sweep. The boot
// sequence attaches the faucet; without this, a sweep run from the panel moved
// the money and left this process paying from the emptied EOA until a restart.
func (p *ProtocolService) SetAfterPanelStep(hook func(step string)) {
	if p == nil {
		return
	}
	p.afterPanelStep = hook
}

func (p *ProtocolService) panelStepDone(step string) {
	if p != nil && p.afterPanelStep != nil {
		p.afterPanelStep(step)
	}
}

// Nothing touches the chain unless it is switched on.
//
// Merging is deploying here: main auto-deploys, so code that acts on chain at
// boot acts the moment a pull request lands. That is the wrong default for
// deploying contracts, moving a live faucet's balance and handing a token role
// around, so both of those wait to be asked — by an environment variable or by
// a button in the Protocol panel.
//
// What stays automatic is everything that only writes to our own database:
// registering the accounts, minting a signer, deriving the Safe addresses
// (a view call) and recording which address held which role. Those are
// idempotent, reversible and needed for the panel to be able to say anything
// at all.
const (
	// EnvDeployEnabled gates putting Safes on chain and handing the redeemer
	// role to one.
	EnvDeployEnabled = "PROTOCOL_DEPLOY_ENABLED"
	// EnvSweepEnabled gates moving funds and revoking the old authority — the
	// irreversible half.
	EnvSweepEnabled = "PROTOCOL_SWEEP_ENABLED"
)

func NewProtocolService(
	appDb *db.AppDB,
	appLogger *logger.LogCloser,
	config *clientconfig.Config,
) *ProtocolService {
	service := &ProtocolService{db: appDb, logger: appLogger, config: config}

	if config == nil {
		service.disabled = fmt.Errorf("client config is not loaded")
		return service
	}
	service.chainID = config.ActiveChainID()

	token, err := config.PrimaryToken()
	if err != nil {
		service.disabled = fmt.Errorf("primary token is unreadable: %w", err)
		return service
	}
	if !common.IsHexAddress(strings.TrimSpace(token.Address)) {
		service.disabled = fmt.Errorf("primary token address %q is not an address", token.Address)
		return service
	}
	service.token = common.HexToAddress(token.Address)

	account, err := config.PrimaryAccount()
	if err != nil {
		service.disabled = fmt.Errorf("primary account config is unreadable: %w", err)
		return service
	}

	client, err := protocol.NewClient(protocol.Config{
		EngineBaseURL: config.PrimaryRPCURL(),
		ReadRPCURL:    config.ReadRPCURL(),
		EntryPoint:    account.EntryPointAddress,
		Factory:       account.AccountFactoryAddress,
		Paymaster:     account.PaymasterAddress,
		PaymasterType: account.PaymasterType,
	})
	if err != nil {
		service.disabled = err
		return service
	}
	service.client = client

	master, err := protocol.MasterKey()
	if err != nil {
		// Not fatal to construction. The service still answers reads — the
		// panel needs to be able to say "no master key" rather than 500 — but
		// every signing path refuses.
		service.disabled = err
		return service
	}
	service.master = master
	return service
}

// Available reports whether the service can sign. The error says why not.
func (p *ProtocolService) Available() error {
	if p == nil {
		return fmt.Errorf("protocol service is not configured")
	}
	if p.disabled != nil {
		return p.disabled
	}
	if p.client == nil {
		return fmt.Errorf("protocol service has no chain client")
	}
	if len(p.master) == 0 {
		return fmt.Errorf("%s is not set", protocol.EnvMasterKey)
	}
	return nil
}

func (p *ProtocolService) ChainID() int { return p.chainID }

// SweepEnabled reflects the env gate on moving funds.
func (p *ProtocolService) SweepEnabled() bool { return envBool(EnvSweepEnabled, false) }

// DeployEnabled reflects the env gate on putting anything on chain.
func (p *ProtocolService) DeployEnabled() bool { return envBool(EnvDeployEnabled, false) }

// CutoverComplete reports whether every protocol account already has a
// deployed Safe with a live signer.
//
// The guard on repeating the on-chain work. Each of these steps is individually
// idempotent and each claims itself in the ledger, but this is the cheaper and
// more obvious check: if the wallets already exist, the cutover happened on an
// earlier deploy and there is nothing on chain left to do. Every later release
// — including the branches queued behind this one, which will boot this same
// sequence again — takes that path and touches nothing.
//
// Read from the recorded addresses rather than inferred from the schema
// version. The version only says the tables exist; the addresses say the work
// is done.
func (p *ProtocolService) CutoverComplete(ctx context.Context) bool {
	if p == nil || p.db == nil {
		return false
	}
	accounts, err := p.db.ListProtocolAccounts(ctx, p.chainID)
	if err != nil || len(accounts) == 0 {
		return false
	}
	expected := len(protocolAccountCatalog(p.chainID))
	if len(accounts) < expected {
		return false
	}
	for _, account := range accounts {
		if !common.IsHexAddress(account.SafeAddress) || account.DeployedAt == nil || account.Signer == nil {
			return false
		}
	}

	// Deployed is not migrated. Between the deploy and the sweep the faucet's
	// money is still in the EOA, and the EOA must keep paying: treating that
	// window as complete made every restart refuse payouts from a funded EOA,
	// and made boot skip the very sweep that would have ended the window. The
	// cutover is complete once the faucet's token has moved (or there was none).
	steps, err := p.db.ListProtocolSteps(ctx)
	if err != nil {
		return false
	}
	faucetSweep := structs.ProtocolAccountFaucet + ":sweep:token"
	for _, step := range steps {
		if step.Step == faucetSweep {
			return step.Status == structs.ProtocolStepDone || step.Status == structs.ProtocolStepSkipped
		}
	}
	return false
}

// protocolAccountCatalog is every function the backend signs for.
//
// Data in code rather than in the database, like the capability catalog: adding
// an account is a code change because something has to use it, and a row that
// nothing signs for would be a Safe nobody can spend from.
func protocolAccountCatalog(chainID int) []structs.ProtocolAccount {
	return []structs.ProtocolAccount{
		{
			Key:   structs.ProtocolAccountFaucet,
			Label: "Faucet",
			Description: "Pays redemption codes and workflow bounties. Held the balance that " +
				"every payout is checked against.",
			Role:       structs.ProtocolRoleFaucet,
			ChainID:    chainID,
			OwnerIndex: 0,
		},
		{
			Key:   structs.ProtocolAccountRedeemerAdmin,
			Label: "Redeemer admin",
			Description: "Grants and revokes the token's REDEEMER_ROLE, which is what lets a " +
				"merchant's till redeem takings.",
			Role:       structs.ProtocolRoleRedeemerAdmin,
			ChainID:    chainID,
			OwnerIndex: 0,
		},
	}
}

// ── Reading the registry ─────────────────────────────────────────────────────

// Accounts is the registry with active signers attached.
func (p *ProtocolService) Accounts(ctx context.Context) ([]structs.ProtocolAccount, error) {
	if p == nil || p.db == nil {
		return nil, fmt.Errorf("protocol service is not configured")
	}
	return p.db.ListProtocolAccounts(ctx, p.chainID)
}

// AddressesForRoleAt is the historical lookup, and the whole point of part one:
// which addresses held a protocol role at a given moment.
func (p *ProtocolService) AddressesForRoleAt(
	ctx context.Context,
	role string,
	chainID int64,
	at time.Time,
) ([]common.Address, error) {
	if p == nil || p.db == nil {
		return nil, fmt.Errorf("protocol service is not configured")
	}
	raw, err := p.db.ProtocolAddressesAt(ctx, role, int(chainID), at)
	if err != nil {
		return nil, err
	}
	addresses := make([]common.Address, 0, len(raw))
	for _, value := range raw {
		if common.IsHexAddress(value) {
			addresses = append(addresses, common.HexToAddress(value))
		}
	}
	return addresses, nil
}

// FaucetSenderResolver is the hook the bot takes, so the payout verifier can
// ask who the faucet was when a transfer happened without this package's
// database reaching into that one.
func (p *ProtocolService) FaucetSenderResolver() func(context.Context, int64, time.Time) ([]common.Address, error) {
	return func(ctx context.Context, chainID int64, at time.Time) ([]common.Address, error) {
		return p.AddressesForRoleAt(ctx, structs.ProtocolRoleFaucet, chainID, at)
	}
}

// TxSenderResolver answers "which of our addresses sent this transaction".
//
// The hook the payout verifier prefers over the timestamp lookup: for anything
// we submitted since the registry existed, this is a recorded fact rather than
// a deduction from when the block landed.
func (p *ProtocolService) TxSenderResolver() func(context.Context, string) (common.Address, bool) {
	return func(ctx context.Context, txHash string) (common.Address, bool) {
		if p == nil || p.db == nil {
			return common.Address{}, false
		}
		sender, _, found, err := p.db.ProtocolTransactionSender(ctx, txHash)
		if err != nil || !found || !common.IsHexAddress(sender) {
			return common.Address{}, false
		}
		return common.HexToAddress(sender), true
	}
}

// CurrentAddress is where a role's money lives now. Empty when the role has no
// address yet, which is the state before the cutover has run.
func (p *ProtocolService) CurrentAddress(ctx context.Context, role string) (common.Address, bool) {
	if p == nil || p.db == nil {
		return common.Address{}, false
	}
	addresses, err := p.db.CurrentProtocolAddresses(ctx, role, p.chainID)
	if err != nil || len(addresses) == 0 {
		return common.Address{}, false
	}
	// The newest open window wins when a role somehow has several. Ordered by
	// started_at in the query, so the last is the most recent.
	last := addresses[len(addresses)-1]
	if !common.IsHexAddress(last) {
		return common.Address{}, false
	}
	return common.HexToAddress(last), true
}

// SafeFor returns a deployed account's Safe address.
func (p *ProtocolService) SafeFor(ctx context.Context, key string) (common.Address, error) {
	account, err := p.db.GetProtocolAccount(ctx, key)
	if err != nil {
		return common.Address{}, err
	}
	if account == nil {
		return common.Address{}, fmt.Errorf("protocol account %s is not registered", key)
	}
	if !common.IsHexAddress(account.SafeAddress) {
		return common.Address{}, fmt.Errorf("protocol account %s has no Safe address yet", key)
	}
	return common.HexToAddress(account.SafeAddress), nil
}

// ── Signing ──────────────────────────────────────────────────────────────────

// signerFor opens an account's active key.
//
// The decrypted key is returned to the caller and never stored on the service:
// it lives for the length of one operation. Keeping a decrypted key on a
// long-lived struct would mean a rotation leaves the old one signing until the
// process restarts.
func (p *ProtocolService) signerFor(ctx context.Context, accountKey string) (*ecdsa.PrivateKey, common.Address, error) {
	if err := p.Available(); err != nil {
		return nil, common.Address{}, err
	}
	sealed, err := p.db.GetActiveProtocolSigner(ctx, accountKey)
	if err != nil {
		return nil, common.Address{}, err
	}
	key, err := protocol.OpenSigner(p.master, accountKey, sealed.SealedKey)
	if err != nil {
		return nil, common.Address{}, err
	}
	return key, common.HexToAddress(sealed.EOAAddress), nil
}

// Execute runs one call as a protocol account, sponsored.
func (p *ProtocolService) Execute(
	ctx context.Context,
	accountKey string,
	to common.Address,
	value *big.Int,
	data []byte,
) (string, error) {
	if err := p.Available(); err != nil {
		return "", err
	}
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
	return p.client.Execute(
		ctx,
		common.HexToAddress(account.SafeAddress),
		signer,
		int64(account.OwnerIndex),
		to,
		value,
		data,
	)
}

// ── Spending from the faucet ─────────────────────────────────────────────────

// ErrFaucetNotMigrated means the faucet Safe is not ready, so the caller should
// keep using the EOA path.
var ErrFaucetNotMigrated = errors.New("faucet Safe is not deployed yet")

// FaucetBalance is the faucet's spendable SFLUV, read from wherever the faucet
// currently is.
func (p *ProtocolService) FaucetBalance(ctx context.Context) (*big.Int, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("protocol service is not configured")
	}
	address, ok := p.CurrentAddress(ctx, structs.ProtocolRoleFaucet)
	if !ok {
		return nil, fmt.Errorf("no current faucet address is recorded")
	}
	return p.client.TokenBalance(ctx, p.token, address)
}

// LogConfig says once, at boot, what state the protocol accounts are in.
//
// Loud about the two silent failures. Without a master key every payout from a
// migrated faucet fails; with an undeployed Safe the old EOA is still the one
// paying, which works but means the cutover has not happened yet. Both look
// like nothing from the outside.
func (p *ProtocolService) LogConfig(ctx context.Context, appLogger *logger.LogCloser) {
	if appLogger == nil {
		return
	}
	if p == nil {
		appLogger.Logf("protocol accounts: service not constructed")
		return
	}
	if err := p.Available(); err != nil {
		appLogger.Logf("protocol accounts DISABLED: %s", err)
		if !protocol.MasterKeyConfigured() {
			appLogger.Logf(
				"protocol accounts: set %s to a 32-byte hex value that has never been deployed before. "+
					"Until it is set, no protocol Safe can be deployed, rotated or spent from.",
				protocol.EnvMasterKey,
			)
		}
		return
	}

	accounts, err := p.Accounts(ctx)
	if err != nil {
		appLogger.Logf("protocol accounts: could not be listed: %s", err)
		return
	}
	for _, account := range accounts {
		switch {
		case !common.IsHexAddress(account.SafeAddress):
			appLogger.Logf("protocol account %s: no Safe derived yet", account.Key)
		case account.DeployedAt == nil:
			appLogger.Logf(
				"protocol account %s: Safe %s derived but NOT deployed — the old EOA is still in use",
				account.Key, account.SafeAddress,
			)
		case account.Signer == nil:
			appLogger.Logf(
				"protocol account %s: Safe %s is deployed but has NO active signer — it cannot be spent from",
				account.Key, account.SafeAddress,
			)
		default:
			appLogger.Logf(
				"protocol account %s: Safe %s, signer %s",
				account.Key, account.SafeAddress, account.Signer.EOAAddress,
			)
		}
	}
	if !p.SweepEnabled() {
		appLogger.Logf("protocol accounts: fund sweep is gated off (%s is not true)", EnvSweepEnabled)
	}
}

// envValue trims an environment variable, for the legacy addresses the cutover
// reads. Kept here rather than inlined so the cutover's reads of the old
// configuration are all visibly the same shape.
func envValue(name string) string { return strings.TrimSpace(os.Getenv(name)) }

// VerifyFaucetSigner checks that the key we hold is actually an owner of the
// faucet Safe, on chain.
//
// The guard against a restored backup. A database recovered from before the
// last rotation holds a signer row that no longer owns anything: the Safe moved
// on, and that key cannot sign for it. Everything else about such a row looks
// healthy — it is active, it decrypts, it has an address — so without this the
// faucet would attach and then fail every payout at validation, which reads as
// the engine being broken rather than as the database being behind.
//
// Also the cheapest check for the other direction: a master key that was
// changed or mistyped produces a signer that decrypts to the wrong key, and
// this catches that too, before anything is sent.
func (p *ProtocolService) VerifyFaucetSigner(ctx context.Context) error {
	if err := p.Available(); err != nil {
		return err
	}
	account, err := p.db.GetProtocolAccount(ctx, structs.ProtocolAccountFaucet)
	if err != nil {
		return err
	}
	if account == nil || account.Signer == nil || !common.IsHexAddress(account.SafeAddress) {
		return fmt.Errorf("the faucet has no deployed Safe with an active signer")
	}

	owners, err := p.client.Owners(ctx, common.HexToAddress(account.SafeAddress))
	if err != nil {
		return err
	}
	held := common.HexToAddress(account.Signer.EOAAddress)
	for _, owner := range owners {
		if owner == held {
			return nil
		}
	}
	return fmt.Errorf(
		"the signer we hold for the faucet (%s) is NOT an owner of Safe %s — the database may predate a rotation, "+
			"or %s may have changed. Refusing to send with it",
		held.Hex(), account.SafeAddress, protocol.EnvMasterKey,
	)
}

// FaucetAccount is the faucet's registry row, with its active signer attached.
func (p *ProtocolService) FaucetAccount(ctx context.Context) (*structs.ProtocolAccount, error) {
	if p == nil || p.db == nil {
		return nil, fmt.Errorf("protocol service is not configured")
	}
	return p.db.GetProtocolAccount(ctx, structs.ProtocolAccountFaucet)
}

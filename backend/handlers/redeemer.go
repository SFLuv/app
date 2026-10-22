package handlers

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/abi"
	"github.com/SFLuv/app/backend/clientconfig"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type RedeemerService struct {
	appDb *db.AppDB
	log   *logger.LogCloser

	enabled bool

	client       *ethclient.Client
	contract     *abi.SFLUVv2
	privateKey   *ecdsa.PrivateKey
	fromAddress  common.Address
	chainID      *big.Int
	redeemerRole [32]byte
	// adminRole is the role that may grant REDEEMER_ROLE. It is what has to
	// move to the Safe for the Safe to be the redeemer admin in any real sense.
	adminRole [32]byte

	// granter, when set, issues the role through the redeemer admin's Safe as a
	// sponsored operation instead of signing with the env key.
	//
	// Installed only once the Safe actually holds the role that lets it grant.
	// Until then the env key still holds it, and this stays nil so the old path
	// keeps working — a half-finished cutover must not stop merchants being
	// able to redeem.
	granter RoleGranter
}

// RoleGranter grants an AccessControl role on the token, by whatever means the
// caller has.
type RoleGranter func(ctx context.Context, role [32]byte, account common.Address) (string, error)

// SetRoleGranter routes grants through the protocol Safe.
func (r *RedeemerService) SetRoleGranter(granter RoleGranter) {
	if r == nil {
		return
	}
	r.granter = granter
}

func NewRedeemerService(appDb *db.AppDB, log *logger.LogCloser, config *clientconfig.Config) *RedeemerService {
	service := &RedeemerService{
		appDb: appDb,
		log:   log,
	}

	inProductionRaw := strings.TrimSpace(os.Getenv("IN_PRODUCTION"))
	if inProductionRaw == "" {
		service.logf("redeemer auto-grant disabled: IN_PRODUCTION is not set (defaults to false)")
		return service
	}
	inProduction, err := strconv.ParseBool(inProductionRaw)
	if err != nil {
		service.logf("redeemer auto-grant disabled: invalid IN_PRODUCTION value %q", inProductionRaw)
		return service
	}
	if !inProduction {
		service.logf("redeemer auto-grant disabled: IN_PRODUCTION is false")
		return service
	}

	if config == nil {
		service.logf("redeemer auto-grant disabled: client config is not loaded")
		return service
	}
	primaryToken, err := config.PrimaryToken()
	if err != nil {
		service.logf("redeemer auto-grant disabled: invalid client config: %s", err)
		return service
	}

	rpcURL := strings.TrimSpace(config.ReadRPCURL())
	tokenID := strings.TrimSpace(primaryToken.Address)
	redeemerAdminKey := strings.TrimPrefix(strings.TrimSpace(os.Getenv("REDEEMER_ADMIN_KEY")), "0x")
	redeemerAdminAddress := strings.TrimSpace(os.Getenv("REDEEMER_ADMIN_ADDRESS"))

	if rpcURL == "" || tokenID == "" || redeemerAdminKey == "" || redeemerAdminAddress == "" {
		service.logf("redeemer auto-grant disabled: missing primary RPC/token config or one of REDEEMER_ADMIN_KEY, REDEEMER_ADMIN_ADDRESS")
		return service
	}

	if !common.IsHexAddress(tokenID) {
		service.logf("redeemer auto-grant disabled: invalid configured token address %q", tokenID)
		return service
	}
	if !common.IsHexAddress(redeemerAdminAddress) {
		service.logf("redeemer auto-grant disabled: invalid REDEEMER_ADMIN_ADDRESS %q", redeemerAdminAddress)
		return service
	}

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		service.logf("redeemer auto-grant disabled: error connecting RPC: %s", err)
		return service
	}

	privateKey, err := crypto.HexToECDSA(redeemerAdminKey)
	if err != nil {
		service.logf("redeemer auto-grant disabled: invalid REDEEMER_ADMIN_KEY: %s", err)
		return service
	}

	fromAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	configuredFrom := common.HexToAddress(redeemerAdminAddress)
	if strings.ToLower(fromAddress.Hex()) != strings.ToLower(configuredFrom.Hex()) {
		service.logf(
			"redeemer auto-grant disabled: REDEEMER_ADMIN_KEY address %s does not match REDEEMER_ADMIN_ADDRESS %s",
			fromAddress.Hex(),
			configuredFrom.Hex(),
		)
		return service
	}

	contract, err := abi.NewSFLUVv2(common.HexToAddress(tokenID), client)
	if err != nil {
		service.logf("redeemer auto-grant disabled: failed to initialize SFLUV contract: %s", err)
		return service
	}

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		service.logf("redeemer auto-grant disabled: failed to read chain id: %s", err)
		return service
	}

	redeemerRole, err := contract.REDEEMERROLE(&bind.CallOpts{Context: context.Background()})
	if err != nil {
		service.logf("redeemer auto-grant disabled: failed to read REDEEMER_ROLE: %s", err)
		return service
	}

	// Preflight: can this wallet actually do the job?
	//
	// Everything checked above proves the config parses, not that it works.
	// Without this the service announced itself enabled while every grant
	// reverted — which is how locations approved after the last manual grant
	// ended up holding takings their tills could not redeem. A wrong-chain or
	// wrong-key deploy now says so at boot instead of failing silently forever.
	adminRole, err := contract.GetRoleAdmin(&bind.CallOpts{Context: context.Background()}, redeemerRole)
	if err != nil {
		service.logf("redeemer auto-grant disabled: failed to read the admin role for REDEEMER_ROLE: %s", err)
		return service
	}
	holdsAdmin, err := contract.HasRole(&bind.CallOpts{Context: context.Background()}, adminRole, fromAddress)
	if err != nil {
		service.logf("redeemer auto-grant disabled: failed to check whether %s may grant REDEEMER_ROLE: %s", fromAddress.Hex(), err)
		return service
	}
	if !holdsAdmin {
		service.logf(
			"redeemer auto-grant disabled: %s does NOT hold the admin role for REDEEMER_ROLE (role 0x%x) on token %s, chain %s. "+
				"Every grant would revert with AccessControlUnauthorizedAccount. Point REDEEMER_ADMIN_KEY/REDEEMER_ADMIN_ADDRESS at a wallet that holds it.",
			fromAddress.Hex(), adminRole, tokenID, chainID.String(),
		)
		return service
	}

	// Gas is a warning, not a gate: it can be topped up without a deploy, and
	// the role checks above are reads that keep working either way.
	if balance, err := client.BalanceAt(context.Background(), fromAddress, nil); err != nil {
		service.logf("redeemer auto-grant: could not read the gas balance of %s: %s", fromAddress.Hex(), err)
	} else if balance.Sign() == 0 {
		service.logf(
			"redeemer auto-grant WARNING: %s holds the right role but has NO gas on chain %s. Grants will fail until it is funded.",
			fromAddress.Hex(), chainID.String(),
		)
	}

	service.client = client
	service.contract = contract
	service.privateKey = privateKey
	service.fromAddress = fromAddress
	service.chainID = chainID
	service.redeemerRole = redeemerRole
	service.adminRole = adminRole
	service.enabled = true

	service.logf("redeemer auto-grant enabled with admin wallet %s on token %s, chain %s", service.fromAddress.Hex(), tokenID, chainID.String())
	return service
}

func (r *RedeemerService) IsEnabled() bool {
	return r != nil && r.enabled
}

func (r *RedeemerService) SyncApprovedMerchants(ctx context.Context) error {
	if !r.IsEnabled() {
		return nil
	}
	if r.appDb == nil {
		return fmt.Errorf("app db is not configured for redeemer sync")
	}

	ownerIDs, err := r.appDb.GetOwnersWithApprovedLocations(ctx)
	if err != nil {
		return fmt.Errorf("error loading approved merchant owners: %w", err)
	}

	for _, ownerID := range ownerIDs {
		if err := r.EnsureMerchantHasRedeemerWallet(ctx, ownerID); err != nil {
			r.logf("error ensuring redeemer wallet for user %s: %s", ownerID, err)
		}
	}

	return nil
}

func (r *RedeemerService) SyncAdmins(ctx context.Context) error {
	if !r.IsEnabled() {
		return nil
	}
	if r.appDb == nil {
		return fmt.Errorf("app db is not configured for redeemer sync")
	}

	adminIDs, err := r.appDb.GetAdminUserIDs(ctx)
	if err != nil {
		return fmt.Errorf("error loading admin users: %w", err)
	}

	for _, adminID := range adminIDs {
		if err := r.EnsureMerchantHasRedeemerWallet(ctx, adminID); err != nil {
			r.logf("error ensuring redeemer wallet for admin %s: %s", adminID, err)
		}
	}

	return nil
}

func (r *RedeemerService) EnsureMerchantHasRedeemerWallet(ctx context.Context, ownerID string) error {
	if !r.IsEnabled() {
		return nil
	}
	if ownerID == "" {
		return fmt.Errorf("owner id is required")
	}

	hasRedeemerWallet, err := r.appDb.UserHasRedeemerWallet(ctx, ownerID)
	if err != nil {
		return fmt.Errorf("error checking redeemer wallet flag for user %s: %w", ownerID, err)
	}
	if hasRedeemerWallet {
		return nil
	}

	wallet, err := r.appDb.GetSmartWalletByOwnerIndex(ctx, ownerID, 0)
	if err != nil {
		return fmt.Errorf("error fetching smartwallet index 0 for user %s: %w", ownerID, err)
	}
	if wallet == nil || wallet.SmartAddress == nil || strings.TrimSpace(*wallet.SmartAddress) == "" {
		return fmt.Errorf("no smartwallet index 0 found for user %s", ownerID)
	}
	if !common.IsHexAddress(*wallet.SmartAddress) {
		return fmt.Errorf("invalid smartwallet address %q for user %s", *wallet.SmartAddress, ownerID)
	}

	walletAddress := common.HexToAddress(*wallet.SmartAddress)
	hasRole, err := r.contract.HasRole(&bind.CallOpts{Context: ctx}, r.redeemerRole, walletAddress)
	if err != nil {
		return fmt.Errorf("error checking REDEEMER_ROLE for %s: %w", walletAddress.Hex(), err)
	}

	if !hasRole {
		if err := r.grantRedeemerRole(ctx, walletAddress); err != nil {
			return err
		}
	}

	if wallet.Id == nil {
		return fmt.Errorf("wallet id missing for %s", walletAddress.Hex())
	}
	if err := r.appDb.SetWalletRedeemerStatus(ctx, *wallet.Id, true); err != nil {
		return fmt.Errorf("error updating wallet is_redeemer for wallet %d: %w", *wallet.Id, err)
	}

	return nil
}

func (r *RedeemerService) grantRedeemerRole(ctx context.Context, walletAddress common.Address) error {
	// The Safe path when the role has moved, the env key when it has not.
	if r.granter != nil {
		hash, err := r.granter(ctx, r.redeemerRole, walletAddress)
		if err != nil {
			return fmt.Errorf("error granting REDEEMER_ROLE to %s through the protocol Safe: %w", walletAddress.Hex(), err)
		}
		r.logf("granted REDEEMER_ROLE to %s through the protocol Safe in %s", walletAddress.Hex(), hash)
		return nil
	}

	opts, err := bind.NewKeyedTransactorWithChainID(r.privateKey, r.chainID)
	if err != nil {
		return fmt.Errorf("error creating transactor: %w", err)
	}
	opts.Context = ctx
	opts.From = r.fromAddress

	tx, err := r.contract.GrantRole(opts, r.redeemerRole, walletAddress)
	if err != nil {
		return fmt.Errorf("error granting REDEEMER_ROLE to %s: %w", walletAddress.Hex(), err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	receipt, err := bind.WaitMined(waitCtx, r.client, tx)
	if err != nil {
		return fmt.Errorf("error waiting for grantRole tx %s: %w", tx.Hash().Hex(), err)
	}
	if receipt == nil || receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("grantRole transaction reverted: %s", tx.Hash().Hex())
	}

	r.logf("granted REDEEMER_ROLE to %s in tx %s", walletAddress.Hex(), tx.Hash().Hex())
	return nil
}

func (r *RedeemerService) logf(format string, args ...any) {
	if r.log == nil {
		return
	}
	r.log.Logf(format, args...)
}

// GrantAuthorityRole is the role that may grant REDEEMER_ROLE, and the one the
// cutover has to move onto the redeemer admin's Safe. Zero when the service is
// disabled, in which case there is nothing to move.
func (r *RedeemerService) GrantAuthorityRole() ([32]byte, bool) {
	if r == nil || !r.enabled {
		return [32]byte{}, false
	}
	return r.adminRole, true
}

// HoldsGrantAuthority reports whether an address may grant REDEEMER_ROLE.
//
// Read from the token rather than inferred from our own bookkeeping: the whole
// question is whether the chain agrees, and the cost of assuming wrongly is a
// revoked key and a Safe that cannot grant — which would stop every merchant
// approved after it from being able to redeem.
func (r *RedeemerService) HoldsGrantAuthority(ctx context.Context, address common.Address) (bool, error) {
	if r == nil || !r.enabled || r.contract == nil {
		return false, fmt.Errorf("redeemer service is not enabled")
	}
	return r.contract.HasRole(&bind.CallOpts{Context: ctx}, r.adminRole, address)
}

// LegacyAdminAddress is the env-key address that holds the granting role today.
func (r *RedeemerService) LegacyAdminAddress() (common.Address, bool) {
	if r == nil || !r.enabled {
		return common.Address{}, false
	}
	return r.fromAddress, true
}

// LegacyGrantRole grants a role with the env key, which is how the granting
// authority is handed to the Safe in the first place: the Safe cannot grant
// itself anything.
func (r *RedeemerService) LegacyGrantRole(ctx context.Context, role [32]byte, account common.Address) (string, error) {
	if r == nil || !r.enabled {
		return "", fmt.Errorf("redeemer service is not enabled")
	}
	opts, err := bind.NewKeyedTransactorWithChainID(r.privateKey, r.chainID)
	if err != nil {
		return "", fmt.Errorf("error creating transactor: %w", err)
	}
	opts.Context = ctx
	opts.From = r.fromAddress

	tx, err := r.contract.GrantRole(opts, role, account)
	if err != nil {
		return "", fmt.Errorf("error granting a role to %s: %w", account.Hex(), err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	receipt, err := bind.WaitMined(waitCtx, r.client, tx)
	if err != nil {
		return tx.Hash().Hex(), fmt.Errorf("error waiting for the grant tx %s: %w", tx.Hash().Hex(), err)
	}
	if receipt == nil || receipt.Status != types.ReceiptStatusSuccessful {
		return tx.Hash().Hex(), fmt.Errorf("the grant transaction reverted: %s", tx.Hash().Hex())
	}
	return tx.Hash().Hex(), nil
}

// LegacyRevokeRole takes a role away with the env key. Used once, to strip the
// old address of the authority the Safe now holds.
func (r *RedeemerService) LegacyRevokeRole(ctx context.Context, role [32]byte, account common.Address) (string, error) {
	if r == nil || !r.enabled {
		return "", fmt.Errorf("redeemer service is not enabled")
	}
	opts, err := bind.NewKeyedTransactorWithChainID(r.privateKey, r.chainID)
	if err != nil {
		return "", fmt.Errorf("error creating transactor: %w", err)
	}
	opts.Context = ctx
	opts.From = r.fromAddress

	tx, err := r.contract.RevokeRole(opts, role, account)
	if err != nil {
		return "", fmt.Errorf("error revoking a role from %s: %w", account.Hex(), err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	receipt, err := bind.WaitMined(waitCtx, r.client, tx)
	if err != nil {
		return tx.Hash().Hex(), fmt.Errorf("error waiting for the revoke tx %s: %w", tx.Hash().Hex(), err)
	}
	if receipt == nil || receipt.Status != types.ReceiptStatusSuccessful {
		return tx.Hash().Hex(), fmt.Errorf("the revoke transaction reverted: %s", tx.Hash().Hex())
	}
	return tx.Hash().Hex(), nil
}

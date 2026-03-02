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
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/structs"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type RedeemerService struct {
	appDb *db.AppDB
	log   *logger.LogCloser

	enabled     bool
	syncEnabled bool

	client       *ethclient.Client
	contract     *abi.SFLUVv2
	privateKey   *ecdsa.PrivateKey
	fromAddress  common.Address
	chainID      *big.Int
	redeemerRole [32]byte
}

func NewRedeemerService(appDb *db.AppDB, log *logger.LogCloser) *RedeemerService {
	service := &RedeemerService{
		appDb: appDb,
		log:   log,
	}

	rpcURL := strings.TrimSpace(os.Getenv("RPC_URL"))
	tokenID := strings.TrimSpace(os.Getenv("TOKEN_ID"))
	if rpcURL == "" || tokenID == "" {
		service.logf("redeemer sync disabled: missing RPC_URL or TOKEN_ID")
		return service
	}
	if !common.IsHexAddress(tokenID) {
		service.logf("redeemer sync disabled: invalid TOKEN_ID address %q", tokenID)
		return service
	}

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		service.logf("redeemer sync disabled: error connecting RPC: %s", err)
		return service
	}

	contract, err := abi.NewSFLUVv2(common.HexToAddress(tokenID), client)
	if err != nil {
		service.logf("redeemer sync disabled: failed to initialize SFLUV contract: %s", err)
		return service
	}

	redeemerRole, err := contract.REDEEMERROLE(&bind.CallOpts{Context: context.Background()})
	if err != nil {
		service.logf("redeemer sync disabled: failed to read REDEEMER_ROLE: %s", err)
		return service
	}

	service.client = client
	service.contract = contract
	service.redeemerRole = redeemerRole
	service.syncEnabled = true
	service.logf("redeemer role sync enabled")

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

	redeemerAdminKey := strings.TrimPrefix(strings.TrimSpace(os.Getenv("REDEEMER_ADMIN_KEY")), "0x")
	redeemerAdminAddress := strings.TrimSpace(os.Getenv("REDEEMER_ADMIN_ADDRESS"))
	if redeemerAdminKey == "" || redeemerAdminAddress == "" {
		service.logf("redeemer auto-grant disabled: missing REDEEMER_ADMIN_KEY or REDEEMER_ADMIN_ADDRESS")
		return service
	}
	if !common.IsHexAddress(redeemerAdminAddress) {
		service.logf("redeemer auto-grant disabled: invalid REDEEMER_ADMIN_ADDRESS %q", redeemerAdminAddress)
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

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		service.logf("redeemer auto-grant disabled: failed to read chain id: %s", err)
		return service
	}

	service.privateKey = privateKey
	service.fromAddress = fromAddress
	service.chainID = chainID
	service.enabled = true
	service.logf("redeemer auto-grant enabled with admin wallet %s", service.fromAddress.Hex())
	return service
}

func (r *RedeemerService) IsEnabled() bool {
	return r != nil && r.enabled
}

func (r *RedeemerService) CanSync() bool {
	return r != nil && r.syncEnabled
}

func (r *RedeemerService) SyncWalletRedeemerStatuses(ctx context.Context) error {
	if !r.CanSync() {
		return nil
	}
	if r.appDb == nil {
		return fmt.Errorf("app db is not configured for redeemer sync")
	}

	wallets, err := r.appDb.GetAllWallets(ctx)
	if err != nil {
		return fmt.Errorf("error loading wallets for redeemer sync: %w", err)
	}

	for _, wallet := range wallets {
		if err := r.syncWalletRedeemerStatus(ctx, wallet); err != nil {
			if wallet != nil && wallet.Id != nil {
				r.logf("error syncing is_redeemer for wallet %d: %s", *wallet.Id, err)
			} else {
				r.logf("error syncing is_redeemer for wallet: %s", err)
			}
		}
	}

	return nil
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

func (r *RedeemerService) StartRoleWatcher(ctx context.Context) {
	if !r.CanSync() {
		return
	}

	go r.watchRoleEvents(ctx)
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
	if err := r.syncWalletRedeemerStatus(ctx, wallet); err != nil {
		return fmt.Errorf("error syncing wallet is_redeemer for wallet %d: %w", *wallet.Id, err)
	}

	return nil
}

func (r *RedeemerService) grantRedeemerRole(ctx context.Context, walletAddress common.Address) error {
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

func (r *RedeemerService) syncWalletRedeemerStatus(ctx context.Context, wallet *structs.Wallet) error {
	if wallet == nil || wallet.Id == nil {
		return nil
	}

	walletAddress := r.walletAddress(wallet)
	if walletAddress == nil {
		if err := r.appDb.SetWalletRedeemerStatus(ctx, *wallet.Id, false); err != nil {
			return fmt.Errorf("error updating wallet %d with no valid address: %w", *wallet.Id, err)
		}
		return nil
	}

	hasRole, err := r.contract.HasRole(&bind.CallOpts{Context: ctx}, r.redeemerRole, *walletAddress)
	if err != nil {
		return fmt.Errorf("error checking REDEEMER_ROLE for %s: %w", walletAddress.Hex(), err)
	}

	if err := r.appDb.SetWalletRedeemerStatus(ctx, *wallet.Id, hasRole); err != nil {
		return fmt.Errorf("error updating is_redeemer for wallet %d (%s): %w", *wallet.Id, walletAddress.Hex(), err)
	}

	return nil
}

func (r *RedeemerService) syncWalletRedeemerStatusByAddress(ctx context.Context, walletAddress common.Address, isRedeemer bool) error {
	if !r.CanSync() || r.appDb == nil {
		return nil
	}

	rows, err := r.appDb.SetWalletRedeemerStatusByAddress(ctx, walletAddress.Hex(), isRedeemer)
	if err != nil {
		return fmt.Errorf("error updating wallet is_redeemer for address %s: %w", walletAddress.Hex(), err)
	}

	if rows == 0 {
		r.logf("redeemer role event for %s had no matching wallet rows", walletAddress.Hex())
	}

	return nil
}

func (r *RedeemerService) watchRoleEvents(ctx context.Context) {
	roleFilter := [][32]byte{r.redeemerRole}

	for {
		if ctx.Err() != nil {
			return
		}

		grants := make(chan *abi.SFLUVv2RoleGranted)
		revokes := make(chan *abi.SFLUVv2RoleRevoked)

		grantSub, err := r.contract.WatchRoleGranted(&bind.WatchOpts{Context: ctx}, grants, roleFilter, nil, nil)
		if err != nil {
			r.logf("error subscribing to RoleGranted events: %s", err)
			if !r.waitForWatcherRetry(ctx) {
				return
			}
			continue
		}

		revokeSub, err := r.contract.WatchRoleRevoked(&bind.WatchOpts{Context: ctx}, revokes, roleFilter, nil, nil)
		if err != nil {
			grantSub.Unsubscribe()
			r.logf("error subscribing to RoleRevoked events: %s", err)
			if !r.waitForWatcherRetry(ctx) {
				return
			}
			continue
		}

		r.logf("redeemer role watcher subscribed")

		subscriptionFailed := false

		for !subscriptionFailed {
			select {
			case <-ctx.Done():
				grantSub.Unsubscribe()
				revokeSub.Unsubscribe()
				return
			case err := <-grantSub.Err():
				grantSub.Unsubscribe()
				revokeSub.Unsubscribe()
				if err != nil {
					r.logf("redeemer RoleGranted watcher error: %s", err)
				}
				subscriptionFailed = true
			case err := <-revokeSub.Err():
				grantSub.Unsubscribe()
				revokeSub.Unsubscribe()
				if err != nil {
					r.logf("redeemer RoleRevoked watcher error: %s", err)
				}
				subscriptionFailed = true
			case event := <-grants:
				if event == nil {
					continue
				}
				updateCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				err := r.syncWalletRedeemerStatusByAddress(updateCtx, event.Account, true)
				cancel()
				if err != nil {
					r.logf("error syncing is_redeemer after RoleGranted for %s: %s", event.Account.Hex(), err)
				}
			case event := <-revokes:
				if event == nil {
					continue
				}
				updateCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				err := r.syncWalletRedeemerStatusByAddress(updateCtx, event.Account, false)
				cancel()
				if err != nil {
					r.logf("error syncing is_redeemer after RoleRevoked for %s: %s", event.Account.Hex(), err)
				}
			}
		}

		if !r.waitForWatcherRetry(ctx) {
			return
		}
	}
}

func (r *RedeemerService) waitForWatcherRetry(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(3 * time.Second):
		return true
	}
}

func (r *RedeemerService) walletAddress(wallet *structs.Wallet) *common.Address {
	if wallet == nil {
		return nil
	}

	if wallet.SmartAddress != nil && strings.TrimSpace(*wallet.SmartAddress) != "" && common.IsHexAddress(*wallet.SmartAddress) {
		address := common.HexToAddress(*wallet.SmartAddress)
		return &address
	}

	eoa := strings.TrimSpace(wallet.EoaAddress)
	if common.IsHexAddress(eoa) {
		address := common.HexToAddress(eoa)
		return &address
	}

	return nil
}

func (r *RedeemerService) logf(format string, args ...any) {
	if r.log == nil {
		return
	}
	r.log.Logf(format, args...)
}

package handlers

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/SFLuv/app/backend/abi"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/structs"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type MinterService struct {
	appDb *db.AppDB
	log   *logger.LogCloser

	enabled bool

	client     *ethclient.Client
	contract   *abi.SFLUVv2
	minterRole [32]byte
}

func NewMinterService(appDb *db.AppDB, log *logger.LogCloser) *MinterService {
	service := &MinterService{
		appDb: appDb,
		log:   log,
	}

	rpcURL := strings.TrimSpace(os.Getenv("RPC_URL"))
	tokenID := strings.TrimSpace(os.Getenv("TOKEN_ID"))
	if rpcURL == "" || tokenID == "" {
		service.logf("minter sync disabled: missing RPC_URL or TOKEN_ID")
		return service
	}
	if !common.IsHexAddress(tokenID) {
		service.logf("minter sync disabled: invalid TOKEN_ID address %q", tokenID)
		return service
	}

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		service.logf("minter sync disabled: error connecting RPC: %s", err)
		return service
	}

	contract, err := abi.NewSFLUVv2(common.HexToAddress(tokenID), client)
	if err != nil {
		service.logf("minter sync disabled: failed to initialize SFLUV contract: %s", err)
		return service
	}

	minterRole, err := contract.MINTERROLE(&bind.CallOpts{Context: context.Background()})
	if err != nil {
		service.logf("minter sync disabled: failed to read MINTER_ROLE: %s", err)
		return service
	}

	service.client = client
	service.contract = contract
	service.minterRole = minterRole
	service.enabled = true
	service.logf("minter role sync enabled")

	return service
}

func (m *MinterService) IsEnabled() bool {
	return m != nil && m.enabled
}

func (m *MinterService) SyncWalletMinterStatuses(ctx context.Context) error {
	if !m.IsEnabled() {
		return nil
	}
	if m.appDb == nil {
		return fmt.Errorf("app db is not configured for minter sync")
	}

	wallets, err := m.appDb.GetAllWallets(ctx)
	if err != nil {
		return fmt.Errorf("error loading wallets for minter sync: %w", err)
	}

	for _, wallet := range wallets {
		if wallet == nil || wallet.Id == nil {
			continue
		}

		walletAddress := m.walletAddress(wallet)
		if walletAddress == nil {
			continue
		}

		hasRole, err := m.contract.HasRole(&bind.CallOpts{Context: ctx}, m.minterRole, *walletAddress)
		if err != nil {
			m.logf("error checking MINTER_ROLE for wallet %d (%s): %s", *wallet.Id, walletAddress.Hex(), err)
			continue
		}

		if err := m.appDb.SetWalletMinterStatus(ctx, *wallet.Id, hasRole); err != nil {
			m.logf("error updating is_minter for wallet %d (%s): %s", *wallet.Id, walletAddress.Hex(), err)
			continue
		}
	}

	return nil
}

func (m *MinterService) walletAddress(wallet *structs.Wallet) *common.Address {
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

func (m *MinterService) logf(format string, args ...any) {
	if m.log == nil {
		return
	}
	m.log.Logf(format, args...)
}

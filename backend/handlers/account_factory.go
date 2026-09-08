package handlers

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// The one function needed off the account factory. getAddress is a view that
// returns the counterfactual address for an owner and salt — the account does
// not have to exist, which is exactly the point: the address can be recorded and
// paid into long before anything is deployed to it.
const accountFactoryABI = `[{
	"inputs": [
		{"internalType": "address", "name": "owner", "type": "address"},
		{"internalType": "uint256", "name": "salt", "type": "uint256"}
	],
	"name": "getAddress",
	"outputs": [{"internalType": "address", "name": "", "type": "address"}],
	"stateMutability": "view",
	"type": "function"
}]`

// deriveSmartAccountAddress asks the account factory what address an owner's
// smart account at the given index will have.
//
// This is deterministic CREATE2 arithmetic, so the answer is stable and the same
// one the client computes — verified against a known wallet before relying on it.
func (a *AppService) deriveSmartAccountAddress(ctx context.Context, ownerEOA string, index int) (string, error) {
	if a.clientConfig == nil {
		return "", fmt.Errorf("client config unavailable")
	}
	if strings.TrimSpace(ownerEOA) == "" {
		return "", fmt.Errorf("owner has no signing address to derive from")
	}

	factory := strings.TrimSpace(a.clientConfig.Community.PrimaryAccountFactory.Address)
	if !common.IsHexAddress(factory) {
		return "", fmt.Errorf("account factory address is not configured")
	}

	rpcURL := strings.TrimSpace(a.clientConfig.ReadRPCURL())
	if rpcURL == "" {
		return "", fmt.Errorf("read RPC URL is missing from client config")
	}

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(dialCtx, rpcURL)
	if err != nil {
		return "", fmt.Errorf("error dialling chain to derive an account address: %w", err)
	}
	defer client.Close()

	parsed, err := abi.JSON(strings.NewReader(accountFactoryABI))
	if err != nil {
		return "", fmt.Errorf("error parsing account factory abi: %w", err)
	}

	payload, err := parsed.Pack("getAddress", common.HexToAddress(ownerEOA), big.NewInt(int64(index)))
	if err != nil {
		return "", fmt.Errorf("error encoding getAddress call: %w", err)
	}

	callCtx, callCancel := context.WithTimeout(ctx, 10*time.Second)
	defer callCancel()
	factoryAddress := common.HexToAddress(factory)
	raw, err := client.CallContract(callCtx, ethereum.CallMsg{To: &factoryAddress, Data: payload}, nil)
	if err != nil {
		return "", fmt.Errorf("error calling account factory: %w", err)
	}

	values, err := parsed.Unpack("getAddress", raw)
	if err != nil || len(values) == 0 {
		return "", fmt.Errorf("account factory returned an unreadable address")
	}
	derived, ok := values[0].(common.Address)
	if !ok || derived == (common.Address{}) {
		return "", fmt.Errorf("account factory returned an empty address")
	}

	return derived.Hex(), nil
}

package protocol

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// The slices of Safe, factory and ERC-20 this package needs. Written out rather
// than generated because it is four functions and a generated binding for each
// contract would be a lot of code nobody reads.
const (
	safeABI = `[
		{"type":"function","name":"execTransactionFromModule","stateMutability":"nonpayable",
		 "inputs":[{"name":"to","type":"address"},{"name":"value","type":"uint256"},
		           {"name":"data","type":"bytes"},{"name":"operation","type":"uint8"}],
		 "outputs":[{"name":"success","type":"bool"}]},
		{"type":"function","name":"getOwners","stateMutability":"view","inputs":[],
		 "outputs":[{"name":"","type":"address[]"}]},
		{"type":"function","name":"getThreshold","stateMutability":"view","inputs":[],
		 "outputs":[{"name":"","type":"uint256"}]},
		{"type":"function","name":"isOwner","stateMutability":"view",
		 "inputs":[{"name":"owner","type":"address"}],"outputs":[{"name":"","type":"bool"}]},
		{"type":"function","name":"swapOwner","stateMutability":"nonpayable",
		 "inputs":[{"name":"prevOwner","type":"address"},{"name":"oldOwner","type":"address"},
		           {"name":"newOwner","type":"address"}],"outputs":[]},
		{"type":"function","name":"addOwnerWithThreshold","stateMutability":"nonpayable",
		 "inputs":[{"name":"owner","type":"address"},{"name":"_threshold","type":"uint256"}],
		 "outputs":[]},
		{"type":"function","name":"removeOwner","stateMutability":"nonpayable",
		 "inputs":[{"name":"prevOwner","type":"address"},{"name":"owner","type":"address"},
		           {"name":"_threshold","type":"uint256"}],"outputs":[]}
	]`

	accountFactoryABI = `[
		{"type":"function","name":"createAccount","stateMutability":"nonpayable",
		 "inputs":[{"name":"owner","type":"address"},{"name":"salt","type":"uint256"}],
		 "outputs":[{"name":"","type":"address"}]},
		{"type":"function","name":"getAddress","stateMutability":"view",
		 "inputs":[{"name":"owner","type":"address"},{"name":"salt","type":"uint256"}],
		 "outputs":[{"name":"","type":"address"}]}
	]`

	erc20ABI = `[
		{"type":"function","name":"transfer","stateMutability":"nonpayable",
		 "inputs":[{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],
		 "outputs":[{"name":"","type":"bool"}]},
		{"type":"function","name":"balanceOf","stateMutability":"view",
		 "inputs":[{"name":"account","type":"address"}],"outputs":[{"name":"","type":"uint256"}]},
		{"type":"function","name":"grantRole","stateMutability":"nonpayable",
		 "inputs":[{"name":"role","type":"bytes32"},{"name":"account","type":"address"}],
		 "outputs":[]},
		{"type":"function","name":"revokeRole","stateMutability":"nonpayable",
		 "inputs":[{"name":"role","type":"bytes32"},{"name":"account","type":"address"}],
		 "outputs":[]},
		{"type":"function","name":"hasRole","stateMutability":"view",
		 "inputs":[{"name":"role","type":"bytes32"},{"name":"account","type":"address"}],
		 "outputs":[{"name":"","type":"bool"}]}
	]`
)

// SentinelOwner heads the Safe's owner linked list. Removing or swapping the
// first owner names it as the predecessor.
var SentinelOwner = common.HexToAddress("0x0000000000000000000000000000000000000001")

func packed(rawABI, method string, args ...any) ([]byte, error) {
	parsed, err := abi.JSON(strings.NewReader(rawABI))
	if err != nil {
		return nil, fmt.Errorf("error parsing an abi: %w", err)
	}
	payload, err := parsed.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("error encoding %s: %w", method, err)
	}
	return payload, nil
}

// SafeModuleCallData wraps an inner call as the operation's callData.
//
// The token entry point is enabled on these Safes as a module, so a sponsored
// operation's callData is a call on the Safe itself: execTransactionFromModule
// performs the inner call with the Safe as msg.sender and no owner signatures,
// because module authority is the signature. operation is 0 — CALL, never
// DELEGATECALL, which would run arbitrary code against the Safe's own storage.
func SafeModuleCallData(to common.Address, value *big.Int, data []byte) ([]byte, error) {
	if value == nil {
		value = big.NewInt(0)
	}
	if data == nil {
		data = []byte{}
	}
	return packed(safeABI, "execTransactionFromModule", to, value, data, uint8(0))
}

// InitCode is the factory call that deploys a Safe at its counterfactual
// address, prefixed with the factory to call it on.
func (c *Client) InitCode(owner common.Address, salt *big.Int) ([]byte, error) {
	call, err := packed(accountFactoryABI, "createAccount", owner, salt)
	if err != nil {
		return nil, err
	}
	return append(c.factory.Bytes(), call...), nil
}

// DeriveAddress asks the factory what address an owner's Safe at this index
// will have. Deterministic CREATE2 arithmetic, so the answer holds before
// anything is deployed — which is what lets the address be recorded, funded and
// checked against while deployment is still pending.
func (c *Client) DeriveAddress(ctx context.Context, owner common.Address, salt *big.Int) (common.Address, error) {
	payload, err := packed(accountFactoryABI, "getAddress", owner, salt)
	if err != nil {
		return common.Address{}, err
	}
	raw, err := c.readCall(ctx, c.factory, payload)
	if err != nil {
		return common.Address{}, fmt.Errorf("error calling the account factory: %w", err)
	}
	if len(raw) < 32 {
		return common.Address{}, fmt.Errorf("the account factory returned %d bytes", len(raw))
	}
	derived := common.BytesToAddress(raw[12:32])
	if derived == (common.Address{}) {
		return common.Address{}, fmt.Errorf("the account factory returned the zero address")
	}
	return derived, nil
}

// Owners reads a Safe's current owner set. The rotation checks itself against
// this afterwards rather than trusting that a submitted operation did what it
// said.
func (c *Client) Owners(ctx context.Context, safe common.Address) ([]common.Address, error) {
	payload, err := packed(safeABI, "getOwners")
	if err != nil {
		return nil, err
	}
	raw, err := c.readCall(ctx, safe, payload)
	if err != nil {
		return nil, fmt.Errorf("error reading the owners of %s: %w", safe.Hex(), err)
	}
	parsed, err := abi.JSON(strings.NewReader(safeABI))
	if err != nil {
		return nil, err
	}
	values, err := parsed.Unpack("getOwners", raw)
	if err != nil || len(values) == 0 {
		return nil, fmt.Errorf("the owners of %s came back unreadable", safe.Hex())
	}
	owners, ok := values[0].([]common.Address)
	if !ok {
		return nil, fmt.Errorf("the owners of %s came back as the wrong type", safe.Hex())
	}
	return owners, nil
}

// SwapOwnerCallData replaces one owner with another in a single call.
//
// Deliberately swapOwner rather than addOwnerWithThreshold followed by
// removeOwner. Those are two operations, and between them the Safe has two
// valid signers — including the one being rotated away from, which is the key
// presumed compromised. A swap has no such window: after one operation the old
// key is already worthless.
//
// prevOwner is the predecessor in the Safe's owner linked list, which is why
// this takes the current owner set rather than just the two addresses.
func SwapOwnerCallData(owners []common.Address, oldOwner, newOwner common.Address) ([]byte, error) {
	previous := SentinelOwner
	found := false
	for i, owner := range owners {
		if owner != oldOwner {
			continue
		}
		found = true
		if i > 0 {
			previous = owners[i-1]
		}
		break
	}
	if !found {
		return nil, fmt.Errorf("%s is not an owner of this Safe, so there is nothing to rotate", oldOwner.Hex())
	}
	for _, owner := range owners {
		if owner == newOwner {
			return nil, fmt.Errorf("%s is already an owner of this Safe", newOwner.Hex())
		}
	}
	return packed(safeABI, "swapOwner", previous, oldOwner, newOwner)
}

// TransferCallData is an ERC-20 transfer, for the inner call of a sweep.
func TransferCallData(to common.Address, amount *big.Int) ([]byte, error) {
	return packed(erc20ABI, "transfer", to, amount)
}

// GrantRoleCallData and RevokeRoleCallData move an AccessControl role, which is
// how the redeemer admin's authority follows it onto its Safe.
func GrantRoleCallData(role [32]byte, account common.Address) ([]byte, error) {
	return packed(erc20ABI, "grantRole", role, account)
}

func RevokeRoleCallData(role [32]byte, account common.Address) ([]byte, error) {
	return packed(erc20ABI, "revokeRole", role, account)
}

// TokenBalance reads an ERC-20 balance through the read RPC.
func (c *Client) TokenBalance(ctx context.Context, token, holder common.Address) (*big.Int, error) {
	payload, err := packed(erc20ABI, "balanceOf", holder)
	if err != nil {
		return nil, err
	}
	raw, err := c.readCall(ctx, token, payload)
	if err != nil {
		return nil, fmt.Errorf("error reading the token balance of %s: %w", holder.Hex(), err)
	}
	return new(big.Int).SetBytes(raw), nil
}

// HasRole reports whether an account holds an AccessControl role on a token.
func (c *Client) HasRole(ctx context.Context, token common.Address, role [32]byte, account common.Address) (bool, error) {
	payload, err := packed(erc20ABI, "hasRole", role, account)
	if err != nil {
		return false, err
	}
	raw, err := c.readCall(ctx, token, payload)
	if err != nil {
		return false, fmt.Errorf("error checking a role on %s: %w", account.Hex(), err)
	}
	return new(big.Int).SetBytes(raw).Sign() != 0, nil
}

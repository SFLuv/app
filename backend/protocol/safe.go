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

// ── Batching ─────────────────────────────────────────────────────────────────

// MultiSendCallOnly is Safe's batching helper, at its canonical deployment.
//
// The "call only" variant deliberately. Batching requires the Safe to
// DELEGATECALL into a helper — that is the only way one operation can make
// several calls — and this is the variant that then performs each inner call as
// an ordinary CALL and reverts on a delegatecall. So the Safe's storage is
// executed against by exactly one contract whose whole job is a loop, rather
// than by whatever the batch happens to contain.
//
// Verified deployed at this address on Celo (410 bytes) before relying on it.
var MultiSendCallOnly = common.HexToAddress("0x9641d764fc13c8B624c04430C7356C1C7C8102e2")

const multiSendABI = `[
	{"type":"function","name":"multiSend","stateMutability":"payable",
	 "inputs":[{"name":"transactions","type":"bytes"}],"outputs":[]}
]`

// BatchedCall is one call inside a batch.
type BatchedCall struct {
	To    common.Address
	Value *big.Int
	Data  []byte
}

// MultiSendCallData encodes a batch for MultiSendCallOnly.
//
// The inner encoding is Safe's own packing, which is not ABI encoding: each
// call is one byte of operation, twenty of address, thirty-two of value,
// thirty-two of data length, then the data, concatenated with no padding
// between entries. Getting it wrong does not fail cleanly — the helper walks
// the bytes by those offsets — so this was checked against the reference
// encoder's output before use.
//
// The operation byte is zero for every call. MultiSendCallOnly would revert on
// anything else, and this keeps that true at the point the bytes are written
// rather than relying on the helper to catch it.
func MultiSendCallData(calls []BatchedCall) ([]byte, error) {
	if len(calls) == 0 {
		return nil, fmt.Errorf("no calls to batch")
	}

	packed := []byte{}
	for _, call := range calls {
		value := call.Value
		if value == nil {
			value = big.NewInt(0)
		}
		packed = append(packed, 0)
		packed = append(packed, call.To.Bytes()...)
		packed = append(packed, common.LeftPadBytes(value.Bytes(), 32)...)
		packed = append(packed, common.LeftPadBytes(big.NewInt(int64(len(call.Data))).Bytes(), 32)...)
		packed = append(packed, call.Data...)
	}

	return packed2(multiSendABI, "multiSend", packed)
}

// packed2 is packed() for a single bytes argument, kept separate only because
// the variadic form cannot take a []byte without it being read as the args
// slice.
func packed2(rawABI, method string, arg []byte) ([]byte, error) {
	return packed(rawABI, method, arg)
}

// BatchCallData wraps a batch as the operation's callData.
//
// Operation 1 — DELEGATECALL — because that is what batching is: the Safe runs
// MultiSendCallOnly's loop as itself, so each transfer inside comes from the
// Safe. Single calls stay on operation 0 and never go near this.
func BatchCallData(calls []BatchedCall) ([]byte, error) {
	inner, err := MultiSendCallData(calls)
	if err != nil {
		return nil, err
	}
	return packed(safeABI, "execTransactionFromModule", MultiSendCallOnly, big.NewInt(0), inner, uint8(1))
}

// AddOwnerCallData adds an owner to a Safe, setting the threshold.
//
// Used for the offline recovery owner, with the threshold kept at 1: the point
// is a second way in, not a second signature. Like swapOwner this is a Safe
// self-call, so it travels as the inner call of an operation the Safe makes to
// itself.
func AddOwnerCallData(owner common.Address, threshold *big.Int) ([]byte, error) {
	if threshold == nil {
		threshold = big.NewInt(1)
	}
	return packed(safeABI, "addOwnerWithThreshold", owner, threshold)
}

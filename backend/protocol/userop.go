// Package protocol owns the backend's own smart accounts: the Safes that hold
// protocol funds and the keys that sign for them.
//
// Why this exists at all. Until now every backend account was an EOA whose
// private key sat in an environment variable, which meant a key could only be
// rotated by a redeploy, and an address could only be rotated by editing every
// check that mentioned it. A Safe separates the two: the address is permanent
// and the signer behind it is replaceable, so a compromised key is a swapOwner
// away from being worthless rather than a migration.
package protocol

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

// UserOperation is ERC-4337 v0.6 as the Citizen Wallet engine speaks it.
//
// Field order matters: it is the order of the tuple getUserOpHash takes, and
// the hash is what gets signed.
type UserOperation struct {
	Sender               common.Address
	Nonce                *big.Int
	InitCode             []byte
	CallData             []byte
	CallGasLimit         *big.Int
	VerificationGasLimit *big.Int
	PreVerificationGas   *big.Int
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	PaymasterAndData     []byte
	Signature            []byte
}

// NewUserOperation is the empty operation the engine expects to be handed
// before it fills the gas fields in — every number zero, every byte string
// empty. Mirrors getEmptyUserOp in the CW SDK.
func NewUserOperation(sender common.Address) *UserOperation {
	return &UserOperation{
		Sender:               sender,
		Nonce:                big.NewInt(0),
		InitCode:             []byte{},
		CallData:             []byte{},
		CallGasLimit:         big.NewInt(0),
		VerificationGasLimit: big.NewInt(0),
		PreVerificationGas:   big.NewInt(0),
		MaxFeePerGas:         big.NewInt(0),
		MaxPriorityFeePerGas: big.NewInt(0),
		PaymasterAndData:     []byte{},
		Signature:            []byte{},
	}
}

// userOpJSON is the wire shape. Every number is a minimal hex string.
type userOpJSON struct {
	Sender               string `json:"sender"`
	Nonce                string `json:"nonce"`
	InitCode             string `json:"initCode"`
	CallData             string `json:"callData"`
	CallGasLimit         string `json:"callGasLimit"`
	VerificationGasLimit string `json:"verificationGasLimit"`
	PreVerificationGas   string `json:"preVerificationGas"`
	MaxFeePerGas         string `json:"maxFeePerGas"`
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas"`
	PaymasterAndData     string `json:"paymasterAndData"`
	Signature            string `json:"signature"`
}

// quantity encodes a number the way the engine wants it.
//
// The CW SDK writes `toBeHex(v).replace("0x0", "0x")`, which strips the leading
// zero nibble off an odd-length value and leaves zero as "0x0". Go's %#x
// produces exactly that for every value — "0x0", "0x1", "0x10", "0xf4240" —
// so this is a one-liner rather than a reimplementation of that replace. The
// values were compared against the SDK's output before relying on it.
func quantity(value *big.Int) string {
	if value == nil {
		return "0x0"
	}
	return fmt.Sprintf("%#x", value)
}

func (op *UserOperation) toJSON() userOpJSON {
	return userOpJSON{
		Sender:               op.Sender.Hex(),
		Nonce:                quantity(op.Nonce),
		InitCode:             hexutil.Encode(op.InitCode),
		CallData:             hexutil.Encode(op.CallData),
		CallGasLimit:         quantity(op.CallGasLimit),
		VerificationGasLimit: quantity(op.VerificationGasLimit),
		PreVerificationGas:   quantity(op.PreVerificationGas),
		MaxFeePerGas:         quantity(op.MaxFeePerGas),
		MaxPriorityFeePerGas: quantity(op.MaxPriorityFeePerGas),
		PaymasterAndData:     hexutil.Encode(op.PaymasterAndData),
		Signature:            hexutil.Encode(op.Signature),
	}
}

func userOpFromJSON(raw userOpJSON) (*UserOperation, error) {
	number := func(field, value string) (*big.Int, error) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || trimmed == "0x" {
			return big.NewInt(0), nil
		}
		parsed, ok := new(big.Int).SetString(strings.TrimPrefix(trimmed, "0x"), 16)
		if !ok {
			return nil, fmt.Errorf("sponsored operation has an unreadable %s %q", field, value)
		}
		return parsed, nil
	}
	bytesField := func(field, value string) ([]byte, error) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || trimmed == "0x" {
			return []byte{}, nil
		}
		decoded, err := hexutil.Decode(trimmed)
		if err != nil {
			return nil, fmt.Errorf("sponsored operation has an unreadable %s: %w", field, err)
		}
		return decoded, nil
	}

	if !common.IsHexAddress(strings.TrimSpace(raw.Sender)) {
		return nil, fmt.Errorf("sponsored operation has an invalid sender %q", raw.Sender)
	}
	op := &UserOperation{Sender: common.HexToAddress(raw.Sender)}

	var err error
	if op.Nonce, err = number("nonce", raw.Nonce); err != nil {
		return nil, err
	}
	if op.InitCode, err = bytesField("initCode", raw.InitCode); err != nil {
		return nil, err
	}
	if op.CallData, err = bytesField("callData", raw.CallData); err != nil {
		return nil, err
	}
	if op.CallGasLimit, err = number("callGasLimit", raw.CallGasLimit); err != nil {
		return nil, err
	}
	if op.VerificationGasLimit, err = number("verificationGasLimit", raw.VerificationGasLimit); err != nil {
		return nil, err
	}
	if op.PreVerificationGas, err = number("preVerificationGas", raw.PreVerificationGas); err != nil {
		return nil, err
	}
	if op.MaxFeePerGas, err = number("maxFeePerGas", raw.MaxFeePerGas); err != nil {
		return nil, err
	}
	if op.MaxPriorityFeePerGas, err = number("maxPriorityFeePerGas", raw.MaxPriorityFeePerGas); err != nil {
		return nil, err
	}
	if op.PaymasterAndData, err = bytesField("paymasterAndData", raw.PaymasterAndData); err != nil {
		return nil, err
	}
	if op.Signature, err = bytesField("signature", raw.Signature); err != nil {
		return nil, err
	}
	return op, nil
}

// The one function needed off the token entry point. The tuple's field order is
// the struct's field order; getting them out of step would produce a valid
// signature over the wrong operation, which the bundler would reject with
// nothing useful to say about why.
const tokenEntryPointABI = `[{
	"inputs": [{
		"components": [
			{"name": "sender", "type": "address"},
			{"name": "nonce", "type": "uint256"},
			{"name": "initCode", "type": "bytes"},
			{"name": "callData", "type": "bytes"},
			{"name": "callGasLimit", "type": "uint256"},
			{"name": "verificationGasLimit", "type": "uint256"},
			{"name": "preVerificationGas", "type": "uint256"},
			{"name": "maxFeePerGas", "type": "uint256"},
			{"name": "maxPriorityFeePerGas", "type": "uint256"},
			{"name": "paymasterAndData", "type": "bytes"},
			{"name": "signature", "type": "bytes"}
		],
		"name": "userOp",
		"type": "tuple"
	}],
	"name": "getUserOpHash",
	"outputs": [{"name": "", "type": "bytes32"}],
	"stateMutability": "view",
	"type": "function"
}]`

// userOpTuple is the ABI-encodable mirror of UserOperation. go-ethereum packs a
// solidity tuple from a struct whose exported fields line up with the
// components, so this has to exist separately from the big.Int-pointer form.
type userOpTuple struct {
	Sender               common.Address
	Nonce                *big.Int
	InitCode             []byte
	CallData             []byte
	CallGasLimit         *big.Int
	VerificationGasLimit *big.Int
	PreVerificationGas   *big.Int
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	PaymasterAndData     []byte
	Signature            []byte
}

func (op *UserOperation) tuple() userOpTuple {
	return userOpTuple{
		Sender:               op.Sender,
		Nonce:                op.Nonce,
		InitCode:             op.InitCode,
		CallData:             op.CallData,
		CallGasLimit:         op.CallGasLimit,
		VerificationGasLimit: op.VerificationGasLimit,
		PreVerificationGas:   op.PreVerificationGas,
		MaxFeePerGas:         op.MaxFeePerGas,
		MaxPriorityFeePerGas: op.MaxPriorityFeePerGas,
		PaymasterAndData:     op.PaymasterAndData,
		Signature:            op.Signature,
	}
}

// Hash asks the token entry point what this operation's hash is.
//
// Deliberately a chain read rather than arithmetic done here. The hash folds in
// the entry point's own address and the chain id, and a local reimplementation
// that drifted from the deployed contract would sign something the bundler
// cannot validate — a failure that looks like a bad key. The contract is the
// authority on its own hash.
func (c *Client) Hash(ctx context.Context, op *UserOperation) ([32]byte, error) {
	var hash [32]byte

	parsed, err := abi.JSON(strings.NewReader(tokenEntryPointABI))
	if err != nil {
		return hash, fmt.Errorf("error parsing the token entry point abi: %w", err)
	}
	payload, err := parsed.Pack("getUserOpHash", op.tuple())
	if err != nil {
		return hash, fmt.Errorf("error encoding getUserOpHash: %w", err)
	}

	raw, err := c.readCall(ctx, c.entryPoint, payload)
	if err != nil {
		return hash, fmt.Errorf("error calling getUserOpHash: %w", err)
	}
	if len(raw) != 32 {
		return hash, fmt.Errorf("getUserOpHash returned %d bytes, want 32", len(raw))
	}
	copy(hash[:], raw)
	return hash, nil
}

// Sign puts the owner's signature on an operation.
//
// The entry point verifies an EIP-191 personal_sign over the 32-byte operation
// hash — not the raw hash — because that is what the CW SDK produces with
// ethers' signMessage, and the account was deployed to expect it. go-ethereum
// returns recovery id 0/1 where the rest of the ecosystem wants 27/28, so the
// last byte is adjusted; without it the recovered signer is garbage and the
// operation fails validation rather than erroring usefully.
func Sign(op *UserOperation, hash [32]byte, key *ecdsa.PrivateKey) error {
	if key == nil {
		return fmt.Errorf("no signing key")
	}
	signature, err := crypto.Sign(accounts.TextHash(hash[:]), key)
	if err != nil {
		return fmt.Errorf("error signing a user operation: %w", err)
	}
	if len(signature) != 65 {
		return fmt.Errorf("signature is %d bytes, want 65", len(signature))
	}
	if signature[64] < 27 {
		signature[64] += 27
	}
	op.Signature = signature
	return nil
}

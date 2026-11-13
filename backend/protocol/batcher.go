package protocol

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Batching payouts through SFLuvBatcher (SFLuv/contracts, src/SFLuvBatcher.sol).
//
// One sponsored operation makes one call, so a burst of payouts used to cost an
// operation each. Safe's MultiSend was tried and could never have worked here:
// the paymaster sponsors an operation only when the call inside it targets a
// whitelisted contract, and the CommunityModule executes every call as a CALL,
// so MultiSend's loop would have run as itself with no tokens to send.
// Whitelisting it would also have let any sponsored account make any call.
//
// The batcher does one thing: batchTransfer(recipients, amounts) pulls the
// token from msg.sender only. The faucet Safe calls it, after approving it once,
// and every transfer comes out as Transfer(safe, recipient, amount) — the same
// log a single payout makes, so finding and claiming transfers is unchanged.

// BatcherCodeHash is the runtime code hash of SFLuvBatcher built for the Celo
// SFLUV token with the contracts repo's batcher profile (solc 0.8.28, EVM
// cancun, optimizer off). Nothing else is approved or batched through: an
// unlimited allowance to the wrong contract would let it drain the faucet.
var BatcherCodeHash = common.HexToHash("0x744ebebb726abddb041df9e34f329a33d174d5d257b1a12ad0250b7518b855e0")

const batcherABI = `[
	{"type":"function","name":"batchTransfer","stateMutability":"nonpayable",
	 "inputs":[{"name":"recipients","type":"address[]"},{"name":"amounts","type":"uint256[]"}],"outputs":[]},
	{"type":"function","name":"token","stateMutability":"view",
	 "inputs":[],"outputs":[{"name":"","type":"address"}]}
]`

const allowanceABI = `[
	{"type":"function","name":"approve","stateMutability":"nonpayable",
	 "inputs":[{"name":"spender","type":"address"},{"name":"amount","type":"uint256"}],
	 "outputs":[{"name":"","type":"bool"}]},
	{"type":"function","name":"allowance","stateMutability":"view",
	 "inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],
	 "outputs":[{"name":"","type":"uint256"}]}
]`

const paymasterValidationABI = `[{
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
	}, {"name": "userOpHash", "type": "bytes32"}, {"name": "maxCost", "type": "uint256"}],
	"name": "validatePaymasterUserOp",
	"outputs": [{"name": "context", "type": "bytes"}, {"name": "validationData", "type": "uint256"}],
	"stateMutability": "view",
	"type": "function"
}]`

// MaxAllowance is the approval the faucet Safe gives the batcher.
var MaxAllowance = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

// BatchTransferCallData is the operation's callData for paying several
// recipients from the Safe: one CALL from the Safe to the batcher.
func BatchTransferCallData(batcher common.Address, recipients []common.Address, amounts []*big.Int) ([]byte, error) {
	if len(recipients) == 0 || len(recipients) != len(amounts) {
		return nil, fmt.Errorf("a batch needs as many amounts as recipients, and at least one")
	}
	inner, err := packed(batcherABI, "batchTransfer", recipients, amounts)
	if err != nil {
		return nil, err
	}
	return SafeModuleCallData(batcher, big.NewInt(0), inner)
}

// ApproveCallData is the token call approving spender for amount.
func ApproveCallData(spender common.Address, amount *big.Int) ([]byte, error) {
	return packed(allowanceABI, "approve", spender, amount)
}

// BatcherCodeHashAt is the keccak of the code deployed at batcher, or the zero
// hash when there is none.
func (c *Client) BatcherCodeHashAt(ctx context.Context, batcher common.Address) (common.Hash, error) {
	client, err := c.dialRead(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	defer client.Close()
	code, err := retryRPC(ctx, func() ([]byte, error) { return client.CodeAt(ctx, batcher, nil) })
	if err != nil {
		return common.Hash{}, fmt.Errorf("error reading the code at %s: %w", batcher.Hex(), err)
	}
	if len(code) == 0 {
		return common.Hash{}, nil
	}
	return crypto.Keccak256Hash(code), nil
}

// BatcherToken is the token a batcher is pinned to.
func (c *Client) BatcherToken(ctx context.Context, batcher common.Address) (common.Address, error) {
	payload, err := packed(batcherABI, "token")
	if err != nil {
		return common.Address{}, err
	}
	raw, err := c.readCall(ctx, batcher, payload)
	if err != nil {
		return common.Address{}, fmt.Errorf("error reading the batcher's token: %w", err)
	}
	if len(raw) != 32 {
		return common.Address{}, fmt.Errorf("the batcher's token() returned %d bytes", len(raw))
	}
	return common.BytesToAddress(raw), nil
}

// Allowance is how much spender may move of owner's token.
func (c *Client) Allowance(ctx context.Context, token, owner, spender common.Address) (*big.Int, error) {
	payload, err := packed(allowanceABI, "allowance", owner, spender)
	if err != nil {
		return nil, err
	}
	raw, err := c.readCall(ctx, token, payload)
	if err != nil {
		return nil, fmt.Errorf("error reading the allowance: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("allowance() returned %d bytes", len(raw))
	}
	return new(big.Int).SetBytes(raw), nil
}

var (
	probeSignatureOnce sync.Once
	probeSignature     []byte
	probeSignatureErr  error
)

// PaymasterSponsors reports whether the paymaster would sponsor an operation
// from sender whose call targets target — whether target is on its whitelist.
//
// The whitelist itself is internal to the paymaster, so it is asked the way an
// operation would be: validatePaymasterUserOp is a public view, and it refuses
// a target that is not whitelisted ("AA38 contract not whitelisted") before it
// looks at the signature. A throwaway, well-formed signature gets the call past
// the signature check without reverting. A clean answer is "sponsored"; AA38 is
// "not"; anything else is returned as an error, so nothing is assumed.
func (c *Client) PaymasterSponsors(ctx context.Context, sender, target common.Address) (bool, error) {
	if c.paymaster == (common.Address{}) {
		return false, fmt.Errorf("no paymaster is configured")
	}
	probeSignatureOnce.Do(func() {
		var key *ecdsa.PrivateKey
		if key, probeSignatureErr = crypto.GenerateKey(); probeSignatureErr != nil {
			return
		}
		probeSignature, probeSignatureErr = crypto.Sign(crypto.Keccak256([]byte("sfluv paymaster probe")), key)
		if probeSignatureErr == nil {
			probeSignature[64] += 27
		}
	})
	if probeSignatureErr != nil {
		return false, probeSignatureErr
	}

	parsed, err := abi.JSON(strings.NewReader(paymasterValidationABI))
	if err != nil {
		return false, err
	}
	callData, err := SafeModuleCallData(target, big.NewInt(0), []byte{})
	if err != nil {
		return false, err
	}
	now := time.Now().Unix()
	paymasterAndData := append([]byte{}, c.paymaster.Bytes()...)
	paymasterAndData = append(paymasterAndData, common.LeftPadBytes(big.NewInt(now+600).Bytes(), 32)...)
	paymasterAndData = append(paymasterAndData, common.LeftPadBytes(big.NewInt(now-60).Bytes(), 32)...)
	paymasterAndData = append(paymasterAndData, probeSignature...)
	op := userOpTuple{
		Sender: sender, Nonce: big.NewInt(0), InitCode: []byte{}, CallData: callData,
		CallGasLimit: big.NewInt(0), VerificationGasLimit: big.NewInt(0), PreVerificationGas: big.NewInt(0),
		MaxFeePerGas: big.NewInt(0), MaxPriorityFeePerGas: big.NewInt(0),
		PaymasterAndData: paymasterAndData, Signature: probeSignature,
	}
	payload, err := parsed.Pack("validatePaymasterUserOp", op, [32]byte{}, big.NewInt(0))
	if err != nil {
		return false, err
	}

	client, err := c.dialRead(ctx)
	if err != nil {
		return false, err
	}
	defer client.Close()
	_, err = client.CallContract(ctx, ethereum.CallMsg{To: &c.paymaster, Data: payload}, nil)
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "AA38") {
		return false, nil
	}
	return false, fmt.Errorf("could not ask the paymaster about %s: %w", target.Hex(), err)
}

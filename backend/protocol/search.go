package protocol

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Searching after the fact, for payouts whose confirmation was lost.
//
// AwaitTransfers watches for one operation while it lands. These look back over
// a window that may be hours long, from a sweep, so they page through the range
// and retry a flaky node a few times rather than waiting on a deadline.

const searchRPCAttempts = 4

// retryRPC runs one read against the node, retrying the transient failures the
// public Celo endpoint is prone to.
func retryRPC[T any](ctx context.Context, read func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt < searchRPCAttempts; attempt++ {
		value, err := read()
		if err == nil {
			return value, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return zero, lastErr
}

// Head is the latest block and its timestamp.
type Head struct {
	Number uint64
	Time   time.Time
}

// ChainHead reads the head, for a sweep that needs to know how far the chain
// has got and how long ago something happened in block terms.
func (c *Client) ChainHead(ctx context.Context) (Head, error) {
	client, err := c.dialRead(ctx)
	if err != nil {
		return Head{}, err
	}
	defer client.Close()
	header, err := retryRPC(ctx, func() (*types.Header, error) { return client.HeaderByNumber(ctx, nil) })
	if err != nil {
		return Head{}, fmt.Errorf("error reading the head block: %w", err)
	}
	return Head{Number: header.Number.Uint64(), Time: time.Unix(int64(header.Time), 0)}, nil
}

// BlockAtOrBefore is the last block mined at or before t, found by binary
// search on block timestamps.
//
// Searched, not estimated from a block time. A starting point that came out
// late would miss the transfer it is looking for, and a payout reported as
// "never landed" is retried — paid a second time.
func (c *Client) BlockAtOrBefore(ctx context.Context, t time.Time) (uint64, error) {
	client, err := c.dialRead(ctx)
	if err != nil {
		return 0, err
	}
	defer client.Close()

	headerAt := func(n *big.Int) (*types.Header, error) {
		return retryRPC(ctx, func() (*types.Header, error) { return client.HeaderByNumber(ctx, n) })
	}
	head, err := headerAt(nil)
	if err != nil {
		return 0, fmt.Errorf("error reading the head block: %w", err)
	}
	target := uint64(t.Unix())
	if head.Time <= target {
		return head.Number.Uint64(), nil
	}

	lo, hi := uint64(0), head.Number.Uint64()

	// Narrow the search with a guess at about one block a second, which is
	// Celo's cadence, then confirm the guess actually brackets the moment. A
	// guess that does not is discarded and the whole chain searched; it costs
	// calls, never correctness.
	if behind := head.Time - target; behind < hi {
		guess := hi - behind
		const slack = 600
		guessLo, guessHi := uint64(0), hi
		if guess > slack {
			guessLo = guess - slack
		}
		if guess+slack < hi {
			guessHi = guess + slack
		}
		low, lowErr := headerAt(new(big.Int).SetUint64(guessLo))
		high, highErr := headerAt(new(big.Int).SetUint64(guessHi))
		if lowErr == nil && highErr == nil && low.Time <= target && high.Time > target {
			lo, hi = guessLo, guessHi-1
		}
	}

	for lo < hi {
		mid := lo + (hi-lo+1)/2
		header, err := headerAt(new(big.Int).SetUint64(mid))
		if err != nil {
			return 0, fmt.Errorf("error reading block %d: %w", mid, err)
		}
		if header.Time <= target {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo, nil
}

// TransfersFrom is every token transfer out of sender between two blocks,
// inclusive, oldest first. The range is paged to stay under the node's cap.
func (c *Client) TransfersFrom(ctx context.Context, token, sender common.Address, fromBlock, toBlock uint64) ([]types.Log, error) {
	client, err := c.dialRead(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	all := []types.Log{}
	for start := fromBlock; start <= toBlock; {
		end := start + uint64(maxLogRange) - 1
		if end > toBlock {
			end = toBlock
		}
		query := ethereum.FilterQuery{
			FromBlock: new(big.Int).SetUint64(start),
			ToBlock:   new(big.Int).SetUint64(end),
			Addresses: []common.Address{token},
			Topics:    [][]common.Hash{{transferTopic}, {common.BytesToHash(sender.Bytes())}},
		}
		logs, err := retryRPC(ctx, func() ([]types.Log, error) { return client.FilterLogs(ctx, query) })
		if err != nil {
			return nil, fmt.Errorf("error searching blocks %d-%d for transfers: %w", start, end, err)
		}
		all = append(all, logs...)
		start = end + 1
	}
	return all, nil
}

// TransferCandidate is one transfer that matches what a payout should have
// made, as far as the chain can say.
type TransferCandidate struct {
	TxHash string
	Block  uint64
	Index  uint
}

// MatchingTransfers narrows a page of transfers to one recipient and amount,
// oldest first.
func MatchingTransfers(logs []types.Log, to common.Address, amount *big.Int) []TransferCandidate {
	found := matchingTransfersTo(logs, to, amount)
	out := make([]TransferCandidate, len(found))
	for i, candidate := range found {
		out[i] = TransferCandidate{TxHash: candidate.hash, Block: candidate.block, Index: candidate.index}
	}
	return out
}

// Finding one operation after the fact.
//
// This deployment's EntryPoint emits no UserOperationEvent, and the engine
// gives every operation a nonce key of its own, chosen at random, at sequence
// zero. So the events a standard EntryPoint would be asked do not exist, and a
// later operation landing says nothing about an earlier one. What does exist is
// the nonce: getNonce(sender, key) moves past the operation's sequence in the
// block that included it, and because the key belongs to that one operation,
// it answers whether that operation ran, whatever it did when it did.

const entryPointLookupABI = `[{
	"inputs": [
		{"name": "sender", "type": "address"},
		{"name": "key", "type": "uint192"}
	],
	"name": "getNonce",
	"outputs": [{"name": "nonce", "type": "uint256"}],
	"stateMutability": "view",
	"type": "function"
}, {
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
		"name": "ops",
		"type": "tuple[]"
	}, {"name": "beneficiary", "type": "address"}],
	"name": "handleOps",
	"outputs": [],
	"stateMutability": "nonpayable",
	"type": "function"
}]`

var nonceSequenceMask = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(1))

// NonceKey and NonceSequence split a 4337 nonce into its 192-bit key and its
// 64-bit sequence.
func NonceKey(nonce *big.Int) *big.Int { return new(big.Int).Rsh(nonce, 64) }
func NonceSequence(nonce *big.Int) *big.Int {
	return new(big.Int).And(nonce, nonceSequenceMask)
}

// HeadAt is one block's number and timestamp.
func (c *Client) HeadAt(ctx context.Context, block uint64) (Head, error) {
	client, err := c.dialRead(ctx)
	if err != nil {
		return Head{}, err
	}
	defer client.Close()
	header, err := retryRPC(ctx, func() (*types.Header, error) {
		return client.HeaderByNumber(ctx, new(big.Int).SetUint64(block))
	})
	if err != nil {
		return Head{}, fmt.Errorf("error reading block %d: %w", block, err)
	}
	return Head{Number: header.Number.Uint64(), Time: time.Unix(int64(header.Time), 0)}, nil
}

// OperationIncludedAt reports whether the operation with this nonce had been
// included as of block: whether the EntryPoint's sequence for its key had
// moved past it.
func (c *Client) OperationIncludedAt(ctx context.Context, sender common.Address, nonce *big.Int, block uint64) (bool, error) {
	client, err := c.dialRead(ctx)
	if err != nil {
		return false, err
	}
	defer client.Close()
	return c.operationIncludedAt(ctx, client, sender, nonce, block)
}

func (c *Client) operationIncludedAt(ctx context.Context, client *ethclient.Client, sender common.Address, nonce *big.Int, block uint64) (bool, error) {
	parsed, err := abi.JSON(strings.NewReader(entryPointLookupABI))
	if err != nil {
		return false, fmt.Errorf("error parsing the entry point lookup abi: %w", err)
	}
	payload, err := parsed.Pack("getNonce", sender, NonceKey(nonce))
	if err != nil {
		return false, fmt.Errorf("error encoding getNonce: %w", err)
	}
	raw, err := retryRPC(ctx, func() ([]byte, error) {
		return client.CallContract(ctx, ethereum.CallMsg{To: &c.entryPoint, Data: payload}, new(big.Int).SetUint64(block))
	})
	if err != nil {
		return false, fmt.Errorf("error reading the entry point nonce at block %d: %w", block, err)
	}
	if len(raw) != 32 {
		return false, fmt.Errorf("getNonce returned %d bytes, want 32", len(raw))
	}
	current := new(big.Int).SetBytes(raw)
	if NonceKey(current).Cmp(NonceKey(nonce)) != 0 {
		return false, fmt.Errorf("getNonce answered for key %s, asked for %s", NonceKey(current), NonceKey(nonce))
	}
	return NonceSequence(current).Cmp(NonceSequence(nonce)) > 0, nil
}

// OperationInclusionBlock is the block that included the operation, found by
// binary search on its nonce between from, where it had not been included, and
// to, where it had.
func (c *Client) OperationInclusionBlock(ctx context.Context, sender common.Address, nonce *big.Int, from, to uint64) (uint64, error) {
	client, err := c.dialRead(ctx)
	if err != nil {
		return 0, err
	}
	defer client.Close()

	if from >= to {
		return 0, fmt.Errorf("empty search range %d-%d", from, to)
	}
	included, err := c.operationIncludedAt(ctx, client, sender, nonce, from)
	if err != nil {
		return 0, err
	}
	if included {
		return 0, fmt.Errorf("operation was already included at block %d, before it was sent", from)
	}
	lo, hi := from, to
	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		included, err := c.operationIncludedAt(ctx, client, sender, nonce, mid)
		if err != nil {
			return 0, err
		}
		if included {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi, nil
}

// OperationTxIn is the transaction in block that carried sender's operation
// with this nonce: a handleOps call to the EntryPoint that lists it.
//
// The block is read as raw JSON. go-ethereum refuses to decode a block holding
// a transaction type it does not know, and Celo blocks carry its fee-currency
// type.
func (c *Client) OperationTxIn(ctx context.Context, block uint64, sender common.Address, nonce *big.Int) (string, bool, error) {
	client, err := c.dialRead(ctx)
	if err != nil {
		return "", false, err
	}
	defer client.Close()

	type rawTx struct {
		Hash  string  `json:"hash"`
		To    *string `json:"to"`
		Input string  `json:"input"`
	}
	type rawBlock struct {
		Transactions []rawTx `json:"transactions"`
	}
	var got *rawBlock
	if _, err := retryRPC(ctx, func() (bool, error) {
		return true, client.Client().CallContext(ctx, &got, "eth_getBlockByNumber", fmt.Sprintf("0x%x", block), true)
	}); err != nil {
		return "", false, fmt.Errorf("error reading block %d: %w", block, err)
	}
	if got == nil {
		return "", false, fmt.Errorf("block %d is not available yet", block)
	}

	parsed, err := abi.JSON(strings.NewReader(entryPointLookupABI))
	if err != nil {
		return "", false, fmt.Errorf("error parsing the entry point lookup abi: %w", err)
	}
	handleOps := parsed.Methods["handleOps"]
	for _, tx := range got.Transactions {
		if tx.To == nil || !strings.EqualFold(*tx.To, c.entryPoint.Hex()) {
			continue
		}
		input := common.FromHex(tx.Input)
		if len(input) < 4 || !bytes.Equal(input[:4], handleOps.ID) {
			continue
		}
		values, err := handleOps.Inputs.Unpack(input[4:])
		if err != nil || len(values) == 0 {
			continue
		}
		ops, ok := abi.ConvertType(values[0], new([]userOpTuple)).(*[]userOpTuple)
		if !ok {
			continue
		}
		for _, op := range *ops {
			if op.Sender == sender && op.Nonce != nil && op.Nonce.Cmp(nonce) == 0 {
				return strings.ToLower(tx.Hash), true, nil
			}
		}
	}
	return "", false, nil
}

// ValidUntil is the end of the window the paymaster signed an operation for,
// read from its paymasterAndData: the paymaster's address, then validUntil and
// validAfter as ABI words, then the signature — the layout this paymaster
// produces. An operation not included by then never can be.
func ValidUntil(paymasterAndData []byte) (time.Time, bool) {
	if len(paymasterAndData) < 20+64 {
		return time.Time{}, false
	}
	word := new(big.Int).SetBytes(paymasterAndData[20:52])
	if !word.IsUint64() || word.Uint64() == 0 || word.Uint64() > 1<<40 {
		return time.Time{}, false
	}
	return time.Unix(int64(word.Uint64()), 0), true
}

// TransfersInReceipt is every token transfer out of sender in one transaction,
// read from its receipt. Found is false when the node has no receipt for it.
//
// A receipt, not a log search, because a receipt is all of a transaction's
// logs or nothing: a node that answers with one cannot have left a transfer
// out, which is what lets an empty answer be read as "nothing was sent".
func (c *Client) TransfersInReceipt(ctx context.Context, txHash string, token, sender common.Address) (logs []types.Log, found bool, err error) {
	client, err := c.dialRead(ctx)
	if err != nil {
		return nil, false, err
	}
	defer client.Close()

	receipt, err := retryRPC(ctx, func() (*types.Receipt, error) {
		receipt, err := client.TransactionReceipt(ctx, common.HexToHash(txHash))
		if errors.Is(err, ethereum.NotFound) {
			return nil, nil
		}
		return receipt, err
	})
	if err != nil {
		return nil, false, fmt.Errorf("error reading the receipt of %s: %w", txHash, err)
	}
	if receipt == nil {
		return nil, false, nil
	}

	from := common.BytesToHash(sender.Bytes())
	out := []types.Log{}
	for _, entry := range receipt.Logs {
		if entry == nil || entry.Address != token || len(entry.Topics) < 3 {
			continue
		}
		if entry.Topics[0] != transferTopic || entry.Topics[1] != from {
			continue
		}
		out = append(out, *entry)
	}
	return out, true, nil
}

func (c *Client) dialRead(ctx context.Context) (*ethclient.Client, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(dialCtx, c.readRPC)
	if err != nil {
		return nil, fmt.Errorf("error dialling the read rpc: %w", err)
	}
	return client, nil
}

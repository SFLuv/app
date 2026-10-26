package protocol

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
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

func (c *Client) dialRead(ctx context.Context) (*ethclient.Client, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(dialCtx, c.readRPC)
	if err != nil {
		return nil, fmt.Errorf("error dialling the read rpc: %w", err)
	}
	return client, nil
}

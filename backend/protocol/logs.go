package protocol

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// transferTopic is the ERC-20 Transfer event's signature hash.
var transferTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

// How long to wait for a sponsored operation to land, and how often to look.
//
// The bundler batches, so an operation is not mined the instant it is accepted.
// A minute is generous for a chain with seconds-long blocks and still bounded,
// because the caller is a payout that must either report a hash or report a
// failure rather than hanging.
const (
	transferWaitTimeout  = 90 * time.Second
	transferPollInterval = 3 * time.Second
	// Celo's public RPC refuses ranges wider than this, and other providers
	// have their own caps. The search only ever spans the blocks since
	// submission, so this is a guard against a stale starting point rather
	// than a limit that is normally reached.
	maxLogRange = 4_000
)

// BlockNumber is the current head, recorded before submitting an operation so
// the search for its effect cannot reach back past it.
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(dialCtx, c.readRPC)
	if err != nil {
		return 0, fmt.Errorf("error dialling the read rpc: %w", err)
	}
	defer client.Close()
	return client.BlockNumber(ctx)
}

// TransferClaim records a candidate transfer as belonging to this payout and
// reports whether the claim succeeded. False means another payout already owns
// it, and the search carries on to the next candidate.
//
// The log index is part of what is claimed, not decoration: batched payouts
// share one transaction, so the hash alone does not identify one payout's
// money.
type TransferClaim func(txHash string, blockNumber uint64, logIndex uint) (bool, error)

// AwaitTransfer waits for the token transfer a sponsored operation was supposed
// to produce, and returns the hash of the transaction that carried it.
//
// Why a log search and not a receipt lookup: the engine answers with a
// user-operation hash, and the CommunityModule that executes these operations
// emits no event tying that hash to a transaction. The transfer itself is the
// only on-chain trace, so it is what gets matched — on sender, recipient and
// exact amount, in a block no earlier than the one before submission.
//
// Matching on those three alone is not enough, which is the thing worth being
// careful about here. Identical payouts are ordinary: three rewards of the same
// size to the same wallet inside a minute is a normal evening. Without an
// exclusive claim, two of them record the same transaction, and a payout that
// actually failed takes credit for one that worked — underpaying somebody with
// nothing to show that it happened.
//
// So every candidate is offered to claim in the order it landed, and the first
// one that is still unowned wins. A payout with no unclaimed transfer to take
// waits, and then reports that it could not find its own — which is the honest
// answer and leaves the ledger retryable.
func (c *Client) AwaitTransfer(
	ctx context.Context,
	token, from, to common.Address,
	amount *big.Int,
	fromBlock uint64,
	claim TransferClaim,
) (string, error) {
	if amount == nil {
		return "", fmt.Errorf("no amount to match a transfer against")
	}

	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(dialCtx, c.readRPC)
	if err != nil {
		return "", fmt.Errorf("error dialling the read rpc: %w", err)
	}
	defer client.Close()

	query := ethereum.FilterQuery{
		Addresses: []common.Address{token},
		Topics: [][]common.Hash{
			{transferTopic},
			{common.BytesToHash(from.Bytes())},
			{common.BytesToHash(to.Bytes())},
		},
	}

	deadline := time.Now().Add(transferWaitTimeout)
	for {
		head, err := client.BlockNumber(ctx)
		if err != nil {
			return "", fmt.Errorf("error reading the head block: %w", err)
		}
		start := fromBlock
		if head > uint64(maxLogRange) && start < head-uint64(maxLogRange) {
			start = head - uint64(maxLogRange)
		}

		query.FromBlock = new(big.Int).SetUint64(start)
		query.ToBlock = new(big.Int).SetUint64(head)

		logs, err := client.FilterLogs(ctx, query)
		if err != nil {
			return "", fmt.Errorf("error searching for the transfer this operation should have made: %w", err)
		}
		for _, candidate := range matchingTransfers(logs, amount) {
			if claim == nil {
				return candidate.hash, nil
			}
			claimed, err := claim(candidate.hash, candidate.block, candidate.index)
			if err != nil {
				return "", err
			}
			if claimed {
				return candidate.hash, nil
			}
		}

		if !time.Now().Before(deadline) {
			return "", fmt.Errorf(
				"no unclaimed transfer of the expected amount from %s to %s appeared within %s",
				from.Hex(), to.Hex(), transferWaitTimeout,
			)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(transferPollInterval):
		}
	}
}

type transferCandidate struct {
	hash  string
	block uint64
	index uint
}

// matchingTransfers is every log of exactly the right amount, oldest first.
//
// Oldest first so that when several payouts are chasing identical transfers,
// each takes the earliest one nobody owns and they end up matched in the order
// they were made. Taking the newest instead would have the last payout claim
// the first transfer and leave the ordering scrambled.
func matchingTransfers(logs []types.Log, amount *big.Int) []transferCandidate {
	candidates := []transferCandidate{}
	for i := range logs {
		entry := logs[i]
		if entry.Removed || len(entry.Topics) < 3 {
			continue
		}
		if new(big.Int).SetBytes(entry.Data).Cmp(amount) != 0 {
			continue
		}
		candidates = append(candidates, transferCandidate{
			hash:  entry.TxHash.Hex(),
			block: entry.BlockNumber,
			index: entry.Index,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].block != candidates[j].block {
			return candidates[i].block < candidates[j].block
		}
		return candidates[i].index < candidates[j].index
	})
	return candidates
}

// NativeBalance reads a CELO balance, for the cutover's sweep.
func (c *Client) NativeBalance(ctx context.Context, holder common.Address) (*big.Int, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(dialCtx, c.readRPC)
	if err != nil {
		return nil, fmt.Errorf("error dialling the read rpc: %w", err)
	}
	defer client.Close()
	return client.BalanceAt(ctx, holder, nil)
}

// ReadClient dials the full node for callers that need more than this package
// wraps — the cutover signs ordinary transactions with the old EOA keys, which
// needs a nonce, a gas price and a sender.
func (c *Client) ReadClient(ctx context.Context) (*ethclient.Client, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(dialCtx, c.readRPC)
	if err != nil {
		return nil, fmt.Errorf("error dialling the read rpc: %w", err)
	}
	return client, nil
}

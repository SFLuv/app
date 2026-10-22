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

// ExpectedTransfer is one payout an operation was supposed to make.
type ExpectedTransfer struct {
	To     common.Address
	Amount *big.Int
}

// BatchClaim is TransferClaim for the i-th payout of a batch.
type BatchClaim func(i int, txHash string, blockNumber uint64, logIndex uint) (bool, error)

// AwaitTransfers waits for every transfer one operation was supposed to make,
// under one shared deadline, and returns each payout's transaction hash or the
// reason it was not found.
//
// One search for the whole batch, not one per payout. Waiting payout by payout
// gave each its own 90 seconds, so an operation that never landed held the
// faucet queue for the batch's whole budget — minutes — while every redemption
// behind it timed out.
//
// An operation is one transaction, and a batch through MultiSendCallOnly is
// all-or-nothing: either every transfer in it happened, in that transaction, or
// none did. So the moment any payout of the batch is found, the operation's
// transaction is known, and a payout whose transfer is not in that transaction
// never will be. Those fail at once rather than waiting out the deadline.
func (c *Client) AwaitTransfers(
	ctx context.Context,
	token, from common.Address,
	expected []ExpectedTransfer,
	fromBlock uint64,
	claim BatchClaim,
) ([]string, []error) {
	hashes := make([]string, len(expected))
	errs := make([]error, len(expected))
	fail := func(err error) ([]string, []error) {
		for i := range expected {
			if hashes[i] == "" && errs[i] == nil {
				errs[i] = err
			}
		}
		return hashes, errs
	}

	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(dialCtx, c.readRPC)
	if err != nil {
		return fail(fmt.Errorf("error dialling the read rpc: %w", err))
	}
	defer client.Close()

	// Every transfer out of the sender, whoever it went to: one query covers
	// the whole batch.
	query := ethereum.FilterQuery{
		Addresses: []common.Address{token},
		Topics: [][]common.Hash{
			{transferTopic},
			{common.BytesToHash(from.Bytes())},
		},
	}

	opTx := ""
	deadline := time.Now().Add(transferWaitTimeout)
	for {
		head, err := client.BlockNumber(ctx)
		if err != nil {
			return fail(fmt.Errorf("error reading the head block: %w", err))
		}
		start := fromBlock
		if head > uint64(maxLogRange) && start < head-uint64(maxLogRange) {
			start = head - uint64(maxLogRange)
		}
		query.FromBlock = new(big.Int).SetUint64(start)
		query.ToBlock = new(big.Int).SetUint64(head)

		logs, err := client.FilterLogs(ctx, query)
		if err != nil {
			return fail(fmt.Errorf("error searching for the transfers this operation should have made: %w", err))
		}

		pending, err := claimBatch(logs, expected, hashes, errs, &opTx, claim)
		if err != nil {
			return fail(err)
		}
		if pending == 0 {
			return hashes, errs
		}

		if !time.Now().Before(deadline) {
			return fail(fmt.Errorf(
				"no unclaimed transfer of the expected amount from %s appeared within %s",
				from.Hex(), transferWaitTimeout,
			))
		}
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case <-time.After(transferPollInterval):
		}
	}
}

// claimBatch matches one poll's logs against the payouts still unresolved and
// reports how many remain. Separate from the polling so the matching can be
// checked without a chain.
//
// A payout may only take a transfer from the operation's own transaction once
// that is known, and once it is known, a payout with nothing in it fails.
func claimBatch(
	logs []types.Log,
	expected []ExpectedTransfer,
	hashes []string,
	errs []error,
	opTx *string,
	claim BatchClaim,
) (int, error) {
	for i, want := range expected {
		if hashes[i] != "" || errs[i] != nil || want.Amount == nil {
			if want.Amount == nil && errs[i] == nil {
				errs[i] = fmt.Errorf("no amount to match a transfer against")
			}
			continue
		}
		for _, candidate := range matchingTransfersTo(logs, want.To, want.Amount) {
			if *opTx != "" && candidate.hash != *opTx {
				continue
			}
			claimed := true
			if claim != nil {
				var err error
				if claimed, err = claim(i, candidate.hash, candidate.block, candidate.index); err != nil {
					return 0, err
				}
			}
			if claimed {
				hashes[i] = candidate.hash
				if *opTx == "" {
					*opTx = candidate.hash
				}
				break
			}
		}
	}

	pending := 0
	for i := range expected {
		if hashes[i] != "" || errs[i] != nil {
			continue
		}
		if *opTx != "" {
			// The operation landed and this payout is not in it. All-or-nothing
			// means it never will be.
			errs[i] = fmt.Errorf("operation landed in %s but this payout's transfer is not in it", *opTx)
			continue
		}
		pending++
	}
	return pending, nil
}

// matchingTransfersTo is matchingTransfers restricted to one recipient.
func matchingTransfersTo(logs []types.Log, to common.Address, amount *big.Int) []transferCandidate {
	recipient := common.BytesToHash(to.Bytes())
	filtered := make([]types.Log, 0, len(logs))
	for _, entry := range logs {
		if len(entry.Topics) >= 3 && entry.Topics[2] == recipient {
			filtered = append(filtered, entry)
		}
	}
	return matchingTransfers(filtered, amount)
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

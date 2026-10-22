package protocol

import (
	"context"
	"fmt"
	"math/big"
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

// AwaitTransfer waits for the token transfer a sponsored operation was supposed
// to produce, and returns the hash of the transaction that carried it.
//
// Why a log search and not a receipt lookup: the engine answers with a
// user-operation hash, and the CommunityModule that executes these operations
// emits no event tying that hash to a transaction. The transfer itself is the
// only on-chain trace, so it is what gets matched — on sender, recipient and
// exact amount, in a block no earlier than the one before submission.
//
// The one case this cannot distinguish is two identical transfers — same
// sender, same recipient, same amount — landing in the same window. The payout
// service upstream is idempotent per payout, so a true duplicate is already
// prevented there; if one ever happened, the hash returned would belong to the
// other transfer of the identical pair, which carries the same money to the
// same place.
func (c *Client) AwaitTransfer(
	ctx context.Context,
	token, from, to common.Address,
	amount *big.Int,
	fromBlock uint64,
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
		if hash, ok := matchTransferAmount(logs, amount); ok {
			return hash, nil
		}

		if !time.Now().Before(deadline) {
			return "", fmt.Errorf(
				"no transfer of the expected amount from %s to %s appeared within %s",
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

// matchTransferAmount picks the newest log whose amount is exactly right.
//
// Newest rather than first: a retried payout that eventually produced two
// transfers should report the one that just happened, and an older identical
// transfer is the thing the block floor already excludes in all but the
// pathological case.
func matchTransferAmount(logs []types.Log, amount *big.Int) (string, bool) {
	var best *types.Log
	for i := range logs {
		entry := logs[i]
		if entry.Removed || len(entry.Topics) < 3 {
			continue
		}
		if new(big.Int).SetBytes(entry.Data).Cmp(amount) != 0 {
			continue
		}
		if best == nil || entry.BlockNumber > best.BlockNumber ||
			(entry.BlockNumber == best.BlockNumber && entry.Index > best.Index) {
			best = &logs[i]
		}
	}
	if best == nil {
		return "", false
	}
	return best.TxHash.Hex(), true
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

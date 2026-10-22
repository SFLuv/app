package bot

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SFLuv/app/backend/abi"
	"github.com/SFLuv/app/backend/clientconfig"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type IBot interface {
	Key() string
	Send(amount uint64, address string) error
	SubmitTransfer(amount uint64, address string) (string, error)
	SubmitTransferBaseUnits(amount *big.Int, address string) (string, error)
	VerifyTransfer(ctx context.Context, txHash string, address string, amount uint64) (*TransferVerificationResult, error)
	VerifyTransferBaseUnits(ctx context.Context, txHash string, address string, amount *big.Int) (*TransferVerificationResult, error)
	// VerifyTransferOnChain checks a payout against the chain it was recorded
	// on, for payouts that predate a chain migration.
	VerifyTransferOnChain(ctx context.Context, chainID int64, txHash string, address string, amount uint64) (*TransferVerificationResult, error)
	Balance() (*big.Int, error)
}

type Bot struct {
	pKey          string
	tokenId       string
	tokenDecimals int
	client        *ethclient.Client

	// senders answers "which addresses were the faucet at this moment on this
	// chain". Injected rather than queried, so this package keeps knowing
	// nothing about the database.
	//
	// Nil is the pre-registry behaviour: the expected sender is derived from
	// the key in hand, which is correct right up until the key changes.
	senders SenderResolver

	// txSenders answers the same question as senders, by transaction hash
	// instead of by time. Consulted first.
	txSenders TxSenderResolver

	// routing is where the faucet's money is and how to spend it.
	//
	// Swapped atomically because it is written from the boot goroutine that
	// brings the Safe up and read from every request goroutine that pays
	// anybody. Two plain fields were a data race, and the kind that would show
	// up as an impossible nil rather than as anything legible.
	routing atomic.Pointer[faucetRouting]
}

// faucetRouting is one coherent answer to "where does the faucet pay from".
//
// The three states are deliberate, and the middle one is the one that matters:
//
//   - cutover false: the EOA pays, which is how this worked before Safes.
//   - cutover true, spender nil: the Safe is the faucet but is not attached
//     yet. Paying is refused rather than falling back, because the fallback
//     would sign with a key that is presumed leaked and draw on an account
//     that was emptied into the Safe. Refusal is retryable; a payout from an
//     empty wallet is a failure that looks like success.
//   - cutover true, spender set: the Safe pays.
type faucetRouting struct {
	spender   Spender
	balanceOf BalanceReader
	cutover   bool
}

func (b *Bot) faucet() faucetRouting {
	if b == nil {
		return faucetRouting{}
	}
	if current := b.routing.Load(); current != nil {
		return *current
	}
	return faucetRouting{}
}

// ErrFaucetSafeNotAttached means the Safe is the faucet but is not usable yet.
//
// Separate from a send failure because nothing was sent: whatever asked to be
// paid may be retried as soon as the Safe attaches, and a redemption code that
// hit this must be released rather than consumed.
var ErrFaucetSafeNotAttached = errors.New("faucet Safe is not attached yet")

// SenderResolver resolves the faucet's addresses as of a moment. A set, not one
// address, because a cutover is a boundary rather than an instant and both
// sides of it are legitimate senders for an item that straddles it.
type SenderResolver func(ctx context.Context, chainID int64, at time.Time) ([]common.Address, error)

// SetSenderResolver installs the historical lookup. Called once at wiring time.
func (b *Bot) SetSenderResolver(resolver SenderResolver) {
	if b == nil {
		return
	}
	b.senders = resolver
}

// TxSenderResolver answers "which of our addresses sent this transaction",
// looked up by hash.
//
// Preferred over the timestamp lookup wherever it answers, because it is a
// record of what happened rather than a deduction from when it happened. The
// window arithmetic stays for everything sent before that record existed.
type TxSenderResolver func(ctx context.Context, txHash string) (common.Address, bool)

// SetTxSenderResolver installs the by-hash lookup.
func (b *Bot) SetTxSenderResolver(resolver TxSenderResolver) {
	if b == nil {
		return
	}
	b.txSenders = resolver
}

// Spender sends tokens from wherever the faucet currently lives, returning the
// hash of the transaction that carried them.
//
// A transaction hash, not an operation hash: everything downstream — the payout
// ledger, the reconciler, the verifier — treats this value as something it can
// look up as a receipt, and handing back something that is not one would break
// all three in ways that look like missing payouts.
type Spender func(ctx context.Context, amountBase *big.Int, to common.Address) (string, error)

// BalanceReader reads the faucet's spendable balance from its current home.
type BalanceReader func(ctx context.Context) (*big.Int, error)

// SetFaucet routes sends and balance reads through the faucet Safe.
//
// Installed once the Safe is deployed, signed for and funded. Before the
// cutover this is never called and the package does exactly what it did before
// Safes existed, which is what makes this safe to deploy ahead of running.
func (b *Bot) SetFaucet(spender Spender, balance BalanceReader) {
	if b == nil {
		return
	}
	next := b.faucet()
	next.spender = spender
	next.balanceOf = balance
	b.routing.Store(&next)
}

// SetFaucetCutover records that the Safe is the faucet, whether or not it is
// attached yet.
//
// Set synchronously at wiring time, from the database, before the server
// answers anything. Doing it from the boot goroutine instead would leave a few
// seconds of every deploy in which the cutover had happened but this did not
// know it — and payouts in that window would be signed with the old key
// against an empty account.
func (b *Bot) SetFaucetCutover(done bool) {
	if b == nil {
		return
	}
	next := b.faucet()
	next.cutover = done
	b.routing.Store(&next)
}

type SendError struct {
	err              error
	revertRedemption bool
}

type TransferVerificationResult struct {
	Found      bool
	Pending    bool
	Successful bool
}

func (e *SendError) Error() string {
	return e.err.Error()
}

func (e *SendError) Unwrap() error {
	return e.err
}

func newSendError(err error, revertRedemption bool) error {
	if err == nil {
		return nil
	}
	return &SendError{
		err:              err,
		revertRedemption: revertRedemption,
	}
}

func ShouldRevertRedemption(err error) bool {
	var sendErr *SendError
	if errors.As(err, &sendErr) {
		return sendErr.revertRedemption
	}
	return true
}

func Init(config *clientconfig.Config) (*Bot, error) {
	if config == nil {
		return nil, fmt.Errorf("client config is required")
	}
	token, err := config.PrimaryToken()
	if err != nil {
		return nil, err
	}

	pkey := os.Getenv("BOT_KEY")
	tokenId := token.Address
	rpcUrl := config.ReadRPCURL()
	if strings.TrimSpace(rpcUrl) == "" {
		return nil, fmt.Errorf("read RPC URL is missing from client config")
	}

	client, err := ethclient.Dial(rpcUrl)
	if err != nil {
		err = fmt.Errorf("error initializing eth client: %s", err)
		return nil, err
	}

	return &Bot{pKey: pkey, tokenId: tokenId, tokenDecimals: token.Decimals, client: client}, nil
}

func (b *Bot) Key() string {
	// get bot's public key and return it
	return ""
}

func (b *Bot) tokenAmountFromWholeUnits(amount uint64) (*big.Int, error) {
	if b == nil || b.tokenDecimals < 0 {
		return nil, fmt.Errorf("invalid token decimals")
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(b.tokenDecimals)), nil)
	return new(big.Int).Mul(scale, big.NewInt(int64(amount))), nil
}

func (b *Bot) deriveFromAddress() (common.Address, error) {
	privateKey, err := crypto.HexToECDSA(b.pKey)
	if err != nil {
		return common.Address{}, fmt.Errorf("error parsing private key: %s", err)
	}
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return common.Address{}, fmt.Errorf("error asserting type: publicKey is not of type ecdsa.PublicKey")
	}
	return crypto.PubkeyToAddress(*publicKeyECDSA), nil
}

func (b *Bot) submitTransferTx(amount uint64, address string) (*types.Transaction, error) {
	tokenAmount, err := b.tokenAmountFromWholeUnits(amount)
	if err != nil {
		return nil, err
	}
	return b.submitTransferTxBaseUnits(tokenAmount, address)
}

// submitTransferTxBaseUnits transfers an exact base-unit token amount without
// whole-unit scaling. Used for migration recovery payouts, which must pay a
// holder's stored fractional balance precisely.
func (b *Bot) submitTransferTxBaseUnits(tokenAmount *big.Int, address string) (*types.Transaction, error) {
	if tokenAmount == nil || tokenAmount.Sign() <= 0 {
		return nil, fmt.Errorf("transfer amount must be positive")
	}
	if !common.IsHexAddress(address) {
		return nil, fmt.Errorf("invalid recipient address: %s", address)
	}
	toAddress := common.HexToAddress(address)
	tokenAddress := common.HexToAddress(b.tokenId)
	contract, err := abi.NewSFLUVv2(tokenAddress, b.client)
	if err != nil {
		return nil, newSendError(fmt.Errorf("error creating sfluv contract instance: %s", err), true)
	}

	privateKey, err := crypto.HexToECDSA(b.pKey)
	if err != nil {
		return nil, newSendError(fmt.Errorf("error parsing private key: %s", err), true)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, newSendError(fmt.Errorf("error asserting type: publicKey is not of type ecdsa.PublicKey"), true)
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	contractABI, err := abi.SFLUVv2MetaData.GetAbi()
	if err != nil {
		return nil, newSendError(fmt.Errorf("error loading sfluv contract abi: %s", err), true)
	}

	callData, err := contractABI.Pack("transfer", toAddress, tokenAmount)
	if err != nil {
		return nil, newSendError(fmt.Errorf("error packing transfer call data: %s", err), true)
	}

	simCtx, simCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer simCancel()
	simResult, err := b.client.CallContract(simCtx, ethereum.CallMsg{
		From: fromAddress,
		To:   &tokenAddress,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, newSendError(fmt.Errorf("transfer simulation failed: %s", err), true)
	}

	decoded, err := contractABI.Unpack("transfer", simResult)
	if err != nil {
		return nil, newSendError(fmt.Errorf("error decoding transfer simulation result: %s", err), true)
	}
	if len(decoded) > 0 {
		if ok, cast := decoded[0].(bool); cast && !ok {
			return nil, newSendError(fmt.Errorf("transfer simulation failed: contract returned false"), true)
		}
	}

	chainId, err := b.client.ChainID(context.Background())
	if err != nil {
		return nil, newSendError(fmt.Errorf("error getting chainId from rpc node: %s", err), true)
	}

	s, err := bind.NewKeyedTransactorWithChainID(privateKey, chainId)
	if err != nil {
		return nil, newSendError(fmt.Errorf("error creating transactor: %s", err), true)
	}
	opts := &bind.TransactOpts{
		From:    fromAddress,
		Signer:  s.Signer,
		Context: context.Background(),
	}

	tx, err := contract.Transfer(opts, toAddress, tokenAmount)
	if err != nil {
		return nil, newSendError(fmt.Errorf("error sending transfer transaction: %s", err), true)
	}

	return tx, nil
}

func (b *Bot) SubmitTransfer(amount uint64, address string) (string, error) {
	tokenAmount, err := b.tokenAmountFromWholeUnits(amount)
	if err != nil {
		return "", err
	}
	return b.SubmitTransferBaseUnits(tokenAmount, address)
}

// SubmitTransferBaseUnits sends an exact base-unit amount and returns the tx
// hash. Used by the migration recovery flow to pay precise stored balances.
func (b *Bot) SubmitTransferBaseUnits(amount *big.Int, address string) (string, error) {
	routing := b.faucet()

	// The Safe path when the faucet has moved, the key path when it has not,
	// and a refusal in between. All three answer the same way to a caller that
	// succeeds, so nothing downstream can tell which ran.
	if routing.spender != nil {
		if amount == nil || amount.Sign() <= 0 {
			return "", fmt.Errorf("transfer amount must be positive")
		}
		if !common.IsHexAddress(address) {
			return "", fmt.Errorf("invalid recipient address: %s", address)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		hash, err := routing.spender(ctx, amount, common.HexToAddress(address))
		if err != nil {
			// Whether the redemption may be released depends on whether
			// anything was actually submitted, which only the spender knows.
			// A send that never went out must release the code so it can be
			// presented again; one that may yet land must not, or the same
			// reward is claimable twice.
			return "", newSendError(err, ShouldReleaseOnFailure(err))
		}
		return hash, nil
	}

	if routing.cutover {
		// The Safe is the faucet and is not attached yet. Signing with the old
		// key would draw on an account with nothing in it, and that key is
		// presumed to be in someone else's hands. Released, because nothing
		// was sent.
		return "", newSendError(ErrFaucetSafeNotAttached, true)
	}

	tx, err := b.submitTransferTxBaseUnits(amount, address)
	if err != nil {
		return "", err
	}
	return tx.Hash().Hex(), nil
}

func (b *Bot) VerifyTransfer(ctx context.Context, txHash string, address string, amount uint64) (*TransferVerificationResult, error) {
	tokenAmount, err := b.tokenAmountFromWholeUnits(amount)
	if err != nil {
		return nil, err
	}
	return b.verifyTransferReceipt(ctx, txHash, address, tokenAmount)
}

// VerifyTransferBaseUnits verifies a transfer of an exact base-unit amount. Used
// by recovery reconciliation, whose payouts are not whole-token amounts.
func (b *Bot) VerifyTransferBaseUnits(ctx context.Context, txHash string, address string, amount *big.Int) (*TransferVerificationResult, error) {
	if amount == nil {
		return nil, fmt.Errorf("amount is required")
	}
	return b.verifyTransferReceipt(ctx, txHash, address, amount)
}

func (b *Bot) verifyTransferReceipt(ctx context.Context, txHash string, address string, tokenAmount *big.Int) (*TransferVerificationResult, error) {
	return b.verifyTransferReceiptOn(ctx, b.client, b.tokenId, txHash, address, tokenAmount)
}

// verifyTransferReceiptOn is the receipt check bound to a specific chain's
// client and token, so a payout recorded before a chain migration can still be
// verified on the chain it actually happened on.
func (b *Bot) verifyTransferReceiptOn(
	ctx context.Context,
	client *ethclient.Client,
	tokenID string,
	txHash string,
	address string,
	tokenAmount *big.Int,
) (*TransferVerificationResult, error) {
	result := &TransferVerificationResult{}
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return result, nil
	}
	if !common.IsHexAddress(address) {
		return nil, fmt.Errorf("invalid recipient address: %s", address)
	}
	if client == nil {
		return nil, fmt.Errorf("no client for the chain this payout was recorded on")
	}

	hash := common.HexToHash(txHash)
	receipt, err := client.TransactionReceipt(ctx, hash)
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			tx, isPending, txErr := client.TransactionByHash(ctx, hash)
			if txErr == nil && tx != nil {
				result.Found = true
				result.Pending = isPending
				return result, nil
			}
			if txErr != nil && !errors.Is(txErr, ethereum.NotFound) {
				return nil, fmt.Errorf("error checking transfer transaction by hash %s: %w", txHash, txErr)
			}
			return result, nil
		}
		return nil, fmt.Errorf("error loading transfer receipt %s: %w", txHash, err)
	}
	if receipt == nil {
		return result, nil
	}

	result.Found = true
	if receipt.Status != types.ReceiptStatusSuccessful {
		return result, nil
	}

	// Who counts as "us" is a question about when this transfer happened, not
	// about which key is in the environment now.
	//
	// The faucet used to be an EOA whose address was the key's address, so the
	// check could derive it and compare. Once the faucet became a Safe with a
	// replaceable signer, that comparison started answering for today and
	// failing every older payout — the money had plainly gone out, and the step
	// read as unpaid forever because the sender no longer matched.
	//
	// The timestamp comes off the receipt's own block rather than from the
	// caller. It is the authoritative moment for this transfer, it needs no
	// threading through five call sites, and it cannot drift from the item the
	// way a separately recorded date can.
	acceptedSenders, err := b.sendersForReceipt(ctx, client, receipt)
	if err != nil {
		return nil, err
	}
	tokenAddress := common.HexToAddress(tokenID)
	transferTopic := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	toAddress := common.HexToAddress(address)

	for _, lg := range receipt.Logs {
		if lg == nil {
			continue
		}
		if lg.Address != tokenAddress || len(lg.Topics) < 3 || lg.Topics[0] != transferTopic {
			continue
		}

		logFrom := common.BytesToAddress(lg.Topics[1].Bytes()[12:])
		logTo := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
		logAmount := new(big.Int).SetBytes(lg.Data)
		if acceptedSenders[logFrom] && logTo == toAddress && logAmount.Cmp(tokenAmount) == 0 {
			result.Successful = true
			return result, nil
		}
	}

	return result, nil
}

// send {amount} tokens to {address}
func (b *Bot) Send(amount uint64, address string) error {
	// On the Safe path there is nothing to wait for here: the spender does not
	// return until it has seen the transfer land, so a submitted hash is
	// already a mined one. Waiting again would mean waiting for a receipt we
	// have in effect already read.
	if routing := b.faucet(); routing.spender != nil || routing.cutover {
		_, err := b.SubmitTransfer(amount, address)
		return err
	}

	tx, err := b.submitTransferTx(amount, address)
	if err != nil {
		return err
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer waitCancel()
	receipt, err := bind.WaitMined(waitCtx, b.client, tx)
	if err != nil {
		return newSendError(fmt.Errorf("error waiting for transfer tx %s: %w", tx.Hash().Hex(), err), false)
	}
	if receipt == nil {
		return newSendError(fmt.Errorf("missing receipt for transfer tx %s", tx.Hash().Hex()), false)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return newSendError(fmt.Errorf("transfer transaction reverted: %s", tx.Hash().Hex()), true)
	}

	fmt.Printf("Sent Transaction: %s\n", tx.Hash().Hex())
	return nil
}

// Draining the faucet lives on BotService, not here.
//
// This used to be a method on the bot that built a transactor from BOT_KEY and
// read BOT_ADDRESS's balance. Both are the wrong account after the cutover: the
// balance is in the Safe and the key is one we intend to delete. BotService's
// Drain resolves the destination from the registry and sends through the
// ordinary transfer path, which routes to the Safe or the EOA depending on
// where the faucet actually is — so there is nothing left for a key-signing
// drain to do.

func (b *Bot) Balance() (*big.Int, error) {
	routing := b.faucet()
	if routing.balanceOf != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return routing.balanceOf(ctx)
	}
	if routing.cutover {
		// Reading the old address here would report the balance of an account
		// that was emptied into the Safe — zero — and a faucet reporting zero
		// refuses every payout and blocks workflow approvals for want of
		// funds. An error says "ask again", which is true.
		return nil, ErrFaucetSafeNotAttached
	}

	tokenAddress := common.HexToAddress(b.tokenId)

	contract, err := abi.NewSFLUVv2(tokenAddress, b.client)
	if err != nil {
		return nil, fmt.Errorf("error creating sfluv contract instance to get balance: %s", err)
	}

	return contract.BalanceOf(nil, common.HexToAddress(os.Getenv("BOT_ADDRESS")))
}

// --- verifying payouts made on a chain we have since left ---------------------

// Payouts recorded before the Celo migration carry a Berachain transaction hash.
// The reconciler used to give up on any payout whose chain differed from the
// active one, so those steps could never settle: the money had gone out, the
// workflow read as unpaid, and the series it belonged to stayed blocked.
//
// Verifying them needs the chain they happened on — its RPC and its token
// address, which is a different contract from today's. Both are read from the
// environment per chain id, so this stays opt-in: with nothing configured the
// behaviour is exactly what it was, and nothing is ever marked paid on trust.
//
//	WORKFLOW_PAYOUT_RPC_80094=https://rpc.berachain.com
//	WORKFLOW_PAYOUT_TOKEN_80094=0x881cad4f885c6701d8481c0ed347f6d35444ea7e

var (
	legacyChainMu      sync.Mutex
	legacyChainClients = map[int64]*ethclient.Client{}
)

// ErrNoLegacyChainConfig means we cannot check this payout, which is different
// from checking it and finding nothing.
var ErrNoLegacyChainConfig = errors.New("no RPC or token configured for that chain")

func legacyChainSettings(chainID int64) (rpcURL string, tokenID string, ok bool) {
	suffix := strconv.FormatInt(chainID, 10)
	rpcURL = strings.TrimSpace(os.Getenv("WORKFLOW_PAYOUT_RPC_" + suffix))
	tokenID = strings.TrimSpace(os.Getenv("WORKFLOW_PAYOUT_TOKEN_" + suffix))
	if rpcURL == "" || !common.IsHexAddress(tokenID) {
		return "", "", false
	}
	return rpcURL, tokenID, true
}

// legacyChainClient dials a past chain once and keeps the connection, since the
// reconciliation sweep walks a batch of payouts at a time.
func legacyChainClient(rpcURL string, chainID int64) (*ethclient.Client, error) {
	legacyChainMu.Lock()
	defer legacyChainMu.Unlock()
	if existing, ok := legacyChainClients[chainID]; ok && existing != nil {
		return existing, nil
	}
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("error connecting to chain %d: %w", chainID, err)
	}
	legacyChainClients[chainID] = client
	return client, nil
}

// VerifyTransferOnChain verifies a payout against the chain it was recorded on.
//
// The active chain takes the ordinary path. Any other chain needs its own RPC
// and token configured; without them this returns ErrNoLegacyChainConfig so the
// caller can say why it could not check, rather than reporting a payout as
// missing when it was simply unreachable.
func (b *Bot) VerifyTransferOnChain(
	ctx context.Context,
	chainID int64,
	txHash string,
	address string,
	amount uint64,
) (*TransferVerificationResult, error) {
	tokenAmount, err := b.tokenAmountFromWholeUnits(amount)
	if err != nil {
		return nil, err
	}

	rpcURL, tokenID, ok := legacyChainSettings(chainID)
	if !ok {
		return nil, ErrNoLegacyChainConfig
	}
	client, err := legacyChainClient(rpcURL, chainID)
	if err != nil {
		return nil, err
	}
	return b.verifyTransferReceiptOn(ctx, client, tokenID, txHash, address, tokenAmount)
}

// sendersForReceipt resolves which addresses may have sent this transfer as the
// faucet.
//
// Addresses only. This never needs a private key, and after the cutover it must
// not want one: BOT_KEY is to be deleted from the environment once the money is
// in the Safe, and a verifier that parses it to learn its own address would
// then report every payout that really went out as unverified.
//
// Three sources, best first:
//
//  1. The transaction map. We record which of our accounts caused each
//     transaction we submit, so for anything sent since that record existed
//     this is a lookup rather than a deduction — and a lookup cannot be wrong
//     about a transfer that straddles a cutover boundary.
//  2. The address windows, keyed on the block's own timestamp. This is how
//     everything older than the record gets answered.
//  3. The addresses we know of without asking anything: BOT_ADDRESS, and the
//     key's address when a key happens to still be present. Both are
//     best-effort and neither is an error when absent.
//
// An empty set is returned rather than an error when nothing can answer, which
// leaves the payout unverified — the honest outcome — instead of failing the
// whole check.
func (b *Bot) sendersForReceipt(
	ctx context.Context,
	client *ethclient.Client,
	receipt *types.Receipt,
) (map[common.Address]bool, error) {
	accepted := map[common.Address]bool{}
	add := func(addresses ...common.Address) {
		for _, address := range addresses {
			if address != (common.Address{}) {
				accepted[address] = true
			}
		}
	}

	// The addresses that need nothing but the environment, added first so they
	// are present whatever else succeeds. A payout sent moments ago by a signer
	// whose window has not been written yet is still ours.
	add(b.knownFaucetAddresses()...)

	if b == nil || receipt == nil || client == nil {
		return accepted, nil
	}

	if b.txSenders != nil {
		if sender, ok := b.txSenders(ctx, receipt.TxHash.Hex()); ok {
			add(sender)
			return accepted, nil
		}
	}

	if b.senders == nil {
		return accepted, nil
	}

	chainID, err := client.ChainID(ctx)
	if err != nil || chainID == nil {
		return accepted, nil
	}
	header, err := client.HeaderByHash(ctx, receipt.BlockHash)
	if err != nil || header == nil {
		return accepted, nil
	}

	addresses, err := b.senders(ctx, chainID.Int64(), time.Unix(int64(header.Time), 0).UTC())
	if err != nil {
		return accepted, nil
	}
	add(addresses...)
	return accepted, nil
}

// knownFaucetAddresses are the faucet addresses obtainable without a chain call
// and without a key.
//
// BOT_ADDRESS is the configured one and is the reason this works after BOT_KEY
// is deleted. The key's address is still added when a key is present, because
// before the cutover that is the account payouts actually come from and the
// configured variable is allowed to be absent — but it is never required, and a
// missing or unparseable key is not an error here.
func (b *Bot) knownFaucetAddresses() []common.Address {
	if b == nil {
		return nil
	}
	out := []common.Address{}
	if configured := strings.TrimSpace(os.Getenv("BOT_ADDRESS")); common.IsHexAddress(configured) {
		out = append(out, common.HexToAddress(configured))
	}
	if strings.TrimSpace(b.pKey) != "" {
		if derived, err := b.deriveFromAddress(); err == nil {
			out = append(out, derived)
		}
	}
	return out
}

// ReleaseOnFailure marks an error as one whose work never left this process, so
// a redemption code that hit it may be presented again.
//
// The distinction the faucet cannot make for itself: by the time an error
// reaches the bot it is either "nothing was submitted" or "something was
// submitted and may yet land", and only the sender knows which. Getting it the
// wrong way round costs either a volunteer their reward or the treasury a
// double payment, so it travels with the error rather than being guessed at.
type ReleaseOnFailure struct{ err error }

func (e *ReleaseOnFailure) Error() string { return e.err.Error() }
func (e *ReleaseOnFailure) Unwrap() error { return e.err }

// Releasable wraps an error whose operation was never submitted.
func Releasable(err error) error {
	if err == nil {
		return nil
	}
	return &ReleaseOnFailure{err: err}
}

// ShouldReleaseOnFailure reports whether this failure happened before anything
// was submitted.
//
// Defaults to false, which is the cautious direction: an unrecognised failure
// is treated as possibly-sent, so a code is held rather than handed back for a
// reward that may already be on its way.
func ShouldReleaseOnFailure(err error) bool {
	var release *ReleaseOnFailure
	return errors.As(err, &release)
}

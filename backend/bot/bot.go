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
	Drain(address common.Address) error
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

	// Set once the faucet has moved to its Safe. Both nil means the EOA path.
	spender   Spender
	balanceOf BalanceReader
}

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
// Installed only once the Safe is deployed and funded. Until then both are nil
// and this package does exactly what it did before: sign an ordinary transfer
// with the key in the environment and read the balance of that key's address.
// That fallback is what makes the cutover safe to deploy ahead of running —
// the new code with no Safe behaves like the old code.
func (b *Bot) SetFaucet(spender Spender, balance BalanceReader) {
	if b == nil {
		return
	}
	b.spender = spender
	b.balanceOf = balance
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
	// The Safe path when the faucet has moved, the key path when it has not.
	// Both answer with a transaction hash, so no caller can tell which ran.
	if b != nil && b.spender != nil {
		if amount == nil || amount.Sign() <= 0 {
			return "", fmt.Errorf("transfer amount must be positive")
		}
		if !common.IsHexAddress(address) {
			return "", fmt.Errorf("invalid recipient address: %s", address)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		hash, err := b.spender(ctx, amount, common.HexToAddress(address))
		if err != nil {
			// Marked as not worth reverting the redemption for: the operation
			// may yet land, and voiding the code would let the same reward be
			// claimed twice.
			return "", newSendError(err, false)
		}
		return hash, nil
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
	if b != nil && b.spender != nil {
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

func (b *Bot) Drain(address common.Address) error {

	tokenAddress := common.HexToAddress(b.tokenId)

	contract, err := abi.NewSFLUVv2(tokenAddress, b.client)
	if err != nil {
		return fmt.Errorf("error creating sfluv contract instance: %s", err)
	}

	amount, err := contract.BalanceOf(nil, common.HexToAddress(os.Getenv("BOT_ADDRESS")))
	if err != nil {
		return fmt.Errorf("error getting bot balance: %s", err)
	}

	chid, err := b.client.ChainID(context.Background())
	if err != nil {
		return fmt.Errorf("error getting chainId:%s", err)
	}

	privKey, err := crypto.HexToECDSA(b.pKey)
	if err != nil {
		return fmt.Errorf("error parsing private key: %s", err)
	}

	pubKey, ok := privKey.Public().(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("error parsing public key into ecdsa type")
	}

	s, err := bind.NewKeyedTransactorWithChainID(privKey, chid)
	if err != nil {
		return fmt.Errorf("error creating transactor: %s", err)
	}

	opts := &bind.TransactOpts{
		From:    crypto.PubkeyToAddress(*pubKey),
		Signer:  s.Signer,
		Context: context.Background(),
	}

	_, err = contract.Transfer(opts, address, amount)
	if err != nil {
		return fmt.Errorf("error draining faucet balance: %s", err)
	}
	// return err if err
	return nil
}

// Balance is the faucet's spendable SFLUV.
//
// Read from wherever the faucet is now, which is the whole of why this is not
// just a BalanceOf on BOT_ADDRESS any more. Once the money is in the Safe, an
// env-address read reports zero, and a faucet that reports zero refuses every
// payout and blocks every workflow approval for want of balance.
func (b *Bot) Balance() (*big.Int, error) {
	if b != nil && b.balanceOf != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return b.balanceOf(ctx)
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
// faucet, as of the block it landed in.
//
// Falls back to the key's own address whenever the registry cannot answer —
// no resolver wired, a chain it has no rows for, an unreadable block header.
// That fallback is exactly the behaviour this check had before the registry
// existed, so a lookup failure degrades to the old answer rather than marking a
// real payout missing.
func (b *Bot) sendersForReceipt(
	ctx context.Context,
	client *ethclient.Client,
	receipt *types.Receipt,
) (map[common.Address]bool, error) {
	fallback := func() (map[common.Address]bool, error) {
		derived, err := b.deriveFromAddress()
		if err != nil {
			return nil, err
		}
		return map[common.Address]bool{derived: true}, nil
	}

	if b == nil || b.senders == nil || receipt == nil || client == nil {
		return fallback()
	}

	chainID, err := client.ChainID(ctx)
	if err != nil || chainID == nil {
		return fallback()
	}
	header, err := client.HeaderByHash(ctx, receipt.BlockHash)
	if err != nil || header == nil {
		return fallback()
	}

	addresses, err := b.senders(ctx, chainID.Int64(), time.Unix(int64(header.Time), 0).UTC())
	if err != nil || len(addresses) == 0 {
		return fallback()
	}

	accepted := make(map[common.Address]bool, len(addresses)+1)
	for _, address := range addresses {
		accepted[address] = true
	}
	// The key in hand is always acceptable. A payout sent moments ago by a
	// signer whose window has not been written yet is still ours, and refusing
	// it would be the same bug in a narrower form.
	if derived, err := b.deriveFromAddress(); err == nil {
		accepted[derived] = true
	}
	return accepted, nil
}

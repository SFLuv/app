package bot

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/abi"
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
	VerifyTransfer(ctx context.Context, txHash string, address string, amount uint64) (*TransferVerificationResult, error)
	Drain(address common.Address) error
	Balance() (*big.Int, error)
}

type Bot struct {
	pKey    string
	tokenId string
	client  *ethclient.Client
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

func Init() (*Bot, error) {
	pkey := os.Getenv("BOT_KEY")
	tokenId := os.Getenv("TOKEN_ID")
	rpcUrl := os.Getenv("RPC_URL")

	client, err := ethclient.Dial(rpcUrl)
	if err != nil {
		err = fmt.Errorf("error initializing eth client: %s", err)
		return nil, err
	}

	return &Bot{pkey, tokenId, client}, nil
}

func (b *Bot) Key() string {
	// get bot's public key and return it
	return ""
}

func tokenAmountFromWholeUnits(amount uint64) (*big.Int, error) {
	decimalString := os.Getenv("TOKEN_DECIMALS")
	decimals, ok := new(big.Int).SetString(decimalString, 10)
	if !ok {
		return nil, fmt.Errorf("invalid TOKEN_DECIMALS value %s", decimalString)
	}
	return new(big.Int).Mul(decimals, big.NewInt(int64(amount))), nil
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
	if !common.IsHexAddress(address) {
		return nil, fmt.Errorf("invalid recipient address: %s", address)
	}

	tokenAmount, err := tokenAmountFromWholeUnits(amount)
	if err != nil {
		return nil, err
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
	tx, err := b.submitTransferTx(amount, address)
	if err != nil {
		return "", err
	}
	return tx.Hash().Hex(), nil
}

func (b *Bot) VerifyTransfer(ctx context.Context, txHash string, address string, amount uint64) (*TransferVerificationResult, error) {
	result := &TransferVerificationResult{}
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return result, nil
	}
	if !common.IsHexAddress(address) {
		return nil, fmt.Errorf("invalid recipient address: %s", address)
	}

	hash := common.HexToHash(txHash)
	receipt, err := b.client.TransactionReceipt(ctx, hash)
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			tx, isPending, txErr := b.client.TransactionByHash(ctx, hash)
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

	tokenAmount, err := tokenAmountFromWholeUnits(amount)
	if err != nil {
		return nil, err
	}
	fromAddress, err := b.deriveFromAddress()
	if err != nil {
		return nil, err
	}
	tokenAddress := common.HexToAddress(b.tokenId)
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
		if logFrom == fromAddress && logTo == toAddress && logAmount.Cmp(tokenAmount) == 0 {
			result.Successful = true
			return result, nil
		}
	}

	return result, nil
}

// send {amount} tokens to {address}
func (b *Bot) Send(amount uint64, address string) error {
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

func (b *Bot) Balance() (*big.Int, error) {
	tokenAddress := common.HexToAddress(b.tokenId)

	contract, err := abi.NewSFLUVv2(tokenAddress, b.client)
	if err != nil {
		return nil, fmt.Errorf("error creating sfluv contract instance to get balance: %s", err)
	}

	return contract.BalanceOf(nil, common.HexToAddress(os.Getenv("BOT_ADDRESS")))
}

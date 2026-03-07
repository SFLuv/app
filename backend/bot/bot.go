package bot

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"os"
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

// send {amount} tokens to {address}
func (b *Bot) Send(amount uint64, address string) error {
	if !common.IsHexAddress(address) {
		return fmt.Errorf("invalid recipient address: %s", address)
	}

	decimalString := os.Getenv("TOKEN_DECIMALS")
	decimals, ok := new(big.Int).SetString(decimalString, 10)
	if !ok {
		return fmt.Errorf("invalid TOKEN_DECIMALS value %s", decimalString)
	}

	tokenAmount := new(big.Int).Mul(decimals, big.NewInt(int64(amount)))
	toAddress := common.HexToAddress(address)
	tokenAddress := common.HexToAddress(b.tokenId)
	contract, err := abi.NewSFLUVv2(tokenAddress, b.client)
	if err != nil {
		return newSendError(fmt.Errorf("error creating sfluv contract instance: %s", err), true)
	}

	privateKey, err := crypto.HexToECDSA(b.pKey)
	if err != nil {
		err = fmt.Errorf("error parsing private key: %s", err)
		return newSendError(err, true)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		err = fmt.Errorf("error asserting type: publicKey is not of type ecdsa.PublicKey")
		return newSendError(err, true)
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	contractABI, err := abi.SFLUVv2MetaData.GetAbi()
	if err != nil {
		return newSendError(fmt.Errorf("error loading sfluv contract abi: %s", err), true)
	}

	callData, err := contractABI.Pack("transfer", toAddress, tokenAmount)
	if err != nil {
		return newSendError(fmt.Errorf("error packing transfer call data: %s", err), true)
	}

	simCtx, simCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer simCancel()
	simResult, err := b.client.CallContract(simCtx, ethereum.CallMsg{
		From: fromAddress,
		To:   &tokenAddress,
		Data: callData,
	}, nil)
	if err != nil {
		return newSendError(fmt.Errorf("transfer simulation failed: %s", err), true)
	}

	decoded, err := contractABI.Unpack("transfer", simResult)
	if err != nil {
		return newSendError(fmt.Errorf("error decoding transfer simulation result: %s", err), true)
	}
	if len(decoded) > 0 {
		if ok, cast := decoded[0].(bool); cast && !ok {
			return newSendError(fmt.Errorf("transfer simulation failed: contract returned false"), true)
		}
	}

	chainId, err := b.client.ChainID(context.Background())
	if err != nil {
		return newSendError(fmt.Errorf("error getting chainId from rpc node: %s", err), true)
	}

	s, err := bind.NewKeyedTransactorWithChainID(privateKey, chainId)
	if err != nil {
		return newSendError(fmt.Errorf("error creating transactor: %s", err), true)
	}
	opts := &bind.TransactOpts{
		From:    fromAddress,
		Signer:  s.Signer,
		Context: context.Background(),
	}

	tx, err := contract.Transfer(opts, toAddress, tokenAmount)
	if err != nil {
		return newSendError(fmt.Errorf("error sending transfer transaction: %s", err), true)
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

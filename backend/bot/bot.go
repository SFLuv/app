package bot

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/crypto/sha3"
)

type IBot interface {
	Key() string
	Send(amount uint64, address string) error
}

type Bot struct {
	pKey    string
	tokenId string
	client  *ethclient.Client
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
	fmt.Println("bot reached")
	decimalString := os.Getenv("TOKEN_DECIMALS")
	decimals, ok := new(big.Int).SetString(decimalString, 10)
	if !ok {
		fmt.Println("invalid TOKEN_DECIMALS value")
	}

	tokenAmount := new(big.Int).Mul(decimals, big.NewInt(int64(amount)))
	toAddress := common.HexToAddress(address)
	fmt.Println("to address:")
	fmt.Println(toAddress)
	tokenAddress := common.HexToAddress(b.tokenId)
	method := methodId("transfer(address,uint256)")

	paddedAmount := common.LeftPadBytes(tokenAmount.Bytes(), 32)
	paddedAddress := common.LeftPadBytes(toAddress.Bytes(), 32)

	var data []byte
	data = append(data, method...)
	data = append(data, paddedAddress...)
	data = append(data, paddedAmount...)

	gasPrice, err := b.client.SuggestGasPrice(context.Background())
	if err != nil {
		err = fmt.Errorf("error getting suggested gas price: %s", err)
		return err
	}

	privateKey, err := crypto.HexToECDSA(b.pKey)
	if err != nil {
		err = fmt.Errorf("error parsing private key: %s", err)
		return err
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		err = fmt.Errorf("error asserting type: publicKey is not of type ecdsa.PublicKey")
		return err
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	nonce, err := b.client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		err = fmt.Errorf("error retrieving nonce: %s", err)
		return err
	}

	fmt.Println("estimating gas costs")
	gasLimit, err := b.client.EstimateGas(context.Background(), ethereum.CallMsg{
		From: fromAddress,
		To:   &tokenAddress,
		Data: data,
	})
	if err != nil {
		err = fmt.Errorf("error estimating gas cost: %s", err)
		return err
	}

	// changed from value to tokenAmount, now not using value
	fmt.Println("creating new transaction")
	tx := types.NewTransaction(nonce, tokenAddress, big.NewInt(0), gasLimit, gasPrice, data)

	chainId, err := b.client.NetworkID(context.Background())
	if err != nil {
		err = fmt.Errorf("error getting chainId from rpc node: %s", err)
		return err
	}

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainId), privateKey)
	if err != nil {
		err = fmt.Errorf("error signing transaction: %s", err)
		return err
	}

	fmt.Println("sending transaction")
	err = b.client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		err = fmt.Errorf("error sending signed transaction: %s", err)
		return err
	}

	fmt.Printf("Sent Transaction: %s\n", signedTx.Hash().Hex())
	// return err if err
	return nil
}

func methodId(signature string) []byte {
	transferFnSignature := []byte(signature)

	hash := sha3.NewLegacyKeccak256()
	hash.Write(transferFnSignature)
	methodId := hash.Sum(nil)[:4]

	return methodId
}

package erc4337

import (
	"github.com/faucet-portal/backend/chain"
	"github.com/faucet-portal/backend/crypto"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"os"
	"testing"
)

func noTestInitCode(t *testing.T) {

	sk, err := crypto.SKFromHex(os.Getenv("CHAIN_SK"))
	require.NoError(t, err)
	k := &chain.EcdsaKey{SK: sk}

	testInitCode, err := MakeDefaultInitCode(k.Address())
	require.NoError(t, err)
	require.Equal(t, 88, len(testInitCode))

	// account creation gas usage: 288,526 per Mumbai
}

func noTestGetSenderAddress(t *testing.T) {

	ec, err := jsonrpc.NewClient(os.Getenv("CHAIN_URL"))
	require.NoError(t, err)

	sk, err := crypto.SKFromHex(os.Getenv("CHAIN_SK"))
	require.NoError(t, err)
	k := &chain.EcdsaKey{SK: sk}

	require.Equal(t, "0x32A629dE3fb4549EB2B204d37eb9C8CFb0b9AdCf", k.Address().String())

	ep, err := chain.LoadContract(ec, "IEntryPoint.sol/IEntryPoint", k, DefaultEntryPoint)
	require.NoError(t, err)

	initCode, err := MakeDefaultInitCode(k.Address())
	require.NoError(t, err)

	_, err = ep.Call("getSenderAddress", ethgo.Latest, initCode)
	// this method is expected to revert
	senderAddr, err := getSenderAddressFromError(err)
	require.NoError(t, err)

	require.Equal(t, ethgo.HexToAddress("0x054dF6203225bB58d9243eBf9DAd55608a436042"), senderAddr)

}

func TestInitABI(t *testing.T) {

	abiAccountFactory, err := chain.LoadABI("SimpleAccountFactory.sol/SimpleAccountFactory")
	require.NoError(t, err)

	checkCreateMethod := abiAccountFactory.GetMethod("createAccount")

	require.Equal(t, checkCreateMethod.Sig(), createAccountMethod.Sig())
}

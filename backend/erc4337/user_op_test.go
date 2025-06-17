package erc4337

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/faucet-portal/backend/chain"
	"github.com/faucet-portal/backend/crypto"
	"github.com/stackup-wallet/stackup-bundler/pkg/userop"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"math/big"
	"os"
	"testing"
)

// cribbed from StackUp...
var (
	MockUserOpData = map[string]any{
		"sender":               "0xa13D69573f994bf662C2714560c44dd7266FC547",
		"nonce":                "0x0",
		"initCode":             "0xe19e9755942bb0bd0cccce25b1742596b8a8250b3bf2c3e700000000000000000000000078d4f01f56b982a3b03c4e127a5d3afa8ebee6860000000000000000000000008b388a082f370d8ac2e2b3997e9151168bd09ff50000000000000000000000000000000000000000000000000000000000000000",
		"callData":             "0x80c5c7d0000000000000000000000000a13d69573f994bf662c2714560c44dd7266fc547000000000000000000000000000000000000000000000000016345785d8a000000000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000000000",
		"callGasLimit":         "0x558c",
		"verificationGasLimit": "0x129727",
		"maxFeePerGas":         "0xa862145e",
		"maxPriorityFeePerGas": "0xa8621440",
		"paymasterAndData":     "0x",
		"preVerificationGas":   "0xc869",
		"signature":            "0xa925dcc5e5131636e244d4405334c25f034ebdd85c0cb12e8cdb13c15249c2d466d0bade18e2cafd3513497f7f968dcbb63e519acd9b76dcae7acd61f11aa8421b",
	}
	MockByteCode = common.Hex2Bytes("6080604052")
)

func TestUserOpBasics(t *testing.T) {
	testUserOp, err := userop.New(MockUserOpData)
	require.NoError(t, err)

	require.Equal(t, 0, testUserOp.Nonce.Cmp(big.NewInt(0)))

	println(testUserOp.CallGasLimit.String())
	println(testUserOp.PreVerificationGas.String())
	println(testUserOp.VerificationGasLimit.String())
}

func noTestUserOpContracts(t *testing.T) {

	//ec, err := jsonrpc.NewClient(os.Getenv("CHAIN_URL"))
	//require.NoError(t, err)

	sk, err := crypto.SKFromHex(os.Getenv("CHAIN_SK"))
	require.NoError(t, err)
	k := &chain.EcdsaKey{SK: sk}

	callData, err := makeExecute(chain.MockMumbaiAddr, big.NewInt(0), mintMethod, k.Address(), ethgo.Ether(100))
	require.NoError(t, err)
	require.Equal(t, 228, len(callData))
	require.Equal(t, abiExec.ID(), callData[:4])
}

func TestOpABIMethods(t *testing.T) {

	var abiMC, err = chain.LoadABI("MockCoin.sol/MockCoin")
	require.NoError(t, err)

	checkMintMethod := abiMC.GetMethod("mint")
	require.Equal(t, checkMintMethod.Sig(), mintMethod.Sig())

	checkApproveMethod := abiMC.GetMethod("approve")
	require.Equal(t, checkApproveMethod.Sig(), approveMethod.Sig())

}

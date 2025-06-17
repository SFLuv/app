package erc4337

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dgraph-io/badger/v3"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/faucet-portal/backend/chain"
	"github.com/faucet-portal/backend/crypto"
	"github.com/stackup-wallet/stackup-bundler/pkg/client"
	"github.com/stackup-wallet/stackup-bundler/pkg/entrypoint/reverts"
	"github.com/stackup-wallet/stackup-bundler/pkg/gas"
	"github.com/stackup-wallet/stackup-bundler/pkg/mempool"
	"github.com/stackup-wallet/stackup-bundler/pkg/modules/checks"
	"github.com/stackup-wallet/stackup-bundler/pkg/modules/paymaster"
	"github.com/stackup-wallet/stackup-bundler/pkg/userop"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/jsonrpc/codec"
	"math/big"
	"os"
	"strings"
	"testing"
)

func noTestCheckUserOp(t *testing.T) {
	nodeRpc, err := rpc.Dial(os.Getenv("SU_NODE_URL"))
	require.NoError(t, err)

	opHash := "0x1410c65c614062a0e3885caf8750d35b3a25d2e0c4506a572aac2f11c6052ac0"

	var reply map[string]any
	err = nodeRpc.Call(&reply, "eth_getUserOperationReceipt", opHash)
	require.NoError(t, err)

	fmt.Printf("Response: %v", reply)
}

func noTestSendUserOp(t *testing.T) {

	nodeRpc, err := rpc.Dial(os.Getenv("SU_NODE_URL"))
	require.NoError(t, err)

	pmRpc, err := rpc.Dial(os.Getenv("SU_PM_URL"))
	require.NoError(t, err)

	var opMap map[string]any

	err = json.Unmarshal([]byte(opJsonNew), &opMap)
	require.NoError(t, err)

	userOp, err := userop.New(opMap)
	require.NoError(t, err)

	k, _ := crypto.SKFromInt(big.NewInt(0))
	userOp, _ = UserOpSeal(userOp, big.NewInt(137), &chain.EcdsaKey{SK: k})
	opMap, _ = userOp.ToMap()

	var pmResp map[string]any
	err = pmRpc.Call(&pmResp, "pm_sponsorUserOperation", opMap, DefaultEntryPoint.String(), map[string]string{"type": "payg"})
	require.NoError(t, err)

	for k, v := range pmResp {
		opMap[k] = v
	}

	var reply string
	err = nodeRpc.Call(&reply, "eth_sendUserOperation", opMap, DefaultEntryPoint.String())
	require.NoError(t, err)
	fmt.Printf("Response: %v", reply)

}

func noTestOpSubmitDirect(t *testing.T) {

	sk, err := crypto.SKFromHex(os.Getenv("CHAIN_SK"))
	require.NoError(t, err)
	k := &chain.EcdsaKey{SK: sk}
	println(k.Address().String())

	ec, err := jsonrpc.NewClient(os.Getenv("CHAIN_URL"))
	require.NoError(t, err)

	ep, err := chain.LoadContract(ec, "IEntryPoint.sol/IEntryPoint", k, DefaultEntryPoint)
	require.NoError(t, err)

	polyBaseAddr := ethgo.HexToAddress("0x00e8383c2E403997d867cc6d0f831557272902F9")
	ret, err := ep.Call("getDepositInfo", ethgo.Latest, polyBaseAddr)
	require.NoError(t, err)
	_ = ret

	var opMap map[string]any

	err = json.Unmarshal([]byte(opJsonNew), &opMap)
	require.NoError(t, err)

	_, err = userop.New(opMap)
	require.NoError(t, err)

	handleError := func(err error) {
		if cerr, ok := err.(*codec.ErrorObject); ok {
			et := errThunk{cerr: cerr}
			if etData, ok := et.ErrorData().(string); ok {
				if strings.HasPrefix(etData, "0xe0cff05f") {
					result, _ := reverts.NewValidationResult(et)
					println(fmt.Sprintf("stake: %v, sig failed: %v", result.SenderInfo.Stake.Int64(), result.ReturnInfo.SigFailed))
				} else {
					revert, _ := reverts.NewFailedOp(et)
					println(revert.Reason)
				}
			}
		}
	}

	err = chain.TxnDoWait(ep.Txn("handleOps", []map[string]any{opMap}, k.Address()))
	if err != nil {
		handleError(err)
	}
	require.NoError(t, err)
}

func noTestStackupClient(t *testing.T) {

	conf := struct {
		SupportedEntryPoints    []common.Address
		EthClientUrl            string
		MaxBatchGasLimit        *big.Int
		MaxVerificationGas      *big.Int
		MaxOpsForUnstakedSender int
	}{
		SupportedEntryPoints:    []common.Address{common.Address(DefaultEntryPoint)},
		EthClientUrl:            os.Getenv("CHAIN_URL"),
		MaxBatchGasLimit:        big.NewInt(10_000_000), // TODO: value?
		MaxVerificationGas:      big.NewInt(10_000_000), // TODO: value?
		MaxOpsForUnstakedSender: 1,
	}

	rpc, err := rpc.Dial(conf.EthClientUrl)
	require.NoError(t, err)

	eth := ethclient.NewClient(rpc)
	chainId, err := eth.ChainID(context.Background())
	require.NoError(t, err)

	bo := badger.DefaultOptions("")
	bo.InMemory = true
	bdb, err := badger.Open(bo)
	require.NoError(t, err)

	mp, err := mempool.New(bdb)
	require.NoError(t, err)

	ov := gas.NewDefaultOverhead()

	check := checks.New(
		bdb,
		rpc,
		ov,
		conf.MaxVerificationGas,
		conf.MaxBatchGasLimit,
		conf.MaxOpsForUnstakedSender,
	)

	paymaster := paymaster.New(bdb)

	// init client - same as StackUp private
	c := client.New(mp, ov, chainId, conf.SupportedEntryPoints)
	c.SetGetUserOpReceiptFunc(client.GetUserOpReceiptWithEthClient(eth))
	c.SetGetGasEstimateFunc(client.GetGasEstimateWithEthClient(rpc, ov, chainId, conf.MaxBatchGasLimit))
	c.SetGetUserOpByHashFunc(client.GetUserOpByHashWithEthClient(eth))
	// c.UseLogger(logr)
	c.UseModules(
		check.ValidateOpValues(),
		paymaster.CheckStatus(),
		check.SimulateOp(),
		paymaster.IncOpsSeen(),
	)

	sk, err := crypto.SKFromHex(os.Getenv("CHAIN_SK"))
	require.NoError(t, err)
	k := &chain.EcdsaKey{SK: sk}

	ec, err := jsonrpc.NewClient(os.Getenv("CHAIN_URL"))
	require.NoError(t, err)

	ep, err := chain.LoadContract(ec, "IEntryPoint.sol/IEntryPoint", k, DefaultEntryPoint)
	require.NoError(t, err)

	res, err := ep.Call("getNonce", ethgo.Latest,
		ethgo.HexToAddress("0x054dF6203225bB58d9243eBf9DAd55608a436042"), big.NewInt(0))
	require.NoError(t, err)
	nonce, ok := res["nonce"].(*big.Int)
	require.True(t, ok)

	op, err := UserOpApprove(
		nonce,
		ethgo.HexToAddress("0x32A629dE3fb4549EB2B204d37eb9C8CFb0b9AdCf"),
		ethgo.HexToAddress("0x054dF6203225bB58d9243eBf9DAd55608a436042"),
		chain.MockMumbaiAddr,
		chain.LuvMumbaiAddr,
		ethgo.Ether(100))
	require.NoError(t, err)

	op, err = UserOpSeal(op, chainId, k)
	require.NoError(t, err)

	opMap, _ := op.ToMap()

	res, err = ep.Call("simulateValidation", ethgo.Latest, opMap)
	if err != nil {
		handleSimulationError(err)
	}

	//     function handleOps(UserOperation[] calldata ops, address payable beneficiary) external;
	err = chain.TxnDoWait(ep.Txn("handleOps", []map[string]any{opMap}, k.Address()))
	require.NoError(t, err)

	//resSend, err := c.SendUserOperation(opMap, DefaultEntryPoint.String())
	//require.NoError(t, err)
	//_ = resSend

}

func noTestStakeManager(t *testing.T) {

	ec, err := jsonrpc.NewClient(os.Getenv("CHAIN_URL"))
	require.NoError(t, err)

	sk, err := crypto.SKFromHex(os.Getenv("CHAIN_SK"))
	require.NoError(t, err)
	k := &chain.EcdsaKey{SK: sk}

	ep, err := chain.LoadContract(ec, "IEntryPoint.sol/IEntryPoint", k, DefaultEntryPoint)
	require.NoError(t, err)

	tx, txErr := ep.Txn("depositTo", ethgo.HexToAddress("0x054dF6203225bB58d9243eBf9DAd55608a436042"))
	require.NoError(t, txErr)
	tx.WithOpts(&contract.TxnOpts{Value: ethgo.Ether(1)})

	err = chain.TxnDoWait(tx, nil)
	require.NoError(t, err)

	res, err := ep.Call("balanceOf", ethgo.Latest, ethgo.HexToAddress("0x054dF6203225bB58d9243eBf9DAd55608a436042"))
	require.NoError(t, err)
	checkBalance, ok := res["0"].(*big.Int)
	require.True(t, ok)
	require.True(t, checkBalance.Cmp(big.NewInt(0)) > 0)

}

var opJsonNew = `{"sender":"0xfD6DD93dCc566f6E8C0A5FFb7322B1302c1d2CC0","nonce":"0x0","initCode":"0x9406cc6185a346906296840746125a0e449764545fbfb9cf0000000000000000000000003bf27b2b37345d08a980e273564473bc3744bb1e0000000000000000000000000000000000000000000000000000000000000001","callData":"0xb61d27f600000000000000000000000058a2993a618afee681de23decbcf535a58a080ba000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000000000000000044205c28780000000000000000000000003bf27b2b37345d08a980e273564473bc3744bb1e00000000000000000000000000000000000000000000000000000000000f424000000000000000000000000000000000000000000000000000000000","callGasLimit":"0x11f80","verificationGasLimit":"0x6ddd0","preVerificationGas":"0xd496","maxFeePerGas":"0x1cd5348a15","maxPriorityFeePerGas":"0x0","paymasterAndData":"0xe93eca6595fe94091dc1af46aac2a8b5d799077000000000000000000000000000000000000000000000000000000000652763f2000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000736d6fef766ac928f021504b1ba87d1a795bee6ed756ffba2464083a4dd615f613bc03b1197b1acde57627337e8b3cb923551e5e38b77848446c4e28deb5b0941c","signature":"0xd1f107ec849624a09134503dd1477345ec3d1a116aed4d4e7c5569e43014055451ece0cc99853236716240c86acfc3b545614ea43ede9c54d759a24d03ba2a161b"}`

func noTestValidateUserOp(t *testing.T) {

	ec, err := jsonrpc.NewClient(os.Getenv("CHAIN_URL"))
	require.NoError(t, err)

	sk, err := crypto.SKFromHex(os.Getenv("CHAIN_SK"))
	require.NoError(t, err)
	k := &chain.EcdsaKey{SK: sk}

	ep, err := chain.LoadContract(ec, "IEntryPoint.sol/IEntryPoint", k, DefaultEntryPoint)
	require.NoError(t, err)

	var opMap map[string]any
	err = json.Unmarshal([]byte(opJsonNew), &opMap)
	require.NoError(t, err)

	res, err := ep.Call("getNonce", ethgo.Latest,
		ethgo.HexToAddress(opMap["sender"].(string)), big.NewInt(0))
	require.NoError(t, err)
	nonce, ok := res["nonce"].(*big.Int)
	require.True(t, ok)
	println(nonce.String())

	res, err = ep.Call("simulateValidation", ethgo.Latest, opMap)
	if err != nil {
		handleSimulationError(err)
	}
}

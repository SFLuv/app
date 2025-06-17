package erc4337

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/faucet-portal/backend/chain"
	"github.com/faucet-portal/backend/crypto"
	"github.com/stackup-wallet/stackup-bundler/pkg/userop"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/wallet"
	"math/big"
)

// for Mumbai, Polygon Mainnet, ETH mainnet, ...
var DefaultEntryPoint = ethgo.HexToAddress("0x5ff137d4b0fdcd49dca30c7cf57e578a026d2789")
var DefaultChainId = big.NewInt(137) // Polygon mainnet for now...

var abiExec, _ = abi.NewMethod("function execute(address to, uint256 value, bytes data)")

func makeExecute(toAddr ethgo.Address, value *big.Int, m *abi.Method, args ...interface{}) (enc []byte, err error) {
	if enc, err = m.Encode(args); err == nil {
		return abiExec.Encode([]interface{}{toAddr, value, enc})
	}
	return
}

var mintMethod, _ = abi.NewMethod("function mint(address sender, uint256 amount)")
var approveMethod, _ = abi.NewMethod("function approve(address spender, uint256 amount) external returns (bool)")
var withdrawToMethod, _ = abi.NewMethod("function withdrawTo(address account, uint256 amount) external returns (bool)")
var transferMethod, _ = abi.NewMethod("function transfer(address,uint256)")

var DefaultInitCodeGas = big.NewInt(300_000)
var DefaultMintGasLimit = big.NewInt(200_000)
var DefaultTransferGasLimit = big.NewInt(200_000) // TODO: too high?
var DefaultApproveGasLimit = big.NewInt(200_000)
var DefaultWithdrawToGasLimit = big.NewInt(200_000)

func makeBaseOp(nonce *big.Int, owner, sender ethgo.Address, callGasLimit, maxFeePerGas *big.Int, callData []byte) (op *userop.UserOperation, err error) {
	var initCode []byte

	if nonce.Int64() == 0 {
		if initCode, err = MakeDefaultInitCode(owner); err != nil {
			return
		}
	}

	vGasLimit := new(big.Int).Add(big.NewInt(150_000), DefaultInitCodeGas)
	opData := map[string]any{
		"sender":               sender.String(),
		"nonce":                nonce,
		"initCode":             hexutil.Encode(initCode),
		"callData":             hexutil.Encode(callData),
		"callGasLimit":         callGasLimit,
		"verificationGasLimit": vGasLimit,
		"maxFeePerGas":         maxFeePerGas,
		"maxPriorityFeePerGas": maxFeePerGas, // TODO: this will end up being too high since it's effectively maxPriorityFeePerGas + block.basefee
		"paymasterAndData":     "0x",
		"preVerificationGas":   big.NewInt(100_000),
		"signature":            "0x00",
	}
	op, err = userop.New(opData)
	return
}

func UserOpMint(nonce *big.Int, owner, sender, mintTargetAddr, toAddr ethgo.Address, amt *big.Int) (*userop.UserOperation, error) {
	if callData, err := makeExecute(mintTargetAddr, big.NewInt(0), mintMethod, toAddr, amt); err != nil {
		return nil, err
	} else {
		return makeBaseOp(nonce, owner, sender, DefaultMintGasLimit, big.NewInt(2_000_000_000), callData)
	}
}

func UserOpTransfer(nonce *big.Int, owner, sender, transferTargetAddr, toAddr ethgo.Address, amt, gas *big.Int) (*userop.UserOperation, error) {
	if callData, err := makeExecute(transferTargetAddr, big.NewInt(0), transferMethod, toAddr, amt); err != nil {
		return nil, err
	} else {
		return makeBaseOp(nonce, owner, sender, DefaultTransferGasLimit, gas, callData)
	}
}

func UserOpApprove(nonce *big.Int, owner, sender, targetAddr, spender ethgo.Address, amt *big.Int) (*userop.UserOperation, error) {
	if callData, err := makeExecute(targetAddr, big.NewInt(0), approveMethod, spender, amt); err != nil {
		return nil, err
	} else {
		return makeBaseOp(nonce, owner, sender, DefaultApproveGasLimit, big.NewInt(2_000_000_000), callData)
	}
}

func UserOpWithdrawTo(nonce *big.Int, owner, sender, targetAddr, toAddr ethgo.Address, amt, gas *big.Int) (*userop.UserOperation, error) {
	if callData, err := makeExecute(targetAddr, big.NewInt(0), withdrawToMethod, toAddr, amt); err != nil {
		return nil, err
	} else {
		return makeBaseOp(nonce, owner, sender, DefaultWithdrawToGasLimit, gas, callData)
	}
}

func UserOpSeal(op *userop.UserOperation, chainId *big.Int, k *chain.EcdsaKey) (*userop.UserOperation, error) {
	opHash := op.GetUserOpHash(common.Address(DefaultEntryPoint), chainId)
	opEthHash := crypto.EthSignedMessageHash(opHash.Bytes())
	if sig, err := crypto.Sign(k.SK, opEthHash); err != nil {
		return nil, err
	} else {
		sig[64] += 27
		op.Signature = sig
	}
	return op, nil
}

func UserOpEcrecover(op *userop.UserOperation, chainId *big.Int) (opHash common.Hash, addr ethgo.Address, err error) {
	opHash = op.GetUserOpHash(common.Address(DefaultEntryPoint), chainId)
	opEthHash := crypto.EthSignedMessageHash(opHash.Bytes())
	if len(op.Signature) != 65 {
		err = fmt.Errorf("should not happen - invalid signature size in user op: %v", len(op.Signature))
	} else {
		var sig [65]byte
		copy(sig[:], op.Signature)
		if sig[64] >= 27 {
			sig[64] -= 27
		}
		addr, err = wallet.Ecrecover(opEthHash, sig[:])
	}
	return
}

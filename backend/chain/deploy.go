package chain

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"github.com/faucet-portal/backend/crypto"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/compiler"
	"github.com/umbracle/ethgo/contract"
	"github.com/umbracle/ethgo/jsonrpc"
	"os"
	"path/filepath"
	"strings"
)

type jsonBytecode struct {
	Object string `json:"object"`
}
type jsonArtifact struct {
	Bytecode jsonBytecode    `json:"bytecode"`
	Abi      json.RawMessage `json:"abi"`
}

func getBuildArtifact(name string) (art *compiler.Artifact, err error) {

	if !strings.HasSuffix(name, ".json") {
		name = name + ".json"
	}
	var jsonBytes []byte
	if jsonBytes, err = os.ReadFile(filepath.Join("../../contracts/out", name)); err != nil {
		if jsonBytes, err = os.ReadFile(filepath.Join("../contracts/out", name)); err != nil {
			return
		}
	}

	art, err = parseBuildArtifact(jsonBytes)
	return
}

func parseBuildArtifact(jsonBytes []byte) (art *compiler.Artifact, err error) {
	var jart jsonArtifact
	if err = json.Unmarshal(jsonBytes, &jart); err != nil {
		return
	}

	bc := jart.Bytecode.Object
	if !strings.HasPrefix(bc, "0x") {
		bc = "0x" + bc
	}
	art = &compiler.Artifact{
		Abi: string(jart.Abi),
		Bin: bc,
	}
	return
}

type EcdsaKey struct {
	SK *ecdsa.PrivateKey
}

func (e *EcdsaKey) Address() ethgo.Address {
	return ethgo.Address(crypto.PubKeyToAddress(&e.SK.PublicKey))
}

func (e *EcdsaKey) Sign(hash []byte) ([]byte, error) {
	return crypto.Sign(e.SK, hash)
}

func (e *EcdsaKey) SKHex() string {
	b := e.SK.D.Bytes()
	bb := make([]byte, 32-len(b))
	bb = append(bb, b...)
	return hex.EncodeToString(bb)
}

var _ ethgo.Key = &EcdsaKey{}

func LoadContract(ec *jsonrpc.Client, name string, withKey ethgo.Key, addr ethgo.Address) (loaded *contract.Contract, err error) {
	var art *compiler.Artifact
	if art, err = getBuildArtifact(name); err != nil {
		return
	}
	var theAbi *abi.ABI
	if theAbi, err = abi.NewABI(art.Abi); err != nil {
		return
	}

	loaded = contract.NewContract(addr, theAbi,
		contract.WithJsonRPC(ec.Eth()),
		contract.WithSender(withKey),
	)

	return
}

func LoadReadContractAbi(ec *jsonrpc.Client, abiBytes []byte, addr ethgo.Address, maybeKey *EcdsaKey) (loaded *contract.Contract, err error) {
	var art *compiler.Artifact
	if art, err = parseBuildArtifact(abiBytes); err != nil {
		return
	}
	var theAbi *abi.ABI
	if theAbi, err = abi.NewABI(art.Abi); err != nil {
		return
	}

	opts := []contract.ContractOption{contract.WithJsonRPC(ec.Eth())}
	if maybeKey != nil {
		opts = append(opts, contract.WithSender(maybeKey))
	}
	loaded = contract.NewContract(addr, theAbi, opts...)

	return
}

func LoadABI(name string) (loaded *abi.ABI, err error) {
	var art *compiler.Artifact
	if art, err = getBuildArtifact(name); err != nil {
		return
	}
	return abi.NewABI(art.Abi)
}

func deployContract(ec *jsonrpc.Client, name string, withKey ethgo.Key, args []interface{}) (deployed *contract.Contract, addr ethgo.Address, err error) {
	var art *compiler.Artifact
	if art, err = getBuildArtifact(name); err != nil {
		return
	}
	var theAbi *abi.ABI
	if theAbi, err = abi.NewABI(art.Abi); err != nil {
		return
	}

	var rcpt *ethgo.Receipt
	var artBin []byte
	if artBin, err = hex.DecodeString(strings.TrimPrefix(art.Bin, "0x")); err != nil {
		return
	}
	var txn contract.Txn
	if txn, err = contract.DeployContract(theAbi, artBin, args,
		contract.WithJsonRPC(ec.Eth()), contract.WithSender(withKey)); err != nil {
		return
	} else {
		if err = txn.Do(); err != nil {
			return
		}
		if rcpt, err = txn.Wait(); err != nil {
			return
		}
	}

	deployed = contract.NewContract(rcpt.ContractAddress, theAbi,
		contract.WithJsonRPC(ec.Eth()),
		contract.WithSender(withKey),
	)
	addr = rcpt.ContractAddress

	return
}

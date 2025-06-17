package erc4337

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc/codec"
	"math/big"
	"strings"
)

// let createAccountTx = await accountFactoryContract.$data().createAccount({ address: owner }, owner, salt);
// let initCode = $abiUtils.encodePacked(['address', 'bytes'], [ accountFactoryContract.address, createAccountTx.data ]);
// let initCodeGas = await this.client.getGasEstimation(entryPointContract.address, createAccountTx);

var DefaultAccountFactory = ethgo.HexToAddress("0x9406Cc6185a346906296840746125a0E44976454")
var DefaultInitSalt = big.NewInt(1)

var createAccountMethod, _ = abi.NewMethod("function createAccount(address owner,uint256 salt) public returns (address ret)")

func MakeDefaultInitCode(owner ethgo.Address) (ret []byte, err error) {
	return makeInitCode(DefaultAccountFactory, owner, DefaultInitSalt)
}

func makeInitCode(factory, owner ethgo.Address, salt *big.Int) (ret []byte, err error) {
	if ret, err = createAccountMethod.Encode([]interface{}{owner, salt}); err != nil {
		return
	}
	ret = append(factory.Bytes(), ret...) // 'packed encoding' ...
	return
}

var senderAddressErrorPrefix = "0x6ca7b806"

func getSenderAddressFromError(errIn error) (addr ethgo.Address, err error) {
	if eo, ok := errIn.(*codec.ErrorObject); !ok {
		err = fmt.Errorf("unexpected - error is not *codec.ErrorObject")
	} else {
		if eod, ok := eo.Data.(string); !ok {
			err = fmt.Errorf("unexpected - codec.ErrorObject Data element is not a string")
		} else {
			if !strings.HasPrefix(eod, senderAddressErrorPrefix) {
				err = fmt.Errorf("unexpected - error data format has invalid 4byte prefix")
			} else {
				var eodBytes []byte
				if eodBytes, err = hexutil.Decode(eod); err != nil || len(eodBytes) != 36 {
					err = fmt.Errorf("unexpected - bad address data a/o invalid length")
				}
				addr = ethgo.BytesToAddress(eodBytes[16:])
			}
		}
	}
	return
}

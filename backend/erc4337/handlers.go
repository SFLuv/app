package erc4337

import (
	"fmt"
	"github.com/apex/log"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/faucet-portal/backend/chain"
	"github.com/faucet-portal/backend/config"
	"github.com/faucet-portal/backend/crypto"
	"github.com/stackup-wallet/stackup-bundler/pkg/entrypoint/execution"
	"github.com/stackup-wallet/stackup-bundler/pkg/entrypoint/reverts"
	"github.com/stackup-wallet/stackup-bundler/pkg/userop"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/contract"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/jsonrpc/codec"
	"math/big"
	"net/http"
	"strings"
)

// GET? erc4337/userop/approve?target=XXXX&spender=YYYY&amount=10000&owner=ZZZZ
var addrHexLength = len(ethgo.ZeroAddress.String())

func handleRequiredAddress(addrHex string) (ret *ethgo.Address) {
	if !strings.HasPrefix(addrHex, "0x") {
		addrHex = "0x" + addrHex
	}
	if len(addrHex) != addrHexLength {
		return
	}
	if addr := ethgo.HexToAddress(addrHex); addr != ethgo.ZeroAddress {
		ret = &addr
	}
	return
}

type errThunk struct {
	cerr *codec.ErrorObject
}

func (e errThunk) Error() string {
	return e.cerr.Error()
}

func (e errThunk) ErrorData() interface{} {
	return e.cerr.Data
}

var _ rpc.DataError = &errThunk{}

func handleSimulationError(err error) {
	if cerr, ok := err.(*codec.ErrorObject); ok {
		et := errThunk{cerr: cerr}
		if etData, ok := et.ErrorData().(string); ok {
			if strings.HasPrefix(etData, "0xe0cff05f") {
				result, _ := reverts.NewValidationResult(et)
				log.Infof("stake: %v, sig failed: %v", result.SenderInfo.Stake.Int64(), result.ReturnInfo.SigFailed)
			} else {
				revert, _ := reverts.NewFailedOp(et)
				log.Infof("validation for failed op: %v", revert.Reason)
			}
		}
	}
}

type HandlerContext struct {
	testContext map[string]string
	chainRpc    *jsonrpc.Client

	suNodeRpc *rpc.Client
	suPMRpc   *rpc.Client

	ChainId      *big.Int
	EntryPoint   *contract.Contract
	SFLUV        *contract.Contract
	ChainKeyAddr ethgo.Address

	simulateUserOp   bool
	sendUserOpDirect bool
}

func makeTestContext(testContext map[string]string) (*HandlerContext, error) {
	return &HandlerContext{testContext: testContext}, nil
}

func MakeContext(config config.Config) (*HandlerContext, error) {
	abiEPBytes, err := abiIEP.ReadFile("abi/IEntryPoint.json")
	if err != nil {
		return nil, err
	}

	abiSLBytes, err := abiSFLUV.ReadFile("abi/SFLUVv1.json")
	if err != nil {
		return nil, err
	}

	chainRpc, err := jsonrpc.NewClient(config.ChainRpcUrl)
	if err != nil {
		return nil, err
	}

	nodeRpc, err := rpc.Dial(config.SUNodeUrl)
	if err != nil {
		return nil, err
	}

	pmRpc, err := rpc.Dial(config.SUPayMasterUrl)
	if err != nil {
		return nil, err
	}

	chainId, err := chainRpc.Eth().ChainID()
	if err != nil {
		log.Errorf("failed to connect to blockchain at %v, error %v", config.ChainRpcUrl, err.Error())
		return nil, err
	}
	hc := &HandlerContext{ChainId: chainId, chainRpc: chainRpc, suNodeRpc: nodeRpc, suPMRpc: pmRpc}
	log.Infof("connected to chain with url %v, got chain id %v", config.ChainRpcUrl, chainId.Int64())

	var maybeKey *chain.EcdsaKey
	if len(config.ChainSKHex) != 0 {
		if sk, err := crypto.SKFromHex(config.ChainSKHex); err != nil {
			return nil, err
		} else {
			maybeKey = &chain.EcdsaKey{SK: sk}
		}
		hc.ChainKeyAddr = maybeKey.Address()
	}

	hc.EntryPoint, err = chain.LoadReadContractAbi(chainRpc, abiEPBytes, DefaultEntryPoint, maybeKey)
	if err != nil {
		return nil, err
	}

	hc.SFLUV, err = chain.LoadReadContractAbi(chainRpc, abiSLBytes, chain.SFLUVPolygonMainnetV1_1, maybeKey)
	if err != nil {
		return nil, err
	}

	hc.simulateUserOp = false
	hc.sendUserOpDirect = false

	return hc, nil
}

// TODO: this might get kind of expensive. in the future we could have a goproc that updates a cached price
func (hc *HandlerContext) getGasPrice() (*big.Int, error) {
	if price, err := hc.chainRpc.Eth().GasPrice(); err != nil {
		return nil, err
	} else {
		return big.NewInt(int64(price)), nil
	}
}

func (hc *HandlerContext) getOwnerInfo(ownerAddr ethgo.Address) (nonce *big.Int, senderAddr ethgo.Address, err error) {
	if len(hc.testContext) != 0 {
		nonce, _ = new(big.Int).SetString(hc.testContext["nonce"], 10)
		senderAddr = ethgo.HexToAddress(hc.testContext["sender"])
		return
	}

	var ownerInitCode []byte
	if ownerInitCode, err = MakeDefaultInitCode(ownerAddr); err != nil {
		return
	}

	_, err = hc.EntryPoint.Call("getSenderAddress", ethgo.Latest, ownerInitCode)
	// this method is expected to revert
	if senderAddr, err = getSenderAddressFromError(err); err != nil {
		return
	}

	var res map[string]interface{}
	if res, err = hc.EntryPoint.Call("getNonce", ethgo.Latest, senderAddr, big.NewInt(0)); err != nil {
		return
	}
	var ok bool
	if nonce, ok = res["nonce"].(*big.Int); !ok {
		err = fmt.Errorf("unexpected - expected *big.Int for nonce return value")
		return
	}

	return
}

func (hc *HandlerContext) getSenderBalance(senderAddr ethgo.Address) (*big.Int, error) {
	if res, err := hc.SFLUV.Call("balanceOf", ethgo.Latest, senderAddr); err != nil {
		return nil, err
	} else {
		if balance, ok := res["0"].(*big.Int); !ok {
			return nil, fmt.Errorf("unexpected - expected *big.Int for balance return value")
		} else {
			return balance, nil
		}
	}
}

func (hc *HandlerContext) getPaymasterInfo(userOp *userop.UserOperation) (newOp *userop.UserOperation, err error) {
	// paymaster API requires signature - can be fake tho ...
	k, _ := crypto.SKFromInt(big.NewInt(0))
	prevSignature := userOp.Signature
	if newOp, err = UserOpSeal(userOp, hc.ChainId, &chain.EcdsaKey{SK: k}); err != nil {
		return nil, err
	} else {
		var pmResp map[string]any
		opMap, _ := newOp.ToMap()
		if err = hc.suPMRpc.Call(&pmResp, "pm_sponsorUserOperation", opMap, DefaultEntryPoint.String(), map[string]string{"type": "payg"}); err != nil {
			return nil, err
		}
		for k, v := range pmResp {
			opMap[k] = v
		}
		newOp, err = userop.New(opMap)
		newOp.Signature = prevSignature
	}
	return
}

func (hc *HandlerContext) sendUserOp(userOp *userop.UserOperation) (reply string, err error) {
	opMap, _ := userOp.ToMap()
	if hc.simulateUserOp {
		if sim, err := execution.SimulateHandleOp(hc.suNodeRpc, common.Address(DefaultEntryPoint), userOp, common.BigToAddress(big.NewInt(0)), nil); err != nil {
			log.Infof("userop failed simulation: %v", err.Error())
		} else {
			_ = sim
		}
	}

	reply = userOp.GetUserOpHash(common.Address(DefaultEntryPoint), hc.ChainId).String()
	if hc.sendUserOpDirect {
		err = chain.TxnDoWait(hc.EntryPoint.Txn("handleOps", []map[string]any{opMap}, hc.ChainKeyAddr))
	} else {
		err = hc.suNodeRpc.Call(&reply, "eth_sendUserOperation", opMap, DefaultEntryPoint.String())
	}
	if err == nil {
		opJson, _ := userOp.MarshalJSON()
		log.Infof("submitted user op hash '%v', '%v'", reply, string(opJson))
	}
	return
}

type Context struct {
	w http.ResponseWriter
	r *http.Request
}

func (c *Context) Status(code int) {
	c.w.WriteHeader(code)
}

func (c *Context) AbortWithError(code int, err error) {
	c.w.WriteHeader(code)
}

func (c *Context) JSON(code int, obj any) {
	// TODO
}

func (c *Context) BindJSON(obj any) error {
	// return c.MustBindWith(obj, binding.JSON)
	// TODO
	return nil
}

func (hc *HandlerContext) HandleGetSenderInfo(w http.ResponseWriter, r *http.Request) {
	c := Context{w: w, r: r}
	q := c.r.URL.Query()
	ownerAddr := handleRequiredAddress(q.Get("owner"))
	if ownerAddr == nil {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("invalid or missing parameter(s)"))
		return
	}

	if nonce, senderAddr, err := hc.getOwnerInfo(*ownerAddr); err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
	} else {
		if balance, err := hc.getSenderBalance(senderAddr); err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
		} else {
			c.JSON(http.StatusOK, fmt.Sprintf(`{"nonce":%v, "sender": "%v", "balance":%v}`,
				nonce.Int64(), senderAddr.String(), balance.String()))
		}
	}
}

// TODO: this looks wrong now ...?

func (hc *HandlerContext) HandleUserOpApprove(w http.ResponseWriter, r *http.Request) {
	c := Context{w: w, r: r}

	q := c.r.URL.Query()

	targetAddr := handleRequiredAddress(q.Get("target"))
	spenderAddr := handleRequiredAddress(q.Get("spender"))
	ownerAddr := handleRequiredAddress(q.Get("owner"))

	amount, ok := new(big.Int).SetString(q.Get("amount"), 10)

	if targetAddr == nil || spenderAddr == nil || ownerAddr == nil || !ok {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("invalid or missing parameter(s)"))
		return
	}

	nonce, senderAddr, err := hc.getOwnerInfo(*ownerAddr)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	if op, err := UserOpApprove(nonce, *ownerAddr, senderAddr, *targetAddr, *spenderAddr, amount); err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	} else {
		opJson, _ := op.ToMap()
		c.JSON(http.StatusOK, opJson)
	}
}

func (hc *HandlerContext) HandleUserOpWithdrawTo(w http.ResponseWriter, r *http.Request) {
	c := Context{w: w, r: r}

	q := c.r.URL.Query()

	targetAddr := handleRequiredAddress(q.Get("target"))
	toAddr := handleRequiredAddress(q.Get("to"))
	ownerAddr := handleRequiredAddress(q.Get("owner"))

	amount, ok := new(big.Int).SetString(q.Get("amount"), 10)

	if targetAddr == nil || toAddr == nil || ownerAddr == nil || !ok {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("invalid or missing parameter(s)"))
		return
	}

	nonce, senderAddr, err := hc.getOwnerInfo(*ownerAddr)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	gasPrice, err := hc.getGasPrice()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	if op, err := UserOpWithdrawTo(nonce, *ownerAddr, senderAddr, *targetAddr, *toAddr, amount, gasPrice); err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	} else {
		if op, err = hc.getPaymasterInfo(op); err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		opJson, _ := op.ToMap()
		c.JSON(http.StatusOK, opJson)
	}
}

// TODO: copy pasta from withdraw ...

func (hc *HandlerContext) HandleUserOpTransfer(w http.ResponseWriter, r *http.Request) {
	c := Context{w: w, r: r}

	q := c.r.URL.Query()

	targetAddr := handleRequiredAddress(q.Get("target"))
	toAddr := handleRequiredAddress(q.Get("to"))
	ownerAddr := handleRequiredAddress(q.Get("owner"))

	amount, ok := new(big.Int).SetString(q.Get("amount"), 10)

	if targetAddr == nil || toAddr == nil || ownerAddr == nil || !ok {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("invalid or missing parameter(s)"))
		return
	}

	nonce, senderAddr, err := hc.getOwnerInfo(*ownerAddr)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	gasPrice, err := hc.getGasPrice()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	if op, err := UserOpTransfer(nonce, *ownerAddr, senderAddr, *targetAddr, *toAddr, amount, gasPrice); err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	} else {
		if op, err = hc.getPaymasterInfo(op); err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		opJson, _ := op.ToMap()
		c.JSON(http.StatusOK, opJson)
	}
}

type userOpSendRequest struct {
	EntryPointAddr string         `json:"entryPoint"`
	Op             map[string]any `json:"op"`
}

func (hc *HandlerContext) HandleUserOpSend(w http.ResponseWriter, r *http.Request) {
	c := Context{w: w, r: r}
	req := userOpSendRequest{}
	if err := c.BindJSON(&req); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if userOp, err := userop.New(req.Op); err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	} else {
		opHash, ownerAddr, err := UserOpEcrecover(userOp, hc.ChainId)
		if err != nil {
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf("ecrecover failure: %v", err.Error()))
			return
		}

		_, senderAddr, err := hc.getOwnerInfo(ownerAddr)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		if senderAddr.String() != userOp.Sender.String() {
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf("op sender address does not match recovered sender address: owner '%v', sender '%v', userop sender '%v', op hash '%v'",
				ownerAddr.String(), senderAddr.String(), userOp.Sender.String(), opHash.String()))
			return
		}
		if reply, err := hc.sendUserOp(userOp); err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		} else {
			c.JSON(http.StatusOK, fmt.Sprintf(`{"op hash":"%v"}`, reply))
			return
		}
	}
}

package protocol

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Client talks to the two endpoints a sponsored operation needs, which are not
// the same endpoint.
//
// The engine is a curated RPC: it sponsors and submits operations and refuses
// ordinary methods. Reads (eth_call, balances, code) have to go to a real node.
// Pointing either at the other is the mistake this split exists to prevent —
// it fails at the first call with a method-not-found that reads like an outage.
type Client struct {
	engineRPC  string
	readRPC    string
	entryPoint common.Address
	factory    common.Address
	accountKey string

	http *http.Client
}

// Config is what the client needs from the loaded Citizen Wallet config.
type Config struct {
	// EngineBaseURL is the engine root, without a path. The bundler endpoint is
	// derived from it and the paymaster address.
	EngineBaseURL string
	ReadRPCURL    string
	EntryPoint    string
	Factory       string
	Paymaster     string
	// PaymasterType selects the account flavour. Only "cw-safe" is supported
	// here: a Safe is executed through its module, and the older CW Account is
	// executed through execute(). Refusing an unknown type at construction is
	// better than encoding the wrong call and watching it revert on chain.
	PaymasterType string
}

const paymasterTypeSafe = "cw-safe"

func NewClient(cfg Config) (*Client, error) {
	engine := strings.TrimRight(strings.TrimSpace(cfg.EngineBaseURL), "/")
	if engine == "" {
		return nil, fmt.Errorf("the engine base url is missing from the client config")
	}
	read := strings.TrimSpace(cfg.ReadRPCURL)
	if read == "" {
		return nil, fmt.Errorf("the read rpc url is missing from the client config")
	}
	if !common.IsHexAddress(strings.TrimSpace(cfg.EntryPoint)) {
		return nil, fmt.Errorf("entry point %q is not an address", cfg.EntryPoint)
	}
	if !common.IsHexAddress(strings.TrimSpace(cfg.Factory)) {
		return nil, fmt.Errorf("account factory %q is not an address", cfg.Factory)
	}
	if !common.IsHexAddress(strings.TrimSpace(cfg.Paymaster)) {
		return nil, fmt.Errorf("paymaster %q is not an address", cfg.Paymaster)
	}
	if strings.TrimSpace(cfg.PaymasterType) != paymasterTypeSafe {
		return nil, fmt.Errorf(
			"paymaster type %q is not supported: protocol accounts are Safes executed through their module",
			cfg.PaymasterType,
		)
	}

	return &Client{
		engineRPC:  fmt.Sprintf("%s/v1/rpc/%s", engine, common.HexToAddress(cfg.Paymaster).Hex()),
		readRPC:    read,
		entryPoint: common.HexToAddress(cfg.EntryPoint),
		factory:    common.HexToAddress(cfg.Factory),
		accountKey: fmt.Sprintf("%s/v1/accounts", engine),
		http:       &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) Factory() common.Address    { return c.factory }
func (c *Client) EntryPoint() common.Address { return c.entryPoint }

// ── JSON-RPC ─────────────────────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// RejectedError is the engine answering, definitively, that it did not accept a
// request: a JSON-RPC error object, or a 4xx. Anything else that fails — a
// dropped connection, a timeout, a 5xx, a response that would not decode — is
// left as a plain error, because for eth_sendUserOperation those cannot tell
// "never received" from "received and accepted, answer lost".
type RejectedError struct {
	Method  string
	Code    int
	Message string
}

func (e *RejectedError) Error() string { return e.Message }

// IsRejected reports whether err is the engine definitively refusing a request.
func IsRejected(err error) bool {
	var rejected *RejectedError
	return errors.As(err, &rejected)
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

func (c *Client) rpc(ctx context.Context, url, method string, params []any, out any) error {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return fmt.Errorf("error encoding %s: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("error building the %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("error calling %s: %w", method, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("error reading the %s response: %w", method, err)
	}
	if resp.StatusCode != http.StatusOK {
		message := fmt.Sprintf("%s returned %d: %s", method, resp.StatusCode, strings.TrimSpace(string(raw)))
		// A 4xx is the engine answering that it will not take the request. A
		// 5xx, or a 408 timeout, says nothing about whether it already did.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout {
			return &RejectedError{Method: method, Code: resp.StatusCode, Message: message}
		}
		return fmt.Errorf("%s", message)
	}

	var parsed rpcResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("error decoding the %s response: %w", method, err)
	}
	if parsed.Error != nil {
		return &RejectedError{
			Method:  method,
			Code:    parsed.Error.Code,
			Message: fmt.Sprintf("%s failed: %s (code %d)", method, parsed.Error.Message, parsed.Error.Code),
		}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(parsed.Result, out); err != nil {
		return fmt.Errorf("error decoding the %s result: %w", method, err)
	}
	return nil
}

// readCall is an eth_call against the full node.
func (c *Client) readCall(ctx context.Context, to common.Address, data []byte) ([]byte, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(dialCtx, c.readRPC)
	if err != nil {
		return nil, fmt.Errorf("error dialling the read rpc: %w", err)
	}
	defer client.Close()

	callCtx, callCancel := context.WithTimeout(ctx, 15*time.Second)
	defer callCancel()
	return client.CallContract(callCtx, ethereum.CallMsg{To: &to, Data: data}, nil)
}

// ── The sponsored-operation flow ─────────────────────────────────────────────

// AccountExists reports whether the engine considers an account deployed.
//
// Asked of the engine rather than derived from eth_getCode because the engine
// is what decides whether it will accept an operation carrying initCode, and
// its answer is the one that matters. A deployed account sent initCode again is
// rejected outright.
func (c *Client) AccountExists(ctx context.Context, account common.Address) (bool, error) {
	url := fmt.Sprintf("%s/%s/exists", c.accountKey, account.Hex())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("error building the account-exists request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("error asking whether %s exists: %w", account.Hex(), err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("the engine answered %d for whether %s exists", resp.StatusCode, account.Hex())
	}
}

// Sponsor hands the operation to the paymaster, which fills in the gas fields
// and the paymasterAndData that makes it free to the sender.
//
// The signature is cleared first and signed afterwards, in that order, because
// the paymaster rewrites the gas numbers and those are inside the hash. Signing
// before sponsorship produces a signature over an operation that no longer
// exists.
func (c *Client) Sponsor(ctx context.Context, op *UserOperation) (*UserOperation, error) {
	params := []any{
		op.toJSON(),
		c.entryPoint.Hex(),
		map[string]string{"type": paymasterTypeSafe},
		1,
	}
	var sponsored []userOpJSON
	if err := c.rpc(ctx, c.engineRPC, "pm_ooSponsorUserOperation", params, &sponsored); err != nil {
		return nil, err
	}
	if len(sponsored) == 0 {
		return nil, fmt.Errorf("the paymaster returned no sponsored operation")
	}
	return userOpFromJSON(sponsored[0])
}

// Send submits a signed operation and returns the operation hash the engine
// reports.
func (c *Client) Send(ctx context.Context, op *UserOperation, extra map[string]any) (string, error) {
	params := []any{op.toJSON(), c.entryPoint.Hex()}
	if extra != nil {
		params = append(params, extra)
	}
	var hash string
	if err := c.rpc(ctx, c.engineRPC, "eth_sendUserOperation", params, &hash); err != nil {
		return "", err
	}
	return hash, nil
}

// Execute is the whole flow for one call made by one Safe: build, sponsor,
// sign, submit.
//
// deployIndex is the factory salt the Safe was derived from, and is only used
// when the account does not exist yet — the initCode it produces has to derive
// the same address the operation is sent to, or the operation is rejected for
// a sender that does not match its own init code.
func (c *Client) Execute(
	ctx context.Context,
	safe common.Address,
	owner *ecdsa.PrivateKey,
	deployIndex int64,
	to common.Address,
	value *big.Int,
	data []byte,
) (string, error) {
	if owner == nil {
		return "", fmt.Errorf("no owner key to sign with")
	}

	exists, err := c.AccountExists(ctx, safe)
	if err != nil {
		return "", err
	}

	op := NewUserOperation(safe)
	op.CallData, err = SafeModuleCallData(to, value, data)
	if err != nil {
		return "", err
	}
	if !exists {
		op.InitCode, err = c.InitCode(pubkeyToAddress(owner), big.NewInt(deployIndex))
		if err != nil {
			return "", err
		}
	}

	sponsored, err := c.Sponsor(ctx, op)
	if err != nil {
		return "", err
	}
	hash, err := c.Hash(ctx, sponsored)
	if err != nil {
		return "", err
	}
	if err := Sign(sponsored, hash, owner); err != nil {
		return "", err
	}
	return c.Send(ctx, sponsored, nil)
}

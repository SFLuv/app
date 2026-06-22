package handlers

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// SigAuthParams is the Citizen Wallet sigAuth parameter set proving control of
// an account, as produced by the CW SDK createConnectedUrl.
type SigAuthParams struct {
	Account   string
	Expiry    string
	Signature string
	Redirect  string
}

// eip1271MagicValue is the bytes4 returned by isValidSignature on success.
var eip1271MagicValue = [4]byte{0x16, 0x26, 0xba, 0x7e}

const sigAuthAccountABI = `[
	{"type":"function","name":"isValidSignature","stateMutability":"view","inputs":[{"name":"hash","type":"bytes32"},{"name":"signature","type":"bytes"}],"outputs":[{"name":"","type":"bytes4"}]},
	{"type":"function","name":"owner","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"address"}]},
	{"type":"function","name":"isOwner","stateMutability":"view","inputs":[{"name":"owner","type":"address"}],"outputs":[{"name":"","type":"bool"}]}
]`

// VerifySigAuth validates a Citizen Wallet sigAuth parameter set and returns the
// verified account address (lowercased) on success. It mirrors the CW SDK's
// verifyConnectedHeaders + verifyAccountOwnership: it checks the expiry, rebuilds
// the exact signed message, recovers the EIP-191 signer, and accepts the account
// if the signer IS the account (EOA) or, for a smart account, if EIP-1271
// isValidSignature returns the magic value or the recovered signer is the
// account's owner()/Safe isOwner (verified via the read RPC).
func VerifySigAuth(ctx context.Context, readRPCURL string, p SigAuthParams) (string, error) {
	accountStr := strings.TrimSpace(p.Account)
	if !common.IsHexAddress(accountStr) {
		return "", fmt.Errorf("invalid sigAuthAccount")
	}
	account := common.HexToAddress(accountStr)

	if err := checkSigAuthExpiry(p.Expiry); err != nil {
		return "", err
	}

	message := generateConnectionMessage(account, strings.TrimSpace(p.Expiry), strings.TrimSpace(p.Redirect))
	sig, err := decodeSignature(p.Signature)
	if err != nil {
		return "", err
	}
	textHash := accounts.TextHash([]byte(message))

	recovered, recoveredOK := recoverSigner(textHash, sig)
	if recoveredOK && recovered == account {
		return strings.ToLower(account.Hex()), nil
	}

	// Smart account: EIP-1271 / owner() / Safe isOwner via the read RPC.
	if verifySmartAccountOwnership(ctx, readRPCURL, account, recovered, recoveredOK, textHash, sig) {
		return strings.ToLower(account.Hex()), nil
	}

	return "", fmt.Errorf("signature does not authorize account %s", account.Hex())
}

// generateConnectionMessage mirrors the CW SDK generateConnectionMessage
// byte-for-byte so the reconstructed message hashes identically.
func generateConnectionMessage(account common.Address, expiry, redirect string) string {
	msg := fmt.Sprintf("Signature auth for %s with expiry %s", account.Hex(), expiry)
	if redirect != "" {
		msg += " and redirect " + encodeURIComponent(redirect)
	}
	return msg
}

func checkSigAuthExpiry(expiry string) error {
	expiry = strings.TrimSpace(expiry)
	if expiry == "" {
		return fmt.Errorf("missing sigAuthExpiry")
	}
	var t time.Time
	if parsed, err := time.Parse(time.RFC3339, expiry); err == nil {
		t = parsed
	} else if n, err := strconv.ParseInt(expiry, 10, 64); err == nil {
		// JS `new Date(number)` treats the value as ms since epoch.
		if n > 1_000_000_000_000 {
			t = time.UnixMilli(n)
		} else {
			t = time.Unix(n, 0)
		}
	} else {
		return fmt.Errorf("invalid sigAuthExpiry %q", expiry)
	}
	if t.Before(time.Now()) {
		return fmt.Errorf("sigAuth expired")
	}
	return nil
}

func decodeSignature(sig string) ([]byte, error) {
	sig = strings.TrimSpace(sig)
	sig = strings.TrimPrefix(sig, "0x")
	raw, err := hex.DecodeString(sig)
	if err != nil {
		return nil, fmt.Errorf("invalid signature hex")
	}
	if len(raw) != 65 {
		return nil, fmt.Errorf("invalid signature length %d", len(raw))
	}
	return raw, nil
}

func recoverSigner(hash, sig []byte) (common.Address, bool) {
	normalized := make([]byte, 65)
	copy(normalized, sig)
	if normalized[64] >= 27 {
		normalized[64] -= 27
	}
	if normalized[64] != 0 && normalized[64] != 1 {
		return common.Address{}, false
	}
	pub, err := crypto.SigToPub(hash, normalized)
	if err != nil {
		return common.Address{}, false
	}
	return crypto.PubkeyToAddress(*pub), true
}

func verifySmartAccountOwnership(ctx context.Context, readRPCURL string, account, recovered common.Address, recoveredOK bool, hash, sig []byte) bool {
	if strings.TrimSpace(readRPCURL) == "" {
		return false
	}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(dialCtx, readRPCURL)
	if err != nil {
		return false
	}
	defer client.Close()

	parsed, err := abi.JSON(strings.NewReader(sigAuthAccountABI))
	if err != nil {
		return false
	}

	var hash32 [32]byte
	copy(hash32[:], hash)

	// EIP-1271 isValidSignature(hash, signature) == magic value.
	if data, err := parsed.Pack("isValidSignature", hash32, sig); err == nil {
		if out, err := callContract(ctx, client, account, data); err == nil {
			if vals, err := parsed.Unpack("isValidSignature", out); err == nil && len(vals) == 1 {
				if magic, ok := vals[0].([4]byte); ok && magic == eip1271MagicValue {
					return true
				}
			}
		}
	}

	if recoveredOK && recovered != (common.Address{}) {
		// owner() == recovered
		if data, err := parsed.Pack("owner"); err == nil {
			if out, err := callContract(ctx, client, account, data); err == nil {
				if vals, err := parsed.Unpack("owner", out); err == nil && len(vals) == 1 {
					if owner, ok := vals[0].(common.Address); ok && owner == recovered {
						return true
					}
				}
			}
		}
		// Safe isOwner(recovered)
		if data, err := parsed.Pack("isOwner", recovered); err == nil {
			if out, err := callContract(ctx, client, account, data); err == nil {
				if vals, err := parsed.Unpack("isOwner", out); err == nil && len(vals) == 1 {
					if isOwner, ok := vals[0].(bool); ok && isOwner {
						return true
					}
				}
			}
		}
	}

	return false
}

func callContract(ctx context.Context, client *ethclient.Client, to common.Address, data []byte) ([]byte, error) {
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return client.CallContract(callCtx, ethereum.CallMsg{To: &to, Data: data}, nil)
}

// encodeURIComponent matches JavaScript's encodeURIComponent: percent-encode all
// bytes except the unreserved set A-Za-z0-9 and -_.!~*'().
func encodeURIComponent(s string) string {
	const safe = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.!~*'()"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if strings.IndexByte(safe, c) >= 0 {
			b.WriteByte(c)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}

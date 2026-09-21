package bridge

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WebhookSignatureHeader carries "t=<unix ms>,v0=<base64 RSA signature>".
const WebhookSignatureHeader = "X-Webhook-Signature"

// webhookReplayWindow is how old a delivery may be before it is refused. Bridge
// retries for far longer than this, but a retry carries a fresh signature over
// a fresh timestamp, so a tight window costs nothing and stops replays cold.
const webhookReplayWindow = 10 * time.Minute

// Event is Bridge's delivery envelope. The object inside is kept raw: the
// handler never acts on the state a delivery claims, it re-reads the object
// from the API. So the envelope only needs to say what kind of thing changed
// and which one.
type Event struct {
	APIVersion       string          `json:"api_version"`
	EventID          string          `json:"event_id"`
	EventCategory    string          `json:"event_category"`
	EventType        string          `json:"event_type"`
	EventObjectID    string          `json:"event_object_id"`
	EventObjectState string          `json:"event_object_status"`
	EventObject      json.RawMessage `json:"event_object"`
	EventCreatedAt   string          `json:"event_created_at"`
}

// WebhookVerifier is the part of the client the webhook handler needs, split
// out so a fake can stand in for it in tests.
type WebhookVerifier interface {
	VerifyWebhookSignature(header string, body []byte, now time.Time) error
}

var (
	ErrWebhookNoKey        = errors.New("bridge webhook: no public key configured")
	ErrWebhookBadHeader    = errors.New("bridge webhook: malformed signature header")
	ErrWebhookStale        = errors.New("bridge webhook: timestamp outside replay window")
	ErrWebhookBadSignature = errors.New("bridge webhook: signature did not verify")
)

// VerifyWebhookSignature implements Bridge's scheme: join the millisecond
// timestamp and the raw body with a dot, SHA-256 it, then RSA-verify that
// digest against the per-webhook public key.
//
// Their reference implementation feeds the digest into an RSA-SHA256 verify,
// which hashes its input again — so the signature is over SHA256(digest), not
// the digest. Their docs do not say whether the digest is passed as raw bytes
// or as hex, so both are tried, and a plain single-hash is tried last. The
// first that verifies wins; the cost of the extra attempts is three RSA
// verifications on a bad delivery, which is nothing.
func (c *Client) VerifyWebhookSignature(header string, body []byte, now time.Time) error {
	if c == nil || c.webhookKey == "" {
		return ErrWebhookNoKey
	}
	pub, err := parseRSAPublicKey(c.webhookKey)
	if err != nil {
		return fmt.Errorf("bridge webhook: %w", err)
	}

	ts, sig, err := parseSignatureHeader(header)
	if err != nil {
		return err
	}

	sent := time.UnixMilli(ts)
	if d := now.Sub(sent); d > webhookReplayWindow || d < -webhookReplayWindow {
		return ErrWebhookStale
	}

	digest := sha256.Sum256(append([]byte(strconv.FormatInt(ts, 10)+"."), body...))

	candidates := [][]byte{
		hashOf(digest[:]), // RSA-SHA256 over raw digest bytes
		hashOf([]byte(hex.EncodeToString(digest[:]))), // RSA-SHA256 over the hex digest string
		digest[:], // signature directly over the digest
	}
	for _, hashed := range candidates {
		if rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashed, sig) == nil {
			return nil
		}
	}
	return ErrWebhookBadSignature
}

func hashOf(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

func parseSignatureHeader(header string) (int64, []byte, error) {
	var tsPart, sigPart string
	for _, piece := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(piece), "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "t":
			tsPart = strings.TrimSpace(v)
		case "v0":
			sigPart = strings.TrimSpace(v)
		}
	}
	if tsPart == "" || sigPart == "" {
		return 0, nil, ErrWebhookBadHeader
	}
	ts, err := strconv.ParseInt(tsPart, 10, 64)
	if err != nil {
		return 0, nil, ErrWebhookBadHeader
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(sigPart)
	if err != nil {
		return 0, nil, ErrWebhookBadHeader
	}
	return ts, sig, nil
}

func parseRSAPublicKey(pemText string) (*rsa.PublicKey, error) {
	// Env files tend to flatten PEM newlines into literal "\n"; accept both.
	normalized := strings.ReplaceAll(strings.TrimSpace(pemText), `\n`, "\n")
	block, _ := pem.Decode([]byte(normalized))
	if block == nil {
		return nil, errors.New("public key is not PEM")
	}
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PublicKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("public key is not RSA")
	}
	if rsaKey, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return rsaKey, nil
	}
	return nil, errors.New("public key could not be parsed")
}

// ParseEvent decodes a delivery body. Kept separate from verification so the
// handler can acknowledge a signed-but-unparseable body without retrying it.
func ParseEvent(body []byte) (*Event, error) {
	var ev Event
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

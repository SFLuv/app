package w9provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// WebhookVerifier is implemented by providers whose callbacks are signed.
//
// Deliberately narrow, and deliberately optional. The credentials that verify a
// delivery are the same ones that authenticate our API calls, so they belong to
// the provider and not to an HTTP handler — a handler that read the client
// secret out of the environment itself would be a second place to keep it in
// step. A provider that does not implement this simply has no callback route.
type WebhookVerifier interface {
	VerifyWebhookSignature(timestamp, signature string, now time.Time) bool
}

// SignWebhook produces the value TaxBandits sends in the `Signature` header.
//
//	base64( HMAC-SHA256( ClientId + "\n" + TimeStamp, ClientSecret ) )
//
// Exported because the Fake has to be able to produce a real one. A stand-in
// that skipped the signature would leave the only branch that matters — the
// rejection — untested until production, which is the same mistake as mirroring
// a guessed API.
func SignWebhook(clientID, clientSecret, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(clientSecret))
	mac.Write([]byte(clientID + "\n" + timestamp))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// WebhookMaxSkew bounds how old a delivery may be.
//
// The timestamp is signed, so it cannot be edited without invalidating the
// signature — which is what makes it useful: it turns a captured-and-replayed
// delivery into one that expires. Generous, because their retries span 24 hours
// and a legitimate retry must still be accepted.
const WebhookMaxSkew = 24 * time.Hour

// webhookTimestampLayout is the format TaxBandits actually sends.
//
//	08/26/2026 05:12:04.029 PM
//
// Captured from a live sandbox delivery, not read off a page. This was written
// as unix seconds first — a reasonable guess, and wrong — which meant a
// correctly signed callback was refused on the age check it had already
// passed the signature for. The console reports that as "invalid URL", which
// says nothing about which half failed.
//
// Their clock is UTC: a delivery stamped 05:12 PM arrived at 10:12 Pacific.
const webhookTimestampLayout = "01/02/2006 03:04:05.000 PM"

// parseWebhookTimestamp reads a delivery's timestamp.
//
// Accepts unix seconds as well, because the signature covers whatever string
// was sent — so anything producing these deliveries, including our own
// stand-in, is free to use either and the HMAC still ties the two together.
func parseWebhookTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.ParseInLocation(webhookTimestampLayout, raw, time.UTC); err == nil {
		return t, true
	}
	if secs, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(secs, 0), true
	}
	return time.Time{}, false
}

// VerifyWebhook reports whether a delivery is genuinely from the vendor.
//
// Compared with hmac.Equal rather than ==, so the comparison does not leak
// where two signatures first differ. Returns false rather than an error for
// every failure: a caller must not be able to accidentally act on one kind of
// bad delivery by only checking some of them.
func VerifyWebhook(clientID, clientSecret, timestamp, signature string, now time.Time) bool {
	if clientSecret == "" || clientID == "" ||
		strings.TrimSpace(timestamp) == "" || strings.TrimSpace(signature) == "" {
		return false
	}

	expected := SignWebhook(clientID, clientSecret, timestamp)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return false
	}

	sent, ok := parseWebhookTimestamp(timestamp)
	if !ok {
		// A value we cannot read is a delivery we cannot age-check, so it is
		// refused rather than waved through.
		return false
	}
	age := now.Sub(sent)
	if age < 0 {
		age = -age
	}
	return age <= WebhookMaxSkew
}

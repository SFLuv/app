package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/w9provider"
)

// webhookBodyLimit caps what we will read from an unauthenticated socket.
//
// The signature is over the headers, not the body, so anyone can make us read
// until we decide otherwise. The real payload is a few hundred bytes.
const webhookBodyLimit = 64 << 10

// webhookAckDeadline is how long the work behind an acknowledged delivery gets.
//
// Detached from the request context on purpose: we answer in milliseconds and
// the vendor hangs up, but the sync behind it makes an API call and may move
// money. Tying it to the request would cancel that mid-flight.
const webhookAckDeadline = 2 * time.Minute

// tbWebhookPayload is the little we take from the body.
//
// Only the SubmissionId is read. The status in the payload is deliberately
// ignored — not because a delivery cannot be trusted once its signature checks
// out, but because there should be exactly one piece of code that decides what
// a vendor status means, and it already exists.
type tbWebhookPayload struct {
	SubmissionID string `json:"SubmissionId"`
	Data         struct {
		SubmissionID string `json:"SubmissionId"`
	} `json:"Data"`
}

func (p tbWebhookPayload) submissionID() string {
	if id := strings.TrimSpace(p.SubmissionID); id != "" {
		return id
	}
	return strings.TrimSpace(p.Data.SubmissionID)
}

// ReceiveW9Webhook handles the vendor's Form W-9 Status Change callback.
//
// Unauthenticated in the routing sense — there is no session behind it — but
// not unauthenticated: every delivery carries an HMAC over the client id and a
// timestamp, keyed on the client secret, and one that does not verify is
// refused before anything reads its body.
//
// Three hard constraints come from their side, and each shapes something here:
//
//   - 200 within 5 seconds, or it counts as a timeout. So the acknowledgement
//     is sent first and the work runs after, detached.
//   - Retried up to 9 times over 24 hours. So the work must be idempotent —
//     SyncFilingFromProvider is, and a duplicate costs one API read.
//   - The subscription only activates once the URL answers 200 to a sample
//     POST. So a delivery naming no submission we know is still a 200: it is
//     how their sample is acknowledged, and refusing it would leave the
//     webhook unsavable.
func (a *AppService) ReceiveW9Webhook(w http.ResponseWriter, r *http.Request) {
	verifier, ok := a.w9Provider.(w9provider.WebhookVerifier)
	if !ok {
		// No signed callbacks from this provider, so nothing here can be
		// trusted. 404 rather than 401: the route effectively does not exist.
		w.WriteHeader(http.StatusNotFound)
		return
	}

	timestamp := r.Header.Get("TimeStamp")
	signature := r.Header.Get("Signature")
	if !verifier.VerifyWebhookSignature(timestamp, signature, time.Now()) {
		// Their documented response for a mismatch, and no body: an attacker
		// probing this should learn nothing beyond "no".
		if a.logger != nil {
			a.logger.Logf("w9 webhook: refused a delivery with a bad signature from %s", r.RemoteAddr)
		}
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, webhookBodyLimit))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var payload tbWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		// Signed but unreadable. Acknowledged anyway: retrying it nine times
		// will not make it parse, and their sample post is not the real shape.
		w.WriteHeader(http.StatusOK)
		return
	}

	submissionID := payload.submissionID()
	// Acknowledge before doing anything slow. Everything below this line runs
	// on a connection the vendor has already been released from.
	w.WriteHeader(http.StatusOK)

	if submissionID == "" || a.payouts == nil {
		return
	}

	go a.syncFilingForSubmission(submissionID)
}

// syncFilingForSubmission is the work an acknowledged delivery triggers.
func (a *AppService) syncFilingForSubmission(submissionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), webhookAckDeadline)
	defer cancel()

	filing, err := a.db.GetW9FilingByProviderRequestID(ctx, submissionID)
	if err != nil {
		if a.logger != nil {
			a.logger.Logf("w9 webhook: could not load the filing for %s: %s", submissionID, err)
		}
		return
	}
	if filing == nil {
		// Their activation sample, or a submission from another environment
		// pointed at this URL. Neither is an error.
		return
	}

	if err := a.payouts.SyncFilingFromProvider(ctx, filing); err != nil && a.logger != nil {
		a.logger.Logf("w9 webhook: could not sync filing %s: %s", submissionID, err)
	}
}

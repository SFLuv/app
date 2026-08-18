package w9provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Fake is an in-process stand-in for a real tax vendor.
//
// It exists so the whole loop — request a form, open it, submit it, receive the
// webhook, release the escrow, watch the money land on the local chain — can be
// exercised on a laptop without a vendor account. That loop is the part most
// likely to be wrong, and it is exactly the part a unit test cannot reach.
//
// The signature scheme is the same HMAC shape a real vendor uses, so
// VerifyWebhook is genuinely tested rather than stubbed out.
type Fake struct {
	secret  []byte
	baseURL string

	mu       sync.Mutex
	requests map[string]*fakeRequest
	seq      int
}

type fakeRequest struct {
	ID          string
	UserID      string
	TaxYear     int
	Status      string
	CompletedAt *time.Time
	TINType     string
}

func NewFake(cfg Config) *Fake {
	secret := cfg.WebhookSecret
	if secret == "" {
		secret = "fake-w9-secret"
	}
	base := strings.TrimSuffix(cfg.BaseURL, "/")
	if base == "" {
		base = "http://localhost:8080"
	}
	return &Fake{
		secret:   []byte(secret),
		baseURL:  base,
		requests: map[string]*fakeRequest{},
	}
}

func (f *Fake) Name() string { return "fake" }

// EnsurePayee mirrors the real contract: idempotent on our user id, so a retry
// cannot create a second payee.
func (f *Fake) EnsurePayee(_ context.Context, in PayeeInput) (PayeeResult, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return PayeeResult{}, fmt.Errorf("user id is required to create a payee")
	}
	return PayeeResult{ProviderPayeeID: "fake-payee-" + in.UserID}, nil
}

func (f *Fake) CreateW9Request(_ context.Context, in W9RequestInput) (W9Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.seq++
	id := fmt.Sprintf("fake-req-%d", f.seq)
	f.requests[id] = &fakeRequest{ID: id, UserID: in.UserID, TaxYear: in.TaxYear, Status: StatusSent}

	return W9Request{
		ProviderRequestID: id,
		FormURL:           f.formURL(id, in.ReturnURL),
		FormURLExpiresAt:  time.Now().Add(24 * time.Hour),
	}, nil
}

func (f *Fake) HostedFormURL(_ context.Context, providerRequestID string, returnURL string) (W9Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	req, ok := f.requests[providerRequestID]
	if !ok {
		return W9Request{}, fmt.Errorf("unknown w9 request %q", providerRequestID)
	}
	if req.Status == StatusSent {
		req.Status = StatusOpened
	}

	return W9Request{
		ProviderRequestID: providerRequestID,
		FormURL:           f.formURL(providerRequestID, returnURL),
		FormURLExpiresAt:  time.Now().Add(24 * time.Hour),
	}, nil
}

func (f *Fake) formURL(id string, returnURL string) string {
	url := fmt.Sprintf("%s/w9/fake/form/%s", f.baseURL, id)
	if strings.TrimSpace(returnURL) != "" {
		url += "?return_url=" + returnURL
	}
	return url
}

func (f *Fake) GetW9Status(_ context.Context, providerRequestID string) (W9Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	req, ok := f.requests[providerRequestID]
	if !ok {
		return W9Status{Status: StatusNotStarted}, nil
	}
	return W9Status{Status: req.Status, CompletedAt: req.CompletedAt, TINType: req.TINType}, nil
}

// Complete is what the stub form's submit button calls. It marks the request
// done and returns a correctly signed webhook body, which the caller posts back
// to the real webhook route — so the production code path is what runs, not a
// shortcut around it.
func (f *Fake) Complete(providerRequestID string, tinType string) ([]byte, http.Header, error) {
	f.mu.Lock()
	req, ok := f.requests[providerRequestID]
	if !ok {
		f.mu.Unlock()
		return nil, nil, fmt.Errorf("unknown w9 request %q", providerRequestID)
	}
	now := time.Now().UTC()
	req.Status = StatusCompleted
	req.CompletedAt = &now
	if tinType == "" {
		tinType = "ssn"
	}
	req.TINType = tinType
	userID := req.UserID
	f.mu.Unlock()

	body, err := json.Marshal(map[string]any{
		"event_id":   "fake-evt-" + providerRequestID + "-completed",
		"type":       "w9.completed",
		"request_id": providerRequestID,
		"status":     StatusCompleted,
		"user_id":    userID,
		"tin_type":   tinType,
		"created_at": now.Format(time.RFC3339),
	})
	if err != nil {
		return nil, nil, err
	}

	header := http.Header{}
	header.Set("Content-Type", "application/json")
	header.Set(fakeSignatureHeader, f.sign(body))
	return body, header, nil
}

const fakeSignatureHeader = "X-W9-Signature"

func (f *Fake) sign(body []byte) string {
	mac := hmac.New(sha256.New, f.secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func (f *Fake) VerifyWebhook(h http.Header, rawBody []byte) (WebhookEvent, error) {
	got := h.Get(fakeSignatureHeader)
	if got == "" {
		return WebhookEvent{}, fmt.Errorf("missing %s header", fakeSignatureHeader)
	}
	// Constant time, because a signature check that leaks timing is not a
	// signature check.
	if !hmac.Equal([]byte(got), []byte(f.sign(rawBody))) {
		return WebhookEvent{}, fmt.Errorf("invalid webhook signature")
	}

	var payload struct {
		EventID   string `json:"event_id"`
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return WebhookEvent{}, fmt.Errorf("unreadable webhook body: %w", err)
	}
	if payload.EventID == "" {
		return WebhookEvent{}, fmt.Errorf("webhook body has no event id")
	}

	return WebhookEvent{
		EventID:           payload.EventID,
		Type:              payload.Type,
		ProviderRequestID: payload.RequestID,
		Status:            payload.Status,
		Raw:               rawBody,
	}, nil
}

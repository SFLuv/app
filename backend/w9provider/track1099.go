package w9provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Track1099 talks to Track1099 (Avalara).
//
// It was chosen because the same payee record that collects a W9 can later be
// used to e-file a 1099-NEC, so turning filing on at year end is configuration
// rather than a second integration.
//
// Everything here is deliberately shallow. The vendor holds the TIN; we hold a
// request id and a status. If a future field looks like it might carry a tax
// identification number, it does not belong in this file.
type Track1099 struct {
	apiKey        string
	baseURL       string
	webhookSecret string
	environment   string
	client        *http.Client
}

func NewTrack1099(cfg Config) *Track1099 {
	base := strings.TrimSuffix(cfg.BaseURL, "/")
	if base == "" {
		base = "https://www.track1099.com/api"
	}
	return &Track1099{
		apiKey:        cfg.APIKey,
		baseURL:       base,
		webhookSecret: cfg.WebhookSecret,
		environment:   cfg.Environment,
		// A tax vendor being slow must not hold a database transaction open, so
		// every call is bounded. Callers do their provider work outside their
		// transactions for the same reason.
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (t *Track1099) Name() string { return "track1099" }

func (t *Track1099) do(ctx context.Context, method string, path string, body any, out any) error {
	if strings.TrimSpace(t.apiKey) == "" {
		return ErrProviderDisabled
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("error encoding %s %s: %w", method, path, err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, t.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("error building %s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("error calling %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("error reading %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Truncated: a vendor error body can be large, and it may echo back
		// whatever was submitted — which on this vendor can include a TIN.
		snippet := string(payload)
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return fmt.Errorf("%s %s returned %d: %s", method, path, resp.StatusCode, snippet)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("error decoding %s %s: %w", method, path, err)
	}
	return nil
}

// EnsurePayee keys the vendor record on our user id. That is what makes this
// idempotent, and what lets a 1099 be filed later against the same record.
func (t *Track1099) EnsurePayee(ctx context.Context, in PayeeInput) (PayeeResult, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return PayeeResult{}, fmt.Errorf("user id is required to create a payee")
	}

	var existing struct {
		Recipients []struct {
			ID string `json:"id"`
		} `json:"recipients"`
	}
	if err := t.do(ctx, http.MethodGet, "/recipients?reference="+in.UserID, nil, &existing); err == nil {
		if len(existing.Recipients) > 0 && existing.Recipients[0].ID != "" {
			return PayeeResult{ProviderPayeeID: existing.Recipients[0].ID}, nil
		}
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := t.do(ctx, http.MethodPost, "/recipients", map[string]any{
		"reference": in.UserID,
		"email":     in.Email,
		"name":      in.Name,
	}, &created); err != nil {
		return PayeeResult{}, err
	}
	if created.ID == "" {
		return PayeeResult{}, fmt.Errorf("provider returned no payee id")
	}
	return PayeeResult{ProviderPayeeID: created.ID}, nil
}

func (t *Track1099) CreateW9Request(ctx context.Context, in W9RequestInput) (W9Request, error) {
	var created struct {
		ID        string `json:"id"`
		URL       string `json:"url"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := t.do(ctx, http.MethodPost, "/w9_requests", map[string]any{
		"recipient_id": in.ProviderPayeeID,
		"reference":    fmt.Sprintf("%s:%d", in.UserID, in.TaxYear),
		"email":        in.Email,
		"tax_year":     in.TaxYear,
		"return_url":   in.ReturnURL,
	}, &created); err != nil {
		return W9Request{}, err
	}
	if created.ID == "" {
		return W9Request{}, fmt.Errorf("provider returned no w9 request id")
	}
	return W9Request{
		ProviderRequestID: created.ID,
		FormURL:           created.URL,
		FormURLExpiresAt:  parseProviderTime(created.ExpiresAt),
	}, nil
}

// HostedFormURL asks for a fresh link every time rather than replaying a stored
// one. These links expire, and a person who taps "complete your tax form" three
// days later must not land on a dead page.
func (t *Track1099) HostedFormURL(ctx context.Context, providerRequestID string, returnURL string) (W9Request, error) {
	var link struct {
		URL       string `json:"url"`
		ExpiresAt string `json:"expires_at"`
	}
	path := "/w9_requests/" + providerRequestID + "/link"
	if strings.TrimSpace(returnURL) != "" {
		path += "?return_url=" + returnURL
	}
	if err := t.do(ctx, http.MethodPost, path, nil, &link); err != nil {
		return W9Request{}, err
	}
	return W9Request{
		ProviderRequestID: providerRequestID,
		FormURL:           link.URL,
		FormURLExpiresAt:  parseProviderTime(link.ExpiresAt),
	}, nil
}

func (t *Track1099) GetW9Status(ctx context.Context, providerRequestID string) (W9Status, error) {
	var status struct {
		Status      string `json:"status"`
		CompletedAt string `json:"completed_at"`
		TINType     string `json:"tin_type"`
		TINMatch    string `json:"tin_match_status"`
	}
	if err := t.do(ctx, http.MethodGet, "/w9_requests/"+providerRequestID, nil, &status); err != nil {
		return W9Status{}, err
	}

	result := W9Status{
		Status:   normaliseTrack1099Status(status.Status),
		TINType:  status.TINType,
		TINMatch: status.TINMatch,
	}
	if completed := parseProviderTime(status.CompletedAt); !completed.IsZero() {
		result.CompletedAt = &completed
	}
	return result, nil
}

// normaliseTrack1099Status maps the vendor's vocabulary onto ours. Keeping this
// mapping inside the adapter is the reason the rest of the system never learns
// a vendor-specific word.
func normaliseTrack1099Status(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "completed", "complete", "signed", "submitted":
		return StatusCompleted
	case "opened", "viewed":
		return StatusOpened
	case "sent", "requested", "pending":
		return StatusSent
	case "invalid", "rejected", "failed":
		return StatusInvalid
	default:
		return StatusNotStarted
	}
}

func parseProviderTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

const track1099SignatureHeader = "X-Track1099-Signature"

func (t *Track1099) VerifyWebhook(h http.Header, rawBody []byte) (WebhookEvent, error) {
	if strings.TrimSpace(t.webhookSecret) == "" {
		// Refusing unsigned webhooks is the point. Without a secret anyone who
		// finds the URL can mark a filing complete and release held money.
		return WebhookEvent{}, fmt.Errorf("no webhook secret is configured")
	}

	provided := h.Get(track1099SignatureHeader)
	if provided == "" {
		return WebhookEvent{}, fmt.Errorf("missing %s header", track1099SignatureHeader)
	}

	mac := hmac.New(sha256.New, []byte(t.webhookSecret))
	mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(provided), []byte(expected)) {
		return WebhookEvent{}, fmt.Errorf("invalid webhook signature")
	}

	var payload struct {
		EventID   string `json:"event_id"`
		Type      string `json:"type"`
		RequestID string `json:"w9_request_id"`
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
		Status:            normaliseTrack1099Status(payload.Status),
		Raw:               rawBody,
	}, nil
}

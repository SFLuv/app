package w9provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Track1099 talks to Track1099 (Avalara 1099 & W-9).
//
// Shaped against the vendor's published docs — see TRACK1099-API-NOTES.md in
// this directory, which records what was verified and what is still unknown.
// An earlier version of this file was written against a guessed API: it
// compiled, its tests passed, and essentially none of its endpoints existed.
// Prefer leaving a gap here over inventing a plausible one.
//
// Three things about this vendor drive the design:
//
//   - Every path is scoped by a Team API ID: /api/v1/{team_api_id}/…
//   - W-9 collection is the Form Requests API. Payers are "issuers"; there is
//     no JSON recipients resource, and reference_id — which we choose — is both
//     the join key and the idempotency mechanism.
//   - There are no webhooks and no sandbox. Completion is discovered by polling.
type Track1099 struct {
	apiKey      string
	baseURL     string
	teamAPIID   string
	environment string
	client      *http.Client
}

func NewTrack1099(cfg Config) *Track1099 {
	base := strings.TrimSuffix(cfg.BaseURL, "/")
	if base == "" {
		base = "https://www.track1099.com"
	}
	return &Track1099{
		apiKey:    cfg.APIKey,
		baseURL:   base,
		teamAPIID: strings.TrimSpace(cfg.TeamAPIID),
		// A tax vendor being slow must not hold a database transaction open, so
		// every call is bounded and callers do provider work outside their
		// transactions.
		environment: cfg.Environment,
		client:      &http.Client{Timeout: 20 * time.Second},
	}
}

func (t *Track1099) Name() string { return "track1099" }

// path builds a team-scoped URL. Every documented endpoint sits under
// /api/v1/{team_api_id}, and omitting either segment 404s.
func (t *Track1099) path(suffix string) string {
	return fmt.Sprintf("%s/api/v1/%s%s", t.baseURL, url.PathEscape(t.teamAPIID), suffix)
}

func (t *Track1099) do(ctx context.Context, method string, suffix string, body any, out any) error {
	if strings.TrimSpace(t.apiKey) == "" {
		return ErrProviderDisabled
	}
	if t.teamAPIID == "" {
		return fmt.Errorf("W9_PROVIDER_TEAM_ID is not set; every Track1099 path is scoped by a team id")
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("error encoding %s %s: %w", method, suffix, err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, t.path(suffix), reader)
	if err != nil {
		return fmt.Errorf("error building %s %s: %w", method, suffix, err)
	}
	// The docs describe an API token entered through the Swagger "Authorize"
	// button but never state the header name. Bearer is the assumption; this is
	// one of the open questions in the notes and needs a real account to settle.
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("error calling %s %s: %w", method, suffix, err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("error reading %s %s: %w", method, suffix, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Truncated: an error body can be large and may echo back what was
		// submitted, which on this vendor can include a tax identification
		// number.
		snippet := string(payload)
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return fmt.Errorf("%s %s returned %d: %s", method, suffix, resp.StatusCode, snippet)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("error decoding %s %s: %w", method, suffix, err)
	}
	return nil
}

// EnsurePayee is a local no-op for this vendor.
//
// There is no recipients resource in the W-9 flow to create anything against.
// Identity is carried by reference_id on the form request itself, so the payee
// id we hand back is just our own user id. The method stays on the interface
// because it is the right shape for vendors that do keep a payee record, and
// the idempotency contract still holds — reusing a reference_id surfaces the
// prior submission rather than creating a second one.
func (t *Track1099) EnsurePayee(_ context.Context, in PayeeInput) (PayeeResult, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return PayeeResult{}, fmt.Errorf("user id is required to identify a payee")
	}
	return PayeeResult{ProviderPayeeID: in.UserID}, nil
}

// referenceID is the join key and the idempotency mechanism, chosen by us.
// Scoping it to the tax year keeps a fresh W-9 per year without colliding.
func referenceID(userID string, taxYear int) string {
	return fmt.Sprintf("sfluv:%s:%d", userID, taxYear)
}

type formRequestResponse struct {
	ReferenceID    string `json:"reference_id"`
	SignedPDF      string `json:"signed_pdf"`
	SignedAt       string `json:"signed_at"`
	TINMatchStatus string `json:"tin_match_status"`
	TINType        string `json:"tin_type"`
	FormURL        string `json:"form_url"`
}

func (t *Track1099) CreateW9Request(ctx context.Context, in W9RequestInput) (W9Request, error) {
	var created formRequestResponse
	if err := t.do(ctx, http.MethodPost, "/form_requests", map[string]any{
		"reference_id": referenceID(in.UserID, in.TaxYear),
		"email":        in.Email,
		"form_type":    "W-9",
	}, &created); err != nil {
		return W9Request{}, err
	}
	return t.toRequest(created, referenceID(in.UserID, in.TaxYear)), nil
}

// HostedFormURL re-reads the form request to get a fresh link.
//
// signed_pdf expires after an hour, so a stored URL is dead by the time someone
// taps "complete your tax form" the next day. Whether the collection step is a
// redirect at all is unsettled — the docs pair the Form Requests API with an
// embedded JavaScript widget, which would mean handing the browser a token
// rather than a URL. That decision changes the client flow, so it is flagged in
// the notes rather than guessed at here.
func (t *Track1099) HostedFormURL(ctx context.Context, providerRequestID string, _ string) (W9Request, error) {
	var current formRequestResponse
	if err := t.do(ctx, http.MethodGet, "/form_requests/"+url.PathEscape(providerRequestID), nil, &current); err != nil {
		return W9Request{}, err
	}
	return t.toRequest(current, providerRequestID), nil
}

func (t *Track1099) toRequest(r formRequestResponse, fallbackRef string) W9Request {
	ref := r.ReferenceID
	if ref == "" {
		ref = fallbackRef
	}
	link := r.FormURL
	if link == "" {
		link = r.SignedPDF
	}
	return W9Request{
		ProviderRequestID: ref,
		FormURL:           link,
		// Stated by the vendor as 3600s. Held as an absolute time so a caller
		// can tell a stale link from a live one.
		FormURLExpiresAt: time.Now().Add(time.Hour),
	}
}

// GetW9Status reports whether the form is signed, and separately how the TIN
// match is going.
//
// Completion is signed_at alone. The TIN match resolves asynchronously — up to
// about a day — and gating on it would hold somebody's money long after they
// did everything asked of them. A rejected match is handled afterwards, as a
// re-collection task, not by clawing anything back.
func (t *Track1099) GetW9Status(ctx context.Context, providerRequestID string) (W9Status, error) {
	var current formRequestResponse
	if err := t.do(ctx, http.MethodGet, "/form_requests/"+url.PathEscape(providerRequestID), nil, &current); err != nil {
		return W9Status{}, err
	}

	result := W9Status{
		Status:   StatusSent,
		TINType:  current.TINType,
		TINMatch: normaliseTINMatch(current.TINMatchStatus),
	}
	if signed := parseProviderTime(current.SignedAt); !signed.IsZero() {
		result.Status = StatusCompleted
		result.CompletedAt = &signed
	}
	return result, nil
}

// normaliseTINMatch maps the vendor's vocabulary onto ours, so nothing above
// this package learns a vendor-specific word.
func normaliseTINMatch(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "matched", "match", "valid":
		return TINMatchMatched
	case "rejected", "mismatch", "invalid", "failed":
		return TINMatchRejected
	case "pending", "processing", "in_progress":
		return TINMatchPending
	default:
		return ""
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

// Package w9provider talks to whichever service collects and stores W9 forms.
//
// The point of the boundary is that a tax identification number never crosses
// it. The vendor collects the form, holds the TIN in its own vault, and hands
// back an opaque payee id and a status. Everything above this package works in
// terms of "has this person filed for this year", which is the only fact the
// rest of the system needs.
//
// That also means a vendor change is a new file in this package rather than a
// migration: nothing outside knows who the provider is.
package w9provider

import (
	"context"
	"strings"
	"time"
)

// How completion is discovered.
//
// It is polled, not pushed. The vendor we integrate with publishes no webhook,
// callback or notification of any kind — verified against its docs and every
// changelog entry from 0.1.0 to 0.7.0 — so GetW9Status is not a backstop for
// dropped deliveries, it is the only path. An earlier version of this package
// carried a webhook receiver with an invented signature header; nothing would
// ever have called it, and code that looks live but is unreachable is worse
// than code that is absent.

// Filing states, normalised across vendors. A vendor's own vocabulary stays in
// its adapter.
const (
	StatusNotStarted = "not_started"
	StatusSent       = "sent"
	StatusOpened     = "opened"
	StatusCompleted  = "completed"
	StatusInvalid    = "invalid"
)

// TIN match outcomes. Asynchronous and independent of whether the form is
// signed: a match can still be pending a day after completion.
const (
	TINMatchPending  = "pending"
	TINMatchMatched  = "matched"
	TINMatchRejected = "rejected"
)

// PayeeInput is what a vendor needs to open a payee record. Deliberately thin:
// an email to send to, and a name only if we happen to have one. Everything
// that actually identifies a taxpayer is collected by the vendor, from the
// person, on their own form.
type PayeeInput struct {
	UserID string
	Email  string
	Name   string
}

type PayeeResult struct {
	ProviderPayeeID string
}

type W9RequestInput struct {
	UserID          string
	ProviderPayeeID string
	TaxYear         int
	Email           string
	// ReturnURL is where the person lands once the form is submitted. On mobile
	// this is an app scheme, which is what closes the loop back into the app.
	ReturnURL string
	// PreferredLanguage is a hint, not a guarantee — vendors support their own
	// short list and fall back to English. Worth passing given who we serve.
	PreferredLanguage string
	// ExistingFormURL is the link we already hold for this filing, if any.
	//
	// It exists because vendors disagree about whether a hosted link is durable.
	// One whose links expire in an hour must re-read on every tap; one whose
	// links do not expire must NOT, because re-requesting is how you end up
	// with a pile of duplicate submissions. The adapter decides; the caller
	// just supplies what it has.
	ExistingFormURL string
}

type W9Request struct {
	ProviderRequestID string
	FormURL           string
	FormURLExpiresAt  time.Time
}

// W9Status is the vendor's answer to "has this person filed".
//
// TINType records whether the vendor holds an SSN or an EIN, because that is
// reportable and harmless. The TIN itself is never returned, and no field here
// should ever be widened to carry it.
type W9Status struct {
	Status      string
	CompletedAt *time.Time
	TINType     string
	// TINMatch is the vendor's IRS TIN-matching verdict where offered. Empty
	// when the vendor does not do it.
	TINMatch string
}

// Provider is the whole surface. Adapters must be safe for concurrent use.
type Provider interface {
	Name() string

	// EnsurePayee is idempotent on our UserID: calling it twice returns the same
	// payee rather than creating a second one. That property is what lets a
	// retry, a re-request, and next year's 1099 all land on one vendor record.
	EnsurePayee(ctx context.Context, in PayeeInput) (PayeeResult, error)

	CreateW9Request(ctx context.Context, in W9RequestInput) (W9Request, error)

	// HostedFormURL returns a fresh link for a request that already exists.
	// Hosted links expire, so this is called when someone opens the form rather
	// than being read from a stored column.
	//
	// It takes the whole input rather than just a return URL because vendors
	// differ in what re-minting needs: some read by their own request id, others
	// re-issue against the payee reference and email we chose. Passing the same
	// input CreateW9Request received covers both without the adapter having to
	// smuggle identifiers through the request id string.
	HostedFormURL(ctx context.Context, providerRequestID string, in W9RequestInput) (W9Request, error)

	// GetW9Status is the only way completion is ever learned. The sweeper polls
	// it for every outstanding filing, and keeps polling past completion until
	// the TIN match resolves — that check is asynchronous and can land a day
	// after the form is signed.
	GetW9Status(ctx context.Context, providerRequestID string) (W9Status, error)
}

// Config is read once at startup.
//
// It carries the union of what the adapters need. Track1099 uses APIKey and
// TeamAPIID; TaxBandits uses the client credential triple plus BusinessID and
// APIVersion. Environment selects the sandbox or production host pair for
// whichever is selected, and is the only field both care about.
type Config struct {
	Provider string

	// Track1099.
	APIKey string
	// TeamAPIID scopes every Track1099 path: /api/v1/{team_api_id}/…. Without
	// it every real call 404s, so it is not optional for that vendor.
	TeamAPIID string

	// TaxBandits. Auth is a JWS we sign with ClientSecret, naming ClientID as
	// both issuer and subject and UserToken as the audience.
	ClientID     string
	ClientSecret string
	UserToken    string
	// BusinessID is the payer GUID from Business/Create. Nothing can be
	// requested until the payer exists, so an empty value is a hard error
	// rather than a silent disable.
	BusinessID string
	// APIVersion is a path segment, e.g. "v1.7.3". Pinned in config because the
	// request shapes differ between major trees and mixing them silently
	// produces 404s.
	APIVersion string
	// AuthURL is the token exchange endpoint, on a different domain from the
	// API host. Empty selects the documented default for Environment.
	AuthURL string

	BaseURL     string
	Environment string
}

// New picks an adapter. An unconfigured or unknown provider yields a disabled
// one rather than an error: money must still be held correctly when the vendor
// is unreachable, and refusing to boot over a tax integration would take the
// whole platform down with it.
func New(cfg Config) Provider {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "taxbandits":
		return NewTaxBandits(cfg)
	case "track1099":
		// Kept as the fallback while TaxBandits Go Live approval is outstanding.
		// It is known to be wrong in its details — see TRACK1099-API-NOTES.md —
		// and should be deleted once TaxBandits is live.
		return NewTrack1099(cfg)
	case "fake":
		return NewFake(cfg)
	default:
		return disabled{}
	}
}

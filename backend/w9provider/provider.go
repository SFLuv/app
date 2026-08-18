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
	"encoding/json"
	"net/http"
	"time"
)

// Filing states, normalised across vendors. A vendor's own vocabulary stays in
// its adapter.
const (
	StatusNotStarted = "not_started"
	StatusSent       = "sent"
	StatusOpened     = "opened"
	StatusCompleted  = "completed"
	StatusInvalid    = "invalid"
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

type WebhookEvent struct {
	EventID           string
	Type              string
	ProviderRequestID string
	Status            string
	Raw               json.RawMessage
}

// Provider is the whole surface. Adapters must be safe for concurrent use.
type Provider interface {
	Name() string

	// EnsurePayee is idempotent on our UserID: calling it twice returns the same
	// payee rather than creating a second one. That property is what lets a
	// retry, a re-request, and next year's 1099 all land on one vendor record.
	EnsurePayee(ctx context.Context, in PayeeInput) (PayeeResult, error)

	CreateW9Request(ctx context.Context, in W9RequestInput) (W9Request, error)

	// HostedFormURL returns a fresh link. Hosted links expire, so this is called
	// when someone opens the form rather than being read from a stored column.
	HostedFormURL(ctx context.Context, providerRequestID string, returnURL string) (W9Request, error)

	// GetW9Status is the guarantee. Webhooks make completion feel instant, but a
	// dropped delivery must never leave money held forever, so a sweeper polls
	// this for every outstanding filing.
	GetW9Status(ctx context.Context, providerRequestID string) (W9Status, error)

	VerifyWebhook(h http.Header, rawBody []byte) (WebhookEvent, error)
}

// Config is read once at startup.
type Config struct {
	Provider      string
	APIKey        string
	BaseURL       string
	WebhookSecret string
	Environment   string
}

// New picks an adapter. An unconfigured or unknown provider yields a disabled
// one rather than an error: money must still be held correctly when the vendor
// is unreachable, and refusing to boot over a tax integration would take the
// whole platform down with it.
func New(cfg Config) Provider {
	switch cfg.Provider {
	case "track1099":
		return NewTrack1099(cfg)
	case "fake":
		return NewFake(cfg)
	default:
		return disabled{}
	}
}

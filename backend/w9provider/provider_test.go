package w9provider

import (
	"context"
	"net/http"
	"testing"
)

// Every adapter must satisfy the interface, including the disabled one — that
// is what lets the rest of the system hold a Provider unconditionally.
func TestAdaptersSatisfyTheInterface(t *testing.T) {
	var _ Provider = NewFake(Config{})
	var _ Provider = NewTrack1099(Config{})
	var _ Provider = disabled{}
}

// A webhook is the instruction to release someone's held money, so an unsigned
// or wrongly signed one must never be honoured.
func TestWebhookSignatureIsEnforced(t *testing.T) {
	fake := NewFake(Config{WebhookSecret: "s3cret"})
	if _, err := fake.CreateW9Request(context.Background(), W9RequestInput{UserID: "u1", TaxYear: 2026}); err != nil {
		t.Fatalf("create: %v", err)
	}
	body, header, err := fake.Complete("fake-req-1", "ssn")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if _, err := fake.VerifyWebhook(header, body); err != nil {
		t.Fatalf("a correctly signed webhook was rejected: %v", err)
	}

	tampered := append([]byte{}, body...)
	tampered[len(tampered)-2] = '0'
	if _, err := fake.VerifyWebhook(header, tampered); err == nil {
		t.Fatal("a tampered body was accepted")
	}

	if _, err := fake.VerifyWebhook(http.Header{}, body); err == nil {
		t.Fatal("an unsigned webhook was accepted")
	}

	other := NewFake(Config{WebhookSecret: "different"})
	if _, err := other.VerifyWebhook(header, body); err == nil {
		t.Fatal("a webhook signed with the wrong secret was accepted")
	}
}

// A missing provider must not read as "this person does not need to file".
func TestDisabledProviderFailsLoudly(t *testing.T) {
	p := New(Config{})
	if _, err := p.EnsurePayee(context.Background(), PayeeInput{UserID: "u1"}); err != ErrProviderDisabled {
		t.Fatalf("expected ErrProviderDisabled, got %v", err)
	}
}

func TestEnsurePayeeIsIdempotentOnUserID(t *testing.T) {
	fake := NewFake(Config{})
	a, err := fake.EnsurePayee(context.Background(), PayeeInput{UserID: "u1", Email: "a@b.c"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := fake.EnsurePayee(context.Background(), PayeeInput{UserID: "u1", Email: "a@b.c"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if a.ProviderPayeeID != b.ProviderPayeeID {
		t.Fatalf("two payees for one user: %q vs %q", a.ProviderPayeeID, b.ProviderPayeeID)
	}
}

func TestTrack1099StatusMapping(t *testing.T) {
	for raw, want := range map[string]string{
		"signed": StatusCompleted, "Completed": StatusCompleted, "submitted": StatusCompleted,
		"viewed": StatusOpened, "sent": StatusSent, "pending": StatusSent,
		"rejected": StatusInvalid, "": StatusNotStarted, "who knows": StatusNotStarted,
	} {
		if got := normaliseTrack1099Status(raw); got != want {
			t.Errorf("%q mapped to %q; want %q", raw, got, want)
		}
	}
}

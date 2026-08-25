package w9provider

import (
	"testing"
	"time"
)

const (
	testClientID = "client-abc"
	testSecret   = "secret-xyz"
)

// Pinned against a value computed outside this package, so it checks the
// implementation against the documented recipe rather than against itself:
//
//	python3 -c "import hmac,hashlib,base64; print(base64.b64encode(
//	  hmac.new(b'secret-xyz', b'client-abc\n1787000000', hashlib.sha256).digest()).decode())"
//
// If this ever fails, the question is whether the recipe changed — not whether
// to update the constant to whatever the code now produces.
func TestSignWebhookIsStable(t *testing.T) {
	got := SignWebhook(testClientID, testSecret, "1787000000")
	const want = "KQ2sQEFsCxy1Dt2Q7ps/ANVlQxzMa7LBkCYl16z2SoU="
	if got != want {
		t.Fatalf("signature drifted:\n got %q\nwant %q", got, want)
	}
}

func TestVerifyWebhook(t *testing.T) {
	now := time.Unix(1787000000, 0)
	ts := "1787000000"
	good := SignWebhook(testClientID, testSecret, ts)

	for _, tc := range []struct {
		name      string
		clientID  string
		secret    string
		timestamp string
		signature string
		now       time.Time
		want      bool
	}{
		{name: "a genuine delivery", clientID: testClientID, secret: testSecret,
			timestamp: ts, signature: good, now: now, want: true},

		// The point of the whole exercise: a delivery nobody could have signed.
		{name: "forged signature", clientID: testClientID, secret: testSecret,
			timestamp: ts, signature: "not-the-signature", now: now, want: false},
		{name: "signed with the wrong secret", clientID: testClientID, secret: testSecret,
			timestamp: ts, signature: SignWebhook(testClientID, "other-secret", ts), now: now, want: false},
		{name: "signed for a different client", clientID: testClientID, secret: testSecret,
			timestamp: ts, signature: SignWebhook("someone-else", testSecret, ts), now: now, want: false},

		// The timestamp is inside the signature, so editing it invalidates the
		// delivery — which is what makes a captured one expire.
		{name: "timestamp altered after signing", clientID: testClientID, secret: testSecret,
			timestamp: "1787000001", signature: good, now: now, want: false},
		{name: "replayed a week later", clientID: testClientID, secret: testSecret,
			timestamp: ts, signature: good, now: now.Add(7 * 24 * time.Hour), want: false},

		// Their retries span 24 hours, so a slow-but-legitimate one still counts.
		{name: "a retry twenty hours on", clientID: testClientID, secret: testSecret,
			timestamp: ts, signature: good, now: now.Add(20 * time.Hour), want: true},

		// An unconfigured secret must refuse everything, not accept everything.
		{name: "no secret configured", clientID: testClientID, secret: "",
			timestamp: ts, signature: good, now: now, want: false},
		{name: "no client id configured", clientID: "", secret: testSecret,
			timestamp: ts, signature: good, now: now, want: false},

		{name: "missing headers", clientID: testClientID, secret: testSecret,
			timestamp: "", signature: "", now: now, want: false},
		{name: "unparseable timestamp", clientID: testClientID, secret: testSecret,
			timestamp: "not-a-number",
			signature: SignWebhook(testClientID, testSecret, "not-a-number"), now: now, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifyWebhook(tc.clientID, tc.secret, tc.timestamp, tc.signature, tc.now); got != tc.want {
				t.Fatalf("VerifyWebhook = %v, want %v", got, tc.want)
			}
		})
	}
}

// The Fake must produce something the real verifier accepts, or a local run
// proves nothing about the path a real delivery takes.
func TestFakeSignsWhatTheVerifierAccepts(t *testing.T) {
	f := NewFake(Config{ClientID: testClientID, ClientSecret: testSecret})
	if !f.WebhookCredentialsConfigured() {
		t.Fatal("credentials were configured but the fake says otherwise")
	}
	ts := "1787000000"
	if !f.VerifyWebhookSignature(ts, f.SignWebhookAs(ts), time.Unix(1787000000, 0)) {
		t.Fatal("the fake will not accept its own signature")
	}

	// And without credentials it must refuse, rather than wave everything through.
	bare := NewFake(Config{})
	if bare.WebhookCredentialsConfigured() {
		t.Fatal("an unconfigured fake claims to have credentials")
	}
	if bare.VerifyWebhookSignature(ts, "anything", time.Unix(1787000000, 0)) {
		t.Fatal("an unconfigured fake accepted a delivery")
	}
}

// A reset that leaves the stand-in remembering a signed form is not a reset.
//
// upsert is idempotent on the payee reference and the map lives in the process,
// so without Forget a person who signed before a reset is handed the same
// completed submission afterwards: the next crossing releases the moment it is
// escrowed, the tiers clear themselves, and it reads as the tier logic failing.
func TestFakeForgetsASignedSubmission(t *testing.T) {
	f := NewFake(Config{})
	const user = "did:privy:someone"
	const year = 2026

	first, err := f.CreateW9Request(t.Context(), W9RequestInput{UserID: user, TaxYear: year})
	if err != nil {
		t.Fatalf("creating the first request: %v", err)
	}
	if err := f.Sign(first.ProviderRequestID, "ssn"); err != nil {
		t.Fatalf("signing: %v", err)
	}

	// Same person, same year: idempotent, so the signed one comes back.
	again, err := f.CreateW9Request(t.Context(), W9RequestInput{UserID: user, TaxYear: year})
	if err != nil {
		t.Fatalf("re-requesting: %v", err)
	}
	if again.ProviderRequestID != first.ProviderRequestID {
		t.Fatalf("expected the same submission back, got %q then %q",
			first.ProviderRequestID, again.ProviderRequestID)
	}
	status, err := f.GetW9Status(t.Context(), again.ProviderRequestID)
	if err != nil || status.Status != StatusCompleted {
		t.Fatalf("expected the reused submission to be completed, got %q err=%v", status.Status, err)
	}

	if removed := f.Forget(user, year); removed != 1 {
		t.Fatalf("Forget removed %d submissions, want 1", removed)
	}

	// Now the same person gets a fresh, unsigned submission.
	fresh, err := f.CreateW9Request(t.Context(), W9RequestInput{UserID: user, TaxYear: year})
	if err != nil {
		t.Fatalf("creating after Forget: %v", err)
	}
	if fresh.ProviderRequestID == first.ProviderRequestID {
		t.Fatal("Forget left the old submission reachable")
	}
	status, err = f.GetW9Status(t.Context(), fresh.ProviderRequestID)
	if err != nil {
		t.Fatalf("reading the fresh submission: %v", err)
	}
	if status.Status == StatusCompleted {
		t.Fatal("a submission created after Forget was already completed")
	}

	// The old submission is gone, not merely unlinked. An unknown id reports
	// not_started rather than erroring — deliberately, so a sweep does not fall
	// over on an id it does not recognise — so that is what to check for.
	forgotten, err := f.GetW9Status(t.Context(), first.ProviderRequestID)
	if err != nil {
		t.Fatalf("reading a forgotten submission should not error: %v", err)
	}
	if forgotten.Status != StatusNotStarted {
		t.Fatalf("the forgotten submission still reports %q", forgotten.Status)
	}

	// Forgetting nothing is not an error.
	if removed := f.Forget("did:privy:nobody", year); removed != 0 {
		t.Fatalf("Forget on an unknown payee removed %d", removed)
	}
}

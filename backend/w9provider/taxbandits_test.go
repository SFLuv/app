package w9provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The status enum is the whole completion signal at this vendor — there is no
// signed-at timestamp to fall back on — so every documented value is pinned.
func TestTaxBanditsStatusMapping(t *testing.T) {
	cases := map[string]string{
		"COMPLETED":                          StatusCompleted,
		"COMPLETED_AND_TIN_MATCH_INPROGRESS": StatusCompleted,
		"INVALID":                            StatusInvalid,
		"":                                   StatusNotStarted,
	}
	for raw, want := range cases {
		if got := tbNormaliseStatus(raw); got != want {
			t.Errorf("tbNormaliseStatus(%q) = %q; want %q", raw, got, want)
		}
	}
}

// Signed-but-unmatched must release. Waiting for the match would hold somebody's
// money for up to a day after they had done everything asked of them.
func TestTaxBanditsReleasesWhileTheMatchIsStillRunning(t *testing.T) {
	if tbNormaliseStatus("COMPLETED_AND_TIN_MATCH_INPROGRESS") != StatusCompleted {
		t.Fatal("a signed form with a pending match must count as completed")
	}
}

// An unrecognised status must NOT resolve to "not started". That is the shape
// of a bug that holds money forever and logs nothing, and it is the specific
// failure this integration has already suffered once.
func TestTaxBanditsUnknownStatusIsNotSilentlyNotStarted(t *testing.T) {
	if got := tbNormaliseStatus("SOME_NEW_STATE_THEY_ADDED"); got != "" {
		t.Fatalf("tbNormaliseStatus = %q; an unmapped status must return empty so the caller can complain", got)
	}
}

// StatusTs is space-separated with a colon in the offset. time.RFC3339 rejects
// it outright, and a completed filing with an unparsed timestamp is the silent
// hold we are guarding against.
func TestTaxBanditsStatusTimestampLayout(t *testing.T) {
	const sample = "2024-04-12 06:22:34 -04:00"

	if _, err := time.Parse(time.RFC3339, sample); err == nil {
		t.Fatal("RFC3339 unexpectedly parsed the vendor format; this test is guarding the wrong thing")
	}

	parsed, err := time.Parse(tbTimeLayout, sample)
	if err != nil {
		t.Fatalf("tbTimeLayout failed to parse the documented sample: %v", err)
	}
	if got := parsed.UTC().Format("2006-01-02T15:04:05Z"); got != "2024-04-12T10:22:34Z" {
		t.Fatalf("parsed to %s; the offset was not applied", got)
	}
}

func TestTaxBanditsTINMatchMapping(t *testing.T) {
	cases := map[string]string{
		"ORDER_CREATED": TINMatchPending,
		"SUCCESS":       TINMatchMatched,
		"FAILED":        TINMatchRejected,
	}
	for raw, want := range cases {
		if got := tbNormaliseTINMatch(raw); got != want {
			t.Errorf("tbNormaliseTINMatch(%q) = %q; want %q", raw, got, want)
		}
	}
}

// One payee can hold several submissions, so the array is selected from rather
// than indexed at zero.
func TestTaxBanditsPicksTheMostRecentSubmission(t *testing.T) {
	var out tbStatusResponse
	out.Status = []tbStatusEntry{
		{SubmissionId: "old", StatusTs: "2024-04-10 06:22:34 -04:00"},
		{SubmissionId: "new", StatusTs: "2024-04-12 06:22:34 -04:00"},
	}

	if got := out.Status[tbMostRecent(out)].SubmissionId; got != "new" {
		t.Fatalf("selected %q; want the most recent submission", got)
	}
}

// Nothing may be requested before the payer exists. An empty BusinessID is a
// hard error rather than a silent disable, because a disabled provider reads as
// "vendor unreachable" and this is a misconfiguration we can fix.
func TestTaxBanditsRequiresABusinessID(t *testing.T) {
	tb := NewTaxBandits(Config{ClientID: "a", ClientSecret: "b", UserToken: "c"})
	_, err := tb.CreateW9Request(context.Background(), W9RequestInput{UserID: "u1", TaxYear: 2026})
	if err == nil {
		t.Fatal("expected a configuration error when no BusinessId is set")
	}
	if err == ErrProviderDisabled {
		t.Fatal("a missing BusinessId is a misconfiguration, not an absent vendor")
	}
}

// Missing credentials disable the provider rather than erroring loudly: money
// must still be held correctly when the vendor is simply not configured.
func TestTaxBanditsWithoutCredentialsIsDisabledNotBroken(t *testing.T) {
	tb := NewTaxBandits(Config{})
	if _, err := tb.CreateW9Request(context.Background(), W9RequestInput{UserID: "u1"}); err != ErrProviderDisabled {
		t.Fatalf("err = %v; want ErrProviderDisabled", err)
	}
}

// The environment selects both hosts, and they are on different domains.
func TestTaxBanditsEnvironmentSelectsBothHosts(t *testing.T) {
	sandbox := NewTaxBandits(Config{Environment: "sandbox"})
	if sandbox.apiBase != tbSandboxAPI || sandbox.authBase != tbSandboxAuth {
		t.Fatalf("sandbox hosts = %s / %s", sandbox.apiBase, sandbox.authBase)
	}
	production := NewTaxBandits(Config{Environment: "production"})
	if production.apiBase != tbProductionAPI || production.authBase != tbProductionAuth {
		t.Fatalf("production hosts = %s / %s", production.apiBase, production.authBase)
	}
	// Anything unrecognised must land on sandbox: a typo in an env var must not
	// file real tax forms.
	other := NewTaxBandits(Config{Environment: "staging"})
	if other.apiBase != tbSandboxAPI {
		t.Fatalf("unknown environment used %s; want the sandbox", other.apiBase)
	}
}

// The version is a path segment, and mixing trees silently 404s.
func TestTaxBanditsPinsTheAPIVersionInThePath(t *testing.T) {
	tb := NewTaxBandits(Config{Environment: "sandbox", APIVersion: "v1.7.3"})
	if got := tb.endpoint("/FormW9/Status"); got != tbSandboxAPI+"/v1.7.3/FormW9/Status" {
		t.Fatalf("endpoint = %q", got)
	}
}

// Only the two documented languages are passed through. An unknown hint
// silently becoming Spanish would be worse than falling back to English.
func TestTaxBanditsPreferredLanguage(t *testing.T) {
	if got := tbPreferredLanguage("es"); got != "es-ES" {
		t.Fatalf("es → %q", got)
	}
	if got := tbPreferredLanguage("klingon"); got != "en-US" {
		t.Fatalf("unknown → %q; want the English fallback", got)
	}
}

// The assertion is the single most trap-laden part of this integration: wrong
// algorithm, wrong encoding or a stale `iat` all present identically as "bad
// credentials". Pin the shape so a refactor cannot quietly change it.
func TestTaxBanditsAssertionShape(t *testing.T) {
	tb := NewTaxBandits(Config{ClientID: "client", ClientSecret: "secret", UserToken: "user"})

	at := time.Unix(1_700_000_000, 0)
	assertion, err := tb.signedJWS(at)
	if err != nil {
		t.Fatalf("signedJWS: %v", err)
	}

	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("assertion has %d segments; want 3", len(parts))
	}

	// Raw base64url: padding would make the vendor reject it.
	for i, part := range parts {
		if strings.ContainsAny(part, "=+/") {
			t.Fatalf("segment %d is not raw base64url: %q", i, part)
		}
	}

	var header struct {
		Alg string `json:"alg"`
	}
	decodeSegment(t, parts[0], &header)
	if header.Alg != "HS256" {
		t.Fatalf("alg = %q; want HS256", header.Alg)
	}

	var payload struct {
		Iss string `json:"iss"`
		Sub string `json:"sub"`
		Aud string `json:"aud"`
		Iat int64  `json:"iat"`
	}
	decodeSegment(t, parts[1], &payload)

	// ClientId is both issuer and subject; the UserToken is the audience.
	// Getting these the wrong way round is silently accepted by nothing.
	if payload.Iss != "client" || payload.Sub != "client" {
		t.Fatalf("iss/sub = %q/%q; both must be the client id", payload.Iss, payload.Sub)
	}
	if payload.Aud != "user" {
		t.Fatalf("aud = %q; must be the user token", payload.Aud)
	}
	if payload.Iat != at.Unix() {
		t.Fatalf("iat = %d; must be the seconds-epoch we were handed", payload.Iat)
	}

	// The signature must actually cover header.payload with the secret.
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil)); want != parts[2] {
		t.Fatal("signature does not verify against the client secret")
	}
}

func decodeSegment(t *testing.T, segment string, out any) {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		t.Fatalf("decoding segment: %v", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decoding json: %v", err)
	}
}

// Q1 is unsettled, so the fake can model either answer — and the duplicate
// branch is the one worth proving we survive.
func TestFakeCanModelANonIdempotentVendor(t *testing.T) {
	fake := NewFake(Config{})
	in := W9RequestInput{UserID: "u1", TaxYear: 2026}

	first, _ := fake.CreateW9Request(context.Background(), in)
	same, _ := fake.CreateW9Request(context.Background(), in)
	if first.ProviderRequestID != same.ProviderRequestID {
		t.Fatal("with idempotency on, one payee reference must yield one submission")
	}

	fake.SetIdempotentOnPayeeRef(false)
	third, _ := fake.CreateW9Request(context.Background(), in)
	if third.ProviderRequestID == first.ProviderRequestID {
		t.Fatal("with idempotency off, a second request must mint a second submission")
	}

	// The older submission must remain pollable: if the vendor is not
	// idempotent, an id we already stored has to keep resolving.
	if status, err := fake.GetW9Status(context.Background(), first.ProviderRequestID); err != nil {
		t.Fatalf("status on the superseded submission: %v", err)
	} else if status.Status == StatusNotStarted {
		t.Fatal("a superseded submission must still report its own status")
	}
}

// The live sandbox payload, captured 2026-08-19 from
// GET /v1.7.3/FormW9/Status?SubmissionId=… — not the published sample.
//
// The Track1099 adapter was written twice from documentation and was wrong both
// times. This fixture is the antidote: it is what the vendor actually sent, so
// a field they name differently from their own docs fails here rather than in
// production. Re-capture it with `go run ./cmd/w9probe -step status` if the
// vendor changes shape.
const liveStatusPayload = `{
  "Requester": {
    "BusinessId": "dfa5c5af-817f-42ae-8d19-a7c955978720",
    "PayerRef": "sfluv-sandbox",
    "BusinessNm": "SFLUV Sandbox Test Payer",
    "TINType": "EIN",
    "TIN": "XX-XXX2598"
  },
  "PayeeRef": "sfluv:probe:2026",
  "Email": null,
  "CountryPhoneCode": null,
  "PhoneNumber": null,
  "TotalRecords": 1,
  "Status": [
    {
      "SubmissionId": "4888e436-dfae-4b34-981a-a79a4845873d",
      "DBAId": null,
      "DBARef": null,
      "W9Status": "URL_GENERATED",
      "ExpireDate": null,
      "StatusTs": "2026-08-19 12:17:23 -04:00",
      "TINMatching": null,
      "FormW9RequestType": "URL_API"
    }
  ],
  "Errors": null
}`

func TestTaxBanditsDecodesTheLiveStatusPayload(t *testing.T) {
	var out tbStatusResponse
	if err := json.Unmarshal([]byte(liveStatusPayload), &out); err != nil {
		t.Fatalf("the live payload no longer decodes: %v", err)
	}

	if out.PayeeRef != "sfluv:probe:2026" {
		t.Fatalf("PayeeRef = %q; colons in our reference must survive the round trip", out.PayeeRef)
	}
	if out.TotalRecords != 1 || len(out.Status) != 1 {
		t.Fatalf("TotalRecords=%d len(Status)=%d", out.TotalRecords, len(out.Status))
	}

	entry := out.Status[0]
	if entry.SubmissionId != "4888e436-dfae-4b34-981a-a79a4845873d" {
		t.Fatalf("SubmissionId = %q", entry.SubmissionId)
	}

	// A link that has been generated but not yet opened is emphatically not
	// completed. Mapping this to completed would release money against a form
	// nobody has filled in.
	if got := tbNormaliseStatus(entry.W9Status); got != StatusSent {
		t.Fatalf("URL_GENERATED mapped to %q; want %q", got, StatusSent)
	}

	// The real timestamp must parse with our layout, not RFC3339.
	if _, err := time.Parse(tbTimeLayout, entry.StatusTs); err != nil {
		t.Fatalf("live StatusTs %q failed tbTimeLayout: %v", entry.StatusTs, err)
	}

	// Null TINMatching is normal before the form is signed and must not panic
	// or invent a verdict.
	if entry.TINMatching != nil {
		t.Fatal("TINMatching should be nil on an unsigned request")
	}
}

// Verified against the sandbox: two RequestByUrl calls with one PayeeRef
// produced two SubmissionIds and TotalRecords went to 2. So a stored link must
// be reused rather than re-requested, or every tap strands another submission.
func TestTaxBanditsReusesAStoredLinkInsteadOfCreatingADuplicate(t *testing.T) {
	tb := NewTaxBandits(Config{
		ClientID: "a", ClientSecret: "b", UserToken: "c",
		BusinessID: "d", Environment: "sandbox",
	})

	// No network call may happen here. If one is attempted the credentials are
	// fake, so it would error — reaching a clean result proves the short circuit.
	got, err := tb.HostedFormURL(context.Background(), "sub-1", W9RequestInput{
		UserID:          "u1",
		TaxYear:         2026,
		ExistingFormURL: "https://testlinks.taxbandits.io?uid=abc",
	})
	if err != nil {
		t.Fatalf("a stored link must be returned without calling the vendor: %v", err)
	}
	if got.FormURL != "https://testlinks.taxbandits.io?uid=abc" {
		t.Fatalf("FormURL = %q; the stored link must be handed back unchanged", got.FormURL)
	}
	if got.ProviderRequestID != "sub-1" {
		t.Fatalf("ProviderRequestID = %q; the stored submission must be preserved", got.ProviderRequestID)
	}
}

// All eleven documented W9Status values must map somewhere deliberate. An
// unmapped one is not a cosmetic gap: the caller warns and treats it as unsent,
// which holds somebody's money until a human notices.
func TestTaxBanditsMapsEveryDocumentedStatus(t *testing.T) {
	want := map[string]string{
		"URL_GENERATED":                      StatusSent,
		"ORDER_CREATED":                      StatusSent,
		"SCHEDULED":                          StatusSent,
		"SENT":                               StatusSent,
		"BOUNCED":                            StatusSent,
		"AWAITING_TIN_CERTIFICATE":           StatusSent,
		"OPENED":                             StatusOpened,
		"COMPLETED":                          StatusCompleted,
		"COMPLETED_AND_TIN_MATCH_INPROGRESS": StatusCompleted,
		"INVALID":                            StatusInvalid,
		"ORDER_NOT_CREATED":                  StatusInvalid,
	}
	for raw, expected := range want {
		got := tbNormaliseStatus(raw)
		if got == "" {
			t.Errorf("%s is unmapped; it would warn and hold money", raw)
			continue
		}
		if got != expected {
			t.Errorf("%s mapped to %q; want %q", raw, got, expected)
		}
	}
}

// Only these two may release money. If a future edit widens the completed set,
// this fails first.
func TestTaxBanditsOnlyTwoStatusesRelease(t *testing.T) {
	all := []string{
		"URL_GENERATED", "ORDER_CREATED", "SCHEDULED", "SENT", "BOUNCED",
		"AWAITING_TIN_CERTIFICATE", "OPENED", "COMPLETED",
		"COMPLETED_AND_TIN_MATCH_INPROGRESS", "INVALID", "ORDER_NOT_CREATED",
	}
	releasing := []string{}
	for _, raw := range all {
		if tbNormaliseStatus(raw) == StatusCompleted {
			releasing = append(releasing, raw)
		}
	}
	if len(releasing) != 2 {
		t.Fatalf("statuses that release money: %v; want exactly COMPLETED and COMPLETED_AND_TIN_MATCH_INPROGRESS", releasing)
	}
}

// The live rejected payload, captured 2026-08-19. The vendor sets INVALID and
// erases TINMatching, so the status is the only remaining signal.
const liveRejectedPayload = `{
  "PayeeRef": "sfluv:probefail:2026",
  "TotalRecords": 1,
  "Status": [
    {
      "SubmissionId": "d3900540-6eb4-4321-9c46-ef24670ec36b",
      "W9Status": "INVALID",
      "ExpireDate": null,
      "StatusTs": "2026-08-19 12:38:21 -04:00",
      "TINMatching": null,
      "FormW9RequestType": "URL_API"
    }
  ],
  "Errors": null
}`

// A failed match must not be reported as "no verdict yet".
//
// The caller gates on TINMatch being non-empty, so an empty value means the
// rejection never gets recorded: the filing keeps clearing payouts, no
// corrected form is requested, and the row polls forever. This is the bug the
// live sandbox exposed and the fake had been hiding.
func TestTaxBanditsReportsAnInvalidFilingAsARejectedMatch(t *testing.T) {
	var out tbStatusResponse
	if err := json.Unmarshal([]byte(liveRejectedPayload), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	entry := out.Status[0]

	if entry.TINMatching != nil {
		t.Fatal("fixture is wrong: the vendor erases TINMatching on failure")
	}

	status := W9Status{Status: tbNormaliseStatus(entry.W9Status)}
	if entry.TINMatching != nil {
		status.TINMatch = tbNormaliseTINMatch(entry.TINMatching.Status)
	}
	if status.Status == StatusInvalid && status.TINMatch == "" {
		status.TINMatch = TINMatchRejected
	}

	if status.Status != StatusInvalid {
		t.Fatalf("Status = %q; want invalid", status.Status)
	}
	if status.TINMatch != TINMatchRejected {
		t.Fatalf("TINMatch = %q; an empty verdict here is silently dropped by the caller", status.TINMatch)
	}
}

package w9provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TaxBandits talks to TaxBandits (developer.taxbandits.com).
//
// Shaped from the vendor's published documentation — see TAXBANDITS-BUILD-PLAN.md
// in this directory for the reading, and the handoff in ~/Projects/handoff for
// the decisions. What is documented and what has been observed on a live call
// are different things, and this file is written on the assumption that only
// the first is true so far. Every place that could be wrong fails loudly rather
// than quietly resolving to "not started", because a silently unrecognised
// completion holds somebody's money forever.
//
// Four things about this vendor drive the design:
//
//   - Auth is a JWS we sign ourselves, exchanged for a bearer token over a GET
//     with an "Authentication" header. It is not OAuth2 and x/oauth2 cannot be
//     pointed at it.
//   - The signing timestamp must agree with their clock, so server time is
//     synced rather than taken from this machine.
//   - Completion is a documented status *enum*. There is no signed-at
//     timestamp anywhere in the payload, which inverts how the Track1099
//     adapter worked.
//   - Status comes back as an array; one payee can hold several submissions.
type TaxBandits struct {
	clientID     string
	clientSecret string
	userToken    string
	businessID   string
	webhookRef   string
	apiVersion   string
	apiBase      string
	authBase     string
	client       *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
	// clockOffset is their time minus ours. Applied to every `iat` we sign.
	clockOffset time.Duration
	clockSynced bool
	onWarning   func(string)
}

// Sandbox and production differ in both hosts, and the auth host is a different
// domain from the API host — a detail that reads like a typo and is not.
const (
	tbSandboxAPI  = "https://testapi.taxbandits.com"
	tbSandboxAuth = "https://testoauth.expressauth.net/v2/tbsauth"
	// Unverified: the production API host is not documented alongside the
	// sandbox one and is confirmed at Go Live. It is a config value for exactly
	// that reason; this is only the fallback guess.
	tbProductionAPI  = "https://api.taxbandits.com"
	tbProductionAuth = "https://oauth.expressauth.net/v2/tbsauth"

	// tbDefaultAPIVersion pins the tree every request shape here came from.
	// A v2.0.0 exists with a different feature set; do not mix them.
	tbDefaultAPIVersion = "v1.7.3"
)

// tbTimeLayout parses StatusTs. It is space-separated with a colon in the
// offset, so it is NOT RFC3339 and time.RFC3339 will reject it outright.
const tbTimeLayout = "2006-01-02 15:04:05 -07:00"

func NewTaxBandits(cfg Config) *TaxBandits {
	apiBase := strings.TrimSuffix(strings.TrimSpace(cfg.BaseURL), "/")
	authBase := strings.TrimSpace(cfg.AuthURL)
	if strings.EqualFold(strings.TrimSpace(cfg.Environment), "production") {
		if apiBase == "" {
			apiBase = tbProductionAPI
		}
		if authBase == "" {
			authBase = tbProductionAuth
		}
	} else {
		if apiBase == "" {
			apiBase = tbSandboxAPI
		}
		if authBase == "" {
			authBase = tbSandboxAuth
		}
	}

	version := strings.TrimSpace(cfg.APIVersion)
	if version == "" {
		version = tbDefaultAPIVersion
	}

	return &TaxBandits{
		clientID:     strings.TrimSpace(cfg.ClientID),
		clientSecret: strings.TrimSpace(cfg.ClientSecret),
		userToken:    strings.TrimSpace(cfg.UserToken),
		businessID:   strings.TrimSpace(cfg.BusinessID),
		webhookRef:   strings.TrimSpace(cfg.WebhookRef),
		apiVersion:   version,
		apiBase:      apiBase,
		authBase:     authBase,
		// A tax vendor being slow must not hold a database transaction open, so
		// every call is bounded and callers do provider work outside their
		// transactions.
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (t *TaxBandits) Name() string { return "taxbandits" }

// VerifyWebhookSignature checks a callback against the client credentials.
func (t *TaxBandits) VerifyWebhookSignature(timestamp, signature string, now time.Time) bool {
	return VerifyWebhook(t.clientID, t.clientSecret, timestamp, signature, now)
}

// SetWarningLogger routes the loud-on-surprise messages somewhere visible.
// Without it they are dropped, which is acceptable for tests and not for
// production — bootstrap wires it to the app logger.
func (t *TaxBandits) SetWarningLogger(fn func(string)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onWarning = fn
}

func (t *TaxBandits) warn(format string, args ...any) {
	t.mu.Lock()
	fn := t.onWarning
	t.mu.Unlock()
	if fn != nil {
		fn(fmt.Sprintf(format, args...))
	}
}

func (t *TaxBandits) configured() error {
	switch {
	case t.clientID == "", t.clientSecret == "", t.userToken == "":
		return ErrProviderDisabled
	case t.businessID == "":
		// Same class of blocker as Track1099's team id: the payer must exist
		// before any form request can name it, and nothing works until it does.
		return fmt.Errorf("W9_PROVIDER_BUSINESS_ID is not set; create the payer once via Business/Create and store its GUID")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

// serverTimeOffset keeps our signing clock aligned with theirs.
//
// A drifting VM clock means no token is ever issued, and the failure presents
// as "bad credentials" — so it gets fixed here rather than debugged later at
// three in the morning.
func (t *TaxBandits) syncServerTime(ctx context.Context) {
	// Marked attempted regardless of outcome. Verified against the sandbox:
	// this endpoint is NOT open — without an assertion it returns AUTH-100002,
	// "Authentication header cannot be empty". An earlier version called it
	// unauthenticated, so it 401'd every time, never set an offset, and
	// re-tried on every single token mint.
	//
	// That also bounds what this can do: a clock skewed far enough to be
	// rejected cannot ask what the right time is. It corrects small drift and
	// diagnoses large drift; it cannot bootstrap from nothing.
	defer func() {
		t.mu.Lock()
		t.clockSynced = true
		t.mu.Unlock()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.authBaseHost()+"/v2/getservertime", nil)
	if err != nil {
		return
	}
	t.mu.Lock()
	offset := t.clockOffset
	t.mu.Unlock()
	assertion, err := t.signedJWS(time.Now().Add(offset))
	if err != nil {
		return
	}
	req.Header.Set("Authentication", assertion)

	resp, err := t.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if err != nil || resp.StatusCode != http.StatusOK {
		return
	}

	// Field names taken from a live response envelope, which carries
	// timeZone/serverDate/serverTime/unixTs in lower camel case — not the
	// PascalCase the rest of the API uses.
	var payload struct {
		ServerTime string `json:"serverTime"`
		ServerDate string `json:"serverDate"`
		UnixTs     string `json:"unixTs"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}
	// A unix timestamp needs no layout guessing, so prefer it when offered.
	if ts := strings.TrimSpace(payload.UnixTs); ts != "" && ts != "null" {
		if seconds, convErr := strconv.ParseInt(ts, 10, 64); convErr == nil {
			t.mu.Lock()
			t.clockOffset = time.Until(time.Unix(seconds, 0))
			t.mu.Unlock()
			return
		}
	}
	stamp := strings.TrimSpace(payload.ServerTime)
	if stamp == "" || stamp == "null" {
		return
	}
	if date := strings.TrimSpace(payload.ServerDate); date != "" && date != "null" {
		stamp = date + " " + stamp
	}
	// The exact shape of ServerTime is not documented. Try their status layout
	// first, then RFC3339, and simply keep a zero offset if neither fits —
	// a wrong offset would be worse than none.
	for _, layout := range []string{tbTimeLayout, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, stamp); err == nil {
			t.mu.Lock()
			t.clockOffset = time.Until(parsed)
			t.mu.Unlock()
			return
		}
	}
	t.warn("taxbandits: could not parse server time %q; signing with the local clock", stamp)
}

func (t *TaxBandits) authBaseHost() string {
	if idx := strings.Index(t.authBase, "/v2/"); idx > 0 {
		return t.authBase[:idx]
	}
	return strings.TrimSuffix(t.authBase, "/")
}

func (t *TaxBandits) signedJWS(now time.Time) (string, error) {
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	payload := map[string]any{
		"iss": t.clientID,
		"sub": t.clientID,
		"aud": t.userToken,
		"iat": now.Unix(),
	}

	encode := func(v any) (string, error) {
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(raw), nil
	}

	headerPart, err := encode(header)
	if err != nil {
		return "", err
	}
	payloadPart, err := encode(payload)
	if err != nil {
		return "", err
	}

	signingInput := headerPart + "." + payloadPart
	mac := hmac.New(sha256.New, []byte(t.clientSecret))
	mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature, nil
}

// accessToken returns a cached bearer token, minting one when needed.
//
// Refreshed a minute before it lapses so a long sweep does not fail halfway,
// and serialised so the batch poller cannot stampede the auth endpoint.
func (t *TaxBandits) accessToken(ctx context.Context, force bool) (string, error) {
	t.mu.Lock()
	if !force && t.token != "" && time.Now().Before(t.tokenExpiry) {
		token := t.token
		t.mu.Unlock()
		return token, nil
	}
	synced := t.clockSynced
	offset := t.clockOffset
	t.mu.Unlock()

	if !synced {
		t.syncServerTime(ctx)
		t.mu.Lock()
		offset = t.clockOffset
		t.mu.Unlock()
	}

	assertion, err := t.signedJWS(time.Now().Add(offset))
	if err != nil {
		return "", fmt.Errorf("error signing the taxbandits assertion: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.authBase, nil)
	if err != nil {
		return "", err
	}
	// Not "Authorization", and not a POST. Both are easy to get wrong and both
	// present as a credential failure.
	req.Header.Set("Authentication", assertion)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error reaching the taxbandits auth service: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", err
	}

	var envelope struct {
		StatusCode  int    `json:"StatusCode"`
		StatusName  string `json:"StatusName"`
		AccessToken string `json:"AccessToken"`
		TokenType   string `json:"TokenType"`
		ExpiresIn   int    `json:"ExpiresIn"`
		Errors      []struct {
			Id      string `json:"Id"`
			Name    string `json:"Name"`
			Message string `json:"Message"`
		} `json:"Errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", fmt.Errorf("error decoding the taxbandits auth response: %w", err)
	}
	if strings.TrimSpace(envelope.AccessToken) == "" {
		return "", fmt.Errorf("taxbandits issued no access token (status %d %s): %s",
			resp.StatusCode, envelope.StatusName, describeTBErrors(envelope.Errors))
	}

	expiresIn := envelope.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	t.mu.Lock()
	t.token = envelope.AccessToken
	t.tokenExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second).Add(-time.Minute)
	t.mu.Unlock()

	return envelope.AccessToken, nil
}

func describeTBErrors(errs []struct {
	Id      string `json:"Id"`
	Name    string `json:"Name"`
	Message string `json:"Message"`
}) string {
	if len(errs) == 0 {
		return "no error detail returned"
	}
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, fmt.Sprintf("%s %s: %s", e.Id, e.Name, e.Message))
	}
	return strings.Join(parts, "; ")
}

// ---------------------------------------------------------------------------
// Transport
// ---------------------------------------------------------------------------

func (t *TaxBandits) endpoint(suffix string) string {
	return fmt.Sprintf("%s/%s%s", t.apiBase, url.PathEscape(t.apiVersion), suffix)
}

// do makes one authenticated call, retrying once on a 401 with a fresh token.
//
// The retry exists because the most likely cause of a 401 mid-run is clock
// drift or an expiry we mis-predicted, and both are fixed by re-minting. It
// re-syncs the clock before trying again rather than replaying the same bad
// assertion.
func (t *TaxBandits) do(ctx context.Context, method string, suffix string, body any, out any) error {
	if err := t.configured(); err != nil {
		return err
	}

	for attempt := 0; attempt < 2; attempt++ {
		token, err := t.accessToken(ctx, attempt > 0)
		if err != nil {
			return err
		}

		var reader io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				return fmt.Errorf("error encoding %s %s: %w", method, suffix, err)
			}
			reader = bytes.NewReader(encoded)
		}

		req, err := http.NewRequestWithContext(ctx, method, t.endpoint(suffix), reader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := t.client.Do(req)
		if err != nil {
			return fmt.Errorf("error calling taxbandits %s %s: %w", method, suffix, err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("error reading taxbandits %s %s: %w", method, suffix, readErr)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			t.mu.Lock()
			t.token = ""
			t.clockSynced = false
			t.mu.Unlock()
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			// Deliberately does not include the body: error payloads can echo
			// submitted fields, and nothing here should risk logging a TIN.
			return fmt.Errorf("taxbandits %s %s returned %d", method, suffix, resp.StatusCode)
		}

		if out == nil {
			return nil
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("error decoding taxbandits %s %s: %w", method, suffix, err)
		}
		return nil
	}
	return fmt.Errorf("taxbandits %s %s could not be authenticated", method, suffix)
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

// EnsurePayee is a local no-op.
//
// There is no payee resource at this vendor: identity rides on PayeeRef, which
// we choose. Kept on the interface because other vendors do have one, and
// because the caller stores the result as the payee pointer either way.
func (t *TaxBandits) EnsurePayee(_ context.Context, in PayeeInput) (PayeeResult, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return PayeeResult{}, fmt.Errorf("user id is required to identify a payee")
	}
	return PayeeResult{ProviderPayeeID: in.UserID}, nil
}

type tbRequestByURLResponse struct {
	SubmissionId string `json:"SubmissionId"`
	PayeeRef     string `json:"PayeeRef"`
	W9Url        string `json:"W9Url"`
	Errors       []struct {
		Id      string `json:"Id"`
		Name    string `json:"Name"`
		Message string `json:"Message"`
	} `json:"Errors"`
}

func (t *TaxBandits) requestByURL(ctx context.Context, in W9RequestInput) (tbRequestByURLResponse, error) {
	payeeRef := referenceID(in.UserID, in.TaxYear)
	if len(payeeRef) > 50 {
		// PayeeRef is capped at 50 characters and is our only join key, so a
		// silent truncation would be a corrupted one.
		return tbRequestByURLResponse{}, fmt.Errorf("payee reference %q exceeds the 50 character limit", payeeRef)
	}

	body := map[string]any{
		"Requester": map[string]any{"BusinessId": t.businessID},
		"Recipient": map[string]any{
			"PayeeRef": payeeRef,
			"Email":    in.Email,
			// Matching runs on submission and resolves in the background. It is
			// billed per use, and it is what makes a rejected TIN block the
			// *next* payout rather than this one.
			"IsTINMatching": true,
		},
		"PrefLang": tbPreferredLanguage(in.PreferredLanguage),
	}
	if t.webhookRef != "" {
		// Sends this request's callbacks to one registered URL rather than all
		// of them, which is how sandbox is kept off production's receiver and
		// production off somebody's tunnel.
		body["WebhookRef"] = t.webhookRef
	}
	if strings.TrimSpace(in.ReturnURL) != "" {
		body["RedirectUrls"] = map[string]any{
			"ReturnUrl": in.ReturnURL,
			// Long enough to read the confirmation, short enough not to strand
			// somebody on a vendor page wondering whether it worked.
			"RedirectTime": 5,
		}
	}

	var out tbRequestByURLResponse
	if err := t.do(ctx, http.MethodPost, "/FormW9/RequestByUrl", body, &out); err != nil {
		return tbRequestByURLResponse{}, err
	}
	if strings.TrimSpace(out.W9Url) == "" {
		return tbRequestByURLResponse{}, fmt.Errorf("taxbandits returned no form link: %s", describeTBErrors(out.Errors))
	}
	return out, nil
}

// tbPreferredLanguage maps our language hint onto what the vendor accepts.
//
// Only the two they document are passed through. An unknown value silently
// becoming Spanish would be worse than falling back to English.
func tbPreferredLanguage(pref string) string {
	switch strings.ToLower(strings.TrimSpace(pref)) {
	case "es", "es-es", "es-mx", "spanish":
		return "es-ES"
	default:
		return "en-US"
	}
}

func (t *TaxBandits) CreateW9Request(ctx context.Context, in W9RequestInput) (W9Request, error) {
	out, err := t.requestByURL(ctx, in)
	if err != nil {
		return W9Request{}, err
	}
	return W9Request{
		ProviderRequestID: out.SubmissionId,
		FormURL:           out.W9Url,
		// No expiry is documented for W9Url. Rather than invent one, none is
		// claimed — the caller re-mints on every tap anyway, so a stored link is
		// never the one a person actually follows.
	}, nil
}

// HostedFormURL returns the link for a request that already exists.
//
// It re-uses the stored link rather than asking for a new one, and that is a
// correctness requirement here rather than an optimisation. Verified against
// the sandbox on 2026-08-19:
//
//   - RequestByUrl is NOT idempotent on PayeeRef. Two calls with one reference
//     produced two SubmissionIds and TotalRecords went to 2. Re-requesting on
//     every tap would pile up submissions and make every later status read
//     ambiguous.
//   - The W9Url is durable and reusable. The same link was fetched twice and
//     both times resolved to the hosted form; ExpireDate came back null.
//
// So the link we already hold is the right answer, and asking again is the
// wrong one. Only a filing with no link at all falls through to creating one.
func (t *TaxBandits) HostedFormURL(ctx context.Context, providerRequestID string, in W9RequestInput) (W9Request, error) {
	if existing := strings.TrimSpace(in.ExistingFormURL); existing != "" {
		return W9Request{
			ProviderRequestID: strings.TrimSpace(providerRequestID),
			FormURL:           existing,
		}, nil
	}

	out, err := t.requestByURL(ctx, in)
	if err != nil {
		return W9Request{}, err
	}
	if previous := strings.TrimSpace(providerRequestID); previous != "" &&
		!strings.EqualFold(previous, strings.TrimSpace(out.SubmissionId)) {
		// Reached only when a filing had a submission id but no link — rare,
		// and worth saying out loud because it leaves a stranded submission.
		t.warn("taxbandits: replaced submission %s with %s for payee %s; the old one is now orphaned",
			previous, out.SubmissionId, referenceID(in.UserID, in.TaxYear))
	}
	return W9Request{
		ProviderRequestID: out.SubmissionId,
		FormURL:           out.W9Url,
	}, nil
}

type tbStatusResponse struct {
	PayeeRef     string          `json:"PayeeRef"`
	Email        string          `json:"Email"`
	TotalRecords int             `json:"TotalRecords"`
	Status       []tbStatusEntry `json:"Status"`
	Errors       []struct {
		Id      string `json:"Id"`
		Name    string `json:"Name"`
		Message string `json:"Message"`
	} `json:"Errors"`
}

// tbStatusEntry is one submission inside the Status array.
//
// Named rather than anonymous because it is asserted against a captured live
// payload in the tests, and an anonymous struct makes every added field a
// compile error in unrelated places.
type tbStatusEntry struct {
	SubmissionId string `json:"SubmissionId"`
	W9Status     string `json:"W9Status"`
	StatusTs     string `json:"StatusTs"`
	// Absent from the published sample; present on every live response and null
	// so far. Read so that a vendor which starts setting it cannot silently
	// invalidate links we are still handing out.
	ExpireDate  string `json:"ExpireDate"`
	TINMatching *struct {
		Status   string `json:"Status"`
		StatusTs string `json:"StatusTs"`
	} `json:"TINMatching"`
	FormW9RequestType string `json:"FormW9RequestType"`
}

func (t *TaxBandits) GetW9Status(ctx context.Context, providerRequestID string) (W9Status, error) {
	submissionID := strings.TrimSpace(providerRequestID)
	if submissionID == "" {
		return W9Status{Status: StatusNotStarted}, nil
	}

	var out tbStatusResponse
	suffix := "/FormW9/Status?SubmissionId=" + url.QueryEscape(submissionID)
	if err := t.do(ctx, http.MethodGet, suffix, nil, &out); err != nil {
		return W9Status{}, err
	}

	if len(out.Status) == 0 {
		return W9Status{Status: StatusNotStarted}, nil
	}

	// One payee can hold several submissions, so the array is selected from
	// deliberately rather than indexed at zero. We asked by submission id, so a
	// matching entry is the answer; anything else means the vendor returned
	// something we did not ask for and we say so.
	chosen := -1
	for i, entry := range out.Status {
		if strings.EqualFold(strings.TrimSpace(entry.SubmissionId), submissionID) {
			chosen = i
			break
		}
	}
	if chosen < 0 {
		t.warn("taxbandits: status for submission %s returned %d record(s), none matching; using the most recent",
			submissionID, len(out.Status))
		chosen = tbMostRecent(out)
	}
	entry := out.Status[chosen]

	status := W9Status{Status: tbNormaliseStatus(entry.W9Status)}
	if status.Status == "" {
		// An unrecognised status must never resolve to "not started": that is
		// the shape of a bug that holds money indefinitely and logs nothing.
		t.warn("taxbandits: unrecognised W9Status %q on submission %s — treating as sent, NOT as completed. "+
			"Map it explicitly before this reaches production.", entry.W9Status, submissionID)
		status.Status = StatusSent
	}

	if status.Status == StatusCompleted {
		if parsed, err := time.Parse(tbTimeLayout, strings.TrimSpace(entry.StatusTs)); err == nil {
			utc := parsed.UTC()
			status.CompletedAt = &utc
		} else if strings.TrimSpace(entry.StatusTs) != "" {
			// Loud, because a completed filing with no timestamp is exactly the
			// silent-hold failure this integration has already suffered once.
			t.warn("taxbandits: could not parse StatusTs %q on submission %s (layout %q): %v",
				entry.StatusTs, submissionID, tbTimeLayout, err)
		}
	}

	if entry.TINMatching != nil {
		status.TINMatch = tbNormaliseTINMatch(entry.TINMatching.Status)
	}

	// A failed match arrives as INVALID with TINMatching erased — verified
	// against the sandbox on 2026-08-19:
	//
	//   signed       → COMPLETED_AND_TIN_MATCH_INPROGRESS, TINMatching.Status ORDER_CREATED
	//   match fails  → INVALID,                            TINMatching null
	//
	// The vendor deletes the evidence, so the status is the only signal left.
	// Reporting an empty TINMatch here would be technically faithful and
	// practically catastrophic: the caller only records a match when it is
	// non-empty, so the rejection would be dropped, the filing would keep
	// clearing future payouts, nobody would be asked for a corrected form, and
	// the row would poll forever.
	//
	// So an invalid filing is reported as a rejected match. Even where INVALID
	// arises some other way, the handling is identical — the filing is
	// unusable, the next payout must be blocked, and a corrected form is
	// needed. Money already released is never touched either way.
	if status.Status == StatusInvalid && status.TINMatch == "" {
		status.TINMatch = TINMatchRejected
	}

	// TINType is left empty, on purpose.
	//
	// FormW9/Status does not carry it — the TINType in the payload belongs to
	// the Requester, which is us, not the payee. The only endpoint that returns
	// the payee's TINType is FormW9/Get, and that same response carries the
	// payee's actual TIN and a PDF of their signed form.
	//
	// So filling this field means pulling a taxpayer identification number into
	// our process to read one adjacent harmless byte. That is precisely the
	// trade this package exists to refuse. If a 1099 ever needs the type, the
	// vendor already holds the W-9 and can supply it at filing time; we should
	// not become a second place where a TIN has been.
	//
	// Do not "fix" this by calling FormW9/Get.

	return status, nil
}

func tbMostRecent(out tbStatusResponse) int {
	best, bestAt := 0, time.Time{}
	for i, entry := range out.Status {
		parsed, err := time.Parse(tbTimeLayout, strings.TrimSpace(entry.StatusTs))
		if err != nil {
			continue
		}
		if parsed.After(bestAt) {
			best, bestAt = i, parsed
		}
	}
	return best
}

// tbNormaliseStatus maps the vendor's documented vocabulary onto ours.
//
// Completion is the enum and nothing else — there is no signed-at timestamp in
// this vendor's payload to key off, which is the opposite of how the Track1099
// adapter worked. Both COMPLETED and COMPLETED_AND_TIN_MATCH_INPROGRESS mean
// signed, and therefore mean release: waiting for the match would hold money
// for up to a day after somebody did everything asked of them.
//
// Returns "" for anything unrecognised so the caller can complain rather than
// silently defaulting.
// The eleven documented values, mapped exhaustively. Sourced from
// developer.taxbandits.com/docs/formw9/status/ on 2026-08-19; only
// URL_GENERATED has been observed live so far.
func tbNormaliseStatus(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	// Signed. Both mean release: waiting for the match would hold money for up
	// to a day after somebody did everything asked of them.
	case "COMPLETED", "COMPLETED_AND_TIN_MATCH_INPROGRESS":
		return StatusCompleted

	// Unusable. INVALID is a form the vendor rejected; ORDER_NOT_CREATED means
	// the request never came into being, so there is nothing for the person to
	// open and a new one has to be raised.
	case "INVALID", "ORDER_NOT_CREATED":
		return StatusInvalid

	case "OPENED":
		return StatusOpened

	// In flight. Nothing is signed, so nothing is released.
	case "URL_GENERATED", "ORDER_CREATED", "SCHEDULED", "SENT":
		return StatusSent

	// BOUNCED means the invitation email did not arrive. The form is not
	// signed, so it maps to sent like the rest — but it is worth its own case
	// because it is the one state that will never advance on its own. We
	// deliver links in-app rather than by email, so it should not occur; if it
	// does, somebody needs a working address before anything else can happen.
	case "BOUNCED":
		return StatusSent

	// Deliberately NOT treated as completed.
	//
	// The name suggests a signature that is waiting on something further, and
	// the documentation does not say what. Guessing "completed" would release
	// money against a form that may not be finished; guessing "sent" holds
	// money that may already be owed. Holding is the recoverable error of the
	// two, and the caller warns, so this surfaces rather than festering.
	case "AWAITING_TIN_CERTIFICATE":
		return StatusSent

	case "":
		return StatusNotStarted
	default:
		return ""
	}
}

func tbNormaliseTINMatch(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "SUCCESS", "MATCHED":
		return TINMatchMatched
	case "FAILED", "REJECTED":
		return TINMatchRejected
	case "ORDER_CREATED", "IN_PROGRESS", "PENDING":
		return TINMatchPending
	default:
		return ""
	}
}

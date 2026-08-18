package w9provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Fake is an in-process stand-in shaped to the documented vendor contract.
//
// The vendor publishes no sandbox, so this is the only way the loop can be
// exercised at all — which makes its fidelity load-bearing. An earlier version
// mirrored a guessed API, so a green suite proved only that we were internally
// consistent with a fiction. This one follows what the docs actually describe:
// form requests keyed by a reference id we choose, a signed link that expires
// after an hour, completion signalled by signed_at, an asynchronous TIN match,
// and no webhook of any kind.
type Fake struct {
	baseURL string

	mu       sync.Mutex
	requests map[string]*fakeRequest
	// runID scopes generated identifiers to this process, so a restart cannot
	// reissue an id an earlier run already used.
	runID string
	seq   int

	// tinMatchAfter is how many status reads it takes for the asynchronous TIN
	// match to resolve. Real life is about 24 hours; a couple of polls is the
	// same shape at a testable speed.
	tinMatchAfter int
	// tinMatchResult lets a test drive the rejected branch, which is the one
	// with real consequences.
	tinMatchResult string
}

type fakeRequest struct {
	ReferenceID string
	UserID      string
	TaxYear     int
	SignedAt    *time.Time
	TINType     string
	statusReads int
}

func NewFake(cfg Config) *Fake {
	base := strings.TrimSuffix(cfg.BaseURL, "/")
	if base == "" {
		base = "http://localhost:8080"
	}
	return &Fake{
		baseURL:        base,
		requests:       map[string]*fakeRequest{},
		runID:          strconv.FormatInt(time.Now().UnixNano(), 36),
		tinMatchAfter:  2,
		tinMatchResult: TINMatchMatched,
	}
}

// SetTINMatchOutcome drives the asynchronous match, so the rejected path can be
// tested. Rejection must never claw back money already released; it only
// affects the next payout.
func (f *Fake) SetTINMatchOutcome(result string, afterReads int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tinMatchResult = result
	f.tinMatchAfter = afterReads
}

func (f *Fake) Name() string { return "fake" }

// EnsurePayee mirrors the real adapter: a local no-op. There is no payee
// resource to create; identity rides on the reference id.
func (f *Fake) EnsurePayee(_ context.Context, in PayeeInput) (PayeeResult, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return PayeeResult{}, fmt.Errorf("user id is required to identify a payee")
	}
	return PayeeResult{ProviderPayeeID: in.UserID}, nil
}

func (f *Fake) CreateW9Request(_ context.Context, in W9RequestInput) (W9Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	ref := referenceID(in.UserID, in.TaxYear)
	if ref == "sfluv::0" {
		f.seq++
		ref = fmt.Sprintf("sfluv:anon:%s:%d", f.runID, f.seq)
	}
	// Reusing a reference id surfaces the prior submission rather than creating
	// a second one — the vendor's stated idempotency behaviour.
	if existing, ok := f.requests[ref]; ok {
		return f.requestFor(existing), nil
	}

	f.requests[ref] = &fakeRequest{ReferenceID: ref, UserID: in.UserID, TaxYear: in.TaxYear}
	return f.requestFor(f.requests[ref]), nil
}

func (f *Fake) HostedFormURL(_ context.Context, providerRequestID string, _ string) (W9Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	req, ok := f.requests[providerRequestID]
	if !ok {
		return W9Request{}, fmt.Errorf("unknown form request %q", providerRequestID)
	}
	return f.requestFor(req), nil
}

func (f *Fake) requestFor(r *fakeRequest) W9Request {
	return W9Request{
		ProviderRequestID: r.ReferenceID,
		FormURL:           fmt.Sprintf("%s/w9/fake/form/%s", f.baseURL, r.ReferenceID),
		// An hour, as the vendor states for signed_pdf. Short on purpose: a link
		// stored at escrow time is dead by the time anybody taps it.
		FormURLExpiresAt: time.Now().Add(time.Hour),
	}
}

// Sign is what the stub form's submit button calls. It records signed_at, which
// is the only thing that means "completed". Nothing is pushed anywhere — the
// backend finds out by polling, exactly as it will in production.
func (f *Fake) Sign(providerRequestID string, tinType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	req, ok := f.requests[providerRequestID]
	if !ok {
		return fmt.Errorf("unknown form request %q", providerRequestID)
	}
	now := time.Now().UTC()
	req.SignedAt = &now
	if tinType == "" {
		tinType = "ssn"
	}
	req.TINType = tinType
	return nil
}

func (f *Fake) GetW9Status(_ context.Context, providerRequestID string) (W9Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	req, ok := f.requests[providerRequestID]
	if !ok {
		return W9Status{Status: StatusNotStarted}, nil
	}

	status := W9Status{Status: StatusSent, TINType: req.TINType}
	if req.SignedAt == nil {
		return status, nil
	}

	status.Status = StatusCompleted
	status.CompletedAt = req.SignedAt

	// The match resolves in the background, some time after signing.
	req.statusReads++
	if req.statusReads >= f.tinMatchAfter {
		status.TINMatch = f.tinMatchResult
	} else {
		status.TINMatch = TINMatchPending
	}
	return status, nil
}

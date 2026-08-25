package w9provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Fake is an in-process stand-in shaped to the TaxBandits contract.
//
// Its fidelity is load-bearing. An earlier version mirrored a guessed API, so a
// green suite proved only that we were internally consistent with a fiction —
// that is how the Track1099 integration was got wrong twice. This one follows
// what TaxBandits actually documents:
//
//   - A PayeeRef we choose is the join key; a SubmissionId they issue is the
//     handle we store and poll on.
//   - Completion is a status *enum*, not a signed-at timestamp. There is no
//     signed timestamp anywhere in their payload.
//   - COMPLETED_AND_TIN_MATCH_INPROGRESS is a real state: signed, released,
//     match still running.
//   - The TIN match is asynchronous and resolves on a later read.
//   - Completion is discovered by polling. That is how this stand-in behaves,
//     and it is NOT what the real vendor does: TaxBandits sends a signed Form
//     W-9 Status Change callback. Reshaping this to deliver one is listed as
//     step 7 of the build plan. Until then, a test passing here says nothing
//     about the callback path.
//
// A sandbox does exist for the real vendor, so this is no longer the only way
// to exercise the loop — but it is still the only way to do it offline, and the
// only way to drive the rejected-match branch on demand.
type Fake struct {
	baseURL string

	mu sync.Mutex
	// bySubmission is the poll path; byPayeeRef is the idempotency path.
	bySubmission map[string]*fakeRequest
	byPayeeRef   map[string]*fakeRequest
	// runID scopes generated identifiers to this process, so a restart cannot
	// reissue an id an earlier run already used.
	runID string
	seq   int

	// idempotentOnPayeeRef mirrors the vendor behaviour we have NOT yet
	// confirmed (Q1 in the build plan). Default true, because that is the
	// assumption the adapter is written against; flip it in a test to exercise
	// the duplicate-submission branch the adapter warns about.
	idempotentOnPayeeRef bool

	// tinMatchAfter is how many status reads it takes for the asynchronous TIN
	// match to resolve. Real life is hours; a couple of polls is the same shape
	// at a testable speed.
	tinMatchAfter int
	// tinMatchResult lets a test drive the rejected branch, which is the one
	// with real consequences.
	tinMatchResult string
}

type fakeRequest struct {
	PayeeRef     string
	SubmissionID string
	UserID       string
	TaxYear      int
	// w9Status holds the vendor's own vocabulary, so the fake exercises the
	// same mapping the real adapter has to do.
	w9Status    string
	completedAt *time.Time
	tinType     string
	statusReads int
}

func NewFake(cfg Config) *Fake {
	base := strings.TrimSuffix(cfg.BaseURL, "/")
	if base == "" {
		base = "http://localhost:8080"
	}
	return &Fake{
		baseURL:              base,
		bySubmission:         map[string]*fakeRequest{},
		byPayeeRef:           map[string]*fakeRequest{},
		runID:                strconv.FormatInt(time.Now().UnixNano(), 36),
		idempotentOnPayeeRef: true,
		tinMatchAfter:        2,
		tinMatchResult:       TINMatchMatched,
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

// SetIdempotentOnPayeeRef switches the behaviour that Q1 in the build plan has
// not settled. With it off, every request mints a new submission — which is
// what makes a growing status array and a stale stored id reproducible here
// rather than in production.
func (f *Fake) SetIdempotentOnPayeeRef(idempotent bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idempotentOnPayeeRef = idempotent
}

func (f *Fake) Name() string { return "fake" }

// EnsurePayee mirrors the real adapter: a local no-op. There is no payee
// resource at this vendor; identity rides on the PayeeRef we choose.
func (f *Fake) EnsurePayee(_ context.Context, in PayeeInput) (PayeeResult, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return PayeeResult{}, fmt.Errorf("user id is required to identify a payee")
	}
	return PayeeResult{ProviderPayeeID: in.UserID}, nil
}

func (f *Fake) CreateW9Request(_ context.Context, in W9RequestInput) (W9Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requestFor(f.upsert(in)), nil
}

// HostedFormURL re-mints against the same payee reference, which is exactly
// what the real adapter does — so whether a second submission appears is
// governed by the same switch.
func (f *Fake) HostedFormURL(_ context.Context, providerRequestID string, in W9RequestInput) (W9Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Mirrors the real adapter: a link we already hold is reused rather than
	// re-requested, because the vendor is not idempotent on the payee
	// reference and its links do not expire.
	if existing := strings.TrimSpace(in.ExistingFormURL); existing != "" {
		return W9Request{ProviderRequestID: providerRequestID, FormURL: existing}, nil
	}
	if strings.TrimSpace(in.UserID) == "" {
		existing, ok := f.bySubmission[providerRequestID]
		if !ok {
			return W9Request{}, fmt.Errorf("unknown form request %q", providerRequestID)
		}
		return f.requestFor(existing), nil
	}
	return f.requestFor(f.upsert(in)), nil
}

func (f *Fake) upsert(in W9RequestInput) *fakeRequest {
	ref := referenceID(in.UserID, in.TaxYear)
	if ref == "sfluv::0" {
		f.seq++
		ref = fmt.Sprintf("sfluv:anon:%s:%d", f.runID, f.seq)
	}

	if existing, ok := f.byPayeeRef[ref]; ok && f.idempotentOnPayeeRef {
		return existing
	}

	f.seq++
	req := &fakeRequest{
		PayeeRef:     ref,
		SubmissionID: fmt.Sprintf("%s-%04d", f.runID, f.seq),
		UserID:       in.UserID,
		TaxYear:      in.TaxYear,
		w9Status:     "REQUEST_SENT",
	}
	f.bySubmission[req.SubmissionID] = req
	// The payee ref points at the most recent submission, mirroring what a
	// status-by-payee lookup would surface.
	f.byPayeeRef[ref] = req
	return req
}

func (f *Fake) requestFor(r *fakeRequest) W9Request {
	return W9Request{
		ProviderRequestID: r.SubmissionID,
		FormURL:           fmt.Sprintf("%s/w9/fake/form/%s", f.baseURL, r.SubmissionID),
		// No expiry is claimed. TaxBandits documents none for W9Url, and
		// inventing one here would be exactly the kind of plausible fiction
		// this file exists to avoid.
	}
}

// Sign is what the stub form's submit button calls.
//
// It moves the request into the signed-but-unmatched state the vendor models
// explicitly, which is the state we release money on. Nothing is pushed
// anywhere — the backend finds out by polling, exactly as it will in
// production.
func (f *Fake) Sign(providerRequestID string, tinType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	req, ok := f.bySubmission[providerRequestID]
	if !ok {
		return fmt.Errorf("unknown form request %q", providerRequestID)
	}
	now := time.Now().UTC()
	req.w9Status = "COMPLETED_AND_TIN_MATCH_INPROGRESS"
	req.completedAt = &now
	if tinType == "" {
		tinType = "ssn"
	}
	req.tinType = tinType
	return nil
}

func (f *Fake) GetW9Status(_ context.Context, providerRequestID string) (W9Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	req, ok := f.bySubmission[providerRequestID]
	if !ok {
		return W9Status{Status: StatusNotStarted}, nil
	}

	// Mapped through the same function the real adapter uses, so a mapping bug
	// fails here too rather than only against the live vendor.
	status := W9Status{Status: tbNormaliseStatus(req.w9Status), TINType: req.tinType}
	if status.Status != StatusCompleted {
		return status, nil
	}
	status.CompletedAt = req.completedAt

	// The match resolves in the background, some reads after signing.
	req.statusReads++
	if req.statusReads < f.tinMatchAfter {
		status.TINMatch = TINMatchPending
		return status, nil
	}

	// Mirrors what the vendor actually does, which is not what its docs imply:
	// a successful match drops the in-progress suffix, and a FAILED one flips
	// the whole request to INVALID and erases the TINMatching object entirely.
	// Modelling the erasure matters — an earlier fake reported a tidy
	// "rejected" verdict that the real vendor never sends, and the caller's
	// rejection handling was dead code as a result.
	switch f.tinMatchResult {
	case TINMatchMatched:
		req.w9Status = "COMPLETED"
	case TINMatchRejected:
		req.w9Status = "INVALID"
	}

	// Re-derived from the new status, exactly as the real adapter does.
	status.Status = tbNormaliseStatus(req.w9Status)
	status.TINMatch = f.tinMatchResult
	if status.Status == StatusInvalid {
		status.CompletedAt = nil
	}
	return status, nil
}

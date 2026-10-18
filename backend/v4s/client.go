package v4s

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Default bounds. A listing page for a daily job runs to ~100 KB over two
// months, so the read limit is generous rather than tight.
const (
	DefaultTTL      = time.Minute
	DefaultTimeout  = 15 * time.Second
	DefaultMonths   = 2
	maxResponseSize = 4 << 20
)

// Snapshot is one job's shifts as of one fetch.
type Snapshot struct {
	Shifts []Shift   `json:"shifts"`
	AsOf   time.Time `json:"as_of"`
	// Err is the failure that produced this snapshot, if the last attempt
	// failed. Shifts may still be populated, from the previous good fetch.
	Err error `json:"-"`
}

// Fresh reports whether the snapshot is young enough to serve without a refetch.
func (s Snapshot) Fresh(ttl time.Duration) bool {
	return !s.AsOf.IsZero() && time.Since(s.AsOf) < ttl
}

// Client reads V4S job listings on demand, never on a timer.
//
// The access pattern this is built for is pull, not poll: a request for an
// event's availability triggers at most one fetch, and only when what we hold
// is older than the TTL. Nothing fetches in the background on its own schedule,
// so a job nobody is looking at costs nothing.
//
// Three things keep us a polite guest on someone else's site:
//
//   - One fetch per job per TTL (a minute by default), whoever asks.
//   - Concurrent askers for the same job share a single fetch (singleflight),
//     so a burst of traffic is still one request.
//   - A stale snapshot is served while a refresh runs, so a slow or failing
//     site degrades to slightly old numbers rather than to a slow page.
type Client struct {
	http    *http.Client
	ttl     time.Duration
	months  int
	group   singleflight.Group
	mu      sync.RWMutex
	cache   map[string]Snapshot
	refresh map[string]bool
	// now is swappable for tests.
	now func() time.Time
}

// NewClient builds a client with the default bounds.
func NewClient() *Client {
	return &Client{
		http:    &http.Client{Timeout: DefaultTimeout},
		ttl:     DefaultTTL,
		months:  DefaultMonths,
		cache:   map[string]Snapshot{},
		refresh: map[string]bool{},
		now:     time.Now,
	}
}

// WithTTL overrides how long a snapshot is served before a refetch is allowed.
func (c *Client) WithTTL(ttl time.Duration) *Client {
	if ttl > 0 {
		c.ttl = ttl
	}
	return c
}

// Cached returns what we hold for a job without fetching anything.
func (c *Client) Cached(src Source) (Snapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	snap, ok := c.cache[src.Key()]
	return snap, ok
}

// Get returns a job's shifts, fetching only if what we hold has aged out.
//
// It blocks on the fetch when we hold nothing at all, because there is nothing
// useful to return. When we hold something stale it returns that immediately
// and refreshes behind the request, so a page render is never waiting on
// someone else's Salesforce instance.
func (c *Client) Get(ctx context.Context, src Source) (Snapshot, error) {
	return c.GetWithTTL(ctx, src, c.ttl)
}

// GetWithTTL is Get with a per-partner freshness bound, so one site can be read
// more or less often than another without a second client.
func (c *Client) GetWithTTL(ctx context.Context, src Source, ttl time.Duration) (Snapshot, error) {
	if ttl <= 0 {
		ttl = c.ttl
	}
	key := src.Key()

	c.mu.RLock()
	snap, held := c.cache[key]
	c.mu.RUnlock()

	if held && snap.Fresh(ttl) {
		return snap, nil
	}
	if held {
		c.refreshInBackground(src, ttl)
		return snap, nil
	}
	return c.fetchShared(ctx, src)
}

// refreshInBackground starts one refresh for a job, if one is not already
// running. The TTL check happens inside the lock so two requests arriving
// together cannot both start one.
func (c *Client) refreshInBackground(src Source, ttl time.Duration) {
	key := src.Key()

	c.mu.Lock()
	if c.refresh[key] {
		c.mu.Unlock()
		return
	}
	if snap, ok := c.cache[key]; ok && snap.Fresh(ttl) {
		c.mu.Unlock()
		return
	}
	c.refresh[key] = true
	c.mu.Unlock()

	go func() {
		defer func() {
			c.mu.Lock()
			delete(c.refresh, key)
			c.mu.Unlock()
		}()
		// Detached from the request that triggered it: the volunteer whose page
		// view started this has already been served.
		ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout+5*time.Second)
		defer cancel()
		_, _ = c.fetchShared(ctx, src)
	}()
}

// fetchShared collapses concurrent fetches of the same job into one.
func (c *Client) fetchShared(ctx context.Context, src Source) (Snapshot, error) {
	key := src.Key()
	out, err, _ := c.group.Do(key, func() (any, error) {
		shifts, ferr := c.fetch(ctx, src)
		now := c.now()

		c.mu.Lock()
		defer c.mu.Unlock()
		if ferr != nil {
			// Keep the last good shifts and mark the failure. Availability that
			// is a few minutes old beats a card that suddenly shows nothing
			// because someone else's site had a bad minute.
			snap := c.cache[key]
			snap.Err = ferr
			if snap.AsOf.IsZero() {
				snap.AsOf = time.Time{}
			}
			c.cache[key] = snap
			return snap, ferr
		}
		snap := Snapshot{Shifts: shifts, AsOf: now}
		c.cache[key] = snap
		return snap, nil
	})

	snap, _ := out.(Snapshot)
	return snap, err
}

func (c *Client) fetch(ctx context.Context, src Source) ([]Shift, error) {
	target := JobListingURL(src.Site, src.JobId, c.months)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	// Identify ourselves. If this ever becomes unwelcome, the organization can
	// see who it is and tell us rather than having to block an anonymous agent.
	req.Header.Set("User-Agent", "SFLuvBot/1.0 (+https://sfluv.org; volunteer availability sync)")
	req.Header.Set("Accept", "text/html")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("v4s: %s returned %d", target, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, err
	}
	return ParseJobListing(string(body))
}

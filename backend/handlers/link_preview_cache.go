package handlers

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// linkPreviewCache remembers recent link scrapes, so a link everybody in a
// group can see is fetched once rather than once per phone.
//
// Link cards are rendered by the client from the message text now, not frozen
// onto the message at send time. That moved the fetch from "once, when sent"
// to "once per device that shows it", which for a link dropped into a busy
// group is a burst of identical outbound requests from this server to someone
// else's. Twenty minutes is long enough to absorb that burst and short enough
// that a page which changes its title is not stuck on the old one for long.
//
// Bounded and in-memory: a restart empties it, which costs one fetch per link.
// Past the bound, the soonest-to-expire entries go first. The zero value is
// ready to use.
type linkPreviewCache struct {
	mu      sync.Mutex
	entries map[string]linkPreviewEntry
	// inflight merges concurrent misses for one URL into one scrape. A link
	// posted into a group reaches every online member at the same instant, and
	// they all ask for the card together — without this they all miss, and the
	// burst the cache exists to absorb goes straight through it.
	inflight singleflight.Group
}

// fetch returns the cached metadata for url, or runs scrape — once, however
// many callers are waiting — and caches what it returns.
func (c *linkPreviewCache) fetch(url string, scrape func() linkMetadata) linkMetadata {
	if meta, ok := c.get(url); ok {
		return meta
	}
	value, _, _ := c.inflight.Do(url, func() (any, error) {
		meta := scrape()
		c.put(url, meta)
		return meta, nil
	})
	return value.(linkMetadata)
}

type linkPreviewEntry struct {
	meta    linkMetadata
	expires time.Time
}

const (
	linkPreviewTTL = 20 * time.Minute
	// An empty result — the page refused us, timed out, or had no metadata —
	// is kept for less time, so a site that was briefly down gets another go.
	linkPreviewEmptyTTL = 2 * time.Minute
	linkPreviewMax      = 2000
)

func (c *linkPreviewCache) get(url string) (linkMetadata, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[url]
	if !ok || time.Now().After(entry.expires) {
		return linkMetadata{}, false
	}
	return entry.meta, true
}

func (c *linkPreviewCache) put(url string, meta linkMetadata) {
	ttl := linkPreviewTTL
	if meta == (linkMetadata{}) {
		ttl = linkPreviewEmptyTTL
	}
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]linkPreviewEntry{}
	}
	if len(c.entries) >= linkPreviewMax {
		// Expired first; if that frees nothing, the one closest to expiring.
		// A linear sweep, run only at the bound, over at most 2000 entries.
		var soonestKey string
		var soonest time.Time
		for key, entry := range c.entries {
			if now.After(entry.expires) {
				delete(c.entries, key)
				continue
			}
			if soonestKey == "" || entry.expires.Before(soonest) {
				soonestKey, soonest = key, entry.expires
			}
		}
		if len(c.entries) >= linkPreviewMax && soonestKey != "" {
			delete(c.entries, soonestKey)
		}
	}
	c.entries[url] = linkPreviewEntry{meta: meta, expires: now.Add(ttl)}
}

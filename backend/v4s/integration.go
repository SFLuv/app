package v4s

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Partner integrations, defined in a JSON file beside the other backend config.
//
// Each entry pairs one organization with one Volunteers for Salesforce site.
// That pairing is the point: an organization may only cause fetches of the site
// it was configured with, so an event belonging to one partner can never send
// us to another's host — or to an arbitrary one, if a link is mistyped or
// malicious.
//
// Everything is closed by default. No file, an unreadable file, a disabled
// file, or an organization with no entry all mean the same thing: the link is
// not parsed and no request is made.
//
//	{
//	  "enabled": true,
//	  "integrations": [
//	    {
//	      "organization": "St. Anthony's",
//	      "site": "https://stanthonysf.my.salesforce-sites.com/volunteer",
//	      "enabled": true
//	    }
//	  ]
//	}
const (
	// DefaultConfigFile is the path used when V4S_CONFIG_FILE is unset.
	DefaultConfigFile = "v4s-integrations.json"
	// EnvConfigFile points at a different file, for a deployment that keeps
	// config outside the image.
	EnvConfigFile = "V4S_CONFIG_FILE"
	// HouseOrganization names SFLuv's own organization in an entry. It is
	// matched on organizations.is_house rather than on the name, so renaming
	// the organization cannot change what is synced.
	HouseOrganization = "house"
)

// Integration is one partner: an organization, and the site we may read for it.
type Integration struct {
	// Organization is the organization's name, or "house" for SFLuv's own.
	Organization string `json:"organization"`
	// Site is the Salesforce Site origin and path prefix, e.g.
	// https://example.my.salesforce-sites.com/volunteer
	Site string `json:"site"`
	// Enabled allows one partner to be switched off without touching the rest.
	// Absent means false: an entry has to say so.
	Enabled bool `json:"enabled"`
	// TTLSeconds overrides how long a snapshot is served before a refetch is
	// allowed. Zero uses the default of one minute.
	TTLSeconds int `json:"ttl_seconds,omitempty"`
	// Notes is for whoever reads this file next. Ignored by the code.
	Notes string `json:"notes,omitempty"`

	// host is derived from Site at load time so the hot path does no parsing.
	host string
	// normalized is the comparison form of Organization.
	normalized string
}

// Config is the whole file.
type Config struct {
	// Enabled is the master switch. False, or absent, disables every entry
	// whatever they say individually.
	Enabled      bool          `json:"enabled"`
	Integrations []Integration `json:"integrations"`

	// loadedFrom and loadErr are for the boot log.
	loadedFrom string
	loadErr    error
}

// LoadConfig reads the integrations file.
//
// A missing file is not an error: it is the ordinary state of a deployment that
// has not opted in, and it yields a disabled config. A malformed file *is*
// reported, but still yields a disabled config — a typo in this file must not
// fail a boot, and must not accidentally enable anything either.
func LoadConfig(path string) Config {
	if path == "" {
		path = strings.TrimSpace(os.Getenv(EnvConfigFile))
	}
	if path == "" {
		path = DefaultConfigFile
	}

	// Try the same candidates the client config does, so the file is found
	// whether the server runs from the repo root or from backend/.
	var raw []byte
	var err error
	found := ""
	for _, candidate := range configCandidates(path) {
		raw, err = os.ReadFile(candidate)
		if err == nil {
			found = candidate
			break
		}
	}
	if found == "" {
		if os.IsNotExist(err) {
			// The ordinary state of a deployment that has not opted in.
			return Config{loadedFrom: path}
		}
		return Config{loadedFrom: path, loadErr: err}
	}
	path = found

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{loadedFrom: path, loadErr: fmt.Errorf("parsing %s: %w", path, err)}
	}
	cfg.loadedFrom = path

	kept := cfg.Integrations[:0]
	for _, entry := range cfg.Integrations {
		entry.normalized = normalizeOrg(entry.Organization)
		entry.Site = strings.TrimRight(strings.TrimSpace(entry.Site), "/")
		if entry.normalized == "" || entry.Site == "" {
			continue // an entry naming nothing, or pointing nowhere
		}
		parsed, perr := url.Parse(entry.Site)
		if perr != nil || parsed.Host == "" {
			continue
		}
		entry.host = strings.ToLower(parsed.Hostname())
		kept = append(kept, entry)
	}
	cfg.Integrations = kept
	return cfg
}

// configCandidates mirrors how the client config resolves its fallback file:
// the path as given, and the same name under backend/, so the server finds it
// whether it was started from the repository root or from the backend
// directory.
func configCandidates(path string) []string {
	if filepath.IsAbs(path) {
		return []string{path}
	}
	candidates := []string{path}
	if filepath.Dir(path) == "." {
		candidates = append(candidates, filepath.Join("backend", path))
	}
	return candidates
}

// normalizeOrg reduces a name to something two people typing it would agree on:
// case, punctuation and spacing removed. "St. Anthony's", "st anthonys" and
// "ST ANTHONYS" are the same organization.
func normalizeOrg(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Active reports whether anything is switched on at all.
func (c Config) Active() bool {
	if !c.Enabled {
		return false
	}
	for _, entry := range c.Integrations {
		if entry.Enabled {
			return true
		}
	}
	return false
}

// For returns the integration covering an organization, if one is enabled.
//
// The house organization is matched on its flag rather than its name, so a
// rename cannot move it in or out of this list.
func (c Config) For(name string, isHouse bool) (Integration, bool) {
	if !c.Enabled {
		return Integration{}, false
	}
	wanted := normalizeOrg(name)
	for _, entry := range c.Integrations {
		if !entry.Enabled {
			continue
		}
		if entry.normalized == HouseOrganization {
			if isHouse {
				return entry, true
			}
			continue
		}
		if entry.normalized == wanted {
			return entry, true
		}
	}
	return Integration{}, false
}

// AllowsSite reports whether a link's site is the one this integration was
// configured with. A partner's event may only cause a fetch of that partner's
// site, so a mistyped or hostile link resolves to nothing.
func (i Integration) AllowsSite(site string) bool {
	parsed, err := url.Parse(strings.TrimSpace(site))
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), i.host)
}

// TTL is how long a snapshot is served before a refetch is allowed.
func (i Integration) TTL() time.Duration {
	if i.TTLSeconds > 0 {
		return time.Duration(i.TTLSeconds) * time.Second
	}
	return DefaultTTL
}

// Describe summarises the resolved config for the boot log, so whether an
// integration that reaches out to a third party is live is visible without
// reading the file back.
func (c Config) Describe() string {
	switch {
	case c.loadErr != nil:
		return fmt.Sprintf("v4s partner availability: DISABLED, could not read config (%s)", c.loadErr)
	case !c.Enabled:
		return fmt.Sprintf("v4s partner availability: disabled (%s)", c.loadedFrom)
	case len(c.Integrations) == 0:
		return fmt.Sprintf("v4s partner availability: enabled but no usable integrations in %s", c.loadedFrom)
	}
	parts := make([]string, 0, len(c.Integrations))
	for _, entry := range c.Integrations {
		state := "off"
		if entry.Enabled {
			state = "on"
		}
		parts = append(parts, fmt.Sprintf("%s -> %s [%s]", entry.Organization, entry.host, state))
	}
	return fmt.Sprintf("v4s partner availability: enabled (%s): %s", c.loadedFrom, strings.Join(parts, "; "))
}

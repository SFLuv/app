// Package funding keeps the protocol's operational accounts in gas.
//
// The system has several accounts that must hold CELO to work, and each one
// failing looks like a different product bug. When the sponsored-transaction
// account ran dry, every merchant payment stopped at once and the first signal
// was a merchant reporting that payments "weren't working" — nothing watched
// the balance, and nothing connected the symptom to the cause.
//
// This module makes the faucet the single account a person has to think about.
// It tops the others up from the faucet when they fall below their floor, and
// it emails when the faucet itself is running low, because that is the one
// balance no amount of automation can refill.
package funding

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultConfigFile is read when FUNDING_CONFIG_FILE is unset.
	DefaultConfigFile = "funding-accounts.json"
	// EnvConfigFile points at a config kept outside the image.
	EnvConfigFile = "FUNDING_CONFIG_FILE"
	// EnvAlertEmails is the comma-separated list told when the faucet is low.
	// Empty falls back to the site admin address.
	EnvAlertEmails = "FUNDING_ALERT_EMAILS"
	// EnvAutoTopUp overrides the file's "enabled" field, so moving funds can be
	// switched on or off without editing a file or shipping a deploy.
	//
	// Unset defers to the file. "true" turns top-ups on; anything else
	// recognisably false turns them off and is the kill switch: if this is ever
	// doing something unexpected in production, setting it to false and
	// restarting stops every transfer without touching the config or the code.
	EnvAutoTopUp = "FUNDING_AUTO_TOPUP"
)

// Account is one address the faucet keeps funded.
type Account struct {
	// Name is what a person calls it, and what appears in the admin panel and
	// in alert emails.
	Name string `json:"name"`
	// Address is the account. Where it is empty, AddressEnv is read instead,
	// so an account whose address already lives in the environment is not
	// configured twice and cannot drift between the two.
	Address string `json:"address,omitempty"`
	// AddressEnv names an environment variable holding the address.
	AddressEnv string `json:"address_env,omitempty"`
	// MinCelo is the floor. Below this the account is topped up.
	MinCelo float64 `json:"min_celo"`
	// TopUpToCelo is the balance to bring it back to. It is a target, not an
	// amount to add, so repeated top-ups cannot ratchet an account upwards.
	TopUpToCelo float64 `json:"top_up_to_celo"`
	// Priority orders funding when the faucet cannot cover every account at
	// once: lower goes first. The paymaster sponsor should be 1 — every
	// sponsored user transaction depends on it, so it is the account whose
	// exhaustion takes the whole system down rather than one feature of it.
	// Unset sorts last.
	Priority int `json:"priority,omitempty"`
	// AutoTopUp allows an account to be watched without being funded — useful
	// for one we do not own, or one being observed before trusting automation
	// with it.
	AutoTopUp bool `json:"auto_top_up"`
	// NeedsGas marks an account that must hold CELO even after the protocol's
	// own transactions became sponsored.
	//
	// There is exactly one: the paymaster, which is the account doing the
	// sponsoring. Everything else of ours sends through it and so needs no gas
	// of its own, and a floor left on one of those would alert about an account
	// that is empty by design. Absent means "no gas needed", which is the right
	// default now that it is the common case.
	NeedsGas bool `json:"needs_gas,omitempty"`
	// Notes is for whoever reads the file next. Ignored by the code.
	Notes string `json:"notes,omitempty"`

	// resolved is the address after consulting AddressEnv.
	resolved string
}

// ResolvedAddress is the account's address, from the file or the environment.
func (a Account) ResolvedAddress() string { return a.resolved }

// sortPriority treats an unset priority as "after everything explicit", so
// adding a new account without thinking about order cannot displace the
// paymaster from the front of the queue.
func (a Account) sortPriority() int {
	if a.Priority <= 0 {
		return 1 << 20
	}
	return a.Priority
}

// Config is the whole file.
type Config struct {
	// Enabled is the master switch for automatic top-ups. With it off the
	// balances are still read and still alerted on — knowing is useful even
	// where acting automatically is not yet wanted — but no funds move.
	Enabled bool `json:"enabled"`

	// Alerts controls the low-balance and drain emails. It defaults to ON:
	// absent means enabled, which is the opposite of how Enabled behaves, and
	// deliberately so. Moving money should require someone to opt in; being
	// told that money is about to run out should not. Set it to false only to
	// silence a known-noisy deployment.
	Alerts *bool `json:"alerts,omitempty"`

	// Faucet describes the source account: the one a person keeps funded.
	Faucet FaucetConfig `json:"faucet"`

	// Accounts are the accounts kept in gas from the faucet.
	Accounts []Account `json:"accounts"`

	loadedFrom string
	loadErr    error
	// enabledSource records where the auto-top-up setting came from, so the
	// boot log can say whether the file or the environment decided it.
	enabledSource string
	// skipped names accounts that were dropped because their address could not
	// be resolved. An account that is silently not watched is the failure this
	// package exists to prevent, so the boot log names them.
	skipped []string
}

// FaucetConfig is the source account and the floors that trigger an alert.
type FaucetConfig struct {
	// Name for display.
	Name string `json:"name"`
	// AddressEnv names the variable holding the faucet address. The faucet's
	// address is derived from its key at runtime where possible; this is the
	// fallback and the thing the admin panel labels.
	AddressEnv string `json:"address_env,omitempty"`
	// MinCelo is the floor below which someone is emailed. It should sit above
	// the sum of what the accounts below need, so there is warning rather than
	// a failure to top up.
	MinCelo float64 `json:"min_celo"`
	// MinSfluv is the floor for the reward balance, in whole SFLUV.
	MinSfluv float64 `json:"min_sfluv"`
	// ReserveCelo is held back from top-ups so the faucet can always pay for
	// its own transactions. Without it a faucet can spend itself down to where
	// it cannot send the transaction that would fix anything.
	ReserveCelo float64 `json:"reserve_celo"`
}

// LoadConfig reads the funding file.
//
// A missing file yields a disabled config: a deployment that has not set this
// up is not broken, it simply does not fund anything. A malformed file is
// reported but also yields a disabled config — a typo must not take out a boot,
// and must not move money either.
func LoadConfig(path string) Config {
	if path == "" {
		path = strings.TrimSpace(os.Getenv(EnvConfigFile))
	}
	if path == "" {
		path = DefaultConfigFile
	}

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
			return Config{loadedFrom: path}
		}
		return Config{loadedFrom: path, loadErr: err}
	}

	var cfg Config
	if jerr := json.Unmarshal(raw, &cfg); jerr != nil {
		return Config{loadedFrom: found, loadErr: fmt.Errorf("parsing %s: %w", found, jerr)}
	}
	cfg.loadedFrom = found

	kept := cfg.Accounts[:0]
	for _, account := range cfg.Accounts {
		account.resolved = strings.TrimSpace(account.Address)
		if account.resolved == "" && account.AddressEnv != "" {
			account.resolved = strings.TrimSpace(os.Getenv(account.AddressEnv))
		}
		if account.resolved == "" {
			// An account we cannot locate is dropped rather than guessed at —
			// but never silently: an unset address means that account is not
			// being watched, which looks identical to it being healthy.
			where := account.AddressEnv
			if where == "" {
				where = "no address or address_env"
			} else {
				where = where + " is unset"
			}
			cfg.skipped = append(cfg.skipped, fmt.Sprintf("%s (%s)", orBlank(account.Name), where))
			continue
		}
		if account.TopUpToCelo < account.MinCelo {
			// A target below the floor would top up into an immediate retop.
			account.TopUpToCelo = account.MinCelo
		}
		kept = append(kept, account)
	}
	cfg.Accounts = kept

	// The environment wins over the file, so the switch can be flipped on a
	// running deployment without editing anything.
	if override, set := autoTopUpOverride(); set {
		cfg.Enabled = override
		cfg.enabledSource = EnvAutoTopUp
	} else {
		cfg.enabledSource = found
	}
	return cfg
}

// autoTopUpOverride reads FUNDING_AUTO_TOPUP. The second return reports whether
// it was set at all, because "unset" and "false" mean different things: unset
// defers to the file, false overrides it.
func autoTopUpOverride() (bool, bool) {
	raw := strings.TrimSpace(os.Getenv(EnvAutoTopUp))
	if raw == "" {
		return false, false
	}
	switch strings.ToLower(raw) {
	case "true", "1", "yes", "on":
		return true, true
	case "false", "0", "no", "off":
		return false, true
	}
	// An unrecognised value is treated as off rather than ignored: a typo in a
	// switch that moves money should fail closed, not fall through to the file.
	return false, true
}

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

// Skipped names accounts dropped for want of an address.
func (c Config) Skipped() []string { return c.skipped }

func orBlank(name string) string {
	if strings.TrimSpace(name) == "" {
		return "(unnamed account)"
	}
	return name
}

// EnabledSource names whatever decided the auto-top-up setting: the config
// file, or the environment variable that overrode it.
func (c Config) EnabledSource() string {
	if c.enabledSource == "" {
		return c.loadedFrom
	}
	return c.enabledSource
}

// AlertsEnabled reports whether alert emails should be sent. Absent means yes.
func (c Config) AlertsEnabled() bool { return c.Alerts == nil || *c.Alerts }

// LoadedFrom is the file the config came from, for the boot log.
func (c Config) LoadedFrom() string { return c.loadedFrom }

// Err is the load failure, if any.
func (c Config) Err() error { return c.loadErr }

// AlertEmails is who to tell when the faucet is low.
//
// FUNDING_ALERT_EMAILS is a comma-separated list. When it is empty the site
// admin address is used, so an unconfigured deployment still tells somebody
// rather than nobody.
func AlertEmails(fallback string) []string {
	out := []string{}
	seen := map[string]bool{}
	for entry := range strings.SplitSeq(os.Getenv(EnvAlertEmails), ",") {
		address := strings.TrimSpace(entry)
		if address == "" || seen[strings.ToLower(address)] {
			continue
		}
		seen[strings.ToLower(address)] = true
		out = append(out, address)
	}
	if len(out) == 0 && strings.TrimSpace(fallback) != "" {
		out = append(out, strings.TrimSpace(fallback))
	}
	return out
}

// ── Units ────────────────────────────────────────────────────────────────────

// weiPerCelo is 1e18. CELO has 18 decimals; SFLUV has 6, and the two are mixed
// in this package, so every conversion names which it is doing.
var weiPerCelo = new(big.Float).SetFloat64(1e18)

// CeloToWei converts a configured CELO amount to wei.
func CeloToWei(celo float64) *big.Int {
	wei, _ := new(big.Float).Mul(new(big.Float).SetFloat64(celo), weiPerCelo).Int(nil)
	return wei
}

// WeiToCelo converts wei to CELO for display.
func WeiToCelo(wei *big.Int) float64 {
	if wei == nil {
		return 0
	}
	out, _ := new(big.Float).Quo(new(big.Float).SetInt(wei), weiPerCelo).Float64()
	return out
}

// sfluvBaseUnits is 1e6: SFLUV has six decimals, not eighteen. Mixing this up
// with wei is the classic error in this codebase's domain, so the conversion
// lives next to the CELO one where the difference is visible.
var sfluvBaseUnits = new(big.Float).SetFloat64(1e6)

// SfluvToBase converts whole SFLUV to base units.
func SfluvToBase(sfluv float64) *big.Int {
	base, _ := new(big.Float).Mul(new(big.Float).SetFloat64(sfluv), sfluvBaseUnits).Int(nil)
	return base
}

// BaseToSfluv converts base units to whole SFLUV for display.
func BaseToSfluv(base *big.Int) float64 {
	if base == nil {
		return 0
	}
	out, _ := new(big.Float).Quo(new(big.Float).SetInt(base), sfluvBaseUnits).Float64()
	return out
}

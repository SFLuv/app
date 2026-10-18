package funding

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"
)

// Chain is the slice of the faucet this package needs. Kept narrow so the
// sweep can be exercised without a chain.
type Chain interface {
	NativeBalance(ctx context.Context, address string) (*big.Int, error)
	TokenBalance(ctx context.Context, address string) (*big.Int, error)
	SendNative(ctx context.Context, to string, amount *big.Int) (string, error)
	Address() (string, error)
}

// Notifier sends an operational alert. Separated from the service so the
// decision to alert can be tested without sending mail.
type Notifier interface {
	Notify(subject string, body string) error
}

// Logf matches the logger used across the backend.
type Logf func(format string, args ...any)

// AccountBalance is one account's state, as the admin panel shows it.
type AccountBalance struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	// Celo is the native balance, in CELO.
	Celo float64 `json:"celo"`
	// MinCelo and TopUpToCelo echo the configured policy so the panel can show
	// a balance against the floor it is judged by rather than a bare number.
	MinCelo     float64 `json:"min_celo"`
	TopUpToCelo float64 `json:"top_up_to_celo"`
	AutoTopUp   bool    `json:"auto_top_up"`
	// Low is whether it is under its floor right now.
	Low bool `json:"low"`
	// Error is set when this account's balance could not be read, so a panel
	// shows "unknown" rather than zero. A zero balance and an unreachable RPC
	// must not look the same.
	Error string `json:"error,omitempty"`
}

// Snapshot is every balance at one moment.
type Snapshot struct {
	// Faucet is the source account, which carries both balances.
	Faucet struct {
		Name     string  `json:"name"`
		Address  string  `json:"address"`
		Celo     float64 `json:"celo"`
		Sfluv    float64 `json:"sfluv"`
		MinCelo  float64 `json:"min_celo"`
		MinSfluv float64 `json:"min_sfluv"`
		LowCelo  bool    `json:"low_celo"`
		LowSfluv bool    `json:"low_sfluv"`
		Error    string  `json:"error,omitempty"`
	} `json:"faucet"`
	Accounts []AccountBalance `json:"accounts"`
	// AutoTopUpEnabled reflects the config, so the panel can say whether a low
	// account will fix itself or needs a person.
	AutoTopUpEnabled bool      `json:"auto_top_up_enabled"`
	TakenAt          time.Time `json:"taken_at"`
}

// Service reads balances, tops accounts up, and alerts on the faucet.
type Service struct {
	cfg      Config
	chain    Chain
	notifier Notifier
	logf     Logf

	mu sync.Mutex
	// lastAlert throttles the low-faucet email. The condition persists until
	// someone funds the faucet, and mailing every sweep would train people to
	// ignore it.
	lastAlert time.Time
	// alertEvery is how often the same standing condition may be re-sent.
	alertEvery time.Duration
	// lastTopUp guards against repeatedly funding the same account while an
	// earlier transaction is still pending: the balance has not moved yet, so
	// without this the next sweep would send again. It is also the hard ceiling
	// on how fast this can move funds at all — one transaction per account per
	// topUpEvery, whatever the balances say.
	lastTopUp  map[string]time.Time
	topUpEvery time.Duration
	// lastRapidAlert throttles the drain alert per account, so an account being
	// emptied over and over produces a warning rather than a mailstorm.
	lastRapidAlert map[string]time.Time
}

// NewService builds the funding service.
func NewService(cfg Config, chain Chain, notifier Notifier, logf Logf) *Service {
	return &Service{
		cfg:        cfg,
		chain:      chain,
		notifier:   notifier,
		logf:       logf,
		alertEvery: 6 * time.Hour,
		lastTopUp:  map[string]time.Time{},
		// One top-up per account per minute. This is a safety ceiling rather
		// than a tuning knob: if something is draining an account faster than
		// that, the right response is a person looking at it, not a faucet
		// keeping pace with the drain.
		topUpEvery:     time.Minute,
		lastRapidAlert: map[string]time.Time{},
	}
}

// Config exposes the loaded settings.
func (s *Service) Config() Config { return s.cfg }

func (s *Service) log(format string, args ...any) {
	if s.logf != nil {
		s.logf(format, args...)
	}
}

// faucetAddress prefers the address derived from the key, because that is the
// account transactions will actually come from. The configured variable is a
// fallback for display when no key is loaded.
func (s *Service) faucetAddress() string {
	if s.chain != nil {
		if address, err := s.chain.Address(); err == nil && address != "" {
			return address
		}
	}
	if s.cfg.Faucet.AddressEnv != "" {
		return envValue(s.cfg.Faucet.AddressEnv)
	}
	return ""
}

// Balances reads every account. Errors are attached per account rather than
// failing the whole snapshot, so one unreachable address does not blank the
// admin panel.
func (s *Service) Balances(ctx context.Context) Snapshot {
	snap := Snapshot{TakenAt: time.Now(), AutoTopUpEnabled: s.cfg.Enabled}

	snap.Faucet.Name = orDefault(s.cfg.Faucet.Name, "Faucet")
	snap.Faucet.Address = s.faucetAddress()
	snap.Faucet.MinCelo = s.cfg.Faucet.MinCelo
	snap.Faucet.MinSfluv = s.cfg.Faucet.MinSfluv

	if s.chain == nil || snap.Faucet.Address == "" {
		snap.Faucet.Error = "faucet address unknown"
	} else {
		if wei, err := s.chain.NativeBalance(ctx, snap.Faucet.Address); err != nil {
			snap.Faucet.Error = err.Error()
		} else {
			snap.Faucet.Celo = WeiToCelo(wei)
			snap.Faucet.LowCelo = s.cfg.Faucet.MinCelo > 0 && snap.Faucet.Celo < s.cfg.Faucet.MinCelo
		}
		if base, err := s.chain.TokenBalance(ctx, snap.Faucet.Address); err != nil {
			if snap.Faucet.Error == "" {
				snap.Faucet.Error = err.Error()
			}
		} else {
			snap.Faucet.Sfluv = BaseToSfluv(base)
			snap.Faucet.LowSfluv = s.cfg.Faucet.MinSfluv > 0 && snap.Faucet.Sfluv < s.cfg.Faucet.MinSfluv
		}
	}

	for _, account := range s.cfg.Accounts {
		entry := AccountBalance{
			Name:        account.Name,
			Address:     account.ResolvedAddress(),
			MinCelo:     account.MinCelo,
			TopUpToCelo: account.TopUpToCelo,
			AutoTopUp:   account.AutoTopUp,
		}
		if s.chain == nil {
			entry.Error = "no chain client"
		} else if wei, err := s.chain.NativeBalance(ctx, entry.Address); err != nil {
			entry.Error = err.Error()
		} else {
			entry.Celo = WeiToCelo(wei)
			entry.Low = account.MinCelo > 0 && entry.Celo < account.MinCelo
		}
		snap.Accounts = append(snap.Accounts, entry)
	}
	return snap
}

// SweepResult records what one sweep did, for the log and for tests.
type SweepResult struct {
	Funded  []string
	Skipped []string
	Errors  []string
	// Rapid names accounts topped up again within rapidWindow of the previous
	// top-up — a drain rather than ordinary use.
	Rapid   []string
	Alerted bool
}

// Sweep tops up every account below its floor, then alerts if the faucet is low.
//
// The order matters: accounts are funded first, so a sweep that finds the
// faucet low has already done whatever it still could, and the alert reports
// the state after that rather than before.
func (s *Service) Sweep(ctx context.Context) SweepResult {
	result := SweepResult{}
	snap := s.Balances(ctx)

	if s.cfg.Enabled && s.chain != nil {
		// What the faucet may spend, keeping its own reserve back. Without the
		// reserve a faucet can fund others down to where it cannot afford the
		// transaction that would fix anything.
		spendable := snap.Faucet.Celo - s.cfg.Faucet.ReserveCelo

		// Fund in priority order, so when the faucet cannot cover everything the
		// most critical account is the one that gets paid. The paymaster
		// sponsor is first: without it every sponsored user transaction fails,
		// which is every merchant payment in the system.
		for _, i := range s.fundingOrder() {
			entry := snap.Accounts[i]
			account := s.cfg.Accounts[i]
			switch {
			case entry.Error != "":
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", entry.Name, entry.Error))
				continue
			case !account.AutoTopUp:
				continue
			case !entry.Low:
				continue
			}

			need := account.TopUpToCelo - entry.Celo
			if need <= 0 {
				continue
			}
			if need > spendable {
				// Report rather than send a partial top-up: half a top-up
				// leaves the account below its floor and spends the faucet
				// down for no benefit.
				result.Skipped = append(result.Skipped, fmt.Sprintf(
					"%s needs %.4f CELO but the faucet can only spare %.4f", entry.Name, need, spendable))
				continue
			}
			if !s.mayTopUp(entry.Address) {
				result.Skipped = append(result.Skipped, fmt.Sprintf(
					"%s was topped up recently; waiting for it to confirm", entry.Name))
				continue
			}

			hash, err := s.chain.SendNative(ctx, entry.Address, CeloToWei(need))
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", entry.Name, err))
				continue
			}
			// An account that needed funding again within a minute or two of
			// the last top-up is being drained, not used. That is either a bug
			// or an attack, and either way it is worth a person looking before
			// the faucet quietly funds it all day.
			if gap, rapid := s.markToppedUp(entry.Address); rapid {
				result.Rapid = append(result.Rapid, fmt.Sprintf(
					"%s was topped up again %s after the last one", entry.Name, gap.Round(time.Second)))
			}
			spendable -= need
			result.Funded = append(result.Funded, fmt.Sprintf(
				"%s +%.4f CELO (%.4f -> %.4f) %s", entry.Name, need, entry.Celo, account.TopUpToCelo, hash))
			s.log("funding: topped up %s with %.4f CELO (%s)", entry.Name, need, hash)
		}
	}

	// A drain is more urgent than a low faucet and is reported separately: a
	// low faucet means "fund this soon", back-to-back top-ups mean "something
	// is wrong right now".
	s.maybeAlertRapid(result)

	if s.maybeAlert(snap, result) {
		result.Alerted = true
	}
	return result
}

// maybeAlertRapid emails when an account had to be topped up again almost
// immediately, throttled per account.
func (s *Service) maybeAlertRapid(result SweepResult) {
	if len(result.Rapid) == 0 || s.notifier == nil || !s.cfg.AlertsEnabled() {
		return
	}

	send := []string{}
	s.mu.Lock()
	for _, line := range result.Rapid {
		last, seen := s.lastRapidAlert[line]
		if seen && time.Since(last) < s.alertEvery {
			continue
		}
		s.lastRapidAlert[line] = time.Now()
		send = append(send, line)
	}
	s.mu.Unlock()
	if len(send) == 0 {
		return
	}

	subject := "SFLuv account is draining faster than it is being funded"
	body := "An account needed topping up again within two minutes of the last top-up.\n\n" +
		"Ordinary use does not do that. Either something is spending from this account much\n" +
		"faster than expected, or something is draining it. The faucet will keep funding it at\n" +
		"most once a minute, so it is rate limited rather than unbounded — but it is worth\n" +
		"looking before it keeps going.\n\n" +
		strings.Join(send, "\n") + "\n"
	if err := s.notifier.Notify(subject, body); err != nil {
		s.log("funding: could not send the rapid top-up alert: %s", err)
		return
	}
	s.log("funding: rapid top-up alert sent (%d account(s))", len(send))
}

// mayTopUp reports whether enough time has passed since this account's last
// top-up for another to be sensible.
func (s *Service) mayTopUp(address string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	last, seen := s.lastTopUp[address]
	return !seen || time.Since(last) >= s.topUpEvery
}

// markToppedUp records a top-up and reports whether it followed hard on the
// heels of the previous one for the same account.
//
// "Back to back" is judged against rapidWindow rather than against the calendar
// minute: two top-ups at 10:00:59 and 10:01:01 are two seconds apart and the
// thing worth noticing, while 10:00:01 and 10:01:59 are nearly two minutes
// apart and ordinary.
func (s *Service) markToppedUp(address string) (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	previous, seen := s.lastTopUp[address]
	s.lastTopUp[address] = now
	if !seen {
		return 0, false
	}
	gap := now.Sub(previous)
	return gap, gap <= rapidWindow
}

// rapidWindow is how close two top-ups of the same account have to be to count
// as back to back. Slightly over the one-minute floor the rate limiter imposes,
// so a top-up that happens at the first opportunity after the previous one is
// caught rather than missed by a second.
const rapidWindow = 125 * time.Second

// fundingOrder returns account indices in the order they should be funded:
// by configured priority, then by file order. Lower priority numbers go first;
// an account that sets none sorts after those that do.
func (s *Service) fundingOrder() []int {
	order := make([]int, len(s.cfg.Accounts))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return s.cfg.Accounts[order[a]].sortPriority() < s.cfg.Accounts[order[b]].sortPriority()
	})
	return order
}

// maybeAlert emails when the faucet is low, at most once per alertEvery.
func (s *Service) maybeAlert(snap Snapshot, result SweepResult) bool {
	// Alert when the faucet is low, and also when an account is below its floor
	// that nothing is going to fix on its own — with top-ups off, or an account
	// marked watch-only, a low balance is a standing problem that needs a
	// person, and saying nothing about it would defeat the point of watching.
	stranded := []string{}
	for i, entry := range snap.Accounts {
		if entry.Low && (!s.cfg.Enabled || !s.cfg.Accounts[i].AutoTopUp) {
			stranded = append(stranded, entry.Name)
		}
	}
	low := snap.Faucet.LowCelo || snap.Faucet.LowSfluv || len(stranded) > 0
	if !low || s.notifier == nil || !s.cfg.AlertsEnabled() {
		return false
	}

	s.mu.Lock()
	if !s.lastAlert.IsZero() && time.Since(s.lastAlert) < s.alertEvery {
		s.mu.Unlock()
		return false
	}
	s.lastAlert = time.Now()
	s.mu.Unlock()

	subject := "SFLuv faucet is running low"
	if !snap.Faucet.LowCelo && !snap.Faucet.LowSfluv {
		subject = "SFLuv protocol account needs funding"
	}
	body := BuildAlertBody(snap, result)
	if err := s.notifier.Notify(subject, body); err != nil {
		s.log("funding: could not send the low-faucet alert: %s", err)
		return false
	}
	s.log("funding: low-faucet alert sent (celo %.4f, sfluv %.2f)", snap.Faucet.Celo, snap.Faucet.Sfluv)
	return true
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

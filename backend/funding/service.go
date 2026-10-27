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

// ProtocolState is what this package needs to know about the smart-wallet
// migration, injected rather than imported because the handlers that own that
// migration already import this one.
//
// Two questions, and the second changes what is worth alerting about at all.
type ProtocolState interface {
	// FaucetAddress is where the faucet's money actually is. After the
	// migration that is a smart wallet, not the address derived from the key.
	FaucetAddress(ctx context.Context) (string, bool)
	// Sponsored reports whether the protocol's own transactions are paid for by
	// the paymaster rather than out of their own balance.
	Sponsored(ctx context.Context) bool
}

// Condition is one standing problem, named so it can be retracted later.
//
// The email notifier does not need this: an email is an event and stays sent.
// A chat message is a condition — it should be on screen while the problem
// exists and gone when it does not — which needs a stable key per problem and
// the complete set of current problems on every sweep. That is what a sink
// gets, so it can work out what is new and what has cleared without the
// funding service knowing anything about how it is displayed.
type Condition struct {
	// Key is stable for as long as the problem is the same problem.
	Key string
	// Title is the one line a person reads first.
	Title string
	// Detail says what to do about it.
	Detail string
	// Fields are label/value pairs, in display order.
	Fields [][2]string
	// Urgent marks a condition nothing will fix on its own.
	Urgent bool
}

// ConditionSink is an optional Notifier extension. A notifier that implements
// it is handed the full set of active conditions after every sweep.
type ConditionSink interface {
	Conditions(ctx context.Context, active []Condition) error
}

// Conditions derives the standing problems from a snapshot.
//
// Separate keys per problem rather than one "something is low" condition, so
// topping up CELO retracts the CELO alert while a low SFLUV balance keeps its
// own. One message per thing that is wrong is also what makes the channel
// readable: the alert either applies or it is gone.
func Conditions(snap Snapshot, cfg Config) []Condition {
	active := []Condition{}

	if snap.Faucet.LowCelo {
		active = append(active, Condition{
			Key:    "funding:faucet:celo",
			Title:  "Faucet is low on CELO",
			Detail: "Nothing can refill this but a person. Sponsored transactions and top-ups both stop when it empties.",
			Fields: [][2]string{
				{"Balance", fmt.Sprintf("%.4f CELO", snap.Faucet.Celo)},
				{"Floor", fmt.Sprintf("%.4f CELO", snap.Faucet.MinCelo)},
				{"Faucet", snap.Faucet.Address},
			},
			Urgent: true,
		})
	}

	if snap.Faucet.LowSfluv {
		active = append(active, Condition{
			Key:    "funding:faucet:sfluv",
			Title:  "Faucet is low on SFLUV",
			Detail: "Rewards and redemptions are paid from this balance.",
			Fields: [][2]string{
				{"Balance", fmt.Sprintf("%.2f SFLUV", snap.Faucet.Sfluv)},
				{"Floor", fmt.Sprintf("%.2f SFLUV", snap.Faucet.MinSfluv)},
				{"Faucet", snap.Faucet.Address},
			},
			Urgent: true,
		})
	}

	// An account below its floor that nothing is going to fix: top-ups off,
	// this one marked watch-only, or — after the migration — every account,
	// because nothing can top anything up any more. An account that will be
	// funded on the next sweep is not a condition; saying so would be an alert
	// about something already being handled.
	for i, entry := range snap.Accounts {
		if !entry.Low {
			continue
		}
		stranded := snap.Sponsored || !cfg.Enabled || (i < len(cfg.Accounts) && !cfg.Accounts[i].AutoTopUp)
		if !stranded {
			continue
		}

		detail := "Needs funding by hand — automatic top-ups are switched off."
		switch {
		case snap.Sponsored:
			// The whole of what this service is for, after the migration. Said
			// at length because it is the one gas balance left and the person
			// reading it needs to know both that only they can fix it and what
			// stops when they do not.
			detail = "This is the account that pays gas for every sponsored transaction, and it is the only CELO " +
				"balance left that anybody has to top up. Nothing can refill it automatically: our own balances " +
				"are in smart wallets, and moving CELO out of one is itself a sponsored transaction that this " +
				"account would have to pay for. When it empties, every payout, every merchant payment and every " +
				"key rotation stops at the same moment."
		case cfg.Enabled:
			detail = "Needs funding by hand — this account is watched, not funded."
		}

		active = append(active, Condition{
			Key:    "funding:account:" + strings.ToLower(entry.Address),
			Title:  entry.Name + " is below its floor",
			Detail: detail,
			Fields: [][2]string{
				{"Balance", fmt.Sprintf("%.4f CELO", entry.Celo)},
				{"Floor", fmt.Sprintf("%.4f CELO", entry.MinCelo)},
				{"Account", entry.Address},
			},
			Urgent: true,
		})
	}

	return active
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
	AutoTopUpEnabled bool `json:"auto_top_up_enabled"`
	// Sponsored is true once the protocol's transactions are paid for by the
	// paymaster. It changes what a low balance means: a protocol account with
	// no CELO is then working as intended, and the paymaster is the only gas
	// balance anybody has to act on.
	Sponsored bool      `json:"sponsored"`
	TakenAt   time.Time `json:"taken_at"`
}

// Service reads balances, tops accounts up, and alerts on the faucet.
type Service struct {
	cfg      Config
	chain    Chain
	notifier Notifier
	logf     Logf

	// protocol answers whether the accounts below still need gas. Nil means
	// they do, which is how this behaved before smart wallets existed.
	protocol ProtocolState

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

// SetProtocolState tells the service about the smart-wallet migration.
func (s *Service) SetProtocolState(state ProtocolState) {
	if s == nil {
		return
	}
	s.protocol = state
}

// sponsored reports whether protocol transactions are paid for by the
// paymaster. False whenever it cannot be established, so an unreachable
// registry keeps the old alerting rather than silencing it.
func (s *Service) sponsored(ctx context.Context) bool {
	return s != nil && s.protocol != nil && s.protocol.Sponsored(ctx)
}

// Config exposes the loaded settings.
func (s *Service) Config() Config { return s.cfg }

func (s *Service) log(format string, args ...any) {
	if s.logf != nil {
		s.logf(format, args...)
	}
}

// faucetAddress is where the faucet's balance is.
//
// The registry first, because after the smart-wallet migration the money is in
// a wallet whose address is not derivable from any key — and reading the key's
// address then reports an empty faucet, which raises a low-balance alert about
// an account that is empty on purpose.
//
// Then the address derived from the key, which is where transactions came from
// before the migration, and finally the configured variable for display.
func (s *Service) faucetAddressAt(ctx context.Context) string {
	if s != nil && s.protocol != nil {
		if address, ok := s.protocol.FaucetAddress(ctx); ok && address != "" {
			return address
		}
	}
	return s.faucetAddress()
}

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
	sponsored := s.sponsored(ctx)
	snap := Snapshot{TakenAt: time.Now(), AutoTopUpEnabled: s.cfg.Enabled, Sponsored: sponsored}

	snap.Faucet.Name = orDefault(s.cfg.Faucet.Name, "Faucet")
	snap.Faucet.Address = s.faucetAddressAt(ctx)
	snap.Faucet.MinCelo = s.cfg.Faucet.MinCelo
	snap.Faucet.MinSfluv = s.cfg.Faucet.MinSfluv

	if s.chain == nil || snap.Faucet.Address == "" {
		snap.Faucet.Error = "faucet address unknown"
	} else {
		if wei, err := s.chain.NativeBalance(ctx, snap.Faucet.Address); err != nil {
			snap.Faucet.Error = err.Error()
		} else {
			snap.Faucet.Celo = WeiToCelo(wei)
			// A sponsored faucet is not meant to hold CELO: its transfers are
			// paid for by the paymaster, and the balance that was swept into
			// its wallet is spendable rather than spent on gas. Judging it
			// against a gas floor would raise an urgent alert about an account
			// that is empty exactly as designed — and would bury the one gas
			// alert that does need a person.
			snap.Faucet.LowCelo = !sponsored && s.cfg.Faucet.MinCelo > 0 && snap.Faucet.Celo < s.cfg.Faucet.MinCelo
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
			// Ours need no gas once they are sponsored; the paymaster always
			// does, because it is the account paying. NeedsGas is what tells
			// the two apart, and it is the only reason a CELO floor still
			// applies to anything after the migration.
			if sponsored && !account.NeedsGas {
				entry.Low = false
			}
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

	// Topping up is off once the protocol is sponsored, whatever the config
	// says, and it cannot be otherwise.
	//
	// The money that used to do the funding is in a smart wallet now, and a
	// smart wallet moves CELO by way of a sponsored operation — which the
	// paymaster pays for. So the one account that still needs gas is the one
	// account whose top-up would depend on it already having some. That circle
	// does not close, which is why the paymaster is funded by a person and this
	// service's job here narrows to telling them when.
	if s.cfg.Enabled && s.chain != nil && !snap.Sponsored {
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

	// The full current set, every sweep. A sink that keeps standing messages
	// needs to hear about the empty set too: that is what tells it everything
	// has recovered and its messages should come down.
	if sink, ok := s.notifier.(ConditionSink); ok {
		if err := sink.Conditions(ctx, Conditions(snap, s.cfg)); err != nil {
			s.log("funding: could not reconcile alert conditions: %s", err)
		}
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

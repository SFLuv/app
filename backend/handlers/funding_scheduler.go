package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/funding"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/utils"
)

// fundingSweepInterval is how often the protocol's accounts are checked.
//
// Fifteen minutes is chosen against what it is protecting: a sponsored
// transaction costs roughly 0.03 CELO, so an account sitting on its floor has
// tens of transactions of headroom, which is far more than a quarter of an hour
// of traffic. Checking harder would spend RPC calls to learn the same thing.
const fundingSweepInterval = 15 * time.Minute

// FundingService keeps the protocol's accounts in gas and reports on them.
type FundingService struct {
	inner  *funding.Service
	logger *logger.LogCloser
}

// NewFundingService wires the funding module to the faucet and the mailer.
func NewFundingService(chain funding.Chain, log *logger.LogCloser) *FundingService {
	cfg := funding.LoadConfig("")
	logf := func(format string, args ...any) {
		if log != nil {
			log.Logf(format, args...)
		}
	}
	svc := &FundingService{
		inner:  funding.NewService(cfg, chain, &fundingNotifier{log: log}, logf),
		logger: log,
	}
	svc.logConfig()
	return svc
}

func (s *FundingService) logConfig() {
	if s.logger == nil {
		return
	}
	cfg := s.inner.Config()
	switch {
	case cfg.Err() != nil:
		s.logger.Logf("funding: DISABLED, could not read %s (%s)", cfg.LoadedFrom(), cfg.Err())
	case len(cfg.Accounts) == 0:
		s.logger.Logf("funding: no accounts configured (%s); balances will not be watched", cfg.LoadedFrom())
	default:
		mode := "watch only, no transfers"
		if cfg.Enabled {
			mode = "auto top-up enabled"
		}
		if cfg.AlertsEnabled() {
			mode += ", alerts on"
		} else {
			mode += ", ALERTS OFF"
		}
		names := make([]string, 0, len(cfg.Accounts))
		for _, account := range cfg.Accounts {
			names = append(names, account.Name)
		}
		s.logger.Logf("funding: %s (top-ups set by %s, accounts from %s), watching: %s",
			mode, cfg.EnabledSource(), cfg.LoadedFrom(), strings.Join(names, ", "))
	}
	// An account configured but not resolvable is not being watched at all,
	// which is indistinguishable from it being healthy. Say so loudly.
	if skipped := cfg.Skipped(); len(skipped) > 0 {
		s.logger.Logf("funding: NOT WATCHING %d account(s) with no resolvable address: %s",
			len(skipped), strings.Join(skipped, ", "))
	}
}

// Balances is the current state of every account, for the admin panel.
func (s *FundingService) Balances(ctx context.Context) funding.Snapshot {
	return s.inner.Balances(ctx)
}

// Start runs the sweep on an interval until the context is cancelled.
//
// The first pass is deliberately not immediate: a boot has enough to do, and an
// account that is low has been low for a while already.
func (s *FundingService) Start(ctx context.Context) {
	if len(s.inner.Config().Accounts) == 0 {
		return
	}
	go func() {
		timer := time.NewTimer(2 * time.Minute)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.RunOnce(ctx)
		}

		ticker := time.NewTicker(fundingSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.RunOnce(ctx)
			}
		}
	}()
}

// RunOnce performs a single sweep. Exported so an admin can trigger one without
// waiting for the timer.
func (s *FundingService) RunOnce(ctx context.Context) funding.SweepResult {
	result := s.inner.Sweep(ctx)
	if s.logger != nil {
		for _, line := range result.Errors {
			s.logger.Logf("funding: %s", line)
		}
		for _, line := range result.Skipped {
			s.logger.Logf("funding: %s", line)
		}
		for _, line := range result.Rapid {
			s.logger.Logf("funding: DRAIN WARNING: %s", line)
		}
	}
	return result
}

// ── Alerting ─────────────────────────────────────────────────────────────────

// fundingNotifier sends the low-faucet email.
type fundingNotifier struct {
	log *logger.LogCloser
}

// Notify emails everyone on FUNDING_ALERT_EMAILS, or the site admin when that
// is unset, so an unconfigured deployment still tells somebody.
func (n *fundingNotifier) Notify(subject string, body string) error {
	recipients := funding.AlertEmails(siteWithdrawalAlertTo())
	if len(recipients) == 0 {
		return fmt.Errorf("no recipients: set %s", funding.EnvAlertEmails)
	}

	sender := utils.NewEmailSender()

	// The body is already plain text with its own alignment; a <pre> keeps the
	// columns lined up rather than collapsing them into a paragraph.
	content := `<pre style="font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace;` +
		`white-space:pre-wrap;margin:0;color:#111827;">` +
		utils.EscapeEmailHTML(body) + `</pre>`
	html := utils.BuildStyledEmail(subject, "A protocol account needs funding.", content)

	var firstErr error
	for _, to := range recipients {
		if err := sender.SendEmail(to, "", subject, html, utils.NotificationFromEmail(), "SFLuv"); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if n.log != nil {
				n.log.Logf("funding: could not email %s: %s", to, err)
			}
		}
	}
	return firstErr
}

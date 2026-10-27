package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/structs"
)

// Uptime monitoring.
//
// This polls the backend's own public health endpoint over the network, which
// catches the failures that matter most in practice: a deploy that comes up
// unreachable, a database the server can no longer open, an edge that has
// stopped routing. What it cannot catch is the process being dead, because
// then nothing is polling — an in-process monitor can never be the whole
// answer, and an external one is still worth having. It is here because it
// covers the common case today at no operational cost.
//
// Flapping is the other thing a monitor has to get right. One failed request
// is a blip; the alert waits for failureThreshold consecutive failures, so a
// single dropped connection does not wake anybody.

const (
	uptimeInterval = time.Minute
	// uptimeFailureThreshold is how many consecutive failures make an outage.
	// Two minutes of silence is an outage; one request is a hiccup.
	uptimeFailureThreshold = 2
	uptimeRequestTimeout   = 10 * time.Second
	// uptimeAlertKey is stable across outages, which is what makes a new
	// outage re-post: an acknowledged alert is resolved, the uniqueness rule
	// only covers live rows, so the next outage raises a fresh one under the
	// same key.
	uptimeAlertKey = "uptime:backend"
	// EnvUptimeURL overrides what is polled. Without it the public base URL is
	// used, which is the right thing to check: a health endpoint reached over
	// localhost tells you the process is alive and nothing about whether
	// anybody can get to it.
	EnvUptimeURL = "UPTIME_CHECK_URL"
)

// UptimeState is the current reading.
type UptimeState struct {
	Healthy bool
	// Detail is why, in the words that go in the alert: "connection refused",
	// "HTTP 502".
	Detail string
	// Since is when the current state began.
	Since time.Time
	// OutageStartedAt is when the present (or most recent) outage began.
	OutageStartedAt time.Time
	// RecoveredAt is when the most recent outage ended.
	RecoveredAt time.Time
}

// LastOutageFor is how long the most recent outage lasted, for the line that
// reports a recovery.
func (s UptimeState) LastOutageFor() string {
	if s.OutageStartedAt.IsZero() {
		return "an unknown time"
	}
	end := s.RecoveredAt
	if end.IsZero() || end.Before(s.OutageStartedAt) {
		end = time.Now()
	}
	return humanDuration(end.Sub(s.OutageStartedAt))
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	default:
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf("%d hours", hours)
		}
		return fmt.Sprintf("%dh%02dm", hours, minutes)
	}
}

// UptimeService polls the health endpoint and alerts the dev set.
type UptimeService struct {
	url    string
	alerts *NotificationService
	logger *logger.LogCloser
	http   *http.Client

	mu       sync.RWMutex
	state    UptimeState
	failures int
	// postedDetail is what the live alert currently says, so it is only
	// rewritten when the reason actually changes rather than every minute.
	postedDetail string
}

// NewUptimeService builds the poller. A service with no URL is inert.
func NewUptimeService(alerts *NotificationService, log *logger.LogCloser) *UptimeService {
	url := strings.TrimRight(strings.TrimSpace(os.Getenv(EnvUptimeURL)), "/")
	if url == "" {
		if base := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/"); base != "" {
			url = base + "/healthz"
		}
	}

	return &UptimeService{
		url:    url,
		alerts: alerts,
		logger: log,
		http:   &http.Client{Timeout: uptimeRequestTimeout},
		// Assumed healthy until a poll says otherwise: the alternative is an
		// outage alert on every boot.
		state: UptimeState{Healthy: true, Since: time.Now()},
	}
}

// State is the current reading, read by the acknowledge button.
func (u *UptimeService) State() UptimeState {
	if u == nil {
		return UptimeState{Healthy: true, Detail: "monitoring is not running"}
	}
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.state
}

// Start begins polling. Call it from the jobs leader only: several replicas
// polling would alert several times for one outage.
func (u *UptimeService) Start(ctx context.Context) {
	if u == nil || u.url == "" {
		if u != nil {
			u.logf("uptime: not polling (no URL; set %s or APP_BASE_URL)", EnvUptimeURL)
		}
		return
	}

	u.logf("uptime: polling %s every %s, alerting after %d consecutive failures",
		u.url, uptimeInterval, uptimeFailureThreshold)

	go func() {
		ticker := time.NewTicker(uptimeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				u.checkOnce(ctx)
			}
		}
	}()
}

func (u *UptimeService) logf(format string, args ...any) {
	if u != nil && u.logger != nil {
		u.logger.Logf(format, args...)
	}
}

// checkOnce performs one poll and reconciles the alert with the result.
func (u *UptimeService) checkOnce(ctx context.Context) {
	detail := u.probe(ctx)

	u.mu.Lock()
	wasHealthy := u.state.Healthy
	if detail == "" {
		u.failures = 0
		if !wasHealthy {
			u.state.Healthy = true
			u.state.Detail = ""
			u.state.Since = time.Now()
			u.state.RecoveredAt = time.Now()
		}
	} else {
		u.failures++
		if u.failures >= uptimeFailureThreshold && wasHealthy {
			u.state.Healthy = false
			u.state.Detail = detail
			u.state.Since = time.Now()
			u.state.OutageStartedAt = time.Now()
			u.state.RecoveredAt = time.Time{}
		} else if !wasHealthy {
			u.state.Detail = detail
		}
	}
	state := u.state
	posted := u.postedDetail
	nowHealthy := state.Healthy
	u.mu.Unlock()

	switch {
	case !nowHealthy && wasHealthy:
		// Just went down.
		u.post(ctx, state, false)

	case !nowHealthy && posted != state.Detail:
		// Still down, but for a different reason than the alert says.
		u.post(ctx, state, false)

	case nowHealthy && !wasHealthy:
		// Recovered. The alert stays until somebody acknowledges it — an
		// outage nobody saw is still worth knowing about — but it now says so,
		// and the acknowledge button will start working.
		u.logf("uptime: recovered after %s", state.LastOutageFor())
		u.post(ctx, state, true)
	}
}

// probe returns "" when healthy, or the reason it is not.
func (u *UptimeService) probe(ctx context.Context) string {
	reqCtx, cancel := context.WithTimeout(ctx, uptimeRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.url, nil)
	if err != nil {
		return "bad monitor URL"
	}
	resp, err := u.http.Do(req)
	if err != nil {
		// net/http wraps the cause in a long URL-prefixed string; the tail is
		// the part a person reads ("connection refused", "i/o timeout").
		msg := err.Error()
		if idx := strings.LastIndex(msg, ": "); idx >= 0 {
			msg = msg[idx+2:]
		}
		return msg
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return ""
}

// post raises or rewrites the standing outage alert.
//
// One function for both because posting is idempotent in the alert key: the
// first call during an outage creates the alert and pushes it, and every later
// call updates the text of the same alert in place. The acknowledge button
// comes from the kind, so it survives a rewrite without being re-declared.
func (u *UptimeService) post(ctx context.Context, state UptimeState, recovered bool) {
	title := "Backend is not answering"
	body := fmt.Sprintf("%d consecutive health checks failed.", uptimeFailureThreshold)
	fields := []structs.NotificationField{
		{Label: "Reason", Value: state.Detail},
		{Label: "Checking", Value: u.url},
		{Label: "Since", Value: state.OutageStartedAt.Format("15:04 MST on 2 Jan")},
	}

	if recovered {
		title = "Backend recovered"
		body = "It is answering again. Acknowledge to clear this."
		fields = []structs.NotificationField{
			{Label: "Was down for", Value: state.LastOutageFor()},
			{Label: "Last reason", Value: state.Detail},
			{Label: "Checking", Value: u.url},
		}
	} else {
		u.logf("uptime: ALERT — %s is not answering (%s)", u.url, state.Detail)
	}

	if err := u.alerts.Post(ctx, Alert{
		Set:    structs.NotificationSetDev,
		Key:    uptimeAlertKey,
		Kind:   structs.InternalKindUptime,
		Title:  title,
		Body:   body,
		Fields: fields,
		// Urgent only while it is actually down. A recovered-but-unacknowledged
		// alert is a record rather than an emergency, and leaving it at the top
		// of the list teaches people the top of the list does not mean much.
		Urgent: !recovered,
	}); err != nil {
		u.logf("uptime: could not post the outage alert: %s", err)
		return
	}

	u.mu.Lock()
	u.postedDetail = state.Detail
	u.mu.Unlock()
}

package handlers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/structs"
	"github.com/jackc/pgx/v5"
)

// AffiliateScheduler refills organization allocation cycles (daily at midnight,
// weekly on Monday midnight, monthly on the 1st — America/Los_Angeles) and
// refunds unredeemed event value back to the owning organization when events
// expire.
type AffiliateScheduler struct {
	appDb  *db.AppDB
	botDb  *db.BotDB
	logger *logger.LogCloser
	loc    *time.Location

	mu     sync.Mutex
	timers map[string]*time.Timer
}

func NewAffiliateScheduler(appDb *db.AppDB, botDb *db.BotDB, logger *logger.LogCloser) *AffiliateScheduler {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PST", -8*60*60)
	}

	return &AffiliateScheduler{
		appDb:  appDb,
		botDb:  botDb,
		logger: logger,
		loc:    loc,
		timers: map[string]*time.Timer{},
	}
}

func (s *AffiliateScheduler) Start(ctx context.Context) {
	if s == nil {
		return
	}

	go s.scheduleExistingEventExpirations(ctx)
	go s.startCycleLoop(ctx, structs.AllocationCycleDaily)
	go s.startCycleLoop(ctx, structs.AllocationCycleWeekly)
	go s.startCycleLoop(ctx, structs.AllocationCycleMonthly)
}

// resetCycle refills every org allocation of the given cycle to its full
// amount. Balances were debited at event creation, so a reset grants the new
// period's budget in full.
func (s *AffiliateScheduler) resetCycle(ctx context.Context, cycle string) error {
	if s == nil || s.appDb == nil {
		return fmt.Errorf("affiliate scheduler missing dependencies")
	}
	return s.appDb.ResetOrganizationAllocations(ctx, cycle, nil)
}

func (s *AffiliateScheduler) ScheduleEventExpiration(eventId string, owner string, expiration uint64) {
	if s == nil || s.appDb == nil || s.botDb == nil {
		return
	}
	if eventId == "" || expiration == 0 {
		return
	}

	expiresAt := time.Unix(int64(expiration), 0)
	delay := time.Until(expiresAt)
	if delay <= 0 {
		go s.handleEventExpiration(eventId)
		return
	}

	s.mu.Lock()
	if existing := s.timers[eventId]; existing != nil {
		existing.Stop()
	}
	s.timers[eventId] = time.AfterFunc(delay, func() {
		s.handleEventExpiration(eventId)
	})
	s.mu.Unlock()
}

func (s *AffiliateScheduler) startCycleLoop(ctx context.Context, cycle string) {
	for {
		next := s.nextCycleReset(cycle, time.Now().In(s.loc))
		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			if err := s.resetCycle(context.Background(), cycle); err != nil {
				s.logf("error resetting %s organization allocations: %s", cycle, err)
			}
		}
	}
}

func (s *AffiliateScheduler) scheduleExistingEventExpirations(ctx context.Context) {
	if s == nil || s.botDb == nil {
		return
	}

	events, err := s.botDb.GetActiveEvents(ctx)
	if err != nil {
		s.logf("error loading active events for affiliate scheduler: %s", err)
		return
	}

	for _, event := range events {
		if ctx.Err() != nil {
			return
		}
		if event == nil || event.Expiration == 0 {
			continue
		}
		s.ScheduleEventExpiration(event.Id, event.Owner, event.Expiration)
	}
}

// handleEventExpiration refunds the unredeemed value of an expired event to the
// owning organization's allocation balances.
func (s *AffiliateScheduler) handleEventExpiration(eventId string) {
	s.mu.Lock()
	delete(s.timers, eventId)
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	value, err := s.botDb.EventUnredeemedValue(ctx, eventId)
	if err != nil {
		if err != pgx.ErrNoRows {
			s.logf("error getting unredeemed value for event %s: %s", eventId, err)
		}
		return
	}
	if value == 0 {
		return
	}

	orgId, err := s.botDb.GetEventOrganization(ctx, eventId)
	if err != nil || orgId == 0 {
		s.logf("expired event %s has no organization; skipping refund", eventId)
		return
	}

	if err := s.appDb.RefundOrganizationBalance(ctx, orgId, value); err != nil {
		s.logf("error refunding organization balance for event %s: %s", eventId, err)
	}
}

// nextCycleReset returns the next boundary for the cycle in the scheduler's
// timezone: daily = next midnight, weekly = next Monday midnight, monthly =
// midnight on the 1st of the next month.
func (s *AffiliateScheduler) nextCycleReset(cycle string, now time.Time) time.Time {
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch cycle {
	case structs.AllocationCycleDaily:
		return startOfDay.AddDate(0, 0, 1)
	case structs.AllocationCycleMonthly:
		firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return firstOfMonth.AddDate(0, 1, 0)
	default: // weekly
		weekday := int(startOfDay.Weekday())
		monday := int(time.Monday)
		daysUntil := (monday - weekday + 7) % 7
		if daysUntil == 0 && now.After(startOfDay) {
			daysUntil = 7
		}
		return startOfDay.AddDate(0, 0, daysUntil)
	}
}

func (s *AffiliateScheduler) logf(message string, args ...any) {
	if s != nil && s.logger != nil {
		s.logger.Logf(message, args...)
		return
	}
	fmt.Printf(message+"\n", args...)
}

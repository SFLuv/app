package handlers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/jackc/pgx/v5"
)

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

	go func() {
		if err := s.RecomputeWeeklyBalances(context.Background()); err != nil {
			s.logf("error recomputing affiliate weekly balances: %s", err)
		}
	}()

	go s.scheduleExistingEventExpirations(ctx)
	go s.startWeeklyLoop(ctx)
}

func (s *AffiliateScheduler) RecomputeWeeklyBalances(ctx context.Context) error {
	if s == nil || s.appDb == nil || s.botDb == nil {
		return fmt.Errorf("affiliate scheduler missing dependencies")
	}

	configs, err := s.appDb.GetAffiliateWeeklyConfigs(ctx)
	if err != nil {
		return err
	}

	for _, cfg := range configs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		reserved, err := s.botDb.AllocatedBalanceByOwner(ctx, cfg.UserId)
		if err != nil {
			s.logf("error getting reserved balance for affiliate %s: %s", cfg.UserId, err)
			continue
		}

		var weekly uint64
		if cfg.WeeklyAllocation > reserved {
			weekly = cfg.WeeklyAllocation - reserved
		}

		err = s.appDb.SetAffiliateWeeklyBalance(ctx, cfg.UserId, weekly)
		if err != nil {
			s.logf("error setting weekly balance for affiliate %s: %s", cfg.UserId, err)
			continue
		}
	}

	return nil
}

func (s *AffiliateScheduler) ScheduleEventExpiration(eventId string, owner string, expiration uint64) {
	if s == nil || s.appDb == nil || s.botDb == nil {
		return
	}
	if eventId == "" || owner == "" || expiration == 0 {
		return
	}

	expiresAt := time.Unix(int64(expiration), 0)
	delay := time.Until(expiresAt)
	if delay <= 0 {
		go s.handleEventExpiration(eventId, owner)
		return
	}

	s.mu.Lock()
	if existing := s.timers[eventId]; existing != nil {
		existing.Stop()
	}
	s.timers[eventId] = time.AfterFunc(delay, func() {
		s.handleEventExpiration(eventId, owner)
	})
	s.mu.Unlock()
}

func (s *AffiliateScheduler) startWeeklyLoop(ctx context.Context) {
	for {
		next := s.nextMondayMidnight(time.Now().In(s.loc))
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
			if err := s.RecomputeWeeklyBalances(context.Background()); err != nil {
				s.logf("error recomputing affiliate weekly balances: %s", err)
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
		if event == nil || event.Owner == "" || event.Expiration == 0 {
			continue
		}
		s.ScheduleEventExpiration(event.Id, event.Owner, event.Expiration)
	}
}

func (s *AffiliateScheduler) handleEventExpiration(eventId string, owner string) {
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

	if err := s.appDb.AddAffiliateWeeklyBalance(ctx, owner, value); err != nil {
		s.logf("error refunding affiliate balance for event %s: %s", eventId, err)
	}
}

func (s *AffiliateScheduler) nextMondayMidnight(now time.Time) time.Time {
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekday := int(startOfDay.Weekday())
	monday := int(time.Monday)
	daysUntil := (monday - weekday + 7) % 7
	if daysUntil == 0 && now.After(startOfDay) {
		daysUntil = 7
	}
	return startOfDay.AddDate(0, 0, daysUntil)
}

func (s *AffiliateScheduler) logf(message string, args ...any) {
	if s != nil && s.logger != nil {
		s.logger.Logf(message, args...)
		return
	}
	fmt.Printf(message+"\n", args...)
}

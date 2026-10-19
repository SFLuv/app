package bootstrap

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/SFLuv/app/backend/logger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// jobsLeaderLockKey is the Postgres advisory lock that decides which backend
// process runs the background jobs. Distinct from migrationLockKey.
const jobsLeaderLockKey int64 = 0x5F1D_0002

const (
	jobsLeaderPingInterval = 15 * time.Second
	jobsLeaderRetryDelay   = 10 * time.Second
)

// JobsLeader reports whether this process currently runs the background jobs.
type JobsLeader struct {
	held atomic.Bool
}

// Held is safe to call from any goroutine, and on a nil receiver.
func (l *JobsLeader) Held() bool {
	return l != nil && l.held.Load()
}

// StartJobsAsLeader runs start only while this process holds the jobs lock.
//
// The background jobs move money and sign transactions: payout reconciliation,
// W-9 escrow release, the Bridge sweep, and the redeemer role grants signed by
// the redeemer admin wallet. Two processes running them at once could pay
// twice or collide on a wallet nonce. One process was always the assumption,
// and on the VM it held: systemd stops the old binary before starting the new.
// A platform that starts the new deployment before stopping the old one
// (Railway does, until the new one passes its healthcheck) breaks it for the
// length of the overlap.
//
// So the jobs wait for a session-level advisory lock on the app database. The
// lock lives on one dedicated connection, opened beside the pool rather than
// borrowed from it (the pool's Close waits for borrowed connections, so a
// borrowed one held forever would hang shutdown), and kept for the life of the
// process. It
// is not released on shutdown on purpose: some job work deliberately outlives
// its context (tax maintenance runs on context.WithoutCancel), and the lock
// must not pass to the next process while that is still running. Postgres
// drops the lock when the process exits and its connection closes, which is
// the moment the old process can no longer be doing anything.
//
// HTTP serving is not gated. The new process answers requests at once and the
// old one drains, exactly as before; only the timers and boot-time sweeps wait.
//
// If the lock's connection is lost, Postgres has already released the lock, so
// the jobs context is cancelled and the loop competes for the lock again.
func StartJobsAsLeader(ctx context.Context, pool *pgxpool.Pool, appLogger *logger.LogCloser, start func(jobsCtx context.Context)) *JobsLeader {
	leader := &JobsLeader{}
	go func() {
		for ctx.Err() == nil {
			if !leadOnce(ctx, pool, appLogger, leader, start) {
				select {
				case <-ctx.Done():
				case <-time.After(jobsLeaderRetryDelay):
				}
			}
		}
	}()
	return leader
}

// leadOnce acquires the lock, runs the jobs until the lock or ctx is lost, and
// reports whether it ever held the lock (false means retry after a delay).
func leadOnce(ctx context.Context, pool *pgxpool.Pool, appLogger *logger.LogCloser, leader *JobsLeader, start func(context.Context)) bool {
	conn, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig.Copy())
	if err != nil {
		if ctx.Err() == nil {
			appLogger.Logf("jobs leader: could not open a database connection for the lock: %s", err)
		}
		return false
	}
	closeConn := func() { conn.Close(context.Background()) }

	var got bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", jobsLeaderLockKey).Scan(&got); err != nil {
		closeConn()
		if ctx.Err() == nil {
			appLogger.Logf("jobs leader: lock check failed: %s", err)
		}
		return false
	}
	if !got {
		appLogger.Logf("jobs leader: another backend process holds the jobs lock; background jobs wait until it exits")
		if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", jobsLeaderLockKey); err != nil {
			closeConn()
			if ctx.Err() == nil {
				appLogger.Logf("jobs leader: waiting for the lock failed: %s", err)
			}
			return false
		}
	}

	leader.held.Store(true)
	appLogger.Logf("jobs leader: lock acquired; starting background jobs")

	jobsCtx, cancel := context.WithCancel(ctx)
	start(jobsCtx)

	ticker := time.NewTicker(jobsLeaderPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Shutting down. The connection, and with it the lock, stays open
			// until the process exits (see StartJobsAsLeader).
			cancel()
			return true
		case <-ticker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pingCtx)
			pingCancel()
			if err != nil && ctx.Err() == nil {
				appLogger.Logf("jobs leader: lost the lock connection (%s); stopping background jobs and competing for the lock again", err)
				leader.held.Store(false)
				cancel()
				closeConn()
				return true
			}
		}
	}
}

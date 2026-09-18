package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/SFLuv/app/backend/logger"
)

// SeedPonderNotifiedTransfers marks every transfer the indexer has already
// recorded as notified, so a Ponder re-index replays the chain without
// re-emailing anyone about payments they received months ago.
//
// This does NOT run as a migration, for two reasons learned the hard way.
//
// Migrations run before the server listens and abort boot when they fail, so a
// migration that reaches for the indexer couples startup to a service the
// backend does not own — on 2026-09-10 an unreachable Ponder database turned a
// routine deploy into an nginx 502 loop. And Ponder owns its schema: it stamps
// _ponder_meta with an app identity and refuses to start against a database
// another app has written, so the migration path stays out of it entirely (see
// MigrationPools, which has no Ponder handle to offer).
//
// So this runs after the server is already serving, reads Ponder inside a
// transaction Postgres itself marks READ ONLY, and writes only to the app
// database. Failure is logged and nothing else — the table keeps whatever it
// already had, and the next boot tries again.
//
// Safe to run on every boot: the insert is ON CONFLICT DO NOTHING and the key
// is the transfer itself, so a seeded database re-runs as a no-op.
func SeedPonderNotifiedTransfers(ctx context.Context, pools *DBPools, appLogger *logger.LogCloser) {
	if pools == nil || pools.Ponder == nil {
		skippedSeed(appLogger, "the ponder database is not configured", nil)
		return
	}

	ponderTx, err := pools.Ponder.Begin(ctx)
	if err != nil {
		skippedSeed(appLogger, "could not open a read transaction on the ponder database", err)
		return
	}
	defer ponderTx.Rollback(context.Background())

	// READ ONLY, enforced by Postgres rather than by intention: an edit that
	// adds a write here fails with "cannot execute INSERT in a read-only
	// transaction" instead of quietly convincing the indexer that its schema is
	// no longer its own.
	if _, err := ponderTx.Exec(ctx, `SET TRANSACTION READ ONLY;`); err != nil {
		skippedSeed(appLogger, "could not make the ponder read transaction read-only", err)
		return
	}

	// Ponder's t.hex() is a TEXT column holding a lowercase 0x string, not a
	// bytea — see PgHex.getSQLType() in the ponder package, and the LOWER()
	// comparisons the rest of the backend already makes against these columns.
	// Migration 1.54 read them with encode(hash, 'hex'), which errors on text,
	// and because its failures were non-fatal it left the table empty for
	// exactly the re-index this exists to prevent.
	rows, err := ponderTx.Query(ctx, `
		SELECT
			hash,
			"to",
			"from",
			amount::text
		FROM transfer_event;
	`)
	if err != nil {
		skippedSeed(appLogger, "could not read indexed transfers", err)
		return
	}
	defer rows.Close()

	type transfer struct{ hash, to, from, amount string }
	seeds := []transfer{}
	for rows.Next() {
		var seed transfer
		if err := rows.Scan(&seed.hash, &seed.to, &seed.from, &seed.amount); err != nil {
			skippedSeed(appLogger, "could not scan an indexed transfer", err)
			return
		}
		seeds = append(seeds, seed)
	}
	if err := rows.Err(); err != nil {
		skippedSeed(appLogger, "could not iterate indexed transfers", err)
		return
	}

	inserted := 0
	for _, seed := range seeds {
		tag, err := pools.App.Exec(ctx, `
			INSERT INTO ponder_notified_transfers
				(tx_hash, to_address, from_address, amount)
			VALUES (LOWER($1), LOWER($2), LOWER($3), $4)
			ON CONFLICT DO NOTHING;
		`, seed.hash, seed.to, seed.from, seed.amount)
		if err != nil {
			skippedSeed(appLogger, fmt.Sprintf("could not record indexed transfer %s", seed.hash), err)
			return
		}
		inserted += int(tag.RowsAffected())
	}

	if appLogger != nil {
		appLogger.Logf(
			"ponder hook dedup: seeded %d of %d indexed transfers as already notified",
			inserted, len(seeds),
		)
	}
}

// SeedPonderNotifiedTransfersOnBoot runs the seed in the background so the
// server starts serving whether or not the indexer is reachable.
func SeedPonderNotifiedTransfersOnBoot(ctx context.Context, pools *DBPools, appLogger *logger.LogCloser) {
	go func() {
		// Bounded: a seed that hangs must never hold a read transaction open on
		// the indexer's database indefinitely.
		seedCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		SeedPonderNotifiedTransfers(seedCtx, pools, appLogger)
	}()
}

// skippedSeed records that dedup seeding could not run. It is never fatal: the
// table is created by migration 1.54 and the hook works without a single row in
// it, claiming each transfer as it arrives. An empty table only matters if a
// re-index happens before the seed succeeds, so the warning says so plainly.
func skippedSeed(appLogger *logger.LogCloser, reason string, err error) {
	if appLogger == nil {
		return
	}
	detail := ""
	if err != nil {
		detail = ": " + err.Error()
	}
	appLogger.Logf(
		"warning: ponder hook dedup seeding SKIPPED (%s%s). The table may still be EMPTY, "+
			"so a ponder re-index before it fills will email users about historical transfers. "+
			"It retries on the next boot.",
		reason, detail,
	)
}

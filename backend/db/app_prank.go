package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// LookupPrankeeUserID resolves a "prank" redirect: if the given pranker user id
// has a row in the pranks table, the prankee's user id is returned so the
// caller can act as that user for the request.
//
// The pranks table is created ONLY by the local dev CLI (./dev-up.sh menu),
// never by a migration, so on any normal database — including production — the
// table does not exist. This method checks for the table's existence FIRST and
// returns (\"\", false, nil) when it is absent, so callers never fail a request
// merely because pranking has never been set up. It is the sole reason the
// prank-forwarding middleware can be a no-op safely.
func (a *AppDB) LookupPrankeeUserID(ctx context.Context, prankerUserID string) (string, bool, error) {
	if prankerUserID == "" {
		return "", false, nil
	}

	var tableExists bool
	if err := a.db.QueryRow(ctx, `
		SELECT to_regclass('public.pranks') IS NOT NULL;
	`).Scan(&tableExists); err != nil {
		return "", false, err
	}
	if !tableExists {
		return "", false, nil
	}

	var prankeeUserID string
	err := a.db.QueryRow(ctx, `
		SELECT prankee_user_id
		FROM pranks
		WHERE pranker_user_id = $1;
	`, prankerUserID).Scan(&prankeeUserID)
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if prankeeUserID == "" {
		return "", false, nil
	}
	return prankeeUserID, true, nil
}

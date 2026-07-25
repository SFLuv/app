package handlers

import "context"

// ResolvePrankTarget returns the user id the given (authenticated) user's
// requests should be forwarded to, if a local-dev prank has been set for them.
//
// This exists purely so the prank-forwarding middleware in the router can swap
// the acting user id without reaching into the db layer directly. It NEVER
// surfaces an error to the caller: any failure (including the far-and-away
// common case of the pranks table not existing) is logged and treated as "no
// prank", so a request can never fail because of pranking. The pranks table is
// created only by the local dev CLI, so on production this always returns
// (\"\", false) after the cheap table-existence check.
func (a *AppService) ResolvePrankTarget(ctx context.Context, prankerUserID string) (string, bool) {
	prankeeUserID, ok, err := a.db.LookupPrankeeUserID(ctx, prankerUserID)
	if err != nil {
		a.logger.Logf("error resolving prank target for %s: %s", prankerUserID, err)
		return "", false
	}
	return prankeeUserID, ok
}

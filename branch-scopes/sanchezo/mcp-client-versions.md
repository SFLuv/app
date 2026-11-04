# sanchezo/mcp-client-versions

**Repos touched:** `SFLuv/app` (this branch).
**Status:** built; each new tool run against a local database and live Sentry through a throwaway test, not through a connected MCP client. Not merged, not deployed.
**Dates:** 2026-11-04. **0.3h active, measured** (11:24–11:40 PST, prompt time to commit by the shell clock; this turn's transcript entries were not yet written when it was measured).

## What it is

Three read-only admin MCP tools, for things that took a database shell or a hand-written Sentry call while rolling out the W-9 changes.

| Feature | Hours | Repo |
|---|---|---|
| `client_versions_report`: signed-in people by platform, version and build, seen in 7 days / 30 days / ever. Counts only | 0.1 | app |
| `w9_person_detail`: one person's filing and TIN match, tier prompts (armed, dismissed), payouts, push device count, app versions. No taxpayer numbers, form links or provider request ids; URLs are stripped from payout errors, which can quote an RPC endpoint | 0.1 | app |
| `sentry_release_health`: users, sessions, crashed/errored sessions and daily users per release, optionally by environment. Uses the existing read token and GET-only client | 0.1 | app |

## Read-only check

All 23 tools carry the read-only annotation. Every report query runs in `withReadOnlyTx`, a read-only Postgres transaction, and the Sentry client only sends GET. The only writes in `backend/mcp` are `oauth_store.go`'s own login and token tables, which signing in needs.

## Decisions

- **People, not installs.** `user_client_versions` keeps one row per person per platform, at the version last used, and only for signed-in requests. The report says so; a store's download count is a different number.
- **Signed-out phone-home metrics are left out.** `client_phone_home_metrics` counts hits, not people, and would read like adoption.

## Not done

- Not exercised through a connected MCP client; it needs deploying first.

**Volume:** 5 files, no migrations, no new HTTP routes.

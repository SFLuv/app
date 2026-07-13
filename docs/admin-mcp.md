# SFLUV Admin MCP (bolted onto the backend)

The main backend exposes a **read-only** admin reporting surface over the Model
Context Protocol (MCP) at `POST /mcp` on the normal API server. It lets an
admin's AI client answer questions like "how much did merchant *Tilted Brim*
receive this month?" against live SFLUV data.

This is **not** a standalone server and it does **not** run its own OAuth
provider. It is a route on the existing backend, gated by the **existing Privy
JWT + SFLUV admin** check, reusing the same DB pools.

## Security model

Access control is the whole point, so it is deliberately minimal and reuses
paths that already protect the app:

- **Auth = existing Privy JWT.** The MCP bearer token *is* a Privy access token.
  It is validated by `middleware.ValidatePrivyToken` — the same ES256 key
  (`PRIVY_VKEY`), audience (`PRIVY_APP_ID`), issuer (`privy.io`), and expiry
  checks used for every other authenticated request. No new token type, no new
  signing key, no token issuance or storage.
- **Admin-gated, checked live.** Every request must resolve to a user who is
  `UserIsActive` **and** `IsAdmin`, read from the live DB on each call. Removing
  someone's admin (or soft-deleting them) takes effect on the very next request.
- **Read-only.** Tools expose *named reports only* — never raw SQL. Every query
  runs inside a Postgres read-only transaction (`BEGIN ... READ ONLY`) with a
  `statement_timeout`. Result sets are paginated and capped at 500 rows.
- **No secrets, ever.** The tools only select curated, non-sensitive columns.
  They never touch API keys or private keys (those live only in env vars, which
  this code never reads), encrypted OAuth credentials (`user_oauth_credentials`),
  push tokens, W9 document URLs (`w9_submissions.w9_url`), or merchant-mode PIN
  hashes.
- **Least new surface.** No new endpoints beyond `/mcp` itself; unauthenticated
  or non-admin callers are rejected with `401` + `WWW-Authenticate` before any
  tool or query runs.

## Connecting a client

Configure your MCP client to call the backend's `/mcp` endpoint with an
`Authorization: Bearer <privy-access-token>` header (the `Access-Token` header is
also accepted for parity with the app).

- Obtain the Privy access token the same way the frontend does
  (`getAccessToken()` in a logged-in admin session).
- Privy access tokens are short-lived, so the token must be refreshed
  periodically. This is intentional: reusing the app's real, expiring token keeps
  the attack surface minimal instead of introducing a long-lived MCP credential.

Example (generic HTTP MCP client / Claude Desktop custom connector):

```
URL:    https://api.sfluv.org/mcp
Header: Authorization: Bearer <PRIVY_ACCESS_TOKEN>
```

> A full browser OAuth flow (dynamic client registration, PKCE, token issuance)
> was intentionally **not** implemented here. That was the design of the earlier
> standalone `cmd/admin-mcp` prototype; folding it into the backend would add a
> large new token-issuance/redirect attack surface, which conflicts with the
> read-only, minimal-surface goal. It can be added later if seamless in-client
> login is required.

## Tools (all read-only)

- `admin_report_catalog` — lists the available tools.
- `search_users` — active users, roles, contact fields, primary wallet, wallets.
- `lookup_wallet` — owner/merchant match, indexed balance, W9 status for one address.
- `financial_summary` — transfers, rewards, merchant payments, redemptions, workflow costs, volunteer events over a date range.
- `transactions` — indexed Ponder transfer rows by address / hash / time range.
- `w9_report` — W9 earnings and submission status (never the stored W9 document URL).
- `merchant_report` — merchant locations, owner contact, approval state, payment wallets.
- `workflow_report` — workflow lifecycle and payout reporting.
- `events_report` — redemption/faucet events with their creator (admin- or affiliate-created).
- `affiliate_report` — affiliate organizations, status, allocations/balances, owner contact.
- `app_structure` — where features and admin controls live, and which tool exposes which data (answers "where do I approve merchants?" style questions).

To answer "how much did *Tilted Brim* receive this month": call `merchant_report`
to find that merchant's location payment wallet address(es), then `transactions`
or `financial_summary` filtered to those addresses and the month's timestamp
range.

## Adding a report

Add a new named tool in `backend/mcp` (`reports.go` for core, `extra_tools.go`
for SFLUV-specific). Keep it read-only (`withReadOnlyTx`), select only
non-sensitive columns, and register it in `newServer`/`registerExtraTools`. Do
**not** add a generic SQL tool.

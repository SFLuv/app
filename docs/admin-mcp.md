# SFLUV Admin MCP (bolted onto the backend)

The main backend exposes a **read-only** admin reporting surface over the Model
Context Protocol (MCP) at `/mcp`, plus an OAuth 2.1 authorization server so an
admin can connect their AI agent with a normal browser login. It answers
questions like "how much did merchant *Tilted Brim* receive this month?" against
live SFLUV data.

It is **not** a standalone server. It is routes on the existing backend, and its
identity provider is **SFLUV's own Privy login** (there is no Google/third-party
IdP). Every issued token is bound to a SFLUV user DID whose admin status is
re-checked live on every request.

## Login flow (what the admin sees)

1. Admin adds the `/mcp` URL to their agent (Claude, etc.).
2. Agent calls `/mcp`, gets `401` + OAuth metadata, registers a client, and opens
   the admin's browser to `/oauth/authorize`.
3. The backend validates the request (PKCE, registered redirect URI, scope) and
   redirects the browser to the SFLUV frontend page **`/mcp/authorize`**.
4. That page logs the admin in via Privy (one click if already logged in), then
   hands their Privy access token + the one-time `login_state` to the backend
   `POST /oauth/mcp/complete`.
5. The backend verifies the Privy token, enforces **active + admin**, mints a
   single-use authorization code bound to that admin's DID, and returns the URL
   to redirect back to the agent.
6. The browser returns to the agent; the agent exchanges the code (with PKCE) at
   `/oauth/token` for an access token + refresh token and starts calling tools.

Access tokens are short-lived (~1h) and silently renewed via the refresh token,
so the admin rarely re-logs in — but admin status is still re-checked on every
`/mcp` call and every refresh, so revoking admin cuts access off immediately.

## Endpoints

- `GET /.well-known/oauth-protected-resource[/mcp]` — resource metadata.
- `GET /.well-known/oauth-authorization-server` — AS metadata.
- `POST /oauth/register` — dynamic client registration (redirect URIs must be
  https or loopback).
- `GET /oauth/authorize` — validates the request; redirects to the frontend login.
- `POST /oauth/mcp/complete` — Privy bridge (frontend → backend); issues the auth
  code after verifying admin.
- `POST /oauth/token` — `authorization_code` and `refresh_token` grants.
- `POST /mcp` — the MCP tool endpoint, gated by a valid AS access token + live
  admin check.

## Security model

- **Identity = SFLUV Privy login.** The frontend `/mcp/authorize` page runs the
  normal Privy login and posts the resulting access token to the backend, which
  validates it with the same `middleware.ValidatePrivyToken` used app-wide. No
  new identity provider, no passwords.
- **Admin-gated, checked live and repeatedly.** `UserIsActive && IsAdmin` is
  enforced when the auth code is issued, when tokens are minted, on every refresh,
  and on **every** `/mcp` request. Losing admin revokes access immediately.
- **OAuth 2.1 hardening.** PKCE (S256) is required and verified; authorization
  codes and login states are single-use and short-lived; redirect URIs must be
  pre-registered and https/loopback (no open redirects); refresh tokens rotate on
  use and revoke the access tokens minted from them.
- **Tokens are opaque and stored hashed.** A database leak exposes no usable
  tokens. Tokens are bound to a user DID, not an email.
- **Read-only tools.** Named reports only (never raw SQL); every query runs in a
  read-only Postgres transaction with a statement timeout; results capped at 500
  rows.
- **No secrets, ever.** Tools select only curated columns — never api keys,
  private keys (which live only in env vars this code never reads), encrypted
  OAuth credentials, push tokens, W9 document URLs, or PIN hashes.

## Configuration

Set on the backend:

- `MCP_PUBLIC_BASE_URL` — public base URL of this backend (used as the OAuth
  issuer and in endpoint URLs). Falls back to `PUBLIC_BACKEND_URL`. Example:
  `https://api.sfluv.org`.
- `MCP_AUTHORIZE_URL` — the frontend login page. Defaults to
  `{APP_BASE_URL}/mcp/authorize`. Only set this if the frontend lives elsewhere.

The frontend `/mcp/authorize` fetches `{NEXT_PUBLIC_BACKEND_URL}/oauth/mcp/complete`
with an `Authorization: Bearer` header, so the backend CORS allow-list must
include the frontend origin (it already allows `Authorization` + `POST`).

Run `backend/cmd/init` (or start the server) so migration `1.21` creates the
`admin_mcp_oauth_*` tables.

## Tools (all read-only)

`admin_report_catalog`, `search_users`, `lookup_wallet`, `financial_summary`,
`transactions`, `w9_report` (never the stored W9 URL), `merchant_report`,
`workflow_report`, `events_report` (admin- and affiliate-created),
`affiliate_report`, and `app_structure` (answers "where is the control for X?").

To answer "how much did *Tilted Brim* receive this month": call `merchant_report`
for that merchant's location payment wallet address(es), then `transactions` or
`financial_summary` for those addresses over the month.

## Adding a report

Add a named tool in `backend/mcp` (`reports.go` core, `extra_tools.go`
SFLUV-specific), keep it read-only (`withReadOnlyTx`), select only non-sensitive
columns, and register it in `newServer`/`registerExtraTools`. Do **not** add a
generic SQL tool.

## Known follow-ups

- A periodic cleanup of expired `admin_mcp_oauth_login_states` / `_auth_codes` /
  expired tokens (they're harmless but accumulate).
- Optional admin UI to list/revoke active MCP connections.

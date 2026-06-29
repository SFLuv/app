# SFLUV Admin MCP

`backend/cmd/admin-mcp` exposes read-only admin reporting tools over MCP. It opens the existing `app`, `bot`, and `ponder` PostgreSQL databases and serves named reports only. It does not expose raw SQL.

## How Access Works

Remote MCP clients connect to `/mcp`. If they do not have a bearer token, the server advertises OAuth metadata through `WWW-Authenticate` and `/.well-known/oauth-protected-resource/mcp`.

The MCP client then starts a browser OAuth flow:

1. The client registers a public OAuth client with `/oauth/register`.
2. The client sends the user to `/oauth/authorize` with PKCE.
3. SFLUV redirects the user to Google.
4. Google redirects back to `/oauth/google/callback`.
5. SFLUV accepts the Google email only if it belongs to an active SFLUV admin.
6. SFLUV issues its own short-lived bearer token for `/mcp`.

MCP access is managed through existing admin infrastructure. For the first admin, use `admin@sfluv.org`; for future access, make the corresponding SFLUV user an admin and ensure their Google email matches their profile email or a verified email row.

Removing admin status takes effect on the next MCP request because every bearer token is checked against the live SFLUV admin state.

## Run Remotely With OAuth

Run backend migrations first so the OAuth tables and initial allowlist row exist.

From the repo root:

```bash
cd backend
ADMIN_MCP_TRANSPORT=http \
MCP_HTTP_ADDR=:8090 \
MCP_PUBLIC_BASE_URL=https://mcp.sfluv.org \
GOOGLE_OAUTH_CLIENT_ID=... \
GOOGLE_OAUTH_CLIENT_SECRET=... \
go run ./cmd/admin-mcp
```

Google OAuth setup:

- Authorized redirect URI: `https://mcp.sfluv.org/oauth/google/callback`
- Scopes requested from Google: `openid email profile`

Use HTTPS in production. Loopback `http://localhost` redirect URIs are accepted for local MCP clients.

## Run Locally With Stdio

The stdio transport still works for local Codex-style use, but it should be treated as a local development mode.

```bash
cd backend
go run ./cmd/admin-mcp
```

Codex stdio config example:

```toml
[mcp_servers.sfluv_admin]
command = "go"
args = ["run", "./cmd/admin-mcp"]
cwd = "/Users/sanchezoleary/Projects/SFLUV_Main/backend"
env_vars = ["DB_USER", "DB_PASSWORD", "DB_URL", "DB_BASE_URL", "APP_DB_NAME", "BOT_DB_NAME", "PONDER_DB_NAME", "SFLUV_CHAIN_ID"]
```

## Database Safety

For the remote OAuth server, use a dedicated Postgres role with:

- `SELECT` on approved reporting tables/views in `app`, `bot`, and `ponder`
- `SELECT` on `users` and `user_verified_emails`
- `INSERT`, `UPDATE`, and `DELETE` on `admin_mcp_oauth_clients`, `admin_mcp_oauth_login_states`, `admin_mcp_oauth_auth_codes`, and `admin_mcp_oauth_tokens`
- A low statement timeout

Do not set `default_transaction_read_only = on` for the remote OAuth role, because OAuth client registration, login state, auth codes, and token hashes are written by the MCP server.

The stdio/local mode can still use a strictly read-only role:

```sql
ALTER ROLE sfluv_mcp_ro SET default_transaction_read_only = on;
ALTER ROLE sfluv_mcp_ro SET statement_timeout = '20s';
```

Every report tool call is wrapped in a read-only transaction. The remote role should still avoid write grants on all normal business tables.

OAuth state, auth codes, and access token hashes live in the `app` database. MCP authorization itself uses existing SFLUV admin users.

## Tools

- `admin_report_catalog`: lists available reports.
- `search_users`: active users, roles, contact fields, primary wallet, wallet list.
- `lookup_wallet`: owner/location match, indexed balance, W9 status.
- `financial_summary`: transfers, rewards, merchant payments, redemptions, workflow costs, volunteer events.
- `transactions`: indexed Ponder transfer rows.
- `w9_report`: W9 earnings and submission status; does not return stored W9 document URLs.
- `merchant_report`: merchant locations and payment wallets.
- `workflow_report`: workflow lifecycle and payout reporting.

Skipped: a generic SQL tool. Add a new named report instead.

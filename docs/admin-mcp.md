# SFLUV Admin MCP

`backend/cmd/admin-mcp` is a read-only stdio MCP server for Codex admin reporting. It opens the existing `app`, `bot`, and `ponder` Postgres databases and exposes named tools only. It does not expose raw SQL.

## Run Locally

From the repo root:

```bash
cd backend
go run ./cmd/admin-mcp
```

Codex config example:

```toml
[mcp_servers.sfluv_admin]
command = "go"
args = ["run", "./cmd/admin-mcp"]
cwd = "/Users/sanchezoleary/Projects/SFLUV_Main/backend"
env_vars = ["DB_USER", "DB_PASSWORD", "DB_URL", "DB_BASE_URL", "APP_DB_NAME", "BOT_DB_NAME", "PONDER_DB_NAME", "SFLUV_CHAIN_ID"]
```

Use a dedicated Postgres role for this server:

```sql
ALTER ROLE sfluv_mcp_ro SET default_transaction_read_only = on;
ALTER ROLE sfluv_mcp_ro SET statement_timeout = '20s';
```

Grant only `SELECT` on the approved reporting tables/views. The server also wraps every tool call in a read-only transaction, but the database role is the real lock.

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

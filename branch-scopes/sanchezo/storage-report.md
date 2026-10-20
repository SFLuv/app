# Branch scope — `sanchezo/storage-report`

Oct 19 2026 · app (backend) · **~0.2h active**

Hours are wall-clock time from this session's timestamps (about 18:21 to 18:35 PDT), not measured with the
`time-accounting` script.

## What this branch is

Phase 0 of moving photos out of Postgres into a Railway bucket. On Oct 19 the `app` database was 2,963 MB,
almost all of it image bytes in `BYTEA` columns. This adds a read-only report that says which tables hold them
and how fast each one grows, so the move can start with the table that matters most.

- `GET /admin/storage-report` (`withAdmin`).
- Admin-MCP tool `storage_report`, registered in `backend/mcp/extra_tools.go`.
- Both call `db.BuildStorageReport` (`backend/db/storage_report.go`), which returns:
  - the sizes of the app and bot databases;
  - for each of the 13 image stores: row count, image bytes, total table size, and **rows and bytes per month**;
  - the 15 largest app tables.
- The report uses `octet_length` and never reads the image bytes. It runs in read-only transactions with a 60 s
  statement timeout. A table or column missing from an older schema is flagged `missing` rather than failing the report.

| Item | Hours | Repo |
|---|---|---|
| Inventory of every image column across app and bot, the report query, route and MCP tool; verified against a fresh local Postgres 18 migrated by `cmd/init` (all 13 stores found, monthly bucketing checked with test rows) | 0.2 | app |

Volume: 4 files (2 new). 0 migrations, 1 new route, 1 new MCP tool.

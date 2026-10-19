# Branch scope — `sanchezo/railway-prep`

Oct 13 2026 · app (backend only) · **~0.6h active** (round 1)

Hours are wall-clock time from the session's own timestamps, **not** measured with the
`time-accounting` script (`measure_sittings.py` was not run). There was one sitting:
reading the backend and writing the migration explainer from 08:41 to 09:13 PDT (outside
this branch; see below), then this branch from 09:13 to about 09:50. The branch figure is
start-to-PR, and rounding is the main weakness. Re-measure with the skill before merge
if exact hours matter.

Not counted here: the backend read-through and `sfluv-backend-explainer.md` (handoff
folder, not in this repo), about 0.5h of the same sitting.

---

# Round 1 — Oct 13

## What this branch is

Preparation for moving the Go backend from the Google Cloud VM (systemd + nginx) to
Railway. Every change is either off by default or only matters on a host that ships just
the binary, so it can be deployed to the VM before the move. It adds **no schema
migration**, so rolling back is redeploying the previous binary.

## Features

| Item | Hours | Repo |
|---|---|---|
| Logger falls back to stdout when the log folder can't be found or created, or when `LOG_TO_STDOUT=true` (`logger/logger.go`). The old behaviour killed the boot: "project root not found" when the binary runs away from its build path. Reproduced with `go build -trimpath` run from an empty folder. | 0.1 | app |
| `TRUST_PROXY=true` rate-limits on the right-most `X-Forwarded-For` entry, the one the proxy itself appended (`handlers/volunteer_signups.go`). It's off by default, and the website proxy-key path is unchanged. | 0.1 | app |
| `DATABASE_URL` (the database name is swapped per pool), `PONDER_DATABASE_URL` (used verbatim) and `DB_SSLMODE` (`db/db.go`). A malformed URL is now a boot error. Before, a bad value silently fell back to localhost. The legacy `DB_USER`/`DB_URL` output is byte-identical. | 0.1 | app |
| Postgres advisory lock around `RunPendingMigrations` (`bootstrap/schema_migrations.go`). Checked locally: three concurrent runners on fresh databases applied 1.1→1.64 exactly once. | 0.1 | app |

## Smaller fixes

| Item | Hours | Repo |
|---|---|---|
| `/ponder/callback` (GET and POST) refuses everything when `PONDER_KEY` is unset. Before, an empty key matched an empty header. Constant-time compare. | 0.1 | app |
| `EnsureLogin` refuses when `ADMIN_KEY` is unset. It's currently uncalled. | <0.1 | app |
| Removed `backend/main`, a committed macOS x86-64 binary nothing references | <0.1 | app |

## Totals

| | Hours |
|---|---|
| Round 1 | ~0.6 |

Volume: 9 files, about 220 insertions. 0 migrations, 0 new routes. New env vars:
`LOG_TO_STDOUT`, `TRUST_PROXY`, `DATABASE_URL`, `PONDER_DATABASE_URL`, `DB_SSLMODE`.

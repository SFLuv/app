# Branch scope — `sanchezo/railway-jobs-leader`

Oct 14 2026 · app (backend + ponder) · **~0.8h active** (round 1)

Hours are wall-clock time from this session's timestamps (10:53 to about 11:40 PDT), **not**
measured with the `time-accounting` script. Rounding is the main weakness. Re-measure with the
skill before merge if exact hours matter. Stacked on `sanchezo/railway-prep` (#181).

---

# Round 1 — Oct 14

## What this branch is

The second piece of prep for moving the backend to Railway. Like #181, it's safe on the VM:
there's one process there, so it takes the lock at once and behaves as before. There's no
schema migration, so rollback is redeploying the previous binary.

## Features

| Item | Hours | Repo |
|---|---|---|
| Background jobs (workflow/W-9 maintenance, Bridge sweep and webhook registration, redeemer location sync, hours refresh, purge loop, Ponder hook reconciliation, analytics sync, notified-transfer seed) start only in the process holding a Postgres advisory lock on the app database (`bootstrap/leader.go`). The lock is kept on a dedicated connection outside the pool until the process exits. A new Railway deployment serves HTTP at once while its jobs wait for the old process to exit. | 0.5 | app |
| `GET /healthz`, answered ahead of the router: no auth, no middleware, no DB writes. Reports `jobs_leader`. | 0.1 | app |
| Commit ponder2's `package-lock.json` (ponder 0.15.18, viem 2.53.1), with a narrow `.gitignore` exception, so Railway installs exactly what built the live Ponder database | 0.2 | app |

## Smaller fixes

| Item | Hours | Repo |
|---|---|---|
| First version held the lock on a pooled connection, which hung shutdown (pool `Close` waits). Caught by the two-process test and fixed. | incl. above | app |

## Totals

| | Hours |
|---|---|
| Round 1 | ~0.8 |

Volume: 4 code files plus the lockfile and the `.gitignore` exception. 0 migrations, 1 new route (`/healthz`).

# Branch scope — `sanchezo/org-is-house-select`

Oct 18 2026 · app (backend) · **~0.2h active**

Hours are wall-clock time from this session's timestamps (21:53 to about 22:05 PDT), not measured with the
`time-accounting` script.

## What this branch is

A one-word fix. #184 (`a6396fe`) added `&org.IsHouse` to the `Scan` in `GetOrganizationsByIds`
(`backend/db/app_organization.go`) but not `is_house` to its `SELECT`. So every call failed with
`number of field descriptions must equal number of destinations, got 5 and 6`, and the public volunteer-events
list lost organiser names and logos. It's been on prod since Oct 15, and it was spotted in the Railway logs on
cutover night. The other two queries that scan `IsHouse` (the user's own organisation, and the organisation list)
already select it.

| Item | Hours | Repo |
|---|---|---|
| Add `is_house` to the `GetOrganizationsByIds` SELECT; reproduce the error on `main` and verify the fix against a local DB at schema 1.65 | 0.2 | app |

Volume: 1 file, 1 line. 0 migrations, 0 routes. Also the first change shipped through Railway's auto-deploy from `main`.

# Branch scope: `sanchezo/sentry-mcp-request-id`

Oct 27 2026 · app (backend) · **~0.2h active** (this repo's part) · based on `main` at `ef066f0`

Hours are wall-clock time from this session's timestamps, not from the `time-accounting` script. The session
spent about 19:02–19:14 PDT on Sentry across two repos; roughly 0.2h of that was this branch (reading the MCP
and router code, writing, and the live test against Sentry). The mobile half is `sanchezo/sentry-mobile` in
SFLuv/mobile-app and is recorded there.

## What this branch is

The backend half of adding Sentry to the mobile app. The backend does **not** report its own errors to Sentry
(free plan: 5,000 events a month for the whole org, kept for the app). It does two things:

**1. Request ids in the logs.** Every request now carries an id, printed as the `[id]` prefix on its access-log
line and returned as the `X-Request-Id` response header. The mobile app sends its own random id with each
request and attaches it to any Sentry report about that request, so an app error can be found in the Railway
logs by searching for the id. Ids that aren't 1–64 letters, digits, `-` or `_` are discarded and chi generates
one instead, so a caller can't write arbitrary text into the log. Requests without an id get the generated one.

**2. Sentry tools on the admin MCP.** Nobody but `ops@sfluv.org` has a Sentry login, so the admin MCP reads
Sentry for everyone already authorized on it:

| Tool | Returns |
|---|---|
| `sentry_issues` | Issues filtered by environment, app version, status, free search; sorted by last seen, first seen or frequency |
| `sentry_issue` | One issue with its latest event: app stack frames (file, function, line, source line), the trail of steps, tags, the contexts the app sets |
| `sentry_events` | Recent occurrences: when, which Privy DID, release, environment, request id |

Read-only throughout. The token is a Sentry internal integration with Read on Project, Issue & Event and
Organization, in `SENTRY_READ_TOKEN`. Without it the tools answer "Sentry is not configured" and nothing else
changes. The app scrubs events before sending them, so the tools return them as stored.

## Config

| Variable | Where | Value |
|---|---|---|
| `SENTRY_READ_TOKEN` | Railway, backendProd | The `sfluv-admin-mcp` internal integration's token |
| `SENTRY_ORG` | optional | Defaults to `sfluv` |

## Verification

- `go build ./...`, `go vet ./mcp ./router`, gofmt clean.
- Request ids (temporary test, removed): a valid client id is kept and echoed; one with a space and newline is
  replaced by a generated id; no header gets a generated id.
- Live against Sentry with the real read token: `sentry_issues` for preview 2.2.0 answers with an empty list
  (no events sent yet) and Sentry accepts the `release:*@2.2.0+*` filter; an unknown issue, a bad `sort` and a
  missing token each return a plain error.
- `summarizeEvent` on a sample event keeps app frames with their source line, counts library frames, and passes
  the trail and request id through.
- Not yet verified: `sentry_issue` against a real event. The first test error from the mobile preview build
  covers it.

## Volume

4 files: `mcp/sentry_tools.go` (new), `router/request_id.go` (new), `mcp/service.go`, `router/router.go`.
No migrations. No new HTTP routes; three new MCP tools.

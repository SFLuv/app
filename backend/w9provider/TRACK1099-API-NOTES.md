# Track1099 (Avalara 1099 & W-9) — real API shape vs what we implemented

Researched 2026-08-18 from the vendor's public docs. Read this before trusting
`track1099.go`: the adapter compiles and its unit tests pass, but several
endpoints in it do not appear in the vendor's documentation and look invented.

Sources: `track1099.com/api_info/readme`, `/api_info/guide_collect_w9s`,
`/api_info/changelog` (0.1.0 2022-07-29 → 0.7.0 2026-04-23),
`developer.avalara.com/api-reference/avalara1099/avalara1099/`.

## Verified

- **Every path is scoped by a Team API ID.** The shape is
  `/api/v1/{team_api_id}/…`. The Team API ID comes from the "Get API access"
  setup step in the W-9 UI. Confirmed examples:
  - `POST /api/v1/{team_api_id}/form_requests`
  - `POST /api/v1/{team_api_id}/issuers/{issuer_id}/import_forms`
  - `POST /api/v1/{team_api_id}/issuers/{issuer_id}/update_forms`
- **W-9 collection is the Form Requests API**, paired with a *low-code embedded
  document collection* JavaScript widget. Added in 0.4.0 for W-9, extended in
  0.6.0 for W-8BEN / W-8BEN-E.
- **Payers are called `issuers`.** There is no JSON `recipients` resource for
  the W-9 flow; recipient forms are loaded by CSV / import-forms.
- **`reference_id` is ours to choose** and is the join key. Reusing the same
  reference id surfaces the prior submission — the docs suggest using that to
  decide whether the form is needed at all. This is the idempotency mechanism.
- **A created form request returns** `reference_id`, `signed_pdf` (a URL that
  **expires after 3600 s**), `signed_at`, and a TIN match status. 201 on create.
- **TIN match is asynchronous**: `matched` | `rejected` | `pending`, resolving in
  the background within about 24 hours.
- **Auth is an API token** entered via the Swagger "Authorize" button. Exact
  header name is not stated in the public docs.

## Not supported — this is the important part

**There are no webhooks.** No webhook, callback, or notification appears
anywhere in the docs or in any changelog entry from 0.1.0 through 0.7.0.

**There is no sandbox or test environment** documented either.

Both of these are load-bearing for us:

1. `VerifyWebhook` in `track1099.go` invents `X-Track1099-Signature` and an
   HMAC-SHA256 scheme. Nothing will ever call it. Completion has to be
   discovered by **polling `GetW9Status`** — the sweeper is not a backstop for
   dropped deliveries, it is the only path. The comment on the interface should
   say so.
2. With no vendor sandbox, `Fake` is not a convenience, it is the only way to
   test the loop. Which makes the next section the real risk.

## Discrepancies against `track1099.go`

| Current code | Documented reality |
|---|---|
| base `https://www.track1099.com/api` + `/w9_requests` | `/api/v1/{team_api_id}/form_requests` — missing `/v1/` and the team id entirely |
| `GET /recipients?reference=` then `POST /recipients` | no such resource in the W-9 flow; `reference_id` lives on the form request |
| `POST /w9_requests/{id}/link` for a fresh URL | undocumented; create returns `signed_pdf`, good for 3600 s |
| `GET /w9_requests/{id}` → `status`, `tin_type`, `tin_match_status` | documented fields are `reference_id`, `signed_pdf`, `signed_at`, TIN match |
| webhook with `X-Track1099-Signature` | no webhooks exist |
| two calls: EnsurePayee then CreateW9Request | one call: create a form request carrying our `reference_id` |

## Changes this implies

- `Config` needs a **`TeamAPIID`**, and `do()` should prefix
  `/api/v1/{team_api_id}`. Without it every call 404s.
- `EnsurePayee` should become a **local no-op for this vendor** — return the
  UserID as the payee id rather than calling an endpoint that does not exist.
  Keep it on the interface; it is right for other vendors and the idempotency
  contract still holds via `reference_id`.
- `HostedFormURL` probably cannot "ask for a fresh link". If the widget is
  client-side, the server hands the browser a form-request token instead of a
  redirect URL. **This changes the frontend flow**, so settle it before building
  more UI on top.
- Model **completion and TIN match as two events**. A form can be `signed_at`
  while TIN match is still `pending` for 24 h.

  **DECIDED 2026-08-18 (Sanchez): escrow releases on `signed_at`, not on
  `matched`.** Waiting on an asynchronous TIN match would hold a volunteer's
  money for up to 24 hours after they have done everything asked of them, and
  the W-9 obligation is discharged by the signed form. So:

  - `StatusCompleted` is driven by `signed_at` alone. Do **not** gate it on TIN
    match.
  - A later `rejected` TIN match must **not** claw back a released payout. Treat
    it as a follow-up task — flag the user for re-collection and let it affect
    the *next* payout, not the one already out the door.
  - Keep polling after release until the match resolves, so `rejected` is still
    recorded. This means the sweeper cannot stop at `completed`; its exit
    condition is a resolved TIN match, not a released payout.
  - `W9Status` already carries both fields, so no type change.

## Testing consequence — read this one twice

`Fake` currently mirrors the **guessed** API, so a green test suite proves only
that we are internally consistent with a fiction. Before wiring more tests:
reshape `Fake` to the documented contract — `form_requests`, `reference_id`,
`signed_pdf` with a 1-hour expiry, async TIN match, and **no webhook**. Then a
passing suite means something.

The existing `provider_test.go` webhook tests should move to whatever the real
completion signal turns out to be, or be deleted with the webhook code.

## Open questions needing a real account

- Exact auth header (`Authorization: Bearer …` vs an `X-` API-token header).
- The literal base hostname, and whether `import_forms` is underscore or hyphen.
- Whether any polling endpoint returns a *list* of form requests by status, or
  whether the sweeper must poll one id at a time (matters for rate limits —
  none are published).
- Whether a form request can be re-opened / re-sent after `rejected`.

## Confirmed against our tree (2026-08-18)

- `Config` is built once at `bootstrap/runtime.go:323`. There is **no team-id
  field anywhere in the backend** — `grep -rn "TeamAPIID\|team_api_id\|TEAM_API"`
  returns nothing. A new `W9_PROVIDER_TEAM_ID` env var and `Config.TeamAPIID`
  are the smallest change that makes any real call possible.
- Existing env surface: `W9_PROVIDER`, `W9_PROVIDER_API_KEY`,
  `W9_PROVIDER_BASE_URL`, `W9_PROVIDER_ENV`, `W9_PROVIDER_WEBHOOK_SECRET`,
  `W9_RETURN_URL`, plus the enforcement/escrow knobs.
  `W9_PROVIDER_WEBHOOK_SECRET` has nothing to protect if the vendor never posts.
- The polling path already exists and is wired into `handlers/payout_escrow.go`,
  `handlers/w9.go` and `router/router.go`. Good — that is the path that has to
  carry all completion detection.
- The webhook route has exactly one caller, `handlers/w9.go:187`. That is the
  single place to change when the webhook comes out.

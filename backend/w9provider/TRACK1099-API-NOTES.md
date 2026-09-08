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

## CORRECTION 2026-08-18 11:36 — read this first, it supersedes parts of the above

Sanchez reached the real API credentials page (`track1099.com/api_tokens`) after
signing up. Three claims earlier in this file were wrong, and the auth model in
`track1099.go` is wrong.

### 1. A sandbox DOES exist

| Environment | API base | Identity token URL |
|---|---|---|
| Production | `https://api.avalara.com/avalara1099` | `https://identity.avalara.com/connect/token` |
| Sandbox | `https://api.sbx.avalara.com/avalara1099` | `https://ai-sbx.avlr.sh/connect/token` |

Note the sandbox identity host is **`avlr.sh`, not `avalara.com`**. Easy to
fat-finger. `Config.Environment` now genuinely selects a *pair* of URLs, so it
has real work to do rather than being decoration.

### 2. The base URL is not track1099.com

It is `https://api.avalara.com/avalara1099`, and the page states **"Legacy API
Deprecated"** with *"LegacyAPI Token creation is disabled."* Our adapter targets
the legacy host. Any static-token design is a dead end — new tokens cannot even
be minted.

### 3. Auth is OAuth2 client credentials, not a static API key

`track1099.go` sends `Authorization: Bearer <apiKey>` with a long-lived key.
Reality is a two-step exchange:

```
POST https://identity.avalara.com/connect/token
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials
client_id=<client id>
client_secret=<client secret>
```

Do **not** send a `scope` parameter — the docs say default scopes configured on
the credential are applied. Response:

```json
{ "access_token": "eyJhbGci...", "expires_in": 3600, "token_type": "Bearer" }
```

Then `Authorization: Bearer <access_token>` per request.

**The token expires in 3600 s**, so the adapter needs a cached token with
refresh-before-expiry, guarded by a mutex since `Provider` must be safe for
concurrent use. Refresh slightly early rather than on 401, so a sweeper run
mid-refresh does not fail a batch.

### Config changes this forces

`Config.APIKey` cannot carry this. Suggested shape:

```go
type Config struct {
    Provider     string
    ClientID     string // W9_PROVIDER_CLIENT_ID
    ClientSecret string // W9_PROVIDER_CLIENT_SECRET
    TeamAPIID    string // W9_PROVIDER_TEAM_ID  — required in every URL path
    Environment  string // "production" | "sandbox" — selects both URLs below
    BaseURL      string // optional override
    IdentityURL  string // optional override
}
```

The Team API ID is a real value on the account. It is **not** written into this
file on purpose — it belongs in env, not in a repo that pushes to GitHub.

`WebhookSecret` can go. Still no webhook documented anywhere, including on the
credentials page.

### Account prerequisites before any of this works

Creating credentials requires: the right permissions, **a company address on the
profile**, and **two-factor authentication enabled**. As of now the account shows
*"No credentials have been created."* So the integration is blocked on account
setup, not on code.

Avalara also recommends inviting developers as team members who generate their
**own** credentials, so access can be scoped to specific Payers and revoked
individually. Worth doing rather than sharing the team leader's.

### Tax year

The year selector offers 2025, 2024, 2023, 2022, 2021 — **no 2026**. Our
`W9RequestInput.TaxYear` will be ahead of what the platform accepts. Confirm what
a form request does with an unopened year before relying on it.

## VERIFIED FROM THE OPENAPI SPEC — 2026-08-18, supersedes all guesswork above

Pulled the real spec from an authenticated session:
`https://www.track1099.com/api-docs/v1/swagger.yaml` — OpenAPI 3.0.1,
"Track1099 API V1", version **v0.7.0**, 121 KB. Everything below is from the
spec itself, not inference.

### The API is JSON:API

The spec says so outright: *"The API generally follows the JSON:API
specification"*. Media type is **`application/vnd.api+json`**, not
`application/json`. Max request size 100 MB.

`track1099.go` currently sets `Content-Type`/`Accept: application/json` and
posts flat objects. Both are wrong.

### Complete endpoint list (16)

```
GET    /api/v1/{team_api_id}/issuers                                List issuers
POST   /api/v1/{team_api_id}/issuers                                Create issuer
GET    /api/v1/{team_api_id}/issuers/{issuer_id}                    Retrieve issuer
PATCH  /api/v1/{team_api_id}/issuers/{issuer_id}                    Update issuer
DELETE /api/v1/{team_api_id}/issuers/{issuer_id}                    Delete issuer
POST   /api/v1/{team_api_id}/issuers/{issuer_id}/import-forms       Upload CSV of forms
POST   /api/v1/{team_api_id}/issuers/{issuer_id}/update-forms       Upload CSV of updates
POST   /api/v1/{team_api_id}/authorized_api_requests                Create download URL
GET    /api/v1/{team_api_id}/authorized_api_requests/{id}           Retrieve download URL
GET    /api/v1/{team_api_id}/authorized_api_requests/{id}/execute   Execute download URL
POST   /api/v1/{team_api_id}/1099/forms                             Create 1099 form
GET    /api/v1/{team_api_id}/w9forms                                List W9/W8/W4 forms
GET    /api/v1/{team_api_id}/form-pdf                               Retrieve single PDF
POST   /api/v1/{team_api_id}/form_requests                          Create form request
GET    /api/v1/{team_api_id}/form_requests/{form_request_id}         Get form request
GET    /api/v1/{team_api_id}/jobs/{job_id}                          Get job status
```

Note the hyphens: `import-forms`, `update-forms`, `form-pdf`. But underscores in
`form_requests`, `authorized_api_requests`. Not consistent — copy exactly.

**No `/recipients` resource exists**, confirming the earlier finding. And there
is **no `servers:` block in the spec**, so the spec does not tell you the host.
Combine with the credentials page (production `https://api.avalara.com/avalara1099`).
How that composes with a `/api/v1/...` path still needs one live call to settle.

### Webhooks: definitively none

Zero matches for `webhook` and zero for `callback` in the whole 121 KB spec.
Polling is the only completion path. This is now settled, not inferred.

### Creating a form request — exact shape

Required attributes are **`form_type`** and **`company_id`**.

```
POST /api/v1/{team_api_id}/form_requests
Content-Type: application/vnd.api+json

{ "data": { "type": "form_request",
            "attributes": { "form_type": "W-9",
                            "company_id": 2345678,
                            "reference_id": "SE-02453450" } } }
```

- `form_type` enum: `W-9` | `W-8BEN` | `W-8BEN-E`. Immutable.
- `company_id` is **an integer**, immutable, and is *"Track1099's ID of your
  company, found in the W-9 UI"*. A wrong or missing one returns **404 Not found
  (company)**.
- `reference_id` is immutable and *"must uniquely identify (to you) the person or
  company from whom you are requesting the form"* — our `UserID` fits.
- Success is **201** with `{ "data": <form_request> }`. Validation failure is
  **422** with an `errors` array.

**`company_id` is a hard prerequisite we do not have.** It is not the Team API
ID and not an `issuer_id`. Someone has to create the company in the W-9 UI and
put its integer id in config, or we fetch it once at startup.

### form_request attributes (the TIN-free object)

`form_type`, `company_id`, `company_name` (read-only), `company_email`,
`reference_id`, `form_id`, `signed_at`, `tin_match_status`, `expires_at`,
`signed_pdf`, `action_validate`, `action_complete`, plus `path` / `self` /
`execute` links.

- `signed_at` is the completion timestamp on **this** object.
- `tin_match_status` is read-only and *"null if `signed_at` is null"*. Which
  independently validates the decision below: it cannot gate release, because it
  does not exist until after signing.
- `action_validate` and `action_complete` are almost certainly what the low-code
  embedded widget drives. Worth reading before rebuilding `HostedFormURL`.

### ⚠️ `GET /w9forms` RETURNS ACTUAL TINs — do not poll it naively

The `w9_form` schema contains **`tin`** (`"example": "23-8234555"`-style),
`type_of_tin`, plus `address`, `city`, `state`, `zip`, `account_number`,
`foreign_address`, `backup_withholding`.

This is the one finding that threatens this package's whole premise — the header
of `provider.go` promises a TIN never crosses the boundary. Polling `/w9forms`
would drag TINs straight into our process memory and any error log that echoes a
response body.

Two safe options:

1. **Poll `GET /form_requests/{form_request_id}`** — verified TIN-free, carries
   `signed_at` and `tin_match_status`. Costs one call per outstanding filing.
2. If the list endpoint is needed for volume, use **JSON:API sparse fieldsets**
   to exclude `tin`, e.g.
   `?fields[w9_form]=reference_id,entry_status,signed_date,tin_match_status`
   — and treat that query string as a security control, with a test that fails
   if `tin` ever appears in a decoded response.

`/w9forms` does support `sort` (`id`, `type`, `entry_status`, `updated_at`,
`reference_id`, `company_id`, …), so a filtered sweep is possible.

Also note the field naming differs between objects: `form_request` has
**`signed_at`**, `w9_form` has **`signed_date`**. Do not share a struct tag.

### Auth, reconciled

The spec declares `securitySchemes: API token: {type: http, scheme: bearer}` and
points at `/api_tokens`. The credentials page says legacy token creation is
**disabled** and mandates the OAuth2 `client_credentials` exchange. These agree
at the wire level — both end as `Authorization: Bearer <token>` — they differ in
how the token is obtained. Use the OAuth2 exchange; the static-token path is a
dead end.

### Still open after all this

- Exact host composition for `api.avalara.com/avalara1099` + `/api/v1/...`.
- Whether `entry_status` has a fixed vocabulary. The spec types it as a nullable
  string with `example: string`, i.e. undocumented, so `normaliseTrack1099Status`
  is guessing. Prefer `signed_at != null` over string matching.
- The embedded widget contract behind `action_validate` / `action_complete`.

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

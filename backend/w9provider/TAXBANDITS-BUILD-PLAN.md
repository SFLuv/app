# TaxBandits — implementation brief

**Decided 2026-08-18: we are going with TaxBandits, not Track1099.** Rates are
published in the console (Payments → Agreement Pricing) and are wholesale:
W-9 $0.25, TIN matching $0.20, 1099 federal e-file $0.65, state $0.85. Roughly
half to a quarter of Track1099 at our volume, with no 25-form W-9 cap.

Background and the full vendor comparison are in `W9-PROVIDER-COMPARISON.md`.
Keep `track1099.go` for now — it is the fallback if Go Live approval stalls.

## Build to the Go-Live checklist — it is the acceptance criteria

The console's Go-Live Checklist asks us to demonstrate exactly four things.
Treat them as the definition of done, then the questionnaire is a formality:

1. **Authentication** — generate a JWS, exchange it for a JWT access token.
2. **Status Retrieval** — retrieve form status via **webhooks *or* the status
   endpoint**. Note the "or": the polling sweeper alone satisfies the checklist,
   so this is not a reason to defer the webhook — only a reason it does not
   block go-live.
3. **Error Handling** — handle validation and server errors.
4. **Whitelisting** — enable and test IP/domain whitelisting.

Tick **1099/W2/1095** and **W9/W8 and TIN Matching** on the form list.

## 1. Auth — the fiddly part, do this first

Two steps. We sign a JWS ourselves, then trade it for a bearer token.

```
payload: { "iss": <ClientId>, "sub": <ClientId>,
           "aud": <UserToken>, "iat": <unix epoch seconds> }
sign:    HS256 with the Client Secret
```

```
GET https://testoauth.expressauth.net/v2/tbsauth     (sandbox)
GET https://oauth.expressauth.net/v2/tbsauth         (production)
Authentication: <header>.<payload>.<signature>
```

Response is a custom envelope, not standard OAuth:

```json
{ "StatusCode": 200, "StatusName": "Ok", "AccessToken": "...",
  "TokenType": "Bearer", "ExpiresIn": 3600, "Errors": null }
```

Then `Authorization: Bearer <AccessToken>` on every API call.

Four traps, all of which will look like "bad credentials" if you hit them:

- The exchange header is **`Authentication:`**, not `Authorization:`.
- It is a **GET**, not a POST.
- **`golang.org/x/oauth2` cannot be used.** Non-standard verb, header and
  response shape. Hand-roll it.
- **`iat` must match their server time** or no token is issued. They expose
  `GET v2/getservertime`. Do not trust the VM clock — sync against that endpoint
  and carry the offset. A drifting clock takes the whole integration down.

Cache the token with refresh slightly before the 3600 s expiry, behind a mutex.
`Provider` must stay safe for concurrent use, and the sweeper will run batches.

## 2. Config

```go
type Config struct {
    Provider     string
    ClientID     string // W9_PROVIDER_CLIENT_ID
    ClientSecret string // W9_PROVIDER_CLIENT_SECRET
    UserToken    string // W9_PROVIDER_USER_TOKEN
    BusinessID   string // W9_PROVIDER_BUSINESS_ID  (GUID from Business/Create)
    WebhookRef   string // W9_PROVIDER_WEBHOOK_REF  (GUID, optional routing)
    Environment  string // "sandbox" | "production" — selects both URL pairs
}
```

`WebhookSecret` is not needed — but see §4: that is because the signing key is
the ClientSecret we already hold, not because deliveries are unsigned.

## 3. Creating a W-9 request

```
POST /FormW9/RequestByUrl
```

Request, with the fields we actually need:

```json
{ "Requester": { "BusinessId": "<our BusinessId GUID>" },
  "Recipient": { "PayeeRef": "<our UserID>",
                 "Email": "<user email>",
                 "Name": "<optional prefill>",
                 "IsTINMatching": true },
  "WebhookRef": "<GUID>",
  "RedirectUrls": { "ReturnUrl": "<app scheme or https>", "RedirectTime": 5 },
  "PrefLang": "en-US" }
```

Response `200`:

```json
{ "SubmissionId": "<GUID>", "PayeeRef": "<our UserID>",
  "W9Url": "https://testlinks.taxbandits.io?uId=<GUID>", "Errors": null }
```

Mapping onto our existing interface:

| Ours | Theirs |
|---|---|
| `W9RequestInput.UserID` | `Recipient.PayeeRef` — **mandatory**, 1–50 chars, our join key |
| `W9RequestInput.ReturnURL` | `RedirectUrls.ReturnUrl` |
| `W9Request.ProviderRequestID` | `SubmissionId` |
| `W9Request.FormURL` | `W9Url` |
| `PayeeInput` / `EnsurePayee` | **no-op** — `PayeeRef` is ours; no payee resource to create |

Prerequisite: create the payer once via **Business/Create** and store the
`BusinessId` GUID in config. `PayerRef` is an alternative we assign ourselves;
`IsDefaultBusiness` can make it implicit. Do this before any W-9 call.

`W9Url` has **no documented expiry** (unlike Track1099's 3600 s). Do not assume
it is permanent either — `HostedFormURL` should re-request rather than store.
Confirm in sandbox whether calling `RequestByUrl` twice with the same `PayeeRef`
is idempotent or creates a second submission. **This matters** — if it is not
idempotent we need to store `SubmissionId` and never re-request blindly.

Errors: `400` with `{Id, Name, Message}` (e.g. `F75-100004` for a missing
PayeeRef), `401` `AUTH-100025` for bad credentials.

**`PrefLang` supports `es-ES`.** Given who we serve in the Tenderloin, wire this
through from the user's language preference rather than hardcoding `en-US`.

## 4. Webhooks — signed, and the primary path

**Correction, 2026-08-24.** This section previously said there was no signature,
no HMAC and no shared secret, and that the security model was IP whitelisting
alone. That is wrong, and it was wrong in a way that shaped the design: it is
why the sweeper became the source of truth and the webhook a mere hint.

TaxBandits signs every delivery. Verified against both the current docs and the
1.7.1 versioned copy:

    Signature  — base64( HMAC-SHA256( ClientId + "\n" + TimeStamp, ClientSecret ) )
    TimeStamp  — sent alongside it, and what makes replay detectable

Recompute it, compare, and return **401 without processing** on a mismatch. The
key is the ClientSecret we already hold, so nothing new has to be provisioned —
`WebhookSecret` does not need to exist, but not for the reason §2 gave.

Because the delivery is authenticated, it can be acted on directly. There is no
need to re-read status on every callback to guard against spoofing; a request
that fails the HMAC never gets that far. IP allowlisting stays as defence in
depth and because it is item 4 on the go-live checklist, not as the only control.

**What still has to poll, and why it is small.** Deliveries can be lost outright:
nine retries over 24 hours is generous, but an endpoint down for a day exhausts
them and the notification is gone for good. Nothing else would ever tell us, and
the cost of not knowing is somebody's money held indefinitely with no way to
recover it. So a slow backstop sweep stays — hours, not minutes, and skewed
towards filings we have not heard about in a long time. It is a safety net for a
dropped delivery, not the mechanism.

The TIN match does not need its own polling either: where TIN matching was
requested the callback carries `TINMatching` (`ORDER_CREATED` | `SUCCESS` |
`FAILED`) alongside `W9Status`, and fires again when it resolves.

Hard requirements from their side:
- Callback URL must be **HTTPS with a valid certificate**, max 500 chars.
- Must return **HTTP 200 within 5 seconds** → acknowledge and enqueue; never
  do chain work inline.
- Retried **up to 9 times over 24 hours** → **idempotency is mandatory**, since a
  duplicate here means paying someone twice. Key on `SubmissionId`.
- TaxBandits activates a subscription only after our endpoint returns 200 to a
  sample POST. **The endpoint must be deployed and reachable before the webhook
  can be saved.** Plan that ordering.
- On the GCP VM, IP allowlisting must read `X-Forwarded-For`, not `RemoteAddr`,
  or the check does nothing.

## 5. Status mapping, and the escrow decision

```
W9Status:    COMPLETED | COMPLETED_AND_TIN_MATCH_INPROGRESS | INVALID
TINMatching: ORDER_CREATED | SUCCESS | FAILED
```

**Decided: release escrow on signature, not on TIN match.**

- `COMPLETED` → `StatusCompleted`
- `COMPLETED_AND_TIN_MATCH_INPROGRESS` → **also `StatusCompleted`.** This is
  precisely the state the decision targets: signed, match still running.
- `INVALID` → `StatusInvalid`

A later `TINMatching: FAILED` must **not** claw back a released payout. Flag the
user for re-collection and let it affect the next one. The sweeper therefore
cannot stop at completed — its exit condition is a resolved TIN match.

## 6. Keep TINs out of the process

The W-9 webhook payload includes the recipient's **TIN by default**, inside
`FormData`. It can be excluded by a toggle in the console — but that toggle is
invisible to this repo and to code review, so do not rely on it alone.

- Decode into a struct with **no TIN field**. Unknown JSON keys are dropped.
- Never log raw webhook or response bodies.
- Add a test that fails if a TIN-shaped value survives decoding.

## 7. Reshape the Fake before writing more tests

`Fake` currently mirrors the *guessed* Track1099 API, so a green suite proves
only that we are consistent with a fiction. Rebuild it against this contract:
`SubmissionId`, `PayeeRef`, `W9Url`, the three `W9Status` values, async
`TINMatching`, and **no webhook signature**. The signature tests in
`provider_test.go` should be deleted or repointed at the IP check.

The console's **Form Status Simulator** drives status transitions on demand —
use it for the end-to-end sandbox run, and keep `Fake` for CI so tests do not
depend on the network or burn quota.

## Suggested order

1. Business/Create → capture `BusinessId`. One-off.
2. Auth: JWS → JWT, with server-time sync and cached refresh. *(checklist 1)*
3. `taxbandits.go` implementing `Provider`; `EnsurePayee` as a no-op.
4. `FormW9/RequestByUrl` + the status endpoint. *(checklist 2)*
5. Error handling on 400/401 shapes. *(checklist 3)*
6. Deploy the callback endpoint, then register the webhook and get its
   `WebhookRef`. Enable IP whitelisting. *(checklist 4)*
7. Reshape `Fake`; rewrite the suite against the real contract.
8. Full end-to-end in sandbox via the Form Status Simulator.
9. Submit the Go-Live questionnaire.

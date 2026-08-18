# W-9 provider comparison — Track1099 vs TaxBandits

Researched 2026-08-18 from public docs only; no account required for any of it.
Companion to `TRACK1099-API-NOTES.md`.

## Why we looked again

The first pass assumed this had to be free. It doesn't — SFLuv has a card. The
criteria that actually matter for our design are: webhooks vs polling, whether
the API hands us TINs, sandbox quality, and cost at our volume (tens of forms).

## TaxBandits auth — fully specified, and more work than Track1099

OAuth 2.0 where **you sign your own JWS** and exchange it for a bearer token.

```
JWS payload:  { "iss": <ClientId>, "sub": <ClientId>,
                "aud": <UserToken>, "iat": <unix epoch> }
Signed:       HS256, using the Client Secret
```

```
GET https://testoauth.expressauth.net/v2/tbsauth      (sandbox)
GET https://oauth.expressauth.net/v2/tbsauth          (production)
Authentication: <header.payload.signature>
```

Response is a **custom envelope**, not standard OAuth:

```json
{ "StatusCode": 200, "StatusName": "Ok", "StatusMessage": "Successful API call",
  "AccessToken": "eyJhbGci...", "TokenType": "Bearer",
  "ExpiresIn": 3600, "Errors": null }
```

Four traps worth knowing before anyone writes this:

1. The header is **`Authentication:`**, not `Authorization:`. Easy to typo, and
   the failure will look like bad credentials.
2. It is a **GET**, not a POST.
3. **`golang.org/x/oauth2` will not work.** Non-standard verb, header and
   response shape. This is hand-rolled.
4. *"The JWT will be returned in the Response only when the `iat` value matches
   our server time."* **Clock skew is a hard auth failure**, and they expose
   `GET v2/getservertime` to sync against. A drifting VM clock takes out the
   whole integration.

Three credentials to carry, not one: `ClientId`, `ClientSecret`, `UserToken`.
Note the auth host is a different domain entirely — `expressauth.net`.

By comparison Track1099 is a plain form-encoded `client_credentials` POST that a
standard library handles. **On auth alone, Track1099 is less work.**

## TaxBandits webhooks — real, with a documented vocabulary

Event: **Form W-9 Status Change**. Payload carries `SubmissionId`, `WebhookRef`
(GUID), `Requester` (payer details), `RecipientId`, `W9Status`, `TINMatching`,
and `FormData`.

```
W9Status:    COMPLETED
             COMPLETED_AND_TIN_MATCH_INPROGRESS
             INVALID
TINMatching: ORDER_CREATED | SUCCESS | FAILED
```

**`COMPLETED_AND_TIN_MATCH_INPROGRESS` is exactly the state Sanchez decided to
release escrow on** — signed, TIN match still running. The vendor models our
chosen policy as a first-class status. Track1099 by contrast types `entry_status`
as a nullable string with `example: string`, i.e. no documented vocabulary at
all, which is why `normaliseTrack1099Status` is guessing.

Delivery: retried **up to 9 times within 24 hours**; our endpoint must return
**HTTP 200 within 5 seconds**.

Two consequences for our handler:
- **Do the work asynchronously.** Releasing escrow on-chain inside the webhook
  will blow the 5-second budget. Acknowledge, enqueue, process.
- **Idempotency is mandatory,** not nice-to-have. Nine retries means duplicate
  delivery is expected, and a duplicate here means paying someone twice.

## Correction: TaxBandits sends the TIN by DEFAULT

Earlier I said TaxBandits "supports the boundary instead of undermining it."
That was too generous. The actual wording:

> "By default, the webhook response will include the recipient's TIN. If you
> prefer not to include the TIN, you can adjust this preference in the console."

So `FormData` contains `"TIN": "11-8338476"` unless someone turns it off — and
the off switch is a **console setting, invisible to this repo**. Nobody reviewing
a diff can see whether it is on. A new sandbox or production console may default
differently.

Honest ranking on TIN exposure:

| | Behaviour |
|---|---|
| TaxBandits | Sends TIN by default; excludable via console toggle |
| Track1099 | `GET /w9forms` returns TIN; `form_requests` never does |

Neither is clean. **The adapter must defend itself either way**: decode into a
struct with no TIN field, never log raw webhook or response bodies, and add a
test that fails if a TIN-shaped value survives decoding. Do not rely on a
dashboard toggle to uphold a guarantee written in `provider.go`.

## Where each one wins

**TaxBandits:** webhooks (no polling), documented status enum that matches our
release policy, unlimited free W-9 requests by email, TIN matching $0.35, five
collection methods, explicit nonprofit practice, free sandbox, SDKs in five
languages, Zapier, and an MCP server.

**Track1099:** simpler auth, self-serve production access **today** (we already
hold the Team API ID), a full public OpenAPI spec, and a `form_requests` object
that is TIN-free by construction.

## The real deciding factor is not technical

TaxBandits production access is gated: Go Live request, questionnaire, a review
of our API logs, a sales conversation, and a **signed License Agreement** before
credentials are issued. Published per-form prices are retail; API pricing is
negotiated. That is why "are API W-9 requests free?" is not answerable from
documentation — only sales can say.

Track1099 is a credit card and a Team API ID.

With 1099 season ahead, the question is whether TaxBandits' better runtime
design is worth an unknown contracting timeline. Start the sandbox now, because
the review is the long-lead item, but do not delete the Track1099 adapter until
TaxBandits pricing and approval are in hand.

## Still unknown

- Webhook **signature verification** header name and algorithm. Documented on a
  separate page not yet located; it exists.
- Whether API-initiated W-9 requests are billed (sales question).
- Whether the console TIN toggle is per-environment or global.

---

## RESOLVED 2026-08-18 — TaxBandits webhooks are NOT cryptographically signed

Checked the current (2.0) and previous (1.7.1) webhook documentation directly.
Term presence across the whole page:

```
signature  false      hmac     false      sha256  false
signing    false      sha-256  false
whitelist  true       https    true       ssl     true
```

The only use of "authenticate" runs the other way — it is *them* validating
*our* endpoint:

> "the API will authenticate this URL by posting a sample JSON. The URL will be
> activated only when we receive a 200 response."

Their stated security guidance is **IP whitelisting**:

> "IP whitelisting: Whitelist TaxBandits webhook IP addresses to prevent
> unauthorized access to your endpoint."

An earlier third-party write-up claimed a signature header. It is not in either
version of the vendor's own docs. Treat that claim as wrong.

### Why this matters more for us than for most callers

This webhook **releases held money**. An unauthenticated endpoint that pays
people is a materially different risk from one that updates a dashboard. The
current `VerifyWebhook(h http.Header, rawBody []byte)` cannot do its job for
this vendor — there is nothing in the header to verify, and the tests in
`provider_test.go` that assert a tampered body is rejected have nothing to
assert against.

### The pattern that fixes it

**Treat the webhook as a hint, never as an instruction.**

On receipt: acknowledge 200 immediately, ignore the payload's claimed status,
and re-read the authoritative status from the API for that submission. Act only
on what the API says. That converts an unauthenticated push into a trusted pull,
and it means a spoofed webhook can at worst cause a redundant API call.

This costs almost nothing to build, because the polling sweeper already exists
and is already the source of truth. The webhook becomes a latency optimisation
on top of it rather than a second, weaker authority.

Layer on top of that:
- IP allowlist of TaxBandits' published ranges. On the GCP VM this means reading
  `X-Forwarded-For` correctly rather than `RemoteAddr`, or the check is worthless.
- A long, unguessable callback path. Defence in depth, not a control.
- Never log the raw body — it carries the recipient TIN by default.

### Effect on the comparison

This narrows the gap. TaxBandits still wins on latency — Track1099 has no
webhook at all, so polling interval is the floor there. But "has webhooks" is
worth less than it first appeared, because ours have to be re-verified by
polling regardless.

Track1099's webhook story is *absent*; TaxBandits' is *unauthenticated*. Neither
is a signed webhook, and neither should be trusted to move money on its own.

## Sandbox console, first look

- **$5,000 BanditCash** granted for testing, with a test credit card to top up.
- **Form Status Simulator** — drive status transitions on demand. This is what
  lets the deploy test suite exercise the full escrow release loop without
  waiting on real filings, and it is a genuine advantage over Track1099.
- Three webhook subscriptions available, all currently unconfigured:
  **E-file Status Change**, **PDF Complete**, **Form W-9 Status Change**.
- Callback URLs must be **HTTPS with a valid certificate**; HTTP is rejected.
  Max 500 characters. TaxBandits activates a URL only after it returns 200 to a
  sample POST — so the endpoint must exist before the subscription can be saved.
- Each saved callback URL gets a **WebhookRef (GUID)**. `FormW9/RequestByUrl`
  accepts an optional `WebhookRef` so a specific request can be routed to a
  specific callback URL. Useful for sending sandbox and production traffic to
  different endpoints, or for isolating test traffic.

---

## RECOMMENDATION 2026-08-18 — stay on Track1099

### The market, after checking everyone

| | 1099-NEC, low volume | API access | Webhooks | Self-serve |
|---|---|---|---|---|
| **Track1099 / Avalara** | $3.10 (1–15) | included; 25 free W-9s | **none** | **yes — we hold credentials now** |
| **TaxBandits** | $2.75 (first 10) | sales-led licence | unsigned | no: review + contract |
| **Tax1099 / Zenwork** | $2.90 (first 20) | **Scale plan ~$349/yr** | yes | partly |
| **Abound** | — | — | — | **DEAD** — acquired Nov 2024, service retired, DNS de-delegated |

At roughly 30 forms a year the three live options land within about twenty
dollars of each other — except Tax1099, whose $349/yr API plan dwarfs the filing
cost at our volume. Price is not the deciding factor.

### Why Track1099 now wins on the merits, not just on inertia

The case for TaxBandits was webhooks. That case weakened when the webhooks
turned out to be **unauthenticated** (see the section above). Because a spoofed
webhook could release escrow, the only safe pattern is to treat any webhook as a
hint and re-read authoritative status from the API before acting.

**That means the polling sweeper is load-bearing in both designs.** The real
difference is latency — seconds versus a poll interval — not architecture. For
volunteer stipends, minutes are fine.

Everything else favours Track1099 for our situation:

- **We already have production credentials.** No approval gate, no licence
  agreement, no unknown timeline, with 1099 season ahead.
- **Published self-serve pricing** on a card.
- **`form_requests` is TIN-free by construction.** TaxBandits sends the TIN by
  default and hides the off switch in a console toggle no code review can see.
- **A complete public OpenAPI spec**, already extracted into
  `TRACK1099-API-NOTES.md` — exact paths, payloads and enums.
- Auth is a standard `client_credentials` POST rather than a hand-rolled JWS
  with a clock-skew failure mode.

### What choosing this costs us, stated plainly

JSON:API envelopes, a `company_id` prerequisite, polling latency, no vendor
status simulator, and `entry_status` having no documented vocabulary — so key
completion off `signed_at != null`, never off a status string.

### Keep the TaxBandits sandbox

It cost nothing and is already created. Revisit if volume grows enough to matter,
if polling latency becomes a real complaint, or if Track1099's W-9 tier
(25 forms free) starts to bind.

### Not blocking on this

The only open question is whether Track1099 charges for API-initiated W-9
requests beyond the free 25. That is a published-tier question, not a sales
negotiation — and unlike TaxBandits, we can find out by using the account.

---

## CORRECTION 2026-08-18 16:10 — TaxBandits API pricing IS published, and it is wholesale

Found at `sandbox.taxbandits.com/User/AgreementPricing` (Payments → Agreement
Pricing). I previously wrote that API pricing was sales-negotiated and
undiscoverable. Wrong. It is a flat rate card in the console, with no volume
tiers to chase.

Relevant lines:

| Service | API rate |
|---|---|
| Form W-9/W-8 | **$0.25** |
| TIN Matching | **$0.20** |
| 1099 Federal E-file | **$0.65** |
| 1099 State E-file | $0.85 |
| Online Access (e-delivery) | $0.25 |
| Postal Mailing | $1.85 |
| 1099 Transactions | $0.08 |
| PIF (Payer Information) | $0.49 |

These are **wholesale rates, roughly a quarter of the retail website prices**
($0.65 vs the $2.75–$3.10/form advertised publicly). The retail pages were
never the right comparison.

### Cost at our volume — 30 recipients, federal only, e-delivery

```
TaxBandits (API rates)
  30 W-9 requests      @ 0.25 =   7.50
  30 TIN matches       @ 0.20 =   6.00
  30 federal e-file    @ 0.65 =  19.50
  30 transactions      @ 0.08 =   2.40
  30 online access     @ 0.25 =   7.50
                                -------
                                 42.90

Track1099 (published tiers)
  15 federal e-file    @ 3.10 =  46.50
  15 federal e-file    @ 2.30 =  34.50
  30 TIN matches       @ 0.45 =  13.50
  W-9s beyond the free 25              = paid tier, unpriced
                                -------
                                 94.50+
```

**TaxBandits is roughly half to a quarter the cost**, and its W-9 pricing has no
25-form cliff — just $0.25 each.

### What this does to the recommendation

It reverses it. On the merits TaxBandits now leads on cost, on webhooks, on the
Form Status Simulator (which directly serves the deploy test suite), and on
having a documented status vocabulary that already models our release policy.

Track1099 retains exactly two advantages: **credentials we hold today**, and a
simpler auth flow.

So the decision now rests on one unknown that has nothing to do with technology:
**how long does Go Live approval take?** Everything else favours TaxBandits.

- If approval is days → go TaxBandits.
- If it is weeks or unknown, with 1099 season ahead → ship on Track1099 and
  revisit, since `track1099.go` already exists and the full spec is extracted.

### Two caveats on the rate card

1. These rates are displayed in the **sandbox** console. The page is titled
   "Agreement Pricing", which implies they are the agreed rates, but production
   parity should be confirmed in writing before committing.
2. "1099 Transactions $0.08" is undefined in the console. Confirm what a
   "transaction" counts as — it may be per API call rather than per form.

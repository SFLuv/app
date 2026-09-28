# Branch scope — `pjol/merchant-unwrap-deployment`

Sep 22–28 2026 · app + webpage · **8.9h** (plus deployment, not measured — see below)

## How these hours were measured

**3.5h measured** from session-transcript timestamps, clustered into sittings on a 30-minute gap
(`time-accounting/scripts/measure_sittings.py`), corroborated against commit times and the six pull
requests merged in the window (#172–#177).

**5.4h stated, not measured** — the requested 2h blocks on Tuesday, Wednesday and Thursday evenings
(22:00–00:00), which come to 6.0h, **less 0.57h that the transcripts already account for inside
those same windows**. Without that subtraction the Tuesday 23:15–23:33 sitting and the Wednesday
23:44–00:00 stretch would be counted twice. There is no transcript at all for Thursday evening, so
that full 2h rests on the statement alone.

**Deployment time is not included in the 8.9h.** Six PRs were merged and deployed in this window;
five of those merges land inside or within minutes of a measured sitting, so their time is already
counted. The sixth — `3b9bcc8` / #177, "merchant refunds enabled", merged Thu 11:01 — has no
transcript, and its duration was not clocked. Per the rule this file follows, a figure that was never
measured is not invented here. Add it explicitly if you have it.

The weakness worth naming: the per-feature split below is an apportionment of measured sitting
totals by what each sitting contained, not an independent measurement of each feature. The sitting
totals are the truth; the split is approximate below 0.1h. A first pass at the split summed to 1.98h
against a measured 1.63h for the app repo — the measured total won and the items were brought down to
it, rather than the total being raised to meet the items.

**Excluded:** the webpage commits of Sep 22–23 (`508d96c`…`e8be84f`, +46,619 lines of FYE 2026
statements, impact report and the announcement banner) are Sanchez's work, not this branch's, and
carry none of these hours.

### Measured sittings

| Sitting | Repo | Span | Measured |
|---|---|---|---|
| Tue 22 Sep | app | 00:00–00:18 (tail of a sitting begun Mon 23:36) | 0.30h |
| Tue 22 Sep | app | 23:15–23:33 | 0.30h |
| Wed 23 Sep | app | 00:28–00:34 | 0.09h |
| Wed 23 Sep | app | 21:48–21:51 | 0.05h |
| Wed 23 Sep → Thu 24 Sep | app | 23:44–00:35 | 0.84h |
| Fri 25 Sep | webpage | 16:11–17:05 | 0.90h |
| Fri 25 Sep | webpage | 17:46–17:58 | 0.20h |
| Fri 25 Sep | webpage | 18:42–18:48 | 0.10h |
| Fri 25 Sep | webpage | 20:35–21:13 | 0.63h |
| Mon 28 Sep | app | 10:47–10:49 | 0.05h |
| | | **measured** | **3.46h** |

---

# Large features

### Merchant refunds — 0.40h · app

The feature specced earlier in the branch and not built until now: a refund is an ordinary transfer
out of the till, so nothing on chain records what it was for.

- Migration **1.58** — `transaction_refunds`, linking a refund's hash to the payment it undoes, with
  amount, location, owner, both addresses and chain. Unique on the refund hash, so a double submit
  returns the existing row rather than recording the same money twice.
- `RefundedTotals`, `ListRefundsForOriginals`, `RefundOriginalsByRefundHash` — all bulk, so a page of
  history costs three queries rather than three per row.
- `GET /locations/{id}/refundability`, `POST /locations/{id}/refunds`.
- The rule the feature rests on: the original's amount comes from the chain index, never the client,
  and the remainder is original − already refunded. Over-refund is refused with a 409 naming the
  remainder. The original must have been paid *into* this till and the refund sent *out of* it.
- Three-step modal (details → amount → confirm), full or partial, 25/50/75% or an exact figure,
  percentages rounded to the nearest cent. Back throughout, click-off to exit, locked while the
  transfer is in flight.
- Refund marks (`refunded` / `partially refunded` / `refund`) are computed server-side so web and
  mobile cannot disagree about whether the button should appear.

### Location transaction history — 0.20h · app

The location page replaces `/wallets` for merchants and had no transaction display at all.

- `GET /locations/{id}/transactions` — paged, ownership-checked, with the refund ledger applied.
- `GetTransfersForAddresses` — a till and its tipping wallet are one shop, so they page as one
  ordering; paging them separately interleaved wrong at every boundary.
- `GetTransferByHash` — the existing parties lookup returned no **amount**, which is the ceiling on a
  refund.
- History list with direction, tip badges, refund marks, cross-references from a refund line back to
  the payment it undid, and the refund button suppressed once nothing is left to refund.

### Bridge terms-of-service acceptance — 0.38h · app

Bridge refuses to attach a bank to a customer that has not accepted its terms, and refuses at the
last step — after the merchant has already logged into their bank through Plaid.

- `Customer.HasAcceptedTOS` read for the first time; ToS had only ever been taken from the KYC link,
  so a customer attached by an admin carried a blank terms status forever.
- `bridgeBlockerForBank` asks Bridge what is actually in the way and phrases it for the merchant,
  checked **before** the Plaid link token is issued rather than after.
- `Client.TOSAcceptanceLink` (`GET /v0/customers/{id}/tos_acceptance_link`) and
  `POST /merchant/payout/tos-link`.
- An in-app modal with the terms one click away, rather than throwing the merchant at a third-party
  page unannounced — the first attempt auto-opened the page and was reworked after review.
- Terms surfaced at the verification step, where the business does its onboarding. Previously
  `tos_status` rendered only in the admin panel, so the merchant could not see or clear it.

### Plaid exchange root cause — 0.10h · app

`/v0/plaid_exchange_public_token/{link_token}` is a POST that **rejects** `Idempotency-Key`, which
the client set on every POST. The exchange had therefore never succeeded for any merchant since the
feature shipped; every attempt returned our own 502.

- `doRequest` takes an idempotency flag; `doNoIdempotency` for the endpoints that refuse it.
- A POST that 422s complaining about the header is retried once without it, body rebuilt per attempt,
  so the same mistake on another endpoint corrects itself.
- Corrected the comment that asserted every POST required the header.

This supersedes an earlier terms-of-service theory for the same symptom. The theory was wrong; the
log line settled it in one reading, which is why `bridge.ErrorSummary` now carries Bridge's own words
to the client instead of leaving them in a server log.

### Webpage annual report carousel — 0.90h · webpage

- Annual impact report carousel rebuilt (`e083fac`, 16 files, +555/−175).

### Webpage scroll behaviour — 0.93h · webpage

- Scroll behaviour fixes and follow-up improvements (`040d8d3`, `70478b0`).
- Hero layout module and global style changes supporting them.

---

# Smaller fixes

| Fix | Hours | Repo |
|---|---|---|
| Volunteer event `series_id` never set on edit — an event switched to recurring showed as recurring everywhere while being invisible to the generator, so it silently never produced a second occurrence. Edit path now opens a series; migration **1.57** strips recurrence from the events left behind rather than backfilling them, which would have replayed months of past occurrences at one per five-minute tick. | 0.19h | app |
| Terms status inherited across a manual Bridge customer replacement, plus `kyb_approved_at` carried over and payout addresses orphaned by the old customer. Boot sweep extended (`reconcileBridgeProfiles`) to correct terms, drop foreign payout destinations and clear mismatched KYC links, so profiles already in that state heal on deploy. | 0.13h | app |
| Failed bank connections reported no reason to the user — the cause existed only in a server log. `bridge.ErrorSummary` renders Bridge's own message and status safely; the client shows it and keeps polling in case the account lands anyway. Background bank pickup added to the sweep for every attached profile, not only those transitioning into approved. | 0.09h | app |
| Monthly SFLUV volume report (`financial_summary`, 22 Aug – 22 Sep vs prior month). | 0.09h | app |
| This branch scope. | 0.05h | app |

---

# Totals

| Source | Hours |
|---|---|
| Measured — app | 1.63h |
| Measured — webpage | 1.83h |
| **Measured subtotal** | **3.46h** |
| Stated evening blocks (Tue/Wed/Thu 22:00–00:00), 6.0h less 0.57h already measured inside them | 5.43h |
| **Total** | **8.9h** |
| Deployment (six PRs merged; five inside measured sittings, #177 Thu 11:01 unclocked) | not measured |

# Volume

**app** — 16 files, +2,337/−57. 2 migrations (1.57, 1.58). 4 new routes
(`/merchant/payout/tos-link`, `/locations/{id}/transactions`, `/locations/{id}/refundability`,
`/locations/{id}/refunds`). 3 new files (`db/app_refunds.go`, `handlers/refunds.go`,
`structs/refund.go`) plus 2 on the web side (`location-transactions.tsx`, `refund-modal.tsx`).
6 PRs merged: #172–#177.

**webpage** — 18 files, +803/−175 across 3 commits. Excludes Sanchez's Sep 22–23 work.

# Not done

- **Mobile merchant mode refunds.** The backend is shared and the marks are server-computed, so this
  is the modal and list again in React Native.
- **Unfunded volunteer events that elapse before a top-up** stay stranded: `GetUnfundedVolunteerEvents`
  filters `expiration > NOW()`, so the shortfall email's promise that "codes will be generated
  automatically" cannot be kept past the event's end.
- **A recurring event that has never generated is undetectable.** Nothing logs or alerts on it; that
  is why this one was found by an affiliate re-creating their event by hand.

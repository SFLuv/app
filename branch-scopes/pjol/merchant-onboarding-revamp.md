# Branch scope — `pjol/merchant-onboarding-revamp`

Aug 28 – Sep 1 2026 · app + mobile-app · **4.8h active** (4.79h raw, across three sittings)

Merchant onboarding and the location request flow, rebuilt around a split between merchant accounts
and personal ones. Some of that split already existed on `main` — `users.account_type`, the read-only
onboarding gate, the merchant wall — and this branch finished it and rebuilt the intake form on top.

Hours are **measured from session-transcript timestamps**, clustered into sittings on a 30-minute
gap, and corroborated against file mtimes. Method: the `time-accounting` skill at
<https://github.com/pjol/SKILLS/tree/main/time-accounting> (local copy: `docs/TIME_ESTIMATION.md`).

> **Correction.** Both rounds below were first written at 8.4h and 3.1h — 11.5h claimed for work that
> measured 2.5h at the time, an inflation of about 4.7x. (The branch total above has since grown past
> that as the second sitting continued; the ratio is against the same body of work, not the running
> total.) The old figures were derived from diff volume and "the shape of the work", which estimates
> how long the work would have taken to type by hand rather than how long it took. The clock was never
> consulted. The numbers below are the clock; the method that produced the old ones is now written down
> as the thing not to do.

---

# Round 1 — Aug 28, 12:18–13:53

**Repos:** `app` · **Total active hours: 1.6 — measured**

Measured from session-transcript timestamps: one unbroken sitting, 12:18 to 13:53, no gap over 30
minutes inside it. Committed two days later as `e0774ed`, which is why commit clustering says
nothing useful about it.

The merchant onboarding flow and the location request form, rebuilt around the account-type split.

The per-feature split below is apportioned by file mtime within the sitting and is approximate; the
1.6h total is not.

## Features

| Feature | hours | repo |
|---|---|---|
| **Location Approval Form as a three-step stepper** — Public Information, Contact, Payment System, each validated before Next will move, with a submitted-successfully screen. Replaces the single sheet at all three entry points; the sheet stays only for editing an existing listing. Includes `POST /locations` returning the new id, and the place finder's manual-entry link becoming a checkbox | 0.5 | app |
| **Locations schema and write path reshaped to the questions actually asked** — migration 1.50 adds `contact_name`, `referral_source`, `accepts_tips`, `has_staff_tablet` and backfills the first three; the seven single-sheet columns are kept but no longer collected | 0.3 | app |
| **Merchant/personal account split finished** — signup's policy gate split into two views; merchant switch moved into Settings both ways; the revert refused server-side in one statement while any location is live or queued; one-time navbar offer for mobile-created accounts, keyed off a new `account_type_selected_at` | 0.3 | app |
| **Wallet provisioning split into payment and tipping halves** — a tipping wallet is minted only on a yes, and a first location can inherit the primary wallet for takings while still being minted a tipping wallet. Provisioning returns to approval time | 0.2 | app |
| **Cancelling a pending application** — owner-scoped soft delete guarded on `approval IS NULL` inside the UPDATE, retiring hours and payment wallets with it; offered on four screens | 0.2 | app |
| **Admin review modal shows the new answers** — contact block, and a Payment System block that spells out what the tips answer will do at approval | 0.1 | app |

## Totals

| | |
|---|---|
| Files changed | 22 modified, 8 added |
| Lines | ~+2,100 / −281 |
| Migrations | 1 (`1.50`) |
| New routes | 4 (`GET /users/account-type/revert-eligibility`, `PUT /users/account-type`, `POST /users/web-merchant-prompt-seen`, `DELETE /locations/{id}`) |

## Worth knowing

- **An admin can mint a location's wallets without anyone signing anything.**
  This was the open question the round started with. The account factory's
  `getAddress` is a view: the address is CREATE2 arithmetic over the merchant's
  own signer and an index, so approval derives and records it with two read
  calls and no transaction. Nothing is deployed until the merchant's first
  outgoing payment, which the paymaster covers; incoming tokens sit at the
  address either way, which is all a till needs. No faucet EOA transaction and
  no merchant-side setup step are required.
- **Provisioning moved back to approval, reversing a decision from the previous branch.** It was
  moved to creation so a merchant had a till address to look at while their
  listing sat in the queue. That trade no longer holds: an application is now
  something they can withdraw, and a withdrawn one should not leave a wallet and
  a burnt smart-account index behind it — and the tips answer, which decides
  whether a tipping wallet exists at all, is editable right up until an admin
  acts. The switch is kept, not deleted, so the old behaviour is one env var
  away.
- **The retired form's columns were kept, not dropped.** `sole_proprietorship`,
  `tipping_policy`, `tipping_division`, `table_coverage`, `service_stations`,
  `tablet_model` and `messaging_service` still hold what the merchants already
  on the map told us, and nothing needs the space back. Dropping them is a
  separate, deliberate decision if it is ever wanted.
- **Contact details are still stored twice.** `admin_email`/`admin_phone` and
  `contact_phone` are the same two answers under two names, a split the
  single-sheet form left behind. The form now writes both and
  `NormalizeForSubmission` mirrors them, so the admin panel, the approval email
  and the MCP merchant report cannot disagree — but the duplication is still
  there, and unpicking it means touching every one of those readers.

# Round 2 — Sep 1, 11:29–13:59

**Repos:** `app`, `mobile-app` · **Total active hours: 2.5 — measured**

Measured from session-transcript timestamps: 11:29 to 13:59 on Sep 1, four days after Round 1 —
which is why this is a separate round rather than more of the first.

The optional location logo, then a design pass driven by review: the stepper read as a wall of
prose, and the copy was doing work that iconography, layout and tooltips do better.

Split apportioned by file mtime; approximate below 0.1h.

## Features

| Feature | hours | repo |
|---|---|---|
| **The merchant flow ported to mobile** — the same three steps, fields and payload as the web form; two lock screens (start / pending) with a log out on each; and `App.tsx` re-gated so a merchant account never reaches the ordinary tabs. Replaced a dead single-sheet screen built on the retired schema | 0.5 | mobile |
| **The location picker rebuilt on the Places data API** — our own input and dropdown, so it is white-labelled at every width and Google's fullscreen mobile picker is gone. One box takes a business or an address and the result's own types decide the path; the checkbox slot becomes a green "Location found" or an amber "Address found" with the advice to search by name | 0.4 | app |
| **The web wall removed** — a merchant with nothing listed browses like anyone else. A one-shot redirect still takes them to the form on sign-in, Cancel leaves for Locations, and Locations becomes the merchant hub carrying the "Set up your merchant account" view | 0.3 | app |
| **Optional location logo on the form's first step** — crop-to-square picker with a live map-pin preview, held as a blob and posted once the location has an id. Reuses `location_icons`, already keyed on `location_id` alone; the crop machinery came out into `lib/location-logo.ts` so the form and the settings icon card cannot frame a logo differently | 0.2 | app |
| **The stepper became a screen** — a full-screen shell over the app's chrome, centred when short and scrolling from its top when tall, with Cancel pinned under a fade (right on mobile, left on desktop) and the confirmation it owns | 0.2 | app |
| **Step one collapsed to its happy path** — nothing but the finder until there is something to fill the rest in from; a confirmed business displays nothing at all; description optional and always shown, like the logo; hours behind their own tick | 0.2 | app |
| **Phone and email formatting** — `lib/contact-format.ts`, lenient in and strict out: every punctuation a merchant thinks in normalises to one rendering, and NANP's leading-digit rules separate a mistyped number from a valid one | 0.2 | app |
| **Prose out, tooltips in** — every helper paragraph, card subtitle and step blurb across the onboarding flow removed or reduced to a hint behind an info icon, via one shared `LabelWithHint` | 0.1 | app |
| **Account chooser as icon tiles** — `Personal` / `Merchant` with a glyph each, the permanence moved to the tile's hint, and equal-width buttons on every modal in the flow | 0.1 | app |
| Expand/collapse animation across the form (`grid-template-rows` 0fr→1fr, `inert` when closed), the sequenced slide when the address box changes place, and a one-row numbered step rail on mobile | 0.1 | app |
| **Errors scoped to their step** — the whole-form message was rendered below every step and only ever set, so a step-one complaint surfaced under step three | 0.1 | app |
| Checkbox rendering as a circle (`rounded-sm` is 8px against a 16px box); the logo crop preview stretching vertically (preflight's `img { max-width: 100% }` clamped the width while the inline height grew, so the saved file was right and only the preview lied); the confirmation surviving the remount that clearing the wall causes; `svgIconMaskURI` dropped from the Places field list | 0.1 | app |

## Totals

| | |
|---|---|
| Files changed | app: 21 modified, 6 added, 1 deleted · mobile: 4 modified, 1 added |
| Lines | app ~+1,500 / −1,200 · mobile ~+700 / −280 |
| Migrations | 0 |
| New routes | 0 — mobile reuses the endpoints the web form already posts to |

## Worth knowing

- **A location's logo was already location-bound, so no new store was built.**
  `location_icons` is keyed on `location_id` alone, cascades with the listing,
  and nothing in its query or handler layer touches `owner` — ownership is only
  ever consulted as a permission check. Adding a `location_logos` table beside
  it would have given the product two competing merchant images with no rule for
  which one wins on the map pin. The form uploads nothing until the listing
  exists, so an abandoned or cancelled application leaves no orphaned image, and
  two locations applied for in one sitting each keep their own. Nothing is ever
  parsed from the Google listing — `svgIconMaskURI` was being requested and never
  read, and is gone.
- **"Has no locations" is not the same test as "is gated".** Cancel was withheld
  from anyone with zero locations, on the reasoning that the app behind is
  switched off for them. It is not, for one person: a merchant who withdraws
  their only pending application has zero locations and a working app, because
  cancelling deliberately does not un-stamp `merchant_onboarding_completed_at`.
  They landed in a full-screen form with no exit. The test is now
  `merchantOnboardingRequired`, and Cancel goes to `/map` — `/locations` bounces
  a zero-location merchant straight back, which is a loop rather than an exit.
- **The two surfaces are gated in opposite directions, deliberately.** The web
  app is where a merchant does setup and can look around, so the wall is gone
  and only writes are refused. A phone is a till, so a merchant account there
  gets the application, the wait, or the till, and nothing else. The old mobile
  behaviour — full dock with the wallet tab swapped — handed them an app whose
  every write the server refuses.
- **The mobile app already had a merchant application screen, and it was dead.**
  Nothing imported `MerchantApplicationScreen`, and it was a single sheet built
  on exactly the columns this branch retired. Replaced rather than extended; its
  `submitMerchantApplication` was dead the same way and posted the old payload.
- **Logo and hours are web-only for now.** Both are optional, both are written
  by their own endpoints after the listing exists, and neither justified a
  cropper and a week of time pickers in a first mobile pass.
- **The merchant wall remounts the onboarding page, and that ate the
  confirmation.** The wall renders the page *in place of* the whole app, so
  listing a first shop — which clears the wall — moves the same component to a
  different position in the tree and React remounts it. The most important
  confirmation there is, a merchant's first, appeared and then vanished under
  them. It is now held in session storage and cleared by both exits.
- **A full-screen overlay, not a layout flag.** Hiding the chrome by having the
  layout read a context flag would mean setting it from inside a render and
  every host of the flow remembering to unset it. Covering the chrome is correct
  wherever the flow is mounted, including in place of the whole app.
- **Not every explanation became a tooltip.** Three stayed as visible text,
  because each says why something the merchant is looking for is *absent* — the
  missing wallet on a pending listing, the missing revert button on a locked
  merchant account, and that saving a rejected listing does not re-queue it. An
  explanation nobody can see until they hover is no explanation.

# Round 3 — Sep 1, 14:49 onwards

**Repos:** `app` · **Total active hours: 0.7 — measured, sitting still open**

Measured from session-transcript timestamps: a third sitting on Sep 1 after a
fifty-minute break, which is what separates it from Round 2. The figure is to
the last message at the time of writing.

Closing the account-type round trip, and recording when it happens.

## Features

| Feature | hours | repo |
|---|---|---|
| **A refused application blocks a revert, and can be withdrawn** — any live listing now holds an account in merchant status, in all three states. Withdrawal widens from pending to anything not approved, which is what keeps the new rule from being a dead end, and the button is offered on rejected listings wherever it already appeared on pending ones | 0.1 | app |
| **Account-type history** — migration 1.51 adds `users.merchant_since`, `users.merchant_since_inferred` and an append-only `user_account_type_events`, written from all three conversion paths (signup, settings, admin repair) inside the same transaction as the change | 0.2 | app |
| Migration 1.51 fixed after it failed on a `users.created_at` that does not exist, then verified by running it: 11 accounts backfilled, all flagged inferred, schema at 1.51 | 0.1 | app |
| **Step one's remaining rough edges** — the suggestion dropdown was being clipped by `Expand`'s own `overflow-hidden`, which now releases once the opening animation finishes; the hours tick appears only where Google left a gap; the description starts as one line and grows to fit; the "can't find my location" control moved to a fixed slot so it holds still while the search box travels; and body scroll is locked behind the overlay, which was letting the page underneath scroll with nothing below it | 0.3 | app |

## Totals

| | |
|---|---|
| Files changed | 14 modified |
| Migrations | 1 (`1.51`), applied and verified against the local clone |
| New routes | 0 |

## Worth knowing

- **Counting rejected against a revert needed a way out first.** The rule the
  product wanted — no approved, pending *or* rejected listings — would otherwise
  have stranded any merchant whose only application was refused: withdrawal
  covered pending only, so nothing could clear the block. Widening it to
  "anything not approved" is what makes the rule a rule rather than a trap.
  Approved stays un-withdrawable: the shop is on the map, money may already have
  arrived, and that is a support conversation rather than a button.
- **A first location already inherits the primary wallet, and needed no change.**
  `NeedsDerivedPaymentWallet` derives one only when the account already has a
  location holding a payment wallet; a converting merchant's first approval has
  none, so it records their existing primary. The tipping wallet is decided
  separately and is still minted whenever they answered yes to tips — which is
  the case the split in Round 1 was built for.
- **The backfilled date is an upper bound, and is flagged as one.** Writing the
  migration assumed a `users.created_at` that does not exist, which failed
  loudly and cheaply. Running it against real data then showed something reading
  the code had not: for the seven oldest merchants the winning evidence is
  itself an artefact — they share one `merchant_onboarding_completed_at` to the
  microsecond, stamped when migration 1.45 ran — so they were merchants well
  before the date now recorded. That makes `merchant_since` a date by which they
  were *already* a merchant rather than the date they became one, and splitting
  income on it would file real takings as personal. Hence
  `merchant_since_inferred`: true for every backfilled row, cleared the first
  time a conversion is actually observed. A tax export must consult it.
- **`merchant_since` is the current stint, not the first one.** Reverting is
  allowed while an account has nothing listed, so regular → merchant → regular →
  merchant is a real sequence and one column cannot describe which intervals
  were which. The column is cleared on a revert and the events table keeps the
  history; a tax export covering a period containing a flip has to read the
  events, and one that does not can take the column.
- **The backfill is a floor, not an observation.** No date was recorded for the
  merchants who already exist, so `merchant_since` is set from their earliest
  location and falls back to when the account was created. It is deliberately
  *not* written as an event: the events table holds changes actually seen, and
  an inferred date sitting among them would read as one.

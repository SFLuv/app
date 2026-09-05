# Branch scope — `pjol/merchant-onboarding-revamp`

Aug 28 – Sep 4 2026 · app + mobile-app · **20.1h active** — 8.1h measured across seven sittings, plus
12.0h of hands-on testing reported by PJ and **not** measured (see *Untracked testing time* at the foot)

Merchant onboarding and the location request flow, rebuilt around a split between merchant accounts
and personal ones. Some of that split already existed on `main` — `users.account_type`, the read-only
onboarding gate, the merchant wall — and this branch finished it and rebuilt the intake form on top.

Hours are **measured from session-transcript timestamps**, clustered into sittings on a 30-minute
gap, and corroborated against file mtimes. Method: the `time-accounting` skill at
<https://github.com/pjol/SKILLS/tree/main/time-accounting> (local copy: `docs/TIME_ESTIMATION.md`).

The one exception is the testing time in the last section, which no transcript records because no
prompts were sent during it. It is reported, not measured, and is kept apart from the measured
figures rather than folded into them.

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

# Round 3 — Sep 1, 14:49–15:54

**Repos:** `app` · **Total active hours: 1.1 — measured**

Measured from session-transcript timestamps: a third sitting on Sep 1 after a
fifty-minute break, which is what separates it from Round 2. Written while the
sitting was still open and first recorded at 0.7; the sitting closed at 15:54,
and the figure is now the whole of it.

Closing the account-type round trip, and recording when it happens.

## Features

| Feature | hours | repo |
|---|---|---|
| **A refused application blocks a revert, and can be withdrawn** — any live listing now holds an account in merchant status, in all three states. Withdrawal widens from pending to anything not approved, which is what keeps the new rule from being a dead end, and the button is offered on rejected listings wherever it already appeared on pending ones | 0.1 | app |
| **Account-type history** — migration 1.51 adds `users.merchant_since`, `users.merchant_since_inferred` and an append-only `user_account_type_events`, written from all three conversion paths (signup, settings, admin repair) inside the same transaction as the change | 0.2 | app |
| Migration 1.51 fixed after it failed on a `users.created_at` that does not exist, then verified by running it: 11 accounts backfilled, all flagged inferred, schema at 1.51 | 0.1 | app |
| **Step one's remaining rough edges** — the suggestion dropdown was being clipped by `Expand`'s own `overflow-hidden`, which now releases once the opening animation finishes; the hours tick appears only where Google left a gap; the description starts as one line and grows to fit; the "can't find my location" control moved to a fixed slot so it holds still while the search box travels; and body scroll is locked behind the overlay, which was letting the page underneath scroll with nothing below it | 0.3 | app |
| Security review launched and run — six parallel read-only reviews, their findings verified against the code rather than taken on report. The write-up lands in Round 4 | 0.4 | app + mobile-app |

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

---

# Round 4 — Sep 2, 13:25–14:43

**Repos:** `app` · `mobile-app` · **Total active hours: 1.3 — measured**

Measured from session-transcript timestamps: one unbroken sitting, no gap over 30 minutes inside it.

A security pass over the whole surface, then the three bugs that first real submission attempt found.

## Features

| Feature | hours | repo |
|---|---|---|
| **Security review written up** — the six reviews from Round 3 consolidated and ranked. One critical (anonymous redemption payouts redirectable to an attacker, composed from three individually modest defects), eleven high, the rest medium and below. Three systemic patterns rather than twenty-six accidents: secrets that fail open when empty, addresses accepted without proof of ownership, and ledger states that read as success. **No code was changed for it** — findings only, and the fixes below are the only ones since applied | 0.3 | app + mobile-app |
| **Privy session tokens no longer fall back to plaintext** — the keychain fallback is scoped to dev builds and degrades to memory in release, every successful keychain access clears any plaintext copy, and a legacy entry left by an earlier version is migrated and deleted on the next read rather than logging anybody out. Committed as `2cbb76d` | 0.3 | mobile-app |
| **Continue on step two was submitting the form** — both footer buttons rendered into one JSX slot, so React reused the DOM node and mutated `type="button"` into `type="submit"`. Advancing flushed the re-render before the browser ran the click's default action, and a plain Continue submitted, failed validation against a Payment System step nobody had reached, and left "Please fix the highlighted fields" over a step just arrived at. Distinct keys make it an unmount and a mount, so the clicked node leaves the document before the default action runs | 0.3 | app |
| **The location description is optional again** — it sat in the required list of both `ValidateForSubmission` and `ValidateForUpdate` while the form marked it optional, so a submission was refused over a field the merchant was told they could leave blank | 0.1 | app |
| **500 on every submission** — `INSERT has more expressions than target columns`: 38 columns, 39 value expressions, 37 arguments. A stray trailing `$38` with no column behind it and nothing to fill it, introduced in `e0774ed` when the four onboarding columns went in and five placeholders were added for four | 0.3 | app |

## Totals

| | |
|---|---|
| Files changed | 5 modified across two repos |
| Migrations | 0 |
| New routes | 0 |

## Worth knowing

- **The critical finding is a composition, and that is the point.** Registering
  an arbitrary address as your own `smart_address` is modest. An owner-scoped
  uniqueness index is modest. A lookup tie-break that ranks a `smart_address`
  match above an `eoa_address` match is modest. Chained, they let an
  unauthenticated attacker collect another person's volunteer reward while the
  victim's code is consumed and not returned. None of the three would be caught
  by reviewing its own file.
- **The insert bug was mine, from Round 1.** Four columns were added and five
  placeholders with them. It is the kind of error a count catches instantly and
  a careful read does not, which is why the fix was verified by counting columns
  and expressions programmatically rather than by eye.

---

# Round 5 — Sep 4, 12:41–13:39

**Repos:** `app` · `mobile-app` · **Total active hours: 0.9 — measured**

Measured from session-transcript timestamps; the sitting was still open when this was written, so the
figure is to the last message at the time of writing.

## Features

| Feature | hours | repo |
|---|---|---|
| **Merchant setup no longer hangs on "Checking your locations…"** — the device installation id was read with raw `SecureStore`, which throws on any build without a keychain entitlement. The throw landed before `/merchant-mode/status` was ever requested, the caller logged it and moved on, and the readiness gate read the resulting null as "still loading" forever. Thirteen call sites shared the hazard. Given its own SecureStore-first, AsyncStorage-fallback pair rather than the Privy adapter, whose release fallback is memory — correct for a session token, and a new device identity on every launch for this | 0.2 | mobile-app |
| **No map flicker between choosing a merchant account and the form** — the gap is two awaits wide, and the app is authenticated on a route that is still the map for the whole of it. An opaque cover, raised before the policy overlay closes and lowered when the form is actually on screen, with every non-navigating branch clearing it and an 8s expiry so it can never become a spinner of its own | 0.2 | app |
| Branch scope remeasured and brought up to date — two unrecorded sittings written up, Round 3's total corrected from 0.7 to 1.1 after its sitting closed, and the header total corrected with them | 0.2 | app |
| **The backend accepts submissions from the mobile build already in the app stores** — the three-step form added three required fields the pre-Jul-21 client never sends, which would have refused every location submission from it the moment the backend deployed. `ValidateForSubmission` now takes the intake form the submission came from and asks for those three only of the current one. Audited the rest of the old client's surface against the new backend at the same time: no route it calls was removed, no JSON key it reads was dropped | 0.3 | app |

## Totals

| | |
|---|---|
| Files changed | 5 modified across two repos |
| Migrations | 0 |
| New routes | 0 |

## Worth knowing

- **A client-version check cannot tell the two mobile builds apart.** It was the
  obvious way to keep the old client working and it does not work: the build in
  the app stores and the build waiting on review both report version `1.0.3`,
  because the version string was never bumped. So the intake form is identified
  by which keys the request actually carries — the five the three-step form
  always sends and no older client sends any of. That asks what the request
  contains rather than what the sender claims to be, which also means the web
  client needs no special case despite sending no version headers at all.
- **The 426 legacy-client block does not catch this client, and that was worth
  checking.** `CLIENT_VERSION_LEGACY_BLOCK_ENABLED` defaults to on and answers
  `GET /users` with 426 for anything that looks like a native client sending no
  version headers. The pre-Jul-21 build sends them from its shared
  `rawAuthFetch` — they were added in May — so it is not caught. The block
  targets builds older than that.
- **The right fix was not the obvious one.** An outside analysis proposed
  routing the installation id through `resilientPrivyStorage`, which would have
  been correct before Round 4 hardened that adapter. Its release fallback is now
  memory, so on the unsigned release build in question the id would have been
  minted afresh on every launch — no hang, so the reported symptom disappears,
  but a PIN prompt every start and an orphan device binding per launch, with no
  error anywhere to notice. A session token is worth losing rather than writing
  to disk; a device identifier hashed server-side is the opposite trade. The two
  do not share a store, and the code now says why.
- **Still open:** `merchantSetupReady` hard-hangs whenever the status fetch
  fails for any other reason — a dropped network gives the same infinite spinner
  from a different cause. Not fixed, because what that screen should show
  instead is a product decision.
- **Two old-client behaviours are changed but not broken, and are left as
  product calls.** A redemption that lands in W-9 escrow answers `202`, which
  the old client reads as plain success and reports as paid; and a merchant
  account created on the web that then signs in on the old build collects `403`
  with an `X-SFLUV-Auth-Reason` header it does not read. Neither stops the app
  working, and both disappear when the pending release lands.

---

# Untracked testing time

**Repos:** `app` · `mobile-app` · **Total hours: 12.0 — reported, not measured**

Three mornings of hands-on testing — Sep 2, Sep 3 and Sep 4, roughly 09:00 to 12:00 each — plus the
afternoon of Sep 4, during which no prompts were sent and no files were changed, so no transcript
records them.

| Session | hours | basis |
|---|---|---|
| Wed Sep 2, ~09:00–12:00 | 3.0 | reported by PJ |
| Thu Sep 3, ~09:00–12:00 | 3.0 | reported by PJ |
| Fri Sep 4, ~09:00–12:00 | 3.0 | reported by PJ |
| Fri Sep 4, afternoon | 3.0 | reported by PJ |

**How this figure was arrived at, and what is wrong with it.** It was not measured. It is PJ's own
account of time spent testing the branch by hand, recorded here because the work happened and the
transcript cannot see it — the measurement method reads message timestamps, and silence looks
identical to absence. The weaknesses are worth naming rather than burying:

- The boundaries are approximate on both ends ("9ish to 12ish"), so each block is a round 3.0h rather
  than a measurement, and the total inherits that.
- Nothing corroborates the morning windows. File mtimes place activity on Sep 3 at 13:58 — the
  afternoon, not the morning — which confirms the day was worked but says nothing about 09:00–12:00.
  Sep 2 and Sep 4 have measured sittings that all start after 12:00, so the morning blocks do not
  overlap anything already counted. The Sep 4 afternoon block sits in the 6.2h gap between that
  day's 13:38 and 19:52 sittings, so it does not overlap either. None of this time is
  double-counted.
- Testing time is real work and belongs in the total. It is kept in its own section, and out of the
  measured figure, so that a later reader can tell which number came from a clock.

---

# Round 6 — Sep 4, 19:52–20:16

**Repos:** none — local toolchain · **Total active hours: 0.4 — measured**

Measured from session-transcript timestamps. No code changed; recorded because the time was spent on
this branch's testing and the conclusion is worth not rediscovering.

## Features

| Feature | hours | repo |
|---|---|---|
| iOS simulator diagnosed and unblocked — "the iOS-18-3 simulator runtime is not available" was a stale `CoreSimulatorService`, up 3 days 8 hours, not a missing runtime. `simctl list runtimes` reported it available the whole time while `simctl boot` refused with "runtime profile not found using System match policy"; restarting the service fixed it first try. Also found the disk at 100% (2.9 GB free), which `npm cache clean --force` relieved by ~15 GB | 0.4 | app |

## Worth knowing

- **The error named the wrong thing, twice.** It says to download a runtime
  that is installed, mounted and `Ready`. The real fault was a long-lived
  service holding a stale view, and the tell was its uptime rather than
  anything in the runtime listing — which is why the first pass at this, six
  hours earlier, checked the runtime, found it healthy, and moved on.
- **A plausible second theory was wrong and cost a download.** Xcode 16.2 does
  ship only the iOS 18.2 SDK against an installed 18.3 runtime, which is a real
  mismatch — but it bites `xcodebuild`, and the boot script never compiles
  anything: it installs cached Expo Go with `simctl` and deep-links the dev
  URL. An 8 GB download for iOS 18.2 was started on that theory and stopped
  once the script was actually read. Do not delete the 18.3 runtime.

---

# Round 7 — Sep 4, 22:12 onwards

**Repos:** `mobile-app` · **Total active hours: 0.3 — measured, sitting still open**

Measured from session-transcript timestamps; the figure is to the last message at the time of writing.

Bringing the mobile location form up to the web one, and closing the last layout flash.

## Features

| Feature | hours | repo |
|---|---|---|
| **The mobile location box behaves like the web one** — predictions as you type on a 220ms debounce instead of a Search button, a clear control on the input, typing past a confirmed place clearing it, and the green "Location found" / amber "Address found" status with the "Can't find my location" way out. Adds the address-only path the mobile form never had: `listing_source` is now sent, so a merchant whose shop Google has no listing for can file one from a phone | 0.2 | mobile-app |
| **`street_address` no longer arrives as a business type** — the details mapping took `types[0]`, which on an address result is the literal string `street_address`, and presented a Google taxonomy token to the merchant as their own answer. Category now comes from the first type that is not address-only, prettified, and is empty rather than wrong when Google has none | 0.05 | mobile-app |
| **Merchant accounts no longer flash the consumer app while signing in** — the dock rendered until the profile and the merchant-mode calls answered, which is the one layout a merchant account is never given. A spinner covers the gap, the dock and notification bell are suppressed with it, and it expires after 10s so a failed merchant-mode call cannot hold the account behind it | 0.05 | mobile-app |

## Totals

| | |
|---|---|
| Files changed | 5 modified, 1 added |
| Migrations | 0 |
| New routes | 0 |

## Worth knowing

- **The description was required on mobile and optional on the web.** Same
  endpoint, same backend rule since Round 4, two different answers to the
  merchant. Now optional on both.
- **Phone and email formatting was missing entirely on mobile.** The web helper
  is ported verbatim rather than reimplemented, for the same reason: a number
  accepted in a browser and refused on a phone is one merchant told two
  different things about one answer.
- **The two paths are decided by the result, not by a mode.** A business carries
  its name, category, hours and phone; an address carries none of those, so its
  name is dropped rather than inherited and the merchant types it. Ticking
  "can't find my location" reorders the step so the address sits last; picking
  an address from the search deliberately does not move anything.

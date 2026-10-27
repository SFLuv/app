# Branch scope — `pjol/mobile-2-1`

Oct 26–27 2026 · app + mobile-app + webpage · **11.3h** (9.3h stated for Oct 26, 2.0h measured for Oct 27)

| Round | When | Hours | Basis |
|---|---|---|---|
| 1–3 | Oct 26 | 9.3h | stated, with 6.2h measured as an upper bound |
| 4 | Oct 27 morning | 0.4h | **measured** |
| 5 | Oct 27 afternoon | 1.6h | **measured** |
| | | **11.3h** | |

Rounds 4 and 5 are measured end to end, from the session transcript's message
timestamps clustered on a 30-minute gap. Rounds 1–3 are not, and the distinction
matters more than the total: **9.3h of this figure is what somebody said about a
day, and 2.0h is what a clock recorded.** Do not average them.

Round 1's two prospective entries are superseded by round 2 — see there. Round
2's total is extended by round 3, which measured the evening sitting running 28
minutes later than round 2 recorded it.

The work behind mobile release 2.1 — tagged `v2.1` on `mobile-app` — plus the
backend it needs and the organization and supervisor cleanups done alongside it.

Cut from `pjol/internal-notifications`, which is itself stacked on
`pjol/admin-org-management`. Those two carry their own scope documents; this one
covers Oct 26 and Oct 27.

## How these hours were recorded

**Read the three kinds separately. Only one of them is a measurement, and even
that one is a ceiling rather than a figure.**

| Window | Hours | Basis |
|---|---|---|
| 11:00–16:00 | 5.00h | stated — "pretty much straight through" |
| 16:00–17:00 — dev meeting | 1.00h | **prospective**, recorded before it happened |
| 17:00–18:00 — mobile build 2.1 session | 1.00h | **prospective**, recorded before it happened |
| | **7.0h** | |

> **Superseded by round 2.** The two prospective rows were corrected once those
> windows had passed: 17:00–18:00 was not a build session, and the pair is
> replaced by one stated window. Left as written rather than edited, because an
> earlier figure in this folder is never silently restated — but do not quote
> the 7.0h, which round 2 revises to 8.9h.

**The two prospective entries are a plan, not a record.** They were written
down at 16:05, before either window began. That is a weaker claim than the rest
of this folder contains: a stated figure is at least somebody's account of time
they spent, and these are an intention to spend it. They are recorded because
they were asked for and because the 2.1 build genuinely is the next thing;
they should be corrected to what actually happened rather than left standing.

**The measured figure is 4.2h and it is an upper bound, not a measurement of
this work.** The session transcript for this repository spans 11:18–13:57 and
14:33–16:04, which is 4.17h. But three other projects were open inside those
same spans — `SFLUV-animations` (1.28h across three sittings), `YT-BOT` (1.72h)
and `flipperdotfamily` (0.39h) — so attention was split and the clock cannot
say how it divided. Every minute of those 4.17h is bounded above by the span
and below by nothing the transcript can see.

The owner's 5.0h and the measured 4.17h are therefore not in conflict: the
stated figure covers 11:00–16:00 continuously, the transcript starts eighteen
minutes later and stops four minutes earlier, and the overlap with other
projects means the measured span is the outer edge of the app work rather than
its size.

## Large features

### Beta access for bulletin and chat, enforced server-side — 1.4h · app + mobile-app

The two surfaces became one server switch each plus a per-user grant list, and
enforcement moved out of the client.

Migration 1.69 adds `beta_feature_access`. Revoking stamps `revoked_at` rather
than deleting, and that is the whole mechanism behind "re-added to the chats
they were in": nothing is ever removed from `conversation_members`, so there is
nothing to restore. Access is a question the gates ask, not a state anybody
repairs.

Every `/bulletin` and `/conversations` route checks the grant per request, so a
revoke takes effect immediately rather than at the next app launch. A hidden
tab is one HTTP call away from being used, so hiding was never enforcement.
Admins are deliberately not exempt, unlike every other guard in this router: a
gate an admin cannot feel is a gate nobody can test.

Chat has a second party, so three more rules: a conversation cannot be started
with somebody who lacks the grant — reported as "could not be found", exactly
as a blocked person is, because a different answer would let anyone enumerate
the beta by trying to message people; a counterparty who has lost access is
marked `other_user_inactive` and dimmed, with the composer replaced; and they
stop receiving message pushes while their conversations and unread counts stay
exactly where they are.

### Migration recovery actually works — 0.4h · app

`sigauth` verified the wrong preimage. The Citizen Wallet SDK's
`generateConnectionMessage` returns `id(message)` — the keccak256 of the text —
and signs `getBytes()` of that, so the EIP-191 preimage is 32 digest bytes and
never the text. The backend hashed the text.

The consequence was total rather than partial: recovering from the wrong
preimage yields an address belonging to nobody, so the EOA comparison failed
and all three smart-account fallbacks failed with it. Every sigAuth was
rejected, which means **no migration recovery balance has ever been
claimable** — and those unclaimed balances are still sitting in the faucet's
allocated total.

Verified against the SDK's own published vectors run through the patched
functions: both digests match `id(message)`, and the signed vector recovers
`0xc7708688514e823b239dc96501456484DC9F1858`, the signer the SDK expects.
Before the fix the same vector recovered an unrelated address.

### Supervisor rewards removed — 0.8h · app

Supervisors keep the role; the payment for it is gone, so a workflow's only
bounties are its steps'.

The input boundary refuses a positive supervisor bounty in the one normalizer
that workflow creation, template creation and the edit-proposal comparison all
share. Refused rather than quietly zeroed: both mean no reward, but zeroing
lets a proposer believe they allocated something and watch it disappear.

Migration 1.72 clears what exists in all three places that hold it —
`workflows`, `workflow_states` and `workflow_templates`. Leaving any one would
hand the create path an amount it now rejects, so the next occurrence of a
recurring workflow would fail to be created at all.

**Only unpaid rewards are cleared.** A reward already marked paid is a record
of money that left the faucet; rewriting it to zero would make the ledger
disagree with the chain. That clearing is also the kill switch, which is why no
payout code was touched: the payout path declines when the bounty is zero, so
the pipeline is now unreachable rather than cut out from under a money path
that cannot be tested here.

### Cross-organization member and permission tooling — 0.7h · app

Every member operation was `/organizations/mine/*`, so the house organization
could see a partner's roster and change its superadmin but not invite, remove
or re-role anybody — leaving the only fix for a partner locked out of its own
admin being to hand the whole organization to somebody else.

Done as a capability rather than a hard-coded check: `admin.orgs.members` joins
the Administrative section, so the house superadmin can delegate it without
making somebody a superadmin. `/admin/organizations/{id}/permissions` was added
later the same day for the fine-grained half. The Administrative group is
deliberately absent from it — offering it on a partner's member list would let
somebody grant another organization's user powers over our own protocol
accounts.

Split onto `pjol/admin-org-management` so it can land in main on its own.

### Support requests route by category — 0.5h · app + webpage

The webpage sends `subject_slug` beside the display label, and that slug is the
routing key. Migration 1.70 carries it on the alert and the stored request and
adds `support_category_subscriptions`.

**No rows means every category**, which is the whole design: everybody
currently receiving support requests keeps receiving all of them, and nothing
changes until somebody narrows it. Reading an empty set as "none" would have
stopped support requests reaching every existing recipient on the deploy that
ran this.

### One form for every contact route — 0.3h · webpage

`/support` was still composing a `mailto:` — the same thing `/contact` did
before the Get in touch page replaced it. It now redirects to the form with
Technical support preselected, and every entry point builds its link through a
typed helper rather than a hand-written `?subject=`, because a stale slug fails
silently: the page falls back to General and nobody notices the link stopped
carrying its origin.

### Standing organization allocations removed — 0.3h · app

Allocations happen one way now: an event is requested, approving it commits
faucet funds, recorded in `event_allocations`.

It was already gone in substance, which is what made this safe: the admin
panel's Allocations tab saved to a route that **does not exist**, so every save
404'd, and nothing called `ReserveOrganizationBalance`,
`RefundOrganizationBalance` or `ResetOrganizationAllocations`. The model had
been cut out from under the UI and the menu left behind. 532 lines out, 12 in.

### Pre-permissioned invites — 0.4h · app

An invite now records what the invitee becomes. Before, every invite produced a
member with `inherit` — every capability of every role the organization holds —
so the only way to give narrow access was to let somebody in with everything
and take it away afterwards. The new default for a new invite is **no
permissions**.

Migration 1.73's column defaults are the OLD behaviour on purpose, which is a
different question from the new default: invites already sent and not yet
accepted are rows in that table, and somebody was told they were joining a
team. They get what they were promised.

## Smaller items

| Item | Hours | Repo |
|---|---|---|
| Beta access cached so the dock stops rearranging on every cold start; mirrored in memory because an effect runs after first paint, which is the reflow itself | 0.3 | mobile-app |
| Dock and Map/List labels shrink to fit instead of running onto their indicators; `chromeLabel` caps the system text-size multiplier | 0.2 | mobile-app |
| Merged `supervisor.workflows.view` and `.export` into one capability; migration 1.71 grants the survivor before deleting the retired rows | 0.2 | app |
| Proposer and role-request alerts that were never posted, and `resolveRoleRequestAlert` which had no call sites at all — so deciding in the panel left the alert on every admin's phone | 0.3 | app |
| Merged main (14 commits), resolving a semantic conflict in `appendW9TierNotification`: main's admin-request feature kept, this branch's "W-9 notices do not ack on view" rule kept | 0.3 | app |
| `dev-up` pointed at the local mobile-app and webpage checkouts, which it was not — it was resetting `tmp/mobile-app` to `origin/main` every boot and silently running last week's app | 0.1 | app |
| Billing report for Oct 19–24 in the template's format | 0.2 | app |

## Totals

| Source | Hours | Basis |
|---|---|---|
| 11:00–16:00 | 5.00h | stated |
| 16:00–17:00 dev meeting | 1.00h | prospective |
| 17:00–18:00 mobile build 2.1 | 1.00h | prospective |
| **Total (round 1 as written)** | **7.0h** | |
| *for reference: app transcript span* | *4.17h* | *upper bound, three concurrent projects* |

> **Superseded by round 2**, which replaces the 2.0h of prospective entries
> with the window that actually covered them. The day's total is 8.9h.

The itemised features and smaller items above come to 6.1h against the 5.0h
stated for 11:00–16:00. They are deliberately not reconciled: the items are
proportional estimates of where the time went within the stated window, not
measurements, and forcing them to add up would dress a split guess as a
measured one. **The stated total is the figure to quote.**

## Volume

| Repo | Files | Lines |
|---|---|---|
| app | 76 | +4,958 / −1,114 |
| mobile-app | 8 | +444 / −79 |
| webpage | 7 | +98 / −28 |

5 migrations (1.69–1.73) · 10 new routes, 1 removed · 0 Ponder changes.

## Backwards compatibility with the shipped build

Checked before tagging, because the 2.1 build is waiting on store approval and
the backend lands first.

The shipped build is `mobile-app` `main` at version **1.0.3**, and it has no
Bulletin or Messages screens — those arrived on this branch. Its client calls
34 endpoints, and **none of them is one this work gated or removed**: no
`/bulletin/*`, no `/conversations/*`, no `/me/messaging-summary`, no
`/supervisors/*`, no `/organizations/*`.

The six `features` flags it reads from `/config` — `migration_banner`,
`sends_enabled`, `redemptions_enabled`, `workflow_payouts_enabled`,
`merchant_payments_enabled`, `volunteer_events_enabled` — are all still served;
the overlay only adds keys. Four of them default to **true** when absent, which
is worth knowing as a hazard for any future change that drops one.

So nothing in this work breaks 1.0.3. Two behaviour changes do reach it, both
intended: supervisor rewards are zeroed, so workflow totals it displays get
smaller; and W-9 notices no longer clear on view.

## Open

- **The organization settings rework is unfinished.** The backend is in place —
  pre-permissioned invites and the cross-org permission endpoints — but the
  "My Organization" page, the member list with search and pagination, the
  per-member modal and the add-member modal are not built, and the admin panel
  has no modal parity yet. The screen still lives inside `/settings` with
  permissions as a section at the bottom.
- `PROTOCOL_RECOVERY_OWNER` must be set before `PROTOCOL_DEPLOY_ENABLED`, and
  the paymaster dry run (`go run ./cmd/protocoldryrun`) has still never been
  run. Every sponsorship assumption in the cutover remains an assumption.
- The support-category checkboxes exist in the API but not in the settings UI,
  so narrowing is API-only.
- Organization role requests, improver credential requests and volunteer event
  approval still raise no internal alert; each needs its own approve/reject
  path rather than just a title.
- `sanchezo/server-reward-labels` is unmerged on `mobile-app`.
- `organization_allocations` and the MCP report that reads it are still there;
  dropping the table is a data decision.
- `v2.1` is tagged at `7de861e`, but `app.config.ts` still reads `1.0.3`. The
  version string was not bumped — that is store submission metadata. Moving the
  tag later needs `-f` and a force-push.

# Round 2 — Oct 26 afternoon and evening

Oct 26 2026 · **3.9h stated** (1.6h measured, inside it) · app + mobile-app

Continuous from where round 1 stopped: the organization screen rework, the
mobile UX pass behind release 2.1, a notification previewer for the dev menu,
and a migration that could not run.

## This supersedes round 1's prospective hours

Round 1 recorded two windows **before they happened** — 16:00–17:00 as a dev
meeting and 17:00–18:00 as a mobile build session, 1.0h each — and said they
should be corrected to what actually occurred rather than left standing. This
is that correction.

| | Round 1 said | Actually |
|---|---|---|
| 16:00–17:00 | dev meeting, 1.00h prospective | no app transcript; a 3-minute blip in another project at 16:41 |
| 17:00–18:00 | mobile build 2.1 session, 1.00h prospective | 13 minutes of app work, 17:37–17:50, ending in the organization rework commit at 17:49 |

So the second guess was wrong about the content: that hour was not a build
session, it was the start of the organization rework. The first is consistent
with a meeting — a meeting is exactly what leaves no transcript — but the clock
cannot confirm one, and this document does not claim it did.

**It does not matter to the total, which is the useful part.** The owner states
16:00 to now was worked straight through, so whether 16:00–17:00 was a meeting
or was work, it is one hour of the same continuous window. The two 1.0h
prospective entries are replaced by one stated window covering all of it.

## How these hours were recorded

**3.9h stated**: 16:00–19:52, continuous, by the owner — "pretty much a
straightshot session".

**1.6h of that is measured**, and the rest is not observed at all:

| Window | Hours | Basis |
|---|---|---|
| 16:00–16:08 | 0.13h | measured — the tail of round 1's second sitting |
| 16:08–17:37 | 1.48h | **not observed** — no app transcript |
| 17:37–17:50 | 0.22h | measured — organization rework, committed 17:49 |
| 17:50–18:37 | 0.78h | **not observed** — no app transcript |
| 18:37–19:52 | 1.25h | measured — mobile UX, dev menu, migration repair *(open as written)* |
| | **3.87h → 3.9h** | |

Two thirds of this window is unobserved. That is not a contradiction of the
stated figure — a meeting and reading a diff both leave the transcript empty —
but it is the weaker two thirds, and the measured 1.6h is all the clock can
support.

The day's app transcript now spans 5.70h in four sittings. Round 1 named its
own weakness and it still holds for the earlier part: three other projects
(`animations`, `YT-BOT`, `flipperdotfamily`) were open inside the 11:18–13:57
and 14:33–16:08 sittings, so those spans are an upper bound on app work rather
than a measurement of it. The two evening sittings have no such overlap.

## Large features

### The organization screen rework — 1.1h · app

Moved out of `/settings` into `/organization` in the sidebar, because it was
the largest tab on a page about the account — an admin managing a team did it
beside their own notification preferences and a delete-account button.

The roster listed everybody twice, once as members and again at the bottom as
permissions, so deciding about one person meant finding their two rows. Now the
person is the row: Add member at the top, a searchable paginated list showing
superadmin / admin / member / pending, and clicking somebody opens everything
about them. Pending invitations sit in that list carrying the role they will
hold, so a row says "Pending admin" rather than just "pending".

Changes draft and save together instead of applying as you tick, which matters
most when narrowing somebody: the intermediate state is the permissive one. The
add-member modal opens with nothing ticked, and the role and permissions ride
on the invite to be applied on acceptance — closing the window where a new
member briefly held everything.

The platform admin panel mounts the same directory and modals against a partner
by id, so there is one implementation of "configure this member" rather than
two that drift.

**Two regressions caught while porting, both from the component being
replaced.** It offered the SFLuv-only Administrative group for every role, not
just members; the first pass had it inside the members-only branch, which would
have quietly removed the only way to grant the house superadmin protocol
access, since administrative grants are never inherited. And that group is the
only place `admin.orgs.members` is granted — the capability built earlier the
same day — so dropping the old component without carrying it over would have
left it ungrantable.

### Mobile UX for release 2.1 — 0.8h · mobile-app

**The dock is five slots, always.** Each beta surface now swaps the tab already
in its slot rather than adding one: Bulletin takes Activity's, Messages takes
Contacts'. With neither beta on it was four, and turning one on reflowed the
row under somebody's thumb.

**Notifications became two tabs**, New and History, inline with the back
button, and the screen opens on History when there is nothing new — stacking
the sections meant scrolling past an empty "Nothing right now" to reach what
somebody came for. Its own duplicate "Notifications" title is gone; the app
header above already says it.

**Activity got a search bar** where the wallet pill was, with the back arrow
inline to its left. It matches the row as displayed — the contact or merchant
name on screen — plus the raw addresses, hash, type and memo. Load more is
gone: a FlatList fetching a screen and a half ahead, page size 10 → 20. It does
not fetch while filtering, because the end of a filtered list is not the end of
the data.

**Both screens swipe right to go back**, on PanResponder rather than
gesture-handler. Not a preference: this app has neither gesture-handler nor
reanimated, so adding one means a new binary — and with 2.1 waiting on store
approval and expo-updates now shipping JavaScript over the air, a change that
can go OTA is worth more than a smoother one that cannot.

The back row on this screen and the volunteer detail page (`paddingTop: 2`)
both sat flush against the app header, which on a tall status area reads as
crowding the top of the screen.

### A notification previewer in the dev menu — 0.4h · app

Every kind the app can render, written straight into the local database so it
can be looked at without waiting for the thing that normally produces it. No
backend endpoint: the menu already works this way, and a dev-only route would
be a production surface to secure in exchange for nothing.

Four mechanisms, kept apart because acknowledging them is not one operation —
an ordinary row acks by being seen, an internal alert resolves for every
recipient at once, a tier prompt comes from `w9_tier_notices`, and "rewards are
waiting" comes from `payout_ledger` instead.

**That last split came from checking rather than assuming.** The first version
had both W-9 kinds as one mechanism writing a tier row; the feed actually
derives them from different tables and suppresses the tier prompt entirely once
anything is escrowed, so folded together it would have previewed the wrong
screen under the right name.

10 of the 12 kinds are offered. The two that are not are documented with the
reason: `bulletin_message` is the one kind the backend deliberately never
records, and `workflow_payout_pending` needs a workflow fixture.

### The migration that could not run — 0.3h · app

Migration 1.70 failed on this database: it `ALTER`s `support_requests`, and
that table **is not created by any migration on this branch** — the statement
that made it left with the Telegram bot and was never replaced. So it exists on
machines that ran the bot's branch and nowhere else, including production.

**No rollback was needed, which is the finding.** Postgres wraps a
multi-statement query in an implicit transaction, so 1.70's whole batch rolled
back and 1.71–1.73 never ran — the database sat consistently at 1.69 with
nothing half-applied. Verified object by object rather than assumed. Rolling
back would have been destructive work for no gain.

Sanchez had already fixed it and it was on main through PR #206, which also
carried both of this stack's merged branches. Merged, three conflicts resolved
— the migration taking main's fix, both frontend files taking this branch's
rework after checking that main's only changes to them were this branch's own
allocation removal arriving back through that PR. Migrations then applied
1.69 → 1.73 cleanly.

## Smaller items

| Item | Hours | Repo |
|---|---|---|
| Pulled and merged `sanchezo/bulletin-notifications-error-fixes`, which fast-forwarded: the reward-label work flagged as unmerged, expo-updates, payment and QR false-failure fixes, and the 2.1.0 version bump that resolved the `app.config.ts` mismatch flagged at tagging | 0.2 | mobile-app |
| `pjol/mobile-2-1` created and pushed on mobile-app and webpage, which were still on `pjol/internal-notifications` | 0.1 | all three |
| Orphans removed rather than left: `member-permissions.tsx`, the dead header row kind, and the styles and props the two screen rewrites stranded | 0.2 | app + mobile-app |

## Totals

| Source | Hours | Basis |
|---|---|---|
| Round 1 — 11:00–16:00 | 5.00h | stated |
| Round 2 — 16:00–19:52 | 3.87h | stated (1.60h measured inside it) |
| **Total for Oct 26** | **8.9h** | |

> **Extended by round 3.** This figure stops at 19:52, where the writing
> stopped; the sitting ran to 20:20. Round 3 measures the remaining 0.47h and
> the day totals 9.3h. Left as written rather than edited, per this folder's
> rule that an earlier figure is never silently restated.

Round 1's 2.0h of prospective entries are **not** added to this; they are
replaced by round 2's window, which covers the same time and more. The itemised
features above come to 3.1h against the 3.87h stated, and are not reconciled
for the reason round 1 gave: they are proportional estimates of where the time
went inside a stated window, and forcing them to add up would dress a split
guess as a measurement. **The stated total is the figure to quote.**

## Volume

| Repo | Files | Lines |
|---|---|---|
| app (round 2) | 18 | +1,504 / −556 |
| mobile-app (round 2) | 5 | +467 / −217 |

Local database now at 1.73. 73 migrations, strictly increasing.

## Open

- **The organization rework is untested beyond type-checking.** It is a
  permissions UI with real consequences and wants a pass through `dev-up`:
  invite somebody with a narrow set and confirm what they hold on accepting,
  and confirm the house superadmin can still reach the Administrative boxes.
- The four mobile UX changes want a device pass. `tsc` cannot say whether the
  swipe threshold feels right or whether five dock labels fit on a narrow
  screen.
- The notification previewer has only been parsed, not run. Its inserts are
  built from the schemas as read; the escrow one touches the most required
  columns and is the likeliest to need a correction on first use.
- Was 16:00–17:00 a dev meeting? The clock cannot see one. It does not change
  the total either way, but the record should say which it was.

# Round 3 — Oct 26 night

Oct 26 2026 · **0.5h measured** · app

The St. Anthony's volunteer-availability integration, reported as not picking up
a recurring occurrence. Two real bugs and 35 lines of change, almost all of the
time spent finding them rather than writing them.

## How these hours were recorded

**0.47h → 0.5h, measured end to end**: 19:52 to 20:20, with no unobserved part.
Nothing in this round is stated rather than measured, which makes it the
best-supported figure in this document and the smallest.

**It is the same sitting round 2 ended inside, not a new one.** Round 2 recorded
that sitting as 18:37–19:52 because 19:52 is where it stopped writing; the
sitting itself ran to 20:20. Re-measured the next morning it is one span of
1.72h, of which round 2 claimed 1.25h and this round claims the remaining
0.47h. The break between them was six minutes — well inside the 30-minute gap
that would make it a second sitting.

Three commits at 20:04, 20:08 and 20:19 sit inside the window, and the last
message of the night is stamped 20:17, so the 20:20 boundary is the end of the
work and not a trailing idle tool call.

The Oct 27 session that wrote this section is not counted in the Oct 26 figure.

## What was wrong

### The integration was switched on and keyed to nobody — 0.20h · app

Partner availability shipped to main in PR #206 and was configured
`enabled: true` with St. Anthony's on, so the reasonable reading was that a
deploy had missed a catch-up. It had not.

`v4s-integrations.json` keyed the entry on `"St. Anthony's"`. The organizations
row reads **"St. Anthony Foundation"**. Matching strips case and punctuation
before comparing, so that put `stanthonys` against `stanthonyfoundation`,
`Config.For` returned false, and enrichment returned before it parsed a link or
made a single request. No availability on any of their events, ever — not just
the recurring one.

**The reason nobody caught it for a deploy and a half is worth keeping.** The
boot log prints `v4s partner availability: enabled (...): St. Anthony's -> ...
[on]`, because `Describe` reports what is in the file, never whether an entry
matches a real organization. Switched on and keyed to nobody logs identically
to working.

Established by running the shipped `LoadConfig` and `For` against the real
config file rather than by reading them, and confirmed afterwards that the
corrected name matches along with its case and punctuation variants, and that
no other organization's name normalizes into it — there are five.

**The deploy-catch-up theory was ruled out rather than left open.** V4S is
pull-on-demand: one fetch per job per TTL, triggered by rendering an event,
with no timer and no backfill. There is nothing for a deploy to catch up, and
if it had ever worked it would have worked on the first page view. The
recurrence sweep that *does* need a catch-up is a separate mechanism, runs only
in the process holding the jobs lock, and reports that state at `/healthz` —
which is where to look if occurrences ever stop advancing.

### The deep link pointed at a list — 0.15h · app

Raised from the other side once availability started resolving: the link we
emit opened a list of shifts rather than the one somebody had clicked.

`ShiftSignupURL` built the URL from `jobId` and `volunteerShiftId` alone, which
does not narrow the page — V4S renders the job's whole listing and leaves the
volunteer to find their own day in it. Measured against the live listing: **31
shifts and 117 KB**, where the narrowed page returns **the single shift and
38 KB**.

`Calendar`, `nDaysToShow` and `dtMonthFilter` are what reduce it to one day.
The shape came from the partner's own links — an unpadded clock,
`2026-11-02 6:30:0` — and both that escaping and `url.QueryEscape` were checked
against the live page before picking the standard one.

The filter names a **local** day, so the shift's start is converted into the
organization's zone at the call site. A UTC instant would ask for the wrong day
for any shift within a few hours of midnight, which is the same daylight-saving
trap the date matching already avoids. A zero time falls back to the plain
listing: a page showing too much beats one showing the wrong day.

### The dev menu option that only existed in the other menu — 0.05h · app

Last round's notification previewer went into `run_menu`, the `--menu`
utilities mode, and nowhere else — so the menu you get after an ordinary
`dev-up.sh` run, the one carrying the admin and prank options, did not have it.
That is the mode where it is most useful, with the backend and the app already
up and the notification landing on a screen you can watch.

Added as `n` beside `a`/`p`/`c`. Nothing else moved: every helper it needs is
defined at the top level rather than inside the `MENU_MODE` branch, and
`APP_DB` is already set above that loop.

## What the fix was verified against

The live listing, not a fixture. Our Nov 2 occurrence resolves to shift
`a1RRi000002saGkMAI` — 8 confirmed, 2 available of 10 — and the URL built from
that match fetches back exactly one shift on exactly that date.

Done with throwaway tests run against the shipped functions and deleted in the
same command, which is the only way to exercise unexported matching code
without leaving a suite behind that this repo does not want.

**One finding to carry forward:** Oct 26, 27, 28 and 30 are absent from their
listing entirely, while Oct 29 parses fine from the identical URL shape — so it
is their data, not our parser or our window. From a public page there is no way
to tell "fully booked" from "no shift that day", so those dates show no numbers
rather than "full". That is the safe direction of the error, and it is the
existing deliberate choice, but it means a fully booked day looks blank instead
of closed.

## Totals

| Source | Hours | Basis |
|---|---|---|
| Round 1 — 11:00–16:00 | 5.00h | stated |
| Round 2 — 16:00–19:52 | 3.87h | stated (1.60h measured inside it) |
| Round 3 — 19:52–20:20 | 0.47h | **measured**, no unobserved part |
| **Total for Oct 26** | **9.3h** | |

The items above come to 0.40h against the 0.47h measured. The remaining 0.07h
is the three pushes, plus committing and pushing round 2's own scope document
at 19:54, which happened inside this window rather than before it. They are
allowed to add up here, where round 1 and round 2 said theirs were not
reconciled, for a reason that is not a change of policy:
this window is 28 minutes long and fully observed, so the split is read off
commit stamps rather than estimated across an unobserved stretch.

## Volume

| Repo | Files | Lines |
|---|---|---|
| app (round 3) | 4 | +35 / −5 |

**35 lines in half an hour, and the diff is the least informative thing about
it.** This folder's standing correction is about hours inflated from diff size;
this round is the same error available in the other direction. Reading the
nine-gate enrichment path, running the config matcher against production's own
organization names, and probing a third party's live page five times is where
the time went. A reviewer sizing this round by its diff would underreport it as
badly as the merchant-onboarding round overreported itself.

No migrations. Local database unchanged at 1.73.

## Open

- **None of this is on main.** Three commits sit on `pjol/mobile-2-1`; the
  one-line config correction is the only part production needs urgently, and it
  is a one-file cherry-pick.
- The key fix is necessary, not sufficient, for any *other* partner: the same
  mismatch is possible for every entry, and nothing warns about it. Cheapest
  guard is for `configureV4S` to read the organization names at boot and warn
  on any enabled entry matching none — it would have caught this on the first
  deploy, and it is offered but not built.
- A recurring successor copies `signup_url` verbatim, so the Nov 2 row still
  carries Oct 26's shift id and date filter. Harmless while enrichment runs,
  since the fetch is rebuilt from `jobId` and the display link is overwritten on
  a match — but if the config is off or the partner's site is down, the stored
  fallback sends volunteers to a date that has passed.

---

# Round 4 — Oct 27 morning

**0.4h measured.** One sitting, 10:42–11:07, from the session transcript's
message timestamps clustered on a 30-minute gap (`measure_sittings.py`). The
`v2.1` force-push and the PR #209 merge are stamped 11:14, seven minutes after
the last message in the sitting — GitHub's clock on a merge button, not working
time, so they are not added.

The release-plumbing round: get `v2.1` onto the right commit, get sanchez's
fixes into both repos, and change the website's partners banner.

## Features

### The v2.1 tag was pointing at the wrong commit — 0.15h · mobile-app

`v2.1` was on `7de861e`; the branch had moved four commits past it, including
the one that set `app.config.ts` to `2.1.0`. Moved to `92599c6` with a rewritten
annotation listing what it now includes and recording that it was moved.

A forced update to a published ref, so anyone who already fetched `v2.1` has to
re-fetch to see it move. That is the cost of the correction and it is the
smaller one.

### Sanchez's error fixes, merged into both repos — 0.15h · app + mobile-app

The app repo was missing the **backend half** of fixes already shipped in mobile
2.1 — the mobile side called endpoints the backend did not have. Merged
`origin/main` then `origin/sanchezo/bulletin-notifications-error-fixes`.

`bot.go` auto-merged and was checked by hand rather than trusted: it is the file
V4S was wired into, and an auto-merge in this stack has produced a silently
wrong result before. `Redeem` reverts and returns early only on a genuine send
failure, and falls through to the `"payout pending"` body otherwise.

**No migration risk**, verified rather than assumed: sanchez's branch touches
four Go files and its own scope doc, nothing under `bootstrap/`. Migrations
stayed at 73, strictly increasing to 1.73, no duplicates.

### Partners banner — 0.1h · webpage

Celo, Citizen Wallet, GLIDE, St. Anthony's, City Hope. INTN.CITY removed from
the banner, the footer nav, the links file and the assets folder.

Logos rasterised from the SVGs the organizations' own sites serve, so none were
drawn by hand. Two stored candidates were rejected: the webpage's existing
`st-anthonys.png` is a photo of a clothing program, not a logo, and the admin
panel's St. Anthony's PNG is the mark with no name beside it.

**The banner this changes is the fallback, not the one a deployment shows.**
`getPartners()` prefers `GET /partners`, and the admin `partners` table holds one
active row — Citizen Wallet — so a deployment reaching the API renders that
alone. Edited anyway, because it is what shows when the API is unset or empty,
but the five-partner banner needs the admin panel updated or cleared. That is
production data and was left alone.

## Totals

| Item | Hours | Repo |
|---|---|---|
| v2.1 tag moved | 0.15 | mobile-app |
| sanchez's fixes merged | 0.15 | app + mobile-app |
| Partners banner | 0.10 | webpage |
| | **0.40** | |

Items are apportioned across one observed 25-minute sitting rather than clocked
separately. The total is the measurement; the split is a reading of the order
the work happened in.

## Volume

| Repo | Files | Lines |
|---|---|---|
| webpage (banner) | 7 | +46 / −19 |
| app, mobile-app | merge commits only | — |

No migrations. No new routes.

---

# Round 5 — Oct 27 afternoon

**1.6h measured.** One sitting, 12:34–14:12, same method. The figure is to the
code commits at 14:11; writing this document is after it and is not counted.

The large round: Messages stops being a two-person text box.

## Features

### Group conversations — 0.30h · app

The 1.67 schema keyed a conversation on `(user_low, user_high, post_id)` with a
CHECK that the pair was ordered, so *a conversation* and *two people* were the
same statement. Membership already lived in its own table, which is what made
this a widening rather than a rewrite — the pair columns stay authoritative for
direct threads, and the unique index enforcing one-thread-per-pair becomes
partial so it stops applying to groups.

One owner always; admins the owner appoints; `left_at` rather than a delete, so
somebody who leaves keeps their messages in the thread and rejoining restores
their history. An admin cannot remove another admin — otherwise two admins can
remove each other and whoever taps first wins.

Verified against the live database rather than reasoned about: two groups with
null pairs both insert, an unordered direct pair is still refused, and a group
carrying a pair is refused.

### Attachments, references, and the link fetcher — 0.20h · app

One table for seven kinds. Photos and files as bytes; links with metadata
scraped once and then frozen, because a card that changed under a message nobody
edited would be a way to rewrite what was said; and references to a transaction,
a merchant, a volunteer event or a support ticket, resolved at read time against
data that may have moved or gone.

A transaction card deliberately does **not** carry the memo. A memo is what the
sender wrote for their own records; attaching a transaction is sharing the amount
and the parties, which are on a public chain anyway, not publishing a private
note about it. So the resolver reads the chain index, which has no memos, rather
than our transactions table, which does.

### The realtime socket — 0.15h · app

`GET /ws`, one per foregrounded client, fanned out by user id. Firebase is for
reaching a device nobody is looking at; this is for one somebody is. The division
is strict: while a client holds a socket the server sends it events and
**suppresses its pushes**, so a message never arrives as a system banner over the
chat it belongs to.

Never a source of truth — every event describes something already committed and
every screen can reach the same state with a GET, which is what lets it stay an
in-memory hub with no broker behind it. Its one real limit is stated at the top
of the package: the registry is in-process, so a second server replica would not
see events published on the first.

The token arrives in the socket's first frame. A header is what the rest of the
API uses but attaches inconsistently in React Native, and its failure shape — a
socket that connects and then silently receives nothing — is the worst one
available; a query parameter works everywhere and puts a bearer credential in the
one part of an HTTPS request that reliably reaches an access log.

Verified live: a silent socket is closed after exactly 10s with `1008 auth
required`, a bad token is refused immediately, and a typing frame sent before
authenticating is refused rather than acted on.

### Chat handlers, hydration and routing — 0.20h · app

Twenty-one new routes. Message hydration batches four queries across a whole
page — attachments, their references, reactions, and the quoted slice of
anything replied to — so a fifty-message page costs four queries regardless of
what is on it.

### Mobile: the chat screen and its composer — 0.40h · mobile-app

The thread was a pageSheet modal, which brought three problems that are all the
same problem — a modal is not in the navigation stack. It could not be swiped
back, the app's top bar stayed above it so the chat had two headers, and the
keyboard fought it.

The list is now `inverted`, which is the actual keyboard fix: index 0 is the
newest message, so the resting position is the bottom and whatever is
bottom-most stays pinned above the composer. The version it replaces was a
normal list with `onContentSizeChange -> scrollToEnd`, and the keyboard changes
the content size — so opening it fired a scroll that fought whatever the person
was reading.

The composer is one row that becomes one of three things: the text field, a grid
of attachment options, or an amount pad. Replacing rather than expanding is why
it works on a small phone.

### Mobile: the rest of the surfaces — 0.15h · mobile-app

My QR became My Profile; the profile prompt became a centred gate on first
opening Chats; contacts became cards with a detail screen; a new-message picker
that searches your own address book alongside the public directory; in-app
banners; the share sheet. `useDockLift` extracts sanchez's contacts measurement,
because the Chats tab grew a floating button of its own and two copies of
something that fiddly is two places for it to be fixed on one screen and not the
other.

### Review, and the three bugs it found — 0.13h · app + mobile-app

Worth itemising because none of them would have been found by the type checker
or the build:

1. **The unread badge would have stopped moving.** It was driven by the message
   push, and the socket now suppresses that push for exactly the devices the
   event reaches — so the count would have frozen for everybody with the app
   open. The socket's handler now moves it.
2. **Two authorization holes around leaving a group.** A membership row outlives
   the membership, so without `left_at IS NULL` somebody removed from a group
   kept reading its files and could still react to its messages.
3. **The link fetcher turned a pasted `mailto:` into an authenticated request.**
   `"mailto:a@b.com"` contains no `"://"`, so prepending `https://` produced
   `https://mailto:a@b.com` — a valid URL whose host is `b.com` and whose
   userinfo is `mailto:a`. On the one path in this backend that fetches a
   user-supplied URL. It now parses before defaulting the scheme and refuses
   userinfo outright.

### The DB file-storage inventory — 0.05h · app

`docs/DB_FILE_STORAGE.md`. Twelve tables hold files as bytes in Postgres and
this is the first time the set has been written down. Asked for explicitly, and
it is the checklist for the move to object storage — including the one entry
that should probably never move, `site_form_signatures`, which is evidence
rather than content.

## Totals

| Item | Hours | Repo |
|---|---|---|
| Mobile chat screen and composer | 0.40 | mobile-app |
| Group conversations | 0.30 | app |
| Attachments and references | 0.20 | app |
| Chat handlers, hydration, routes | 0.20 | app |
| The realtime socket | 0.15 | app |
| Mobile: the rest of the surfaces | 0.15 | mobile-app |
| Review and the three bugs it found | 0.13 | both |
| DB file-storage inventory | 0.05 | app |
| Survey and merges | 0.05 | both |
| | **1.63 → 1.6** | |

Items are apportioned across one observed 98-minute sitting rather than clocked
separately: there is one commit per repo, both at the end, so there are no
intermediate stamps to read the split off. **The total is the measurement; the
split is a reading of the order the work happened in** and should be treated as
the weaker number.

## Volume

| Repo | Files | Lines |
|---|---|---|
| app | 29 | +5,477 / −95 |
| mobile-app | 30 | +8,030 / −693 |

One migration (1.74), applied and verified locally. 21 new backend routes. One
new native dependency.

**13,500 lines, and the diff is misleading in both directions here.** Most of it
is new files rather than edited ones, which is the cheap kind of volume — but
this folder's standing correction cuts the other way too, and 1.6h for this
much is only possible because almost none of it was spent deciding what to
build. The request specified the behaviour in detail; the time went on the parts
that were not specified, which is where all three bugs above were found.

## Open

- **`expo-document-picker` is a new native dependency, so sending FILES needs
  the next binary.** It is required lazily inside a try, so the build in the
  store reports "needs a newer version" rather than crashing. Photos go through
  `expo-image-picker`, already in the binary, and work over the air. Worth
  knowing before anyone tests the file path on the current build.
- **Nothing is on main.** Migration 1.74 has run locally only.
- **The chat beta gates all of it.** Nobody sees any of this until
  `beta_feature_access` says so, and the socket is deliberately not gated —
  it is what carries the event saying access was granted.
- **Two profile editors now exist.** The Bulletin keeps its bottom-sheet
  `ProfileSetupSheet` for "you need a profile to post"; chat has the centred
  gate. Only the chat one was asked for, so the Bulletin's was left alone, but
  they should probably converge.
- **The realtime hub is single-process.** Stated in the package comment with the
  way out (a `LISTEN/NOTIFY` relay or Redis feeding `Publish`). Correct today
  because the backend runs as one process; the first thing to check when a
  second is added.
- **Opening a stored file in the app is not implemented.** The attachment card
  says so rather than doing nothing. Downloading needs a file-system dependency
  that is not in the binary either.

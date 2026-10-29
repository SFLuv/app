# Branch scope — `pjol/mobile-2-1`

Oct 26–29 2026 · app + mobile-app + webpage, and the board deck in animations · **22.7h** (11.7h stated: 9.3h for Oct 26 and 2.4h for Oct 28's presentation practice and board presentation; 11.0h measured for Oct 27–29)

| Round | When | Hours | Basis |
|---|---|---|---|
| 1–3 | Oct 26 | 9.3h | stated, with 6.8h measured as an upper bound |
| 4 | Oct 27 morning | ~~0.4h~~ → **1.3h** | **measured**, corrected in round 8 |
| 5 | Oct 27 afternoon | 1.6h | **measured** |
| 6 | Oct 27 afternoon–evening | 3.9h | **measured** |
| 7 | Oct 27 evening | ~~0.1h~~ → **0.2h** | **measured**, corrected in round 8 |
| 8 | Oct 28 morning | 0.1h | **measured** |
| 9 | Oct 28 late morning–midday | 2.0h | **measured** |
| 10 | Oct 28 afternoon | 1.0h | **measured** |
| 11 | Oct 28 late afternoon | 0.3h | **measured** |
| 12 | Oct 28 late afternoon–evening | 2.4h | **stated** — presentation practice and board presentation — with 0.07h measured |
| 13 | Oct 29 midday, with Oct 28's last six minutes | 0.5h | **measured** |
| | | **22.7h** | |

Rounds 4 through 11, and 13, are measured end to end, by clustering
**session-transcript message timestamps and commit timestamps together** on a
30-minute gap. Rounds 1–3 are not, and nor is most of round 12, which is a
presentation practice and a board presentation that no transcript can see. The
distinction matters more than the total: **11.7h of this figure is what
somebody said, and 11.0h is what a clock recorded.** Do not average them.

Rounds 4 to 7 originally clustered messages alone, which undercounted Oct 27 by
0.93h — see round 8. Commits are now part of the evidence set because work done
outside a Claude session leaves one and no messages at all.

Rounds 5 and 6 are one continuous sitting (12:34–18:05, 5.53h measured) split at
the point round 5 was written up. Round 6 is the remainder, appended rather than
folded in, so round 5's figure is never silently restated.

Oct 27's measured sittings are 1.34h (10:42–12:03), 5.53h (12:34–18:05) and
0.22h (19:02–19:15) — **7.09h**, which rounds to the 7.1h this document now
carries for the day. The rounded per-round rows add to 7.0h; where a measured
total and an itemised split disagree the measured total wins, and the 0.1h is
rounding in the rows rather than work that went missing.

With round 13 the rounded rows add to 22.6h against the 22.7h above. The
difference is the same kind of rounding in the rows — 0.09h on Oct 27 and 0.07h
on Oct 28, less the 0.03h that round 13 rounds up — not work that went missing.

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

**1.3h measured.** One sitting, 10:42–12:03, clustering message timestamps and
commit timestamps together on a 30-minute gap.

> **Corrected by round 8 from 0.4h (10:42–11:07).** As first written, this round
> saw the 11:14 PR #209 merge, called it "GitHub's clock on a merge button, not
> working time", and stopped there. Two mistakes in one judgement: a merge button
> is pressed by a person, and — decisively — the reasoning was applied to the one
> commit that had been looked at without checking whether there were others.
> There were. **11:20** merged webpage PR #1, and **11:45** is `3505dec`, 62
> lines of hand-written `ContactsScreen.tsx` with no session behind it at all.
> With those in, nothing in 10:42 → 11:07 → 11:14 → 11:20 → 11:45 → 12:03
> exceeds the 30-minute threshold, so it is one sitting of 1.34h. The threshold
> was never relaxed; the evidence set was too narrow.

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
| Contacts: dock cleared on Home-button iPhones, add sheet lifted over the keyboard (`3505dec`, +62/−5) | 0.30 | mobile-app |
| Unitemised remainder — three PR merges and whatever surrounded them, 11:07–12:03 | 0.64 | — |
| | **1.34** | |

Items are apportioned across one observed sitting rather than clocked
separately. The total is the measurement; the split is a reading of the order
the work happened in.

The unitemised row is the honest shape of this correction. 0.64h of the sitting
left commits but no transcript, so *that* it was worked is evidenced and *what
happened inside it* cannot be split further from here. Recorded as one row
rather than spread across the named items, which would dress a gap in the
evidence up as detail.

## Volume

| Repo | Files | Lines |
|---|---|---|
| webpage (banner) | 7 | +46 / −19 |
| mobile-app (contacts) | 1 | +62 / −5 |
| app | merge commits only | — |

No migrations. No new routes. The contacts row was missing until round 8 — the
same blind spot as the hours, since the commit it is in left no session behind
it.

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
new native dependency, and a version bump to 2.2.0 that the dependency requires.

**13,500 lines, and the diff is misleading in both directions here.** Most of it
is new files rather than edited ones, which is the cheap kind of volume — but
this folder's standing correction cuts the other way too, and 1.6h for this
much is only possible because almost none of it was spent deciding what to
build. The request specified the behaviour in detail; the time went on the parts
that were not specified, which is where all three bugs above were found.

## Open

- **This release is a fresh binary, version 2.2.0, and the bump is required.**
  `expo-document-picker` is a new native dependency. It was first wired up
  behind a lazy `require` so the 2.1 build could receive the rest of this work
  over the air; that was dropped once the decision was made to cut a new binary
  and not OTA 2.1, and it is now a plain static import.

  **What that decision exposed:** `runtimeVersion` is `policy: "appVersion"`, so
  the `version` string IS the over-the-air compatibility boundary. Had the
  version stayed at 2.1.0, an `eas update --channel production` would have been
  delivered to the 2.1 build in the store — which has no document picker, and
  `expo-document-picker` throws at import when its native side is absent, so
  those installs would have stopped booting. The bump to 2.2.0 is what prevents
  that, not tidiness.

  The comment above `runtimeVersion` claimed the *fingerprint* policy was in use
  and that "an update can never reach a build whose native side it does not
  match". It was not, and it cannot. The comment is corrected and
  `policy: "fingerprint"` is offered — it would make the guarantee structural
  rather than a matter of remembering to bump a string — but not taken, because
  it changes which builds a hotfix can reach and that is a release decision.
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

# Round 6 — Oct 27 afternoon and evening

**3.9h measured.** 14:12–18:05, the rest of the sitting round 5 cut at 14:12,
from the session transcript's message timestamps clustered on a 30-minute gap
(`measure_sittings.py`). The script reports that sitting as a single 5.53h span;
round 5 claimed 1.63h of it, and this is the remainder.

> **Revised by round 7 from 3.8h, and the reason is worth keeping.** This round
> was written up while its own sitting was still open, so the last message the
> script could see was the one before the write-up: 18:00, giving 5.45h for the
> sitting and 3.8h for the remainder. The sitting's real last message is 18:05,
> the commit that carried this section. The correction is to where the
> measurement stopped, not to the work — same sitting, five more minutes of it
> visible. **A round cannot measure its own end while it is still running**, and
> the fix is to re-run the script after the round's commits land.

Two things in the figure are named rather than buried, because both are time
that did not go into features:

- The first ~0.2h is writing up rounds 4 and 5 and the document-picker change.
  Round 5 explicitly excluded its own write-up from its figure, so counting it
  here is what stops it falling out of the total altogether.
- ~0.1h went to a full disk. The machine hit 100% on the data volume mid-round
  and `Bash` stopped working entirely — no `git`, no `tsc`, no `go build` —
  until legacy dev caches were cleared. Diagnosis and clearing, not product
  work.

Writing *this* section and committing the round are inside the span, not after
it: the write-up ran up to the 18:05 commits, which is where the sitting ends.
As first written this said the time was after 18:00 and uncounted. It is
counted, and it is most of the five minutes round 7 added.

The polish round: Messages goes from working to finished, and then the bug
reports start coming back from the device rather than from reading the code.

## Features

### The chat flicker, the keyboard, the frozen share, and a real share sheet — 0.9h · mobile-app

Four reports from one pass on a device, and the first is the one worth recording.

The chat refreshed itself every few seconds, flickering and throwing away
whatever the person was doing — out of the amount pad, out of a half-typed
message. The cause was a dependency chain rather than a timer: `refresh`
depended on the `onRead` and `onToast` callbacks, the mount effect depended on
`refresh`, and the shell passed those callbacks as inline arrows. So every
render of the shell — a balance poll, a socket frame, anything — gave them new
identities, which re-ran the mount effect, which called `setMessages([])`,
`setMode("text")` and `setDraft("")`. The screen was resetting itself several
times a minute and the flicker was the list being emptied and refilled.

Fixed by holding those callbacks in refs inside the screen, so `refresh` depends
only on what changes *what is fetched* — the client and the thread — and the
screen is immune to its parent re-rendering. The comment on those refs says so,
because the obvious "tidy-up" is to list them in the dependency array again.

"Send to chat" froze the whole app, every tab. A stacked `Modal` on iOS: the
share sheet asked the shell to open a transaction sheet behind it with a double
`requestAnimationFrame` (~32ms) while its own dismissal took 220ms, so the
second modal was presented into a window that was still going away and the app
kept a modal that could not be reached or dismissed. Now the open is fired where
`setMounted(false)` happens rather than on a guessed delay.

The share buttons themselves became a whitelabelled sheet — internal chats, copy
link, and the native share sheet — rather than a "Send to chat" button at the
bottom of each card. Copy and external share stay available without the chat
beta, because a link to a merchant is something the website publishes to
everybody and gating it behind messaging was wrong.

### Message management, timestamps, and payment versus record — 0.5h · app + mobile-app

Edit within five minutes, delete for everyone, multi-select delete, and
"Message was deleted" tombstones — schema 1.75. The edit window is enforced in
the `WHERE` clause rather than checked first, so a message cannot be edited by a
request that arrives while the check is passing.

`deleted_by` is what makes a tombstone and an absence different things: a
sender's deletion shows as "Message was deleted", because a reply to it would
otherwise be a non-sequitur and the thread would quietly look like it was always
that way, while an **admin** removal does not appear at all — a gap that
announces itself tells the whole thread something was actioned. Bodies are kept
on disk and blanked in `scanMessage`, so moderation can still see what was said.

iMessage's timestamps: centred separators for a real jump between sessions, and
swipe-left-and-hold to reveal a time per message. The whole list slides as one
so the times line up in a column.

A payment made in the chat and a transaction record shared out of history looked
nearly identical, which was the problem — "I paid you 5 SFLUV just now" and
"look at this transfer from March" are not the same claim and a reader should not
have to work out which one they are looking at. The record gets a receipt icon,
a title and the amount demoted to a subtitle; the payment gets the amount and
nothing else.

### Swipe actions, muting, leaving, read receipts, and saving files — 0.6h · app + mobile-app

Schema 1.76. Swipe a conversation for delete / leave / mute, with the icon
changing to a leave glyph for a group, and the modal offering "leave" or "delete
messages and leave" rather than the direct-thread choices.

All deletions are soft. `conversations.deleted_at` hides a thread from one
person and nothing else: the messages stay, and another message repopulates the
history — which is what "delete for me" means to the person doing it.

Muting is `muted_until`, and "forever" is a hundred years rather than a null,
so one comparison answers the question everywhere. A muted thread still arrives,
still shows unread and still counts toward the badge; muting silences the phone,
it does not hide the conversation.

Read state across a group is `MIN(last_read_message_id)` of the others, not
`MAX`: "Read" has to mean everybody, or it means whoever happened to open it
first.

Files download and photos save to the camera roll, with an in-app preview and
type-specific icons in the file search.

### Attachments staged rather than sent, and emoji-only messages — 0.4h · mobile-app

Picking a photo used to post it immediately, so there was no moment at which you
could say anything about it: the picture arrived, then a second message
explaining it. Now attachments stage in the composer with the draft split at the
caret, so text before the attachment is its own message before it and text after
is its own message after — iMessage's behaviour.

The link attachment mode was removed entirely in favour of detecting a URL as it
is typed or pasted. A message that is *only* a link renders as the card alone; a
link inside a sentence keeps the sentence as a normal bubble with the card under
it.

Emoji-only messages under five graphemes render large and without a bubble,
counted with `Intl.Segmenter` so a family emoji is one character rather than
seven.

### Attachments: the upload that failed and the thumbnails that blocked the thread — 0.5h · app + mobile-app

Reported as "file attachments are not being sent correctly, and other items get
stuck loading". Two unrelated causes, both measured rather than guessed at.

The upload shared the API's 25-second timeout. The limit on an attachment is
25 MB and up to ten at a time, which over a phone's uplink is minutes — so the
client aborted mid-stream while the server often had the file already. The send
reported failure, and the message turned up anyway. Multipart uploads now get
their own 240-second ceiling.

The stuck loading was the thumbnails. `getConversationAttachmentDataUri` pulls
an image through `fetch` and base64-encodes it **on the JS thread** — for a
multi-megabyte photo that is a third again as much string, built synchronously,
per image. Several photos in a thread serialise behind each other and everything
else sits on its placeholder waiting for a turn. Thumbnails and avatars now use
`<Image source={{uri, headers}}>`, so the fetch, the decode and the caching all
happen natively. The data-URI reader stays for the case where the *bytes* are
what is wanted — writing a file to disk to save or share it.

Files were already preview-only with a manual download, which is the behaviour
asked for; what they lacked was a readable name. The staged chip was a 60pt
square with the filename at 8pt on one line, which for a real document name
rendered as an ellipsis. A file has no thumbnail, so the name *is* the preview.

### The in-app previewer learns to read documents — 0.3h · mobile-app

It previously said "save it to open in another app" for anything that was not a
photo. Now PDFs render in a `WebView` over the downloaded file — WKWebView has a
PDF renderer, so on iOS that is pages, scrolling and pinch-to-zoom — and text,
CSV and calendar files render as text in a monospaced scroll.

Android keeps the card for PDFs, deliberately: its WebView has no PDF renderer,
and a blank page that looks broken is worse than a sentence saying where to open
it. Word, Excel and PowerPoint keep it on both platforms, because those
genuinely need a converter.

### In-app banners: behind the pill, and firing several times for one notification — 0.3h · mobile-app

The position was a React Native layout quirk worth writing down. The banner was
an absolutely positioned child of the root `SafeAreaView` with a small top
offset. RN runs Yoga with the legacy absolute-positioning errata, under which an
absolute child is placed against its parent's **border** box and the parent's
padding is ignored — and core `SafeAreaView` applies the safe-area inset *as
padding*. So the inset did nothing for this child and `top: 6` meant six points
from the physical top of the screen. On a notched phone the cutout is narrow and
the card's own side margins hid it; on a Dynamic Island, which is centred and
wide, the card landed squarely behind it. The banner is now a child of the top
bar at `top: 0`, which is correct on every device without knowing any insets.

The retriggering was three causes, none of them a duplicate notification:

1. The shell passed `onDismiss` as an inline arrow, so `close` churned identity
   every render, so the effect that has to depend on `close` re-ran on every
   render of the whole app — replaying the entry animation and clearing the
   4.2-second timer a few milliseconds before it fired. One banner
   re-animating, and never leaving on its own.
2. Notification banners were keyed `notification:${Date.now()}`, unique by
   construction, which defeated any dedupe.
3. `setNotificationHandler` returned `shouldShowBanner: true`, so a push
   arriving in the foreground drew a **system** banner on top of the in-app
   one. Push suppression is a server-side snapshot taken before the send, so a
   device that connects in the gap gets both.

The system banner is now suppressed only while the socket is live — when it is
down the OS still draws it, so nothing is silently dropped — and `message_id`
was added to the message push so the socket and push paths dedupe on one key.

### Chats opened from a notification could load forever — 0.2h · mobile-app

The reporter guessed the profile had not loaded. It was the backend client. A
notification cold start mounts the chat before sign-in finishes, so `refresh`
returned without fetching; the reset effect was keyed on the thread alone —
deliberately, to stop a parent re-render wiping the composer — so nothing re-ran
when the client arrived. The fifteen-second poll eventually rescued it, which is
why it looked intermittent rather than broken. Fetching is now its own effect
keyed on `refresh`, which resets nothing.

### In-chat payments are previewed before they are spent — 0.2h · mobile-app

The amount pad paid straight from its own button, which made a transfer the one
thing in the composer with no review step. It now stages: the pad composes,
a chip shows the amount with SFLuv's own mark, and the composer's send button is
what spends. The tray refuses to open the pad while a file is staged, rather
than allowing it and then discarding one of the two — a payment is one message
and cannot carry a photo.

The green `logo-usd` is gone with it. This is not dollars, and a dollar sign on a
payment of a local currency names the wrong unit.

### Group messages show who sent them — 0.2h · mobile-app

Telegram's arrangement: a small picture beside the bubble, on the last message
of a run from one person, bottom-aligned, with the rest of the run indented to
match. The name still appears once at the top of the run; the picture is what
makes a busy group scannable without reading any names.

Initials are a complete fallback, and also what happens for somebody with no
picture — the avatar route answers 404 for them, so the image errors and the
initials take over. Asking the server first who has a picture would be a request
per member to avoid a 404 per member.

## Smaller fixes

| Fix | Hours | Repo |
|---|---|---|
| Round 5's write-up and the document picker built into the binary at 2.2.0 | 0.2h | app + mobile-app |
| Partner banner at half speed; Telegram step and email fallback dropped from the contact form | 0.2h | webpage |
| Reaction previews in the inbox ("Sanchez reacted to a message") and a `/link-preview` endpoint | 0.2h | app |
| Swipe tray showing through the row permanently, timestamp overlapping the mute button, tap-to-close that never appeared (open-ness was a ref, which does not re-render), icons invisible in dark mode (`palette.navy` inverts) | 0.2h | mobile-app |
| Copying an internal reference now copies its universal link rather than a bare id or hash | 0.1h | mobile-app |
| The info button dismisses the keyboard on its way out | 0.05h | mobile-app |
| Typing indicator taken out of the inverted list, where a cell inserted at the same edge overlapped it for a frame | 0.05h | mobile-app |
| Sending bar moved onto the header's bottom edge and given a completion run-out, so a fast send is seen to finish rather than cut off | 0.1h | mobile-app |
| A full disk — `Bash` unusable until legacy dev caches were cleared | 0.1h | — |

## Totals

| | Hours |
|---|---|
| Features | 4.1h |
| Smaller fixes | 1.0h |
| **Itemised** | **5.1h** |
| **Measured** | **3.9h** |

The itemised split overshoots the measurement by 1.2h, and the measurement
wins. Recording both rather than scaling the items down to fit: the overshoot is
itself the finding — ten features and nine fixes in 3.9h means most of these
items took single-digit minutes, and rounding each to a plausible-looking 0.1
or 0.2 inflates the sum by more than a third. The same mistake as the 0.5h floor
in round 2, at a smaller scale.

## Volume

| Repo | Files | Insertions | Deletions |
|---|---|---|---|
| app | 15 | 1,492 | 31 |
| mobile-app | 27 | 5,099 | 700 |
| webpage | 6 | 130 | 139 |
| **Total** | **48** | **6,721** | **870** |

2 migrations (1.75 message management, 1.76 conversation actions) · 6 new routes
· schema at 1.76.

580 of app's insertions are this document. The code is 14 files, +912/−29.
Counted as 1,210 when first written, for the same reason the hours were: the
document was still being written when its own volume was measured. Round 7's
additions to the document are deliberately *not* folded back in — a document
that counts itself never converges — so **+912/−29 is the figure to quote** and
the row above is the code plus the document as it stood at the end of round 6.

## Known gaps

- **Android cannot preview a PDF in the app.** Its WebView has no PDF renderer.
  The card and the share sheet are the honest answer there, and the platform
  check that chooses between them says why.
- **A transaction shared by a third party has no addresses to label** when the
  viewer has no history row for it and the attachment carries none. The sheet
  says "Unknown" for that side rather than showing a blank.
- **The in-app banner replaces rather than queues.** Five messages at once show
  the last one; a queue would mean twenty-one seconds of banners, and the last
  is the only one still worth reading by then.

---

# Round 7 — Oct 27 evening

**0.1h measured.** 19:02–19:10, a sitting of its own — the 57-minute gap after
round 6's 18:05 commit closes that round under the 30-minute rule. No code
changed. This is the bookkeeping pass: re-running `measure_sittings.py` once
round 6's commits had landed, and correcting the three things round 6 could not
see about itself.

| Fix | Hours | Repo |
|---|---|---|
| Re-measured round 6 after its own commits landed: 3.8h → 3.9h, sitting 5.45h → 5.53h, Oct 27 5.8h → 6.1h, branch 15.1h → 15.4h | 0.05h | app |
| app volume corrected, 1,210 → 1,492 insertions, with the document's own 580 named separately from the 912 lines of code | 0.05h | app |
| Round 6's "writing this section is not counted" withdrawn — it is inside the span | — | app |

## What this round is for

Three figures in round 6 were measured from inside the round they described, and
all three were short by the same mechanism: **a round cannot see its own tail.**
The hours stopped at the last message before the write-up; the volume counted the
document while it was half written. Neither is a judgement call that went wrong,
and nothing was inflated — both errors ran downward — which is exactly why they
would have survived a reading. Re-running the measurement after the commit is
what catches them, and that is the practice this round adds.

Recorded as its own round rather than folded into round 6, per the convention
that an earlier figure in this folder is never silently restated. Round 6 keeps
the superseded figure with a note pointing here.

And this round has the blind spot it is describing. 0.14h is the span up to the
measurement; the commit carrying this section falls after it, so the next run of
the script will show this sitting slightly longer. Left at 0.1h rather than
chased: the correction would be bookkeeping about bookkeeping, and it is written
down here so it is not discovered a third time as though it were news.

> **It was exactly that, and round 8 recorded it: 0.22h, so 0.2h.** The sitting
> ran to 19:15, the commit included. The prediction was right and the decision
> not to chase it was wrong by 0.1h — cheap to have taken, since re-measuring
> after the commit is one command.

## Verified, not assumed

- `app`, `mobile-app` and `webpage`: `git status` clean, nothing unpushed, all
  three on `pjol/mobile-2-1`.
- Volume re-derived from `git diff --shortstat` across the round's real
  boundaries (`bb83e8b..HEAD`, `f13650f^..HEAD`, `51f4961..HEAD`) rather than
  carried forward. `mobile-app` (27 files, +5,099/−700) and `webpage` (6 files,
  +130/−139) came back unchanged; only `app` had moved.
- The 48 files / +6,721 / −870 total is the sum of the three re-derived rows.

## Known gap

**Oct 26 and Oct 27 are not in an invoice yet.**
`branch-scopes/report-2026-10-26.csv` covers the service period 2026-10-19 to
2026-10-24 and is closed. This branch's hours — **16.5h**, 9.3h stated for Oct 26
and 7.2h measured for Oct 27–28, revised by round 8 from the 15.4h this round
first wrote — belong to the next period, whose end date is the contractor's to
set, so no CSV was written for it here. The figures are ready
for one: the rounds table at the top of this document is the itemisation, and
the stated and measured halves are already separated the way that report's
METHOD and WEAKNESSES rows need them.

---

# Round 8 — Oct 28 morning

**0.1h measured.** 11:15–11:23, messages and commits clustered on a 30-minute gap.
Prompted by the contractor's read of the Oct 27 figure — "i feel like yesterday
should be at least 7 hours total" — which was right, and for a reason no amount
of re-reading the document would have surfaced.

## The error: a narrow evidence set, not a lenient threshold

The question asked was how lenient the session definition was being. It was not
being lenient at all, and that was never the lever:

| Evidence clustered (30-min gap) | Oct 27 |
|---|---|
| session messages only — what rounds 4–7 used | 6.15h |
| messages **+ commits** | **7.09h** |
| messages + commits + source-file mtimes | 7.09h |

The gap threshold stayed at the skill's default 30 minutes throughout. What
changed is *what counts as evidence that somebody was working.* A Claude session
transcript only sees work done **through Claude**. Merging a PR in a browser and
hand-writing a commit in an editor leave no messages, so three activity points
in the 11:07–12:03 window were invisible: the 11:14 and 11:20 PR merges, and
`3505dec` at 11:45 — 62 hand-written lines of `ContactsScreen.tsx`, a feature
that was missing from this document as well as from its hours.

Chain those in and the morning has no gap over 30 minutes, so it is one sitting
of 1.34h rather than 0.40h. **+0.93h, all of it in round 4.**

Source-file mtimes were added as a third tier and moved nothing — every one of
them already falls inside a span. They stay in the method as a check that came
back clean rather than as a contributor.

## Why the threshold is not where the slack was

Tested, because "be more lenient" was the obvious thing to try and it is the
wrong answer:

| Gap | Oct 27 | Verdict |
|---|---|---|
| 30 min | 7.09h | **kept** |
| 45 min | 7.60h | merges a 31-minute gap at 12:03–12:34 |
| 60 min | 8.53h | swallows two ~56-minute breaks with no activity of any kind |

The 12:03–12:34 gap is **31.0 minutes** — one minute over the threshold, and
0.52h turns on that minute. It stays split, because the rule has to be decided
before the number is known or it is not a rule. Worth naming as the most fragile
boundary in the branch: every other gap on Oct 27 is either under 25 minutes or
over 55.

## Oct 26 moves too, and changes nothing

Same method on Oct 26: 6.17h messages-only → **6.76h** with commits. Still well
under the 9.3h the contractor stated for that day, so the stated figure stands
and only the upper-bound note in the header changes (6.2h → 6.8h). The gap
between 6.8h measured and 9.3h stated is the dev meeting and the windows away
from the keyboard that rounds 1–3 already name.

## What this costs the folder

**Every measured figure in `branch-scopes/` was taken from transcripts alone**,
including the other files in this directory and the closed
`report-2026-10-26.csv`. All of them are therefore low by whatever share of the
work was done outside a Claude session, which for a day of merges and hand-edits
is not negligible. This branch is now measured the wider way; the others are not
re-measured here, and that inconsistency is named rather than left to be found.

The `time-accounting` skill at <https://github.com/pjol/SKILLS> ships
`measure_sittings.py`, which reads transcripts only. Teaching it to union commit
timestamps from the repositories it is pointed at would fix this at the source
for every future branch, and is the obvious next change to that repo.

## Corrections made

| Figure | Was | Now |
|---|---|---|
| Round 4 (Oct 27 morning) | 0.4h | **1.3h** |
| Round 7 (Oct 27 evening) | 0.1h | **0.2h** |
| Oct 27 measured | 6.1h | **7.1h** |
| Oct 26 measured upper bound | 6.2h | **6.8h** |
| Branch total | 15.4h | **16.5h** |
| Round 4 volume | banner only | **+ contacts, 1 file, +62/−5** |

Round 4 and round 7 keep their superseded figures with notes pointing here.

## The pattern across rounds 6, 7 and 8

Three rounds in a row have corrected a measurement, and the errors are not
independent:

- **Round 6** stopped the clock before its own write-up had happened.
- **Round 7** caught that, then left its own tail uncounted on the grounds that
  chasing it was not worth 0.1h.
- **Round 8** found 0.93h that neither had looked for, because both were reading
  the same single source.

Every one ran **downward**. None was a judgement call that went the generous way,
which is the useful thing to notice: this folder's standing worry is inflation
— the 4.7x overstatement at the top of `merchant-onboarding-revamp.md` — and the
guard against it has been working. Undercounting has no such guard, and it is
invisible precisely because it looks rigorous. The contractor's own sense of the
day was the only thing that caught it, and a figure that survives only because
nobody intuited otherwise is not measured, it is unchallenged.

**The fix is procedural, not arithmetic: measure after the commits land, and
cluster commits alongside messages.** Both are one command.

## Measurement note

0.1h is the span to 11:23, the last evidence point before this section was
committed. The commit itself lands a minute or two later and is not in the
figure — bounded, named, and not chased, which is round 7's resolution applied
on purpose rather than by oversight.

---

# Round 9 — Oct 28, late morning to midday

**2.0h measured.** 11:23–13:23, messages and commits clustered on a 30-minute
gap. One sitting with round 8's (2.13h from 11:15), split at round 8's commit
the way round 6 was split from round 5, so round 8's figure is not restated.
Measured to the last evidence point before this section was committed.

Four subagents ran inside this sitting — a boot audit, a structural audit,
research on the two native features, and an independent review of the diff —
in parallel with the main thread. They are inside the measured span, not added
to it: the clock does not run four times because four things were reading.

The second request this branch has taken from somebody using the app rather
than reading it, and most of it is the same disease in different places: things
that unmounted when they should have hidden, state replaced when nothing had
changed, and screens that waited for the slowest thing in boot before drawing
what they already knew.

## Features

### Structural pass, and painting on cache at boot — 0.36h · mobile-app

Two read-only audits of the shell (10,344 lines, 129 `useState`, 80 `useEffect`)
were run before anything was changed, and every claim was checked against the
code before it was acted on. What was fixed, each verified first:

- **The whole app re-rendered every two seconds** on Wallet or Activity —
  under an open chat too. `mergeTransactions` returned a new array, with every
  row rebuilt, on every poll whether or not anything had moved. It now returns
  the array it was given when nothing differs, and React skips the render.
- **The wallet bootstrap re-ran on every foreground and every send**, and at
  least twice on every cold start. Its effect depended on the hidden-wallets
  ARRAY, which `syncWalletPreferences` replaced with an equal copy each call —
  and the bootstrap calls it itself. Each re-run flipped the runtime to
  loading, fetched the profile and the Privy provider again, re-ran wallet
  discovery and set a new runtime object that re-fired everything keyed on it.
  Equal values now keep the same object, and the effect keys on the Privy id
  rather than the Privy user object.
- **The W-9 modal flapped on a network blip**: a failed status poll set the
  status to null, which closed the modal until the next good poll. A failure now
  keeps the last good answer, and W-9 and merchant-mode status are replaced only
  when they change — they had been re-rendering the shell every 8 and 15 seconds.
- **The banner replayed, and the inbox jumped.** Both had one cause: the top bar
  unmounted while a chat was open. The in-app banner lived in it, so one showing
  when a chat opened replayed when it closed, and one for another conversation
  that arrived mid-chat waited until you left. And the content area grew upward
  by the bar's height in the same commit the chat began sliding in, so the inbox
  jumped beside it. The top bar and content now sit in one wrapper that the chat
  overlays; the bar stays mounted, inert and hidden from VoiceOver under the
  chat; the banner hangs from a zero-height anchor in normal flow, which keeps it
  below the Dynamic Island without a safe-area library.
- **The dock popped in late** after Send or a chat, and everything lifted above
  it jumped: it unmounted, re-measured on return, and reported no top edge while
  gone. It is hidden now, not removed.
- **Tab changes flickered**: the slide's start values were set in an effect after
  the new tab had painted. Now a layout effect. Activity and Notifications also
  stop animating their own entrance on top of the tab's.
- **Two more stacked-Modal races** of the kind that froze the app before — the
  reaction picker opened as the action sheet closed, and the OS share sheet
  opened one frame after ours — now wait for `onDismiss` on iOS.
- `showToast` is stable, so effects keyed on `onToast` stop re-running with
  every shell render; history polls pause while a chat covers the screen; and
  the composer follows the keyboard on its own curve instead of jumping.

Boot now paints the person's own app on the first frame. The tabs, the dock and
the bell had all waited for `appUser`, which arrived only after the whole wallet
bootstrap and a second profile fetch. A per-user profile cache seeds it, the
wallet's first page of history and the public merchant locations are cached
beside the balance, and the bell's existing loading spinner — never visible on a
cold start, because it hid while routing was pending — is now what says the
background work is still going. Every cache is keyed to who is signed in and
cleared on sign-out.

### Native: share to SFLuv, and chat pushes with the sender's face — 0.22h · app + mobile-app

**Share to SFLuv.** `expo-share-intent@5.1.1`, the last release for SDK 54,
patched in three places: a separate target name, because by default the share
sheet label is also the Xcode target name and "SFLuv" collides with the app's
own target and is silently skipped; and two Android bugs in that version — a
path traversal through the shared file's display name, and a crash on a provider
that returns no cursor. Shares open the existing share sheet, so sharing in and
sharing out are one surface; the shared copy is deleted when the sheet closes.

**Chat pushes as communication notifications.** `expo-notifications` cannot do
these, so a Notification Service Extension in Swift
(`targets/notification-service/`, built in by `@bacons/apple-targets@4.0.7`)
builds an `INSendMessageIntent` with the sender's picture, or the group's, and
initials drawn on the app's colours when there is none. The extension has no
session, so pushes carry signed, expiring picture URLs served by a new
unauthenticated `GET /push-image`; a group's is signed for the recipient and
re-checked against their membership when fetched.

Version 2.3.0: both add native code, and with `runtimeVersion: appVersion` an
over-the-air update carrying the share module to a 2.2 binary would fail at
import. Prebuild was run in a scratch copy for both platforms and generates both
targets with the right bundle ids, entitlements and display name. **Neither has
been compiled** — that needs an EAS build, and the first iOS build must be run
interactively so EAS can register the two new extension identifiers.

### One wording for push and banner, and pictures everywhere — 0.14h · app + mobile-app

Telegram's shape: a group titled with the group and the line naming who spoke,
"Maria: 📷 Photo"; a direct thread titled with the person. The server builds it
once and sends it as the push and as a `notice` on the socket event, so the two
cannot drift — they had: photo-with-caption pushed as only the caption, a file
pushed as "Sent a file", and every in-app banner for a direct message titled
"New message", because the sender's name was only ever filled in for groups.
VoiceOver reads "Maria said: hello". Banners, inbox rows, the chat header and
the member list show the person's or group's picture over their initials.

### Caching: chat text on disk, attachment bytes on demand — 0.11h · mobile-app

The inbox and the newest 120 messages of the 30 most recent threads are kept per
user, and draw on the first frame — no spinner opening Messages or a thread read
before. Attachment bytes are fetched natively into a cache when asked for, and
shown from disk after: photos up to 8 MB fetch themselves, anything else shows a
"not downloaded" card with its size and a tap, and a file says when it is on the
device. Your own sent photos are put in the cache at send time.

### Server-side ordering — 0.08h · app

`last_message_at` could move backwards: it is the inserting transaction's start
time, and two overlapping sends commit in either order. It is now written with
`GREATEST`, the inbox sort gains an id tiebreak, and each recipient's pushes go
through a FIFO instead of racing as separate goroutines. Clients insert messages
in id order rather than appending, so every copy of a thread agrees.

### Links as text, rendered by the client — 0.08h · app + mobile-app

No more card above the composer asking whether to send a link. What was typed is
what is sent; every client linkifies it and draws the card under the bubble,
from a cached lookup — the server caches scrapes for twenty minutes so a link in
a busy group is fetched once.

## The independent review, and what it caught

Five read-only reviewers ran in parallel over the uncommitted diff, one slice
each — the shell, the chat surfaces, the cache and link modules, the native
extension and config, the backend — given what each change was meant to do and
none of the reasoning behind it. The native reviewer type-checked the Swift with
`swiftc` against the iOS 18.2 SDK and dry-ran both patches; the module reviewer
fuzzed the link parser with half a million inputs. Every finding was checked
against the code before it was acted on. Fixed:

- **A merchant till opened as the owner's personal app on every cold start.**
  The profile cache put `appUser` on screen before the bootstrap, and nothing
  cached whether this device is a till — so for the whole boot a locked counter
  showed chats, contacts, history and, once the wallet was up, Send. Routing now
  waits for an owner's merchant-mode answer, which is asked for as soon as the
  profile appears. Sharing into SFLuv is refused on a till outright.
- **Cached threads left permanent holes.** The first page after opening kept
  everything older from the cache, so a thread that had moved on by more than a
  page showed the cached tail, a gap, then the page — and paginated backwards
  past the gap forever, saving it back to disk. The first page of a visit is now
  authoritative; messages sent or delivered while it was in flight survive it
  (a long-standing bug that made a just-sent message vanish until the next poll).
- **On Android a half-finished download counted as downloaded**, forever: the
  platform writes straight to the destination. Downloads now land in a `.part`
  file and move into place when complete.
- **Links went to the wrong site.** `bbc.co.uk` linked to `bbc.co`, `música.com`
  to `sica.com`; the half of an email address before the @ became a link.
- Smaller: link taps skipped double-tap-to-like; one avatar error hid that
  person's picture for the session; a stale thread's 404 could close the thread
  just opened; shared-file clean-up could, on Android 9 and below, delete the
  person's original rather than SFLuv's copy; link previews and other users'
  threads accumulated on disk without bound; several sign-out paths bypassed the
  cache clears (the clears now run whenever the signed-in person changes);
  muted threads still popped in-app banners; the server's link cache did not
  merge simultaneous requests and cached one client's disconnect as a failure;
  signed picture URLs were written to the access log; the read pointer had the
  same backwards-write race as `last_message_at`; the extension decoded avatars
  at full size against a 24 MB memory limit, and could deliver twice at the
  deadline.

## Smaller fixes

| Fix | Hours | Repo |
|---|---|---|
| Sanchez's latest merged (the app branch already carried the Sentry PR squashed, so it was the only one taken; mobile was a fast-forward); his new packages installed | 0.17h | app + mobile-app |
| Timestamps visible at rest on an iPhone 16 — parked against the row, not the screen, so the list padding left the leading digit showing | 0.06h | mobile-app |
| The inbox preview one or two messages behind — out-of-order responses from overlapping reads, the later one winning; the thread had the same race and could drop messages | 0.06h | mobile-app |
| Paste: photos, links and text, through the system's paste button where iOS has one | 0.05h | mobile-app |
| A staged payment silently dropped when a photo was staged after it | inside the above | mobile-app |

## Totals

| | Hours |
|---|---|
| Features | 0.99h |
| Smaller fixes | 0.34h |
| **Itemised** | **1.33h** |
| The independent review, its fixes, this write-up and the commit | 0.68h |
| **Measured** | **2.0h** |

Itemised from the first appearance of each piece of work in the session
transcript — the structural pass includes writing the three audit briefs that
started it — so the split is a reading of when things began rather than a
stopwatch on each. The measured total is the figure.

## Volume

| Repo | Files | Insertions | Deletions |
|---|---|---|---|
| app | 15 | 748 | 78 |
| mobile-app | 32 | 4368 | 1149 |
| **Total** | **47** | **5116** | **1227** |

No migrations. One new route (`GET /push-image`). Four packages
(`expo-share-intent@5.1.1`, `@bacons/apple-targets@4.0.7`,
`@expo/prebuild-config@54.0.8`, `patch-package`) and two patches.

## Needs a build, then a device

Everything in this round type-checks and the backend builds and vets, but the
native half has only been prebuilt, never compiled, and none of it has run:

- **Share to SFLuv** on both platforms, cold start and warm. Android cold-start
  shares are documented not to arrive in dev-client builds — test a release build.
- **The notification extension**: a direct message and a group, with and without
  a picture. One open question for a device: whether iOS adds the sender's name
  to a group's line itself, in which case "Maria: " appears twice.
- **The shell restructure**: open and close chats from the inbox and from a
  banner; Send; the dock; the banner over a chat header.
- **Boot from cache** as a returning user, and as somebody new on the same phone.

## Known gaps

- **Left from the review, named rather than fixed:** expo-share-intent 5.1.1's
  own Android crash paths for video metadata (upstream, unchanged here); its
  plugin replaces rather than merges the app's app-group entitlement, harmless
  until a second group is added; both extensions build at version 1 unless EAS
  remote versioning writes theirs too — a warning from App Store Connect, not a
  rejection; and pasting replaces nothing when text is selected.
- **Android pushes keep the app icon.** The equivalent there is a
  MessagingStyle notification built by a native FCM service; `expo-notifications`
  has no hook for it.
- **Files cannot be pasted**, only photos, links and text — the clipboard API
  exposes nothing else. Sharing to SFLuv is the route for a document.
- **Still to do from the audits:** the dock bubble animates layout on the JS
  thread; SendScreen's keyboard offsets are wrong; screens are not memoised at
  the shell boundary; and the shell's data still lives in one component rather
  than in stores. Each is real, and each is a larger change than this round
  should make blind.

---

# Round 10 — Oct 28, afternoon

**1.0h measured.** 13:23–14:23, the same sitting as rounds 8 and 9 — no gap
in it reaches thirty minutes — split at round 9's commit. Messages and commits
clustered as before; measured to the last evidence point before this section
was committed.

The first build of round 9 in somebody's hands, and what they found.

## Features

### Pushes for accounts no longer signed in — 0.08h · app + mobile-app

Some people kept getting notifications for an account they had signed out of.
A new sign-in on the same phone already deactivated the previous account's
subscriptions for that push token; what nothing covered was a phone left
signed out. Sign-out unregisters through the ordinary authenticated sync, and
several ways out skip it — a session that ends by itself, the terms and
deleted-account screens returning to login, an unregister lost to a bad
connection — while a signed-out app cannot authenticate to say so.

So a signed-out app now reports itself to a new unauthenticated
`POST /push/signed-out` once per signed-out spell, with its push token and its
installation id, and every subscription matching both is deactivated. Both are
secrets only the phone and the server hold, so a token alone cannot silence
anybody, and the route can only turn notifications off. The sign-in sync also
clears other accounts on the same installation whatever their token. Nobody is
signed out by any of this; stale rows clear themselves the next time the
current user's app syncs.

### The message field's own Paste takes photos and files — 0.12h · mobile-app

The "photo on your clipboard" chip is gone — it guessed at the clipboard,
appeared for things that were not photos, and sat in the way. The composer's
text field is now `@mattermost/react-native-paste-input`, a drop-in TextInput
whose native Paste (long-press, or ⌘V) delivers images and files as well as
text; they are staged like anything picked from the library.

On iOS the library needs a call in the AppDelegate, which Expo generates, so a
local config plugin (`plugins/withPasteInput.js`) adds it — by looking the class
up at runtime rather than importing the pod, since a Swift `import` of an
Objective-C pod needs a module map nothing here guarantees, and a missing one
fails the whole build. The plugin also sets `RCT_NEW_ARCH_ENABLED` before
autolinking: the library's podspec refuses to install without it, and Expo's
Podfile loads every podspec before React Native sets it. Both prebuilt in a
scratch copy; the inserted Swift type-checks against the iOS 18.2 SDK. The
component is loaded defensively, so a binary without it keeps a plain text
field instead of crashing.

Version **2.4.0**: a new native module again — but no extension or entitlement,
so none of the Apple-portal work 2.3.0 needed.

### Opening a chat from a notification — 0.05h · mobile-app

Two causes. On a cold start, a tapped conversation push was dropped outright
whenever beta access had not loaded yet — "not known" was treated as "off", and
the tap was marked handled either way. It is now held and opened once access is
known. And on an ordinary tap the chat opened on its cached copy, which lacked
the new message, while the first fetch — the one most likely to fail just after
waking, with a token to refresh — was silent and not retried until the
fifteen-second poll.

The push's message id now travels with the tap. If the cached thread does not
have that message, the chat paints its name and picture — from the cache, or
the push's own title — over a spinner instead of stale messages, and a failed
first fetch retries at 1.5, 3 and 6 seconds. An in-app banner's message is also
merged into that thread's cached copy as it arrives, so tapping the banner opens
on it. A thread no longer writes its pre-network state back to disk, and "Say
hello." waits until the server has said the thread is empty.

### Shared items: the sheet's bottom, and the photo at once — 0.03h · mobile-app

The share sheet had no bottom padding, so its last row — the send row, for
anything shared in from another app — sat under the home indicator. It now
clears it on every phone. And a photo shared in appears in the chat the moment
it opens: the sheet seeds the attachment cache with the local file and the
message into the thread, instead of the chat fetching it and then downloading
your own photo back.

## Smaller fixes

| Fix | Hours | Repo |
|---|---|---|
| Contacts' add button sat high — a hard-coded 108pt from before the dock lift was measured, with the lift then added on top; now the Messages button's positioning | 0.03h | mobile-app |
| Answering two questions: whether the current build fixes the top bar vanishing before a chat slides in (it should — the bar is no longer unmounted — with the dock, the orbs and Android's bottom padding still showing the same family of artifact), and what Apple-portal work the two new extensions created | 0.57h | — |
| Round 9's push and summary, after its measured end | 0.10h | app + mobile-app |

The second row is mostly the time between messages, which the sitting counts.

## Totals

| | Hours |
|---|---|
| Features | 0.28h |
| Smaller fixes | 0.70h |
| **Itemised** | **0.98h** |
| Reading the code behind each report, verification, this write-up and the commit | 0.01h |
| **Measured** | **1.0h** |

## Volume

| Repo | Files | Insertions | Deletions |
|---|---|---|---|
| app | 3 | 114 | 0 |
| mobile-app | 13 | 506 | 139 |
| **Total** | **16** | **620** | **139** |

No migrations. One new route (`POST /push/signed-out`). One package
(`@mattermost/react-native-paste-input@2.0.1`) and one local config plugin.

## Needs a build

2.4.0, for the paste input: long-press Paste of a photo and of a PDF in a chat,
and ⌘V with a hardware keyboard, on both platforms. Sign out by an unusual
route — let a session lapse, or decline the terms — and check the old account's
pushes stop. Tap a chat push from a cold start and from the background.

# Round 11 — Oct 28, late afternoon

**0.3h measured.** 16:09–16:27, its own sitting — 1h46m after round 10's,
past the thirty-minute gap. Messages clustered as before, to the last evidence
point before this section was written. Not committed in the sitting.

Round 10's build in somebody's hands, and four more reports from it.

## Features

### Paste from the message field's own menu, for real — 0.10h · mobile-app

Round 10 put `@mattermost/react-native-paste-input` under the message field
and added the AppDelegate call its README asks for, and on iOS the long-press
Paste still never delivered a photo. The library's `setup:` reads the root view
factory's React host and keeps it, and with expo-updates in the app that host
does not exist yet at the end of `didFinishLaunching`: Expo defers creating it
until the update to run has been chosen (its own code asserts the host is nil
there). Every later lookup of a text field therefore found nothing, the field
was never hooked, and paste stayed text-only — silently.

`patches/@mattermost+react-native-paste-input+2.0.1.patch` keeps the factory
and reads its host when a field registers, which is long after launch; and if
the host still cannot be reached, it finds the field from the windows by its
React tag (Fabric sets every component view's UIView tag to it), accepting only
a TextInput's own component view. The same patch keeps rich text pasting as
text: text copied from Notes, Mail or a web page carries an RTF copy as raw
bytes, which the library's file scan could detect and attach as a `.rtf`, so a
clipboard with text and no image now always takes the ordinary paste. Both
changes syntax-check against the iOS SDK.

The Paste tile in the attachment tray is gone, and with it
`PasteControl.tsx`. The field's own menu is the only way to paste, as in
Telegram and Messages. A build where the native input fails to load now reports
it to Sentry from the first composer that mounts, rather than quietly falling
back to plain text again.

### Picture pickers open at once, and show that they are working — 0.08h · mobile-app

Tapping a contact's picture took a long moment to bring up the picker. All
three picture pickers — a contact's, your own, a group's — asked
expo-image-picker for `allowsEditing` to get a square crop, and on iOS that one
option swaps the system photo picker for UIImagePickerController, the pre-iOS 14
one, which loads the library before it draws. Each also asked for photo-library
permission first, which the system picker does not need.

`src/utils/avatarPicker.ts` opens the system picker with no editing and no
permission request, then makes the crop itself — the largest centred square,
which is what every round avatar shows anyway — scaled to 512px as a JPEG. That
also keeps uploads under the server's 4 MB limit, which a full-size photo from
a recent iPhone is not. The cropping is `expo-image-manipulator`, a new native
module, loaded defensively: a binary built before it falls back to the old
cropping picker, so an over-the-air update cannot break one.

From the tap until the picture is saved or the picker is cancelled, the
picture shows a spinner, so the beat before the picker appears reads as working,
and the upload after choosing is visible too.

### The attachment tray, opened with the keyboard up — 0.03h · mobile-app

Pressing + while typing dropped the keyboard and left an empty panel where the
tray should be. The tray fades in on the native driver, and the keyboard's
LayoutAnimation moves the composer at the same moment; Fabric then re-applies
the props React last rendered — opacity 0 — over the faded-in view. With the
keyboard up the tray now mounts already shown, and the composer sliding down
with the keyboard is the whole motion. Opening it with the keyboard down is
unchanged.

### Toasts under the screen that raised them — 0.01h · mobile-app

A toast raised from a contact's page appeared only after going back to the
list. `chromeStack` is a plain flex view, so Fabric flattens it, and the chat
stack overlay inside it — which holds a chat, a contact, a group's details and
the new-message screen — competes with the toast's z-index directly: 40 against
30. Every toast from those screens was drawn under them. The toast is now 50.

### Copy buttons that answer — 0.01h · mobile-app

Copying a contact's address, or your own on your profile, turns the copy icon
into a checkmark for a moment where it was pressed, and says "Address copied" to
VoiceOver and TalkBack. `src/hooks/useCopyFeedback.ts`.

### Sentry, through the admin MCP — 0.03h · app

The Sentry tools (`sentry_issues`, `sentry_issue`, `sentry_events`) are on
`main` and in production: the request-id change shipped in the same PR answers
on api.sfluv.org. This session could not call them. The claude.ai SFLuv
connector listed only the report tools — most likely a tool list taken before
they deployed — and the project's own `sfluv` server needs its OAuth sign-in in
an interactive session. Neither the MCP's tool catalog nor its "where things
live" guide named them, so a client that reads those first never learned they
existed. Both now list them, with a worked example of going from a user's report
to the occurrence that hit them.

## Totals

| | Hours |
|---|---|
| Features | 0.26h |
| Reading the code behind each report, type-checking, measurement and this write-up | 0.03h |
| **Measured** | **0.29h**, shown as **0.3h** |

## Volume

| Repo | Files | Insertions | Deletions |
|---|---|---|---|
| app | 3 | 134 | 4 |
| mobile-app | 12 | 387 | 284 |
| **Total** | **15** | **521** | **288** |

`package-lock.json` excluded. No migrations, no new routes. One package
(`expo-image-manipulator@~14.0.8`) and one patch. Version stays **2.4.0**: both
native changes are guarded or invisible to JavaScript, so this round's JS is
safe to send over the air to a 2.4.0 binary built before it.

## Needs a build

A new 2.4.0 binary, for the patch and the manipulator. In a chat, on iOS:
long-press Paste with a photo copied from Photos, a PDF copied from Files, and
text copied from Notes (which should paste as words); then the same on
Android. Press + with the keyboard up. Change a contact's picture, your own and
a group's — the picker should be up at once, and a large photo should save.
Raise a toast from a contact's page, and copy an address there.

# Round 12 — Oct 28, late afternoon and evening

**2.4h, nearly all of it stated.** 15:56–18:40 is one continuous stretch: a
board-deck sitting from 15:56, presentation practice from 16:00 to 18:00, and
the board presentation from 18:00 to 18:40. The owner gave the times for the
practice and the presentation, and no transcript can see either one. Round 11's
16:09–16:27 sits inside the practice window and is counted once, in round 11.

## How these hours were recorded

| Window | Hours | Basis |
|---|---|---|
| 15:56–16:00 | 0.07h | measured — the board-deck session's transcript |
| 16:00–18:00 — presentation practice | 2.00h | **stated** |
| 18:00–18:40 — board presentation | 0.67h | **stated** |
| less 16:09–16:27, round 11 | −0.29h | measured, and counted there |
| | **2.44h → 2.4h** | |

Three pieces of work left evidence inside the practice window:

- Round 11's app work, 16:09–16:28. This includes the commit and push at 16:28,
  which round 11 left uncounted.
- A board-deck sitting, 16:00–16:03.
- A second board-deck sitting, 17:29–17:45.

The deck changes were asked for between run-throughs. All three pieces are part
of the two stated hours, not added to them: the window is wall-clock time, and
anything done inside it is counted once.

Sanchez's deck commit at 17:14 is not evidence of this time. It is their work,
and as in rounds 8–11, nobody else's commit counts toward these hours.

Measured to the end of the board presentation, at 18:40. This write-up began at
18:40 and is not included.

## Features

### Presentation practice — 1.40h · stated

16:00–18:00, minus round 11 and the two deck sittings inside it. The deck
changes were asked for between run-throughs, so the line between practice and
deck work is approximate. The total is not.

### The board presentation — 0.67h · stated

18:00–18:40.

### The board deck: months on every slide, and jumping to a slide from the presenter window — 0.26h · animations

17:29–17:45. The deck is not on this branch: it is in `SFLUV/animations`, under
`board/deck/`.

- Sanchez's preview of the current slide in the presenter window was pulled in.
- Every slide now shows the months it covers, on a Jun–Oct strip. A slide can
  cover one month or a range. A range for work still in development runs open
  and dashed, like the map's planned lines. The you-are-here strip moved left to
  make room.
- The presenter window now uses the deck's navigation keys, through one shared
  `navKey`. Before, it took only the arrows and clicker keys. Now it also takes
  a slide number then Enter, and Home and End. Its counter reads "Go to 9 ↵"
  while a number is being typed.

Not yet committed in `animations` at the time of writing.

### The deck goes into git, without secrets or footage — 0.12h · animations

15:56–16:03. A `.gitignore` of 78 lines keeps these out of the repository:

- `.env` files and key files;
- data exported from the SFLuv MCP that contains wallet addresses;
- final renders, which are shared elsewhere or rendered again from source;
- outside footage, and anything cut from it.

What is kept in git: the components, the timelines, and the brand assets they
use. Committed as `016c767`, the repository's initial commit, at 16:03.

## The deck's earlier sittings add no hours

The deck was built in one Claude session that ran across all three days. Its
Oct 26 and Oct 27 sittings fall inside windows this document already counts:

| Deck sitting | Span | Already inside |
|---|---|---|
| Oct 26, 13:26–15:05 and 15:46–15:58 | 1.85h | round 1's stated 11:00–16:00 |
| Oct 26, 16:41–16:44 | 0.05h | round 2's 16:08–17:37, which notes it as "a 3-minute blip in another project" |
| Oct 27, 13:15–14:05 | 0.83h | round 5's measured 12:34–18:05 |

The owner was running the deck session and the app sessions at the same time.
Adding the deck's spans would count the same clock twice, so they add **0.0h**.
They are listed here so the work is on record:

- **Oct 26:** the brief. A 15–20 minute timeline of the platform's milestones
  since July, cut down to the big ones and placed in the larger story.
- **Oct 27:** the deck, built in the chosen style:
  - two sections renamed, to "creating merchant self-redeeming tools" and
    "architecture upgrades";
  - the SFLUV logo on the Bera → Celo migration slide;
  - the partners slide corrected.

## Totals

| | Hours |
|---|---|
| Presentation practice | 1.40h |
| Board presentation | 0.67h |
| Board deck, Oct 28 | 0.38h |
| Board deck, Oct 26–27 | 0.00h — counted in rounds 1, 2 and 5 |
| **Round 12** | **2.44h**, shown as **2.4h**: 2.38h stated, 0.07h measured |

Each item is rounded on its own. Rounded, they add to 2.45h; the total of 2.44h
is unrounded.

## Volume

| Repo | Files | Insertions | Deletions |
|---|---|---|---|
| app | 1 | 143 | 5 |
| animations | 8 | 161 | 34 |
| **Total** | **9** | **304** | **39** |

In `app`, only this document changed. In `animations`:

- the `.gitignore`, committed;
- seven deck files, +83/−34, not yet committed.

The initial commit holds 490 files and +244,486 lines. Nearly all of that is
the rest of `animations`, which this branch does not cover. `board/deck/` is 19
of those files, +2,892 lines: the deck as built on Oct 26 and 27, whose time is
covered above.

No migrations, no new routes, nothing needing a build.

# Round 13 — Oct 29, midday

**0.5h measured** (0.47h): two stretches.

| Window | Hours | Basis |
|---|---|---|
| Oct 28, 18:40–18:46 | 0.10h | measured: round 12's write-up, which round 12 left out |
| Oct 29, 11:43–12:05 | 0.37h | measured: this round's work, up to the last evidence point before this section |
| | **0.47h → 0.5h** | |

Messages and PJ's commits are clustered as before. Within the Oct 29 sitting,
each feature is timed by when its tool calls and file writes happened. That
split is approximate below 0.05h.

This round starts by pulling Sanchez's newest work. The app repo has nothing
new: `origin/main` matches this branch exactly. Mobile-app fast-forwards three
commits on `sanchezo/sentry-mobile`:

- a dead-tap report fix;
- the picture you gave a contact, everywhere that person appears;
- the name you gave a contact, in every chat surface, including an open chat.

They are his work and are not counted here. Then come four reports.

## Features

### A chat counts as read the moment it opens — 0.13h · app + mobile-app

Opening a chat could take up to fifteen seconds to clear its unread state. The
read was sent only after the thread's own two fetches returned. Then the shell
re-counted the Messages badge at the same moment the read was sent, so the
count usually came back before the read had landed. The badge kept the old
number until the next poll.

`src/services/pendingReads.ts` now holds reads until the server confirms them.

- **When a read is recorded.** As soon as a thread is on screen, including from
  the cache before any network. It records the newest message showing, and
  records again when a newer message arrives.
- **What uses it.** The inbox rows and the Messages badge both apply it, so both
  clear immediately.
- **Why a watermark.** A conversation counts as read while its newest message is
  no newer than the recorded one. A message that arrives after the read shows
  as unread again on its own. A server reply from before the read cannot bring
  the dot back.
- **Until the server confirms.** The read is sent, then retried with backoff and
  again each time the app returns to the foreground. An unconfirmed read is kept
  on disk, so one made just before closing the app still shows and still sends
  on the next launch. A confirmed read is dropped once a server reply agrees
  with it.
- **Scope.** Reads belong to the signed-in person and are cleared on sign-out,
  like the chat cache.

The backend changes are backwards compatible:

- `POST /conversations/{id}/read` takes an optional `up_to`, the newest message
  the client showed. The server never marks read past it, so a read sent from a
  cached view does not cover a message that reached the server but not the
  phone. A request with no body marks read to the newest message, as before.
- `GET /me/messaging-summary` also lists each unread conversation with its
  newest unread message. The client can then remove its own unconfirmed reads
  from the count exactly. The count is unchanged, and older clients read only
  the count.

Against an older server the badge shows the plain count, as it did before.

### Pushes show the picture and name you gave someone — 0.12h · app + mobile-app

The report was that chat pushes don't show the person's or the chat's picture.
Since Sanchez's change, the app draws the picture you gave a contact wherever
that person appears. The push still fetched only the sender's own profile
picture. A contact you had given a photo, who had none of their own, appeared
in the app with your photo and in the push as initials.

The push now does what the app does, for each recipient:

- The recipient's contact for the sender is looked up, the same one the app
  uses (favourites first, then the oldest).
- If you gave them a picture, the push carries a signed URL for it. This is a
  new `push-image` kind, `c`, checked against the recipient still owning that
  contact.
- If you gave them a name, the push uses it: a direct thread's title, the
  "Maria: " at the start of a group's line, and the spoken form.

In-app banners make the same swap on the device, because the socket's notice is
one wording shared by every member.

`publicBackendBase` read only two of the three settings that every other public
URL here uses. A deployment that set its public address only as
`MCP_PUBLIC_BASE_URL` sent no picture URLs at all. It now uses
`utils.PublicBackendBase`.

`/push-image` is live in production. It returns its own empty-body 404, not the
router's "404 page not found", so the route itself was not the cause. This round
could not see whether production sets a public URL.

The extension now logs each fallback to the unified log, for Console.app under
the subsystem `SFLuv.NotificationService`:

- the push carried no picture URL;
- a picture download failed, with its status;
- a picture did not decode;
- iOS refused the communication-notification style.

The fallbacks tell the two failures apart:

- **Initials where a picture should be** means the server half failed.
- **The SFLuv logo** means iOS refused the communication-notification style.

The extension's setup matches Apple's guide: the entitlement and
`NSUserActivityTypes` on the app, the interaction donated before the content is
updated, the direction set to incoming, and the content handler called once.

### Names in an open chat — 0.05h · mobile-app

Sanchez's change covers this report: renaming a contact re-renders an open
chat's header, its bubbles, reply quotes, chat info and the inbox. Two gaps
remained.

- **Edits could be lost.** The contact card saved its fields only when they lost
  focus. A back tap or swipe while still typing unmounted the screen first, so
  the rename could be lost without anyone noticing. Leaving the card now saves
  any edit still in a field. The shell also stopped re-opening the card when
  that late save came back.
- **Banners.** In-app banners now use the contact name too.

Not covered: when the other person changes their *own* profile name, an open
chat picks it up on its next refresh, up to fifteen seconds later.

### "📷 Photo" in the inbox, as in the push — 0.04h · app + mobile-app

Pushes already described attachments: "📷 Photo", "📎 menu.pdf",
"📍 Shared Café Réveille". The inbox did not, because its query never loaded the
last message's attachments. A photo or file sent without a caption previewed as
an empty line, or just "You: ". The client already had labels for every kind,
with nothing to label.

The inbox now loads attachment metadata for every last message in one query,
and fills in the merchant and event names the preview prints. A video is named
for what it is ("🎥 Video") in both places, not by its file name. The inbox
label now counts photos, videos and files together the way the push does: two
photos and a PDF are "3 attachments", not "3 photos".

## Smaller items

| Item | Hours | Repo |
|---|---|---|
| Pulled Sanchez's newest on both repos; app had nothing new | 0.01 | both |
| Review of the diff. It caught the inbox's unread callback, which had become an inline arrow; `loadInbox` depends on it, so the inbox would have refetched on every shell render. Now a stable callback | 0.02 | mobile-app |

## Totals

| | Hours |
|---|---|
| Features | 0.34h |
| Smaller items | 0.03h |
| Round 12's write-up, Oct 28 | 0.10h |
| **Measured** | **0.47h**, shown as **0.5h** |

## Volume

| Repo | Files | Insertions | Deletions |
|---|---|---|---|
| app (code) | 6 | 236 | 42 |
| mobile-app | 9 | 620 | 43 |
| **Total** | **15** | **856** | **85** |

This document is not counted, and neither are Sanchez's three merged commits.
No migrations and no new routes; one new kind on an existing route
(`push-image?k=c`). Nothing is committed yet.

## Needs a deploy and a build

- **Backend deploy:** pushes with your contact's picture and name, the fallback
  public URL, attachment previews in the inbox, video wording, `up_to` on reads,
  and the per-conversation unread list. The preview fix reaches installed 2.4.0
  phones as soon as it deploys.
- **A rebuilt binary:** for everything in mobile-app. There is no production
  build yet, so nothing is sent over the air; each change ships in the next
  binary.

To test:

- Open an unread chat. The badge and the inbox row should clear at once, and
  stay clear in airplane mode, until the read is sent.
- Give a contact a photo and a name. Have them message you with the app
  closed, then check the push.
- Rename a contact and tap Back while still typing.
- Send a photo with no caption. The inbox should say "📷 Photo".

# Branch scope — `pjol/mobile-2-1`

Oct 26 2026 · app + mobile-app + webpage · **8.9h** stated (5.7h measured, as an upper bound for the earlier part)

Round 1's two prospective entries are superseded by round 2 — see there.

The work behind mobile release 2.1 — tagged `v2.1` on `mobile-app` — plus the
backend it needs and the organization and supervisor cleanups done alongside it.

Cut from `pjol/internal-notifications`, which is itself stacked on
`pjol/admin-org-management`. Those two carry their own scope documents; this one
covers Oct 26 only.

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

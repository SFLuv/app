# Branch scope — `pjol/internal-notifications`

Oct 19–21 2026 · app + mobile-app + webpage · **3.4h** (2.0h stated, 1.4h measured)

Internal organization notifications, delivered to the mobile app, plus the website's Get in touch
flow and a rebuilt mobile boot sequence.

# Round 1 — Oct 19–20, before any code

Oct 19–20 2026 · **2.0h stated**

Internal organization notifications on the mobile app, plus a round of website updates.

**This branch changed direction mid-flight.** It began as `pjol/telegram-notifications` and a working
Telegram bot was built on it — three alert channels, in-chat action buttons, server-side action tokens.
None of it was committed. Sanchez then pushed a bulletin-board and group-chat featureset to both
repositories, and the decision was to build the same notification behaviour on top of that instead of on
a third-party chat service. The Telegram layer was removed rather than left dormant; what survived it is
named under "Salvaged" below, because that work is the reason the replacement was quick.

## How these hours were measured

**2.0h stated** by the owner. **Nothing is measured yet**, because there is no session work on this
branch to measure: the only SFLuv transcript activity since Oct 18 is this session, which opened at
10:48 today.

| Source | Hours | Basis |
|---|---|---|
| Mon 19 Oct 14:00–15:00 — dev meeting | 1.00h | stated |
| Tue 20 Oct 10:00–11:00 — website reviewing | 1.00h | stated |
| | **2.0h** | |

**Both figures are stated and neither is corroborated.** A meeting leaves no trace in a transcript or a
repository, and reviewing a site is reading rather than writing, so there is nothing for the clock to
find. Recorded as stated, per the convention.

One thing the records do show, named because this is where a measurement's weaknesses go: 0.13h of
activity in another, non-SFLuv project overlaps the end of the review window (10:42–10:50).

## Where this branch starts

Cut from `pjol/affiliate-touchups` at `00d1241`, not from main — so it **carries five unmerged commits**
that belong to the other branch:

| Commit | What |
|---|---|
| `0a3c79a` | Salesforce V4S partner availability |
| `effdbc6` | Protocol account funding / auto top-up |
| `07c709b` | Merge of main (Railway prep, jobs leader) |
| `20d942f`, `00d1241` | Affiliate branch scope checkins |

This matters at merge time rather than now: a PR from here against main would carry the affiliate work
with it, and that work's own scope document describes it. Either merge `pjol/affiliate-touchups` first,
or expect both to arrive together. It is recorded here so whoever opens the PR is not surprised by the
diff.

## Large features

*Nothing yet.*

## Smaller items

*Nothing yet.*

## Totals

| Source | Hours |
|---|---|
| Stated — Mon 19 Oct dev meeting | 1.00h |
| Stated — Tue 20 Oct website reviewing | 1.00h |
| **Measured** | **0.00h** |
| **Total** | **2.0h** |

## Volume

Nothing committed on this branch. `app` and `webpage` are both at their branch points.

## Open

- **The website repo is on another branch with uncommitted work in it.** `webpage` is on
  `pjol/volunteer-gallery` with one modified file, `src/lib/volunteer-events/format.ts` — the partner
  spot-count change from the affiliate branch's round 2, still unshipped. Website edits made now would
  sit on top of it, so it wants committing or moving before this branch's work starts.
- **Nothing is pushed.** `pjol/internal-notifications` is local only.


---

# Round 2 — Oct 20 · 1.2h measured

The branch changed direction mid-sitting and most of what round 2 built was built twice.

## How these hours were measured

**1.2h measured** from session-transcript timestamps, clustered into sittings on a 30-minute gap,
unioned across every project worked in — the `time-accounting` method. Corroborated by the commit
timestamps on this branch and by file mtimes in all three repositories.

| Sitting | Span | Measured | On this branch | What |
|---|---|---|---|---|
| A | 11:24–11:55 | 0.52h | **0.45h** | Telegram bot + website Get in touch |
| B | 13:54–14:37 | 0.70h | **0.70h** | The pivot: merges, removal, internal notifications, mobile |
| | | | **1.15h → 1.2h** | |

**Two weaknesses, both named rather than smoothed over:**

- **0.07h of sitting A was spent in `SFLUV/animations`**, not on this branch, and is deducted above.
  It sits inside the sitting rather than beside it, so the clock cannot separate them more finely
  than this — 0.45h is the sitting minus the other repo's own measured span.
- **Sitting B was still open when this was written.** 14:37 is the last transcript entry, not the
  end of the work. The figure is a floor, and whatever follows belongs in round 3.

**0.04h at 10:48–10:51 is deliberately not counted.** It falls inside round 1's stated 10:00–11:00
website-review window, which is already booked. Round 1's own weakness note stands and extends
slightly: the non-SFLuv project overlapping that window ran 10:42–**10:53**, not 10:50.

## Large features

### Internal organization notifications — 0.8h · app, mobile-app

Operational alerts — low protocol balances, the backend failing its health check, role requests
waiting on a decision, website support messages — delivered to the people designated to receive
them, with the buttons to act on them attached.

Three notification sets on the house organization (Dev alerts, Admin alerts, Support requests),
administered from the organization settings page beside the existing permissions panel. Scoped to an
organization rather than global, so partner organizations can have their own later; only the house
organization has any today and the handler enforces that.

The design decision everything else follows from: **an alert is a condition, not an event.**

- Posting is idempotent in the alert key, enforced by a partial unique index over unresolved rows.
  The funding sweep reports the same low balance every fifteen minutes; that is one alert, pushed
  once.
- Resolution is global rather than per-recipient. One admin approving a merchant answers the
  question for everybody, so it stops asking everybody.
- Each kind declares how it ends. A low balance has **no buttons at all** — the only fix is sending
  money, and a button that merely hid it would make an unsolved problem look handled. An outage has
  an acknowledge that is refused while it is still down. A role request has Approve and Reject that
  run the same code the admin panel runs.
- Resolved alerts are kept with a sentence about how they ended, because "who approved this
  merchant" is the question the history gets asked.

**Delivery and authority are deliberately separate.** Being designated for a set decides what you
see; whether you may act is the role that endpoint already required. A recipient of admin alerts who
is not an administrator sees the request with no buttons on it, rather than a button that errors.

Migration **1.67**: `organization_notification_subscriptions`, `internal_notifications`,
`user_notifications`. Eight new routes (339 → 347).

### Mobile notification screen — 0.25h · mobile-app

A modal holding fifty pushes in AsyncStorage on one device became a screen with server-side history.
Active notifications sit above a lazily-paged history, and the two follow different rules: an
ordinary notification retires when it has been looked at, an internal alert when it has been dealt
with.

History is keyset-paginated on `(created_at, id)` rather than offset, because the feed is written to
while it is being read — an offset page shows one notification twice and skips another.

Removed rather than ported: the modal's deferred-navigation workaround, which stashed a tapped
notification's intent and ran it from the modal's dismissal callback because iOS silently dropped a
presentation made mid-dismissal.

### Mobile boot sequence — 0.15h · mobile-app

A cold start made two serial network calls and showed "Checking app compatibility…" until both
landed — nothing on screen, nothing interactive. The policy and config are now cached, the app boots
from them, and the real check runs behind it. While Privy reads the keychain, the app's own chrome
goes up as a skeleton.

**The version gate is untouched by this, by four rules:** only a non-blocking policy is ever cached;
the cache is keyed by the binary, so a new build gets no use of the old one's verdict; it expires
after seven days; and the real check still runs every launch and every foreground, with a blocking
policy taking effect the moment it arrives. What is given up is the second between launch and that
reply.

### Website Get in touch — 0.45h · webpage

Built in sitting A and **unaffected by the pivot**, because it only ever needed somewhere to send an
alert. Two-step page (community chat first, form second), a whitelabel `Listbox` built from our own
parts, a server-side proxy with a honeypot, a quotable reference (`SUP-7F3KQ2`), a confirmation
email to the sender, and a new admin Support tab. `/contact` permanently redirects; the Get Involved
Google Form links now point at the page with the subject preselected.

## Smaller items

| What | Hours | Repo |
|---|---|---|
| Merge `sanchezo/bulletin-backend` (4 commits, 4,545 lines) | 0.05h | app |
| Merge `sanchezo/bulletin-mobile` (2 commits, 2,925 lines) | 0.02h | mobile-app |
| Remove the Telegram layer, keeping the three parts that outlived it | 0.1h | app |
| Verify 1.67 against a throwaway Postgres — 9 behaviours | 0.1h | app |
| `?tab=balances` did not deep-link; "balances" was missing from the valid-tab list | 0.02h | app |

## Totals

| Source | Hours |
|---|---|
| Round 1 — stated (dev meeting, website review) | 2.00h |
| Round 2 — measured | 1.15h |
| **Total** | **3.2h** |

Round 3 came after this table was written; its own totals table below supersedes it.

## Volume

| Repo | Files | Lines | Other |
|---|---|---|---|
| app | 10 modified, 12 new | +3,577 / −5 | 1 migration (67 total), 8 new routes (347 total) |
| mobile-app | 2 modified, 4 new | +1,749 / −236 | — |
| webpage | 7 modified, 5 new | +44 / −41 modified, 654 new | committed as `080ac8a` |

Merged in from Sanchez, not counted above: 4,545 lines (app) and 2,925 lines (mobile-app).

## Decisions worth finding later

- **The Telegram bot was built and removed on this branch, unlanded.** Three things outlived it,
  and they are the reason the replacement was quick: `funding.Conditions` (the sweep reports its
  full set of active problems every run rather than an event per problem, which is what lets an
  inbox read as current state); `applyLocationApproval`, extracted out of the HTTP handler so
  approving a merchant derives wallets in exactly one place; and the website support flow.
- **Removing Telegram simplified the action layer rather than complicating it.** The bot needed a
  server-minted token per button, in its own table, because `callback_data` was capped at 64 bytes
  and arrived from the open internet. An authenticated request from our own app needs neither, so
  the table and the token plumbing are gone. The reply-by-email button also loses its server-side
  302: Telegram rejects `mailto:` in buttons, a phone does not.
- **One behaviour the Telegram version had to compromise on is now correct.** There, a role decision
  could not both disappear and leave a record, so it collapsed to a one-line "Approved by @x" and
  stayed on screen. Here it leaves the active list and lands in history carrying that sentence.
- **Ack-on-view has one real cost, and it is the W-9 notice.** Per the brief, everything that is not
  an internal alert is acked as soon as it is seen — including the derived tax and payout entries,
  which previously stayed in the list while their condition held. A volunteer who owes a form now
  loses the bell nag after one look. The obligation is still surfaced by the escrow card and the
  tier modal, and the acking policy is one line per kind if this should change.
  **Changed in round 3**: the owner's call was to exempt the tax notices, which now end only when
  the W-9 is filed. The line per kind is where round 3 put the rule.
- **The webpage still links to the community Telegram group.** That is a public group people join,
  not the bot integration, so it was left alone — `NEXT_PUBLIC_TELEGRAM_COMMUNITY_URL` is unrelated
  to anything removed here.

## Open

- **Nothing is pushed.** All three repositories are local only: `848d060` (app), `6f186bf`
  (mobile-app), `080ac8a` (webpage).
- **Sanchez's other two branches are not merged.** `sanchezo/photo-bucket` also claims migration
  **1.66** and will collide with the bulletin migration on whichever lands second.
  `sanchezo/fix-invisible-toasts` fixes `toast()` not showing messages, which the new admin panels
  on this branch rely on — worth having, but out of the scope named for this branch.
- **This branch still carries five unmerged commits** from `pjol/affiliate-touchups`, as round 1
  recorded, plus Sanchez's bulletin work. A PR from here against main arrives with all of it.
- **The uptime monitor cannot detect a dead process**, because then nothing is polling. It covers
  unreachable deploys, an unopenable database and bad status codes. An external monitor is still
  worth having.
- **`user_notifications` starts empty.** Nothing can backfill it; the notifications it would be
  backfilled from were never recorded anywhere. **Round 3 found the worse half of this**: nothing
  was writing new rows either, so see round 3 rather than this line.
- **8 `console.error("[w9] …")` debug lines remain in `mobile/App.tsx`.** Pre-existing and left
  alone; one went with the notification modal.

# Round 3 — Oct 20, the acking rule and the history that was not being written

Oct 20 2026 · **0.2h measured** · app + mobile-app

One instruction — "ack on view for everything except the w9 notice, that should be resolved when the
user successfully submits their w9" — and the defect it exposed.

## How these hours were measured

**0.2h measured** from session-transcript timestamps on the `time-accounting` method: a 30-minute
sitting gap, unioned across every project worked in.

| Sitting | Span | Measured | On this branch |
|---|---|---|---|
| C | 16:18–16:28 | 0.17h | **0.17h → 0.2h** |

**Two weaknesses:**

- **The sitting was still open when this was written**, exactly as round 2's was. 16:28 is the last
  transcript entry at the time of writing, not the end of the work, so the figure is a floor.
- **A 0.00h sitting at 15:36 is not booked anywhere.** It is a single message — the hand-off that
  closed round 2 — and 42 minutes clear of this sitting on either side, so it clusters alone and
  rounds to nothing. Noted rather than absorbed, because round 2's figure was already published.

Nothing else was worked on after 15:00 today: the two other projects with transcript activity today
(`SFLUV/animations` 11:40–11:44, and one non-SFLuv project 10:42–10:53) both sit inside windows
rounds 1 and 2 already accounted for. This round's clock is this work alone.

## The change asked for

**The tax notices no longer ack on view; they end when the W-9 is on file.** Everything else keeps
the rule round 2 shipped.

The rule is one function, `structs.NotificationAcksOnView`, and the write path is the only place it
is consulted: `MarkImproverNotificationsSeen` splits a key into `<type>:<subject>` and drops any key
whose type does not ack on view. A client asking for a tax notice by name cannot silence it, and
neither can a caller written later by somebody who never read this file.

The read path lost its seen state rather than ignoring it. Both tax queries dropped their
`improver_notification_reads` join, because a column nobody is allowed to act on is worse than no
column — the next person to touch the query would reasonably assume it meant something.

**What ends one is unchanged, and was already right**: the notices are derived from live state on
every request, so a filing reaching `completed` (which is what the vendor callback writes when the
form is submitted) removes the row from the query's own results. The notice does not get resolved —
it stops existing, like every other derived notification.

Existing seen markers for tax notices are now dead data. The housekeeping sweep deletes them
unconditionally instead of checking whether they are still live, which also makes it two clauses
shorter.

## The defect this exposed — ordinary notifications were not reaching the history at all

Round 2 built the history table, the recorder, the pagination and the screen. **The recorder had no
call sites.** Only internal alerts ever wrote a row, so the history on the new screen could only
ever show alerts, and "ack on view for transactions" was a rule about rows that did not exist.

The six push paths now each write their row: transaction receipts, volunteer reminders, organizer
blasts, bulletin moderation notices, direct messages, and the W-9 messages. Written before the
device lookup and regardless of its outcome, which is the doctrine the file already stated — a push
that fails to send still leaves the row, because the notification is still true.

Three things fell out of doing it:

- **A transaction receipt had no destination.** Its payload named the transfer but carried no
  `type`, so `targetFromData` returned "nowhere" and tapping a receipt left you on whatever screen
  you were already on. It now carries `transaction` and lands on Activity.
- **The badge counted some notifications twice.** It summed the server's unseen count and the
  length of the device's own tray mirror, which was harmless while the ledger held only alerts and
  wrong the moment both described the same arrival. The ledger is now the single source; the mirror
  keeps the job only it can do, which is knowing what is still in the OS shade.
- **The W-9 messages are deliberately not recorded** — and the condition for skipping them is
  `NotificationAcksOnView` again, which is not a coincidence. A kind that does not ack on view is a
  kind the feed already derives and keeps on screen until it is answered, so a row for it would put
  the same thing in the list twice: once as the standing "complete your W-9" entry, once as the push
  that said so. The release confirmation does get a row, because the condition it reports is over by
  definition and nothing else would ever show it.

## Smaller items

| What | Hours | Repo |
|---|---|---|
| Verify the rewritten queries and the sweep against a throwaway Postgres | 0.03h | app |
| Transaction receipts route to Activity (`type` was missing from the payload) | 0.01h | app |
| Drop the tray mirror from the badge count | 0.01h | mobile-app |

## Totals

| Source | Hours |
|---|---|
| Round 1 — stated (dev meeting, website review) | 2.00h |
| Round 2 — measured | 1.15h |
| Round 3 — measured | 0.17h |
| **Total** | **3.3h** |

Round 4 came after this table; its own totals table below supersedes it.

## Volume

| Repo | Files | Lines |
|---|---|---|
| app | 9 modified | +145 / −86 |
| mobile-app | 1 modified | +26 / −13 |

No migration: the tax notices never had a table of their own, and the one they borrowed a seen
marker from is unchanged.

## What was verified, and what was not

Against a throwaway Postgres, dropped afterwards: an escrow notice is still returned for a user who
has an old seen marker; so is a tier notice; a `completed` filing returns the status that removes
both; and the housekeeping sweep deleted exactly three markers — both tax markers and a settled
payout's — while keeping the live payout's.

**Not executed:** the Go key-splitting filter. The root volume has 80MB free, so nothing links on
this machine; `go vet` type-checks every touched package and that is the whole of it. The filter is
four lines over `strings.Cut`, but it has not been run.

## Open

- **Chat messages now land in notification history.** Every direct message writes a row, matching
  what the device's tray list used to show. An active conversation will bury receipts under it.
  Including them was the safer default — a notification somebody remembers receiving should be
  findable — but it is one `recordNotification` call in `notifyNewMessage` if it reads wrong in use,
  since Messages carries its own unread state anyway. **Answered in round 4**: the call is gone, the
  push stays, and the count moved to the Messages tab.
- **History still starts empty**, and the first rows appear only as new notifications are sent.

# Round 4 — Oct 21, messages leave the history and the chat tab gets its own count

Oct 21 2026 · **0.1h measured, and the figure is an upper bound** · app + mobile-app

Round 3 flagged chat messages landing in notification history as a thing to decide. The decision:
they keep the iOS push, they write no history row, and the Messages tab carries their count.

## How these hours were measured

**0.1h measured** from session-transcript timestamps, 30-minute sitting gap, `time-accounting`.

| Sitting | Span | Measured |
|---|---|---|
| D | 11:55–12:02 | 0.12h → **0.1h** |

**This one's weakness is the opposite of round 3's, and worth reading before quoting the number.**
Round 3's figure was a floor because its sitting was still open. This figure is an **upper bound**:
a second project (`ChatMTG`) was active 11:39–12:02, which covers every minute of this sitting. The
two sessions were interleaved, so the clock cannot say how the seven minutes divided between them.

Round 2's rule — deduct the other project's span from the sitting — gives 0.0h here, which is false:
a backend change, two mobile files and a verification pass are in this round. So the span is reported
whole, with the overlap named, rather than deducted into nothing or quietly ignored. **Somewhere
between 0.0h and 0.1h of this is this branch**, and the honest half of that sentence is the second
one.

## The change

**Messages still push; they are no longer recorded.** Round 3 wired the history recorder into six
push paths, chat messages among them, and said in its own open items that an active conversation
would bury receipts. It does — and the row was never needed, because a message is the one
notification with somewhere better to be counted. The thread carries its own unread state, the inbox
shows a dot per conversation, and now the dock shows the total.

Rows written during the one day that behaviour existed are swept by the housekeeping sweep, whatever
their age, rather than by a migration — the same treatment the dead tax markers got in round 3.
Nothing writes one, so the sweep drains them and then does nothing forever.

**The Messages tab carries the count.** `GET /me/messaging-summary` already existed, unused by the
app: Sanchez wrote it for "the notification bell", which is exactly where messages turned out not to
belong. Three sources keep it honest, and the cheapest does most of the work:

- **The Messages screen reports its own**, counted from the inbox it already polls every fifteen
  seconds. So the badge clears when you back out of a thread, not on the shell's next poll.
- **An arriving push bumps it**, filtered to `bulletin_message` — the same pattern the merchant
  till's incoming-payment confirmation uses. The server does the counting, because two messages in
  one conversation are one unread conversation and only it knows that.
- **A 60s poll covers the person who has push turned off**, and skips itself while the Messages
  screen is up, since that screen is already answering the question.

A failed request leaves the badge where it was. Clearing it on a dropped connection is the one wrong
answer available.

**The invariant the dock already had is intact.** The Participate tab carries no dot, with a comment
saying why: a notification is counted in exactly one place, and a second indicator either repeats the
bell or disagrees with it. Messages are now the kind that is *not* in the bell's ledger at all, so
the tab is that one place for them and nothing says it twice.

## Smaller items

| What | Hours | Repo |
|---|---|---|
| Reading a thread clears its iOS shade entry, by any route in | 0.02h | mobile-app |

Three code paths open a thread — a deep link, a search result, a tap in the inbox — and only the
first cleared the notification, because only it arrived through the push handler. One effect on the
open thread id now covers all three, so a message read from the inbox stops sitting in the shade.

## Totals

| Source | Hours |
|---|---|
| Round 1 — stated (dev meeting, website review) | 2.00h |
| Round 2 — measured | 1.15h |
| Round 3 — measured | 0.17h |
| Round 4 — measured, upper bound | 0.12h |
| **Total** | **3.4h** |

## Volume

| Repo | Files | Lines |
|---|---|---|
| app | 2 modified | +22 / −1 |
| mobile-app | 2 modified | +174 / −4 |

## Open

- **The dock badge counts conversations, not messages.** Ten messages in one thread read as "1".
  That is what the server's summary counts and what the inbox's own dots mean, so all three agree,
  but it is a choice rather than an oversight.

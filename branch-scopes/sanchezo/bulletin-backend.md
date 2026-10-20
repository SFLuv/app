# Branch scope — `sanchezo/bulletin-backend`

Oct 19 2026 · app (backend only) · **1.5h active** (design 1.2h, build 0.3h)

Hours are wall-clock time from the live `UserPromptSubmit` stamps in the
session, corroborated by file mtimes, per the `time-accounting` skill. One
sitting, 15:27–16:59 PDT, with no gap over 30 minutes. Design is 15:27–16:39
(the product conversation that produced the spec, from "start with a bulletin
board" to the go-ahead); build is 16:39–16:59 (worktree cut, every file
written, local end-to-end run). File mtimes fall 16:42–16:57, inside the build
window.

Weakness of the measurement: the build figure is short because the code was
written by an agent; a reader wanting "how long would this take by hand"
should not read it as that, and nothing here estimates it. Earlier legal and
product research on the same idea (Sep 13 and Oct 10, in the Boundless grant
session) is not counted.

Status: **local only. Not pushed, not merged, no PR.** Mobile screens are on
`sanchezo/bulletin-mobile` in `SFLuv/mobile-app`.

---

# Round 1 — Oct 19

## What this branch is

A neighborhood Bulletin board and direct messaging for the mobile app:
Facebook-Marketplace-style posts sorted by distance (Free, For sale,
Neighborhood notice), messages between neighbors, paying in SFLUV from a
chat, and the safety tools a user-generated-content app needs (block, report,
an admin review queue, suspension).

Product decisions and the full spec live outside the repo, in
`SFLUV_Boundless/services-list.md` and `bulletin-spec.md`.

## Features

### Bulletin posts — design 0.5h, build 0.1h
- Migration 1.66: `user_profiles`, `bulletin_posts`, `bulletin_post_photos`.
- Posts store an **approximate location only**: the server snaps the device's
  coordinates to a ~250 m grid before anything is written. The area label
  ("Tenderloin") comes from the city's 41 analysis neighborhoods, simplified
  and embedded as data (`handlers/bulletin_neighborhoods_data.go`, generated
  from DataSF `j2bu-swwd`).
- Feed: active, unexpired posts within a radius, nearest first, cursor-paged.
  Excludes blocked users (both directions), suspended authors, and anything
  the viewer reported.
- For-sale posts carry a price (whole SFLUV, ≤2 decimals) and SFLUV and/or
  cash. Free and notice posts carry neither; the DB enforces both.
- Notices may link a Participate event; the post expires when the event ends.
  For volunteer events `events.expiration` is the event end (codes expire at
  `qr_expires_at`); an event without one runs to local midnight. Cancelled or
  finished events drop the post from the feed.
- Optional meetup spot: an approved partner merchant.
- Up to 6 photos per post (4 MB each, the mobile workflow-photo cap), raster
  only (same sniff as location icons),
  served only to people who can see the post.
- Rate limit: 5 posts per user per 24h.

### Messages — design 0.3h, build 0.1h
- `conversations`, `conversation_members`, `messages`. One thread per pair per
  post, plus one plain thread per pair; reopening returns the same thread.
- Inbox with unread counts and the post chip; unread summary for the bell.
- **Pay from chat**: the client pays through the existing send flow, then
  attaches the tx hash. The server checks the transfer in Ponder goes from a
  wallet the sender owns to one the recipient owns; unindexed transfers are
  stored pending and verified when the thread is next read. A transfer can be
  attached once. The recipient's wallet is returned only on the single-thread
  view, and not when the post is cash-only or either side has blocked.
- Push to the recipient (existing Expo path), in the background.
- Rate limit: 20 new threads per user per 24h.

### Safety — design 0.3h, build 0.1h
- `user_blocks`, `content_reports`, `bulletin_moderation_actions`.
- Report a post, message, or person. Three distinct reports hide a post until
  reviewed. Repeat reports are harmless. Admins are emailed
  (`BULLETIN_ADMIN_EMAIL`, falling back to `AFFILIATE_ADMIN_EMAIL`).
- Admin queue groups open reports by target with the content, the person's
  summary, and prior actions. Actions: dismiss, remove post, remove message,
  warn, suspend (days or indefinite), unsuspend. Suspension covers the
  Bulletin and Messages only; the wallet keeps working.

### Profiles, search, trade feedback — design 0.1h, build <0.1h
- Public name ("First L." suggested from `contact_name`), optional @handle,
  opt-in "let neighbors find me".
- Search returns approved merchants, discoverable people, and exact-handle
  matches. Never a location.
- Thumbs-up/down between a post's author and someone who messaged them about
  it; the positive count shows as "good trades".
- `contacts.user_id` added so a saved contact can be a messageable user.

## Routes

All user routes use `withActiveAuth` (active account + accepted privacy
policy); the queue uses `withAdmin`. 27 user routes and 2 admin routes, in
`AddBulletinRoutes`.

## Testing

Per `TESTING.md` there is no automated suite and none was added. The branch
was exercised against a local server on throwaway databases (`bulletin_test_*`,
dropped and recreated each run) with locally signed tokens for four fake
users: 71 scripted checks across profiles, posting rules, photos, feed and
paging, messaging, payment verification (verified, pending-then-verified,
wrong direction, duplicate), feedback, search, blocking, reporting and the
auto-hide threshold, the admin queue, suspension, status changes, renewal,
rate limits, and deletion — all passing. The script lives outside the repo.

## Totals

| Item | Hours |
|---|---|
| Design (spec conversation) | 1.2 |
| Build (code + local run) | 0.3 |
| **Total** | **1.5** |

Volume: 13 files (11 new), +4,311 lines (260 of them a generated table of
neighborhood boundaries), 1 migration (1.66), 29 new routes.

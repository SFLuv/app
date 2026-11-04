# sanchezo/live-event-photo-tools

**Repos touched:** `SFLuv/app` (this branch) and `SFLuv/webpage` (branches `feat/event-cover-whole-image`, `feat/event-gallery-from-api`).
**Status:** built. Backend routes exercised against a copy of the local databases; admin screens viewed by Sanchez with a local login. Not merged, not deployed.
**Dates:** 2026-11-04. **0.6h active, measured.**

## What it is

Staff asked to change the cover photos on recurring volunteer events, and to show photos of past
clean-ups and shifts under the live event. Recurring events never become past events, and event
photo routes were admin-only, so neither was possible for the social media coordinator.

This adds a Live events section under Website, gated by a new private credential,
`website_live_events`. See "Live events" in `docs/features/website-editing-and-forms.md`.

## Time

**0.6h, measured** from the live timestamps on each message, per the `time-accounting` skill: from
the message that asked for the build (12:01 PST) to the PR (about 12:36 PST), one sitting with no gap
over 30 minutes. It includes setting up the local preview and time spent waiting on Sanchez's
walkthrough. Excluded, as work on `SFLuv/webpage`: the earlier website-only gallery (11:24 to 11:38)
and the whole-cover change (11:49 to 11:53). The split below is apportioned by message times and
is approximate; the total is measured. Work was done with Claude Code.

**Volume:** see the PR diff: 1 migration (1.78), 9 new admin routes, 1 new public route.

## Features, largest first

| Feature | Hours | Repo |
|---|---|---|
| Live events section: list, cover photos, gallery | 0.2 | app |
| Local preview: database copies, seeded events, fork, login fixes | 0.2 | app |
| `website_live_events` credential, keeping live galleries away from Past events routes | 0.1 | app |
| Docs, this file, PR | 0.1 | app |

## Decisions

- **A gallery is a `site_past_events` row with a `series_key`,** not a new table, so uploads,
  captions and ordering are shared with Past events rather than copied.
- **Separate credential.** Sanchez first chose to reuse `website_past_events`, then asked for a
  split after seeing the tools. Live galleries are therefore invisible to the Past events routes.
- **`series_key` has no foreign key.** Events live in the bot database, galleries in the app database.

## Follow-ups

- Grant `website_live_events` to whoever should have it. Holding `website_past_events` doesn't carry over.
- Merge the `SFLuv/webpage` branches after this deploys.
- Starting a gallery twice logs twice in Recent changes, though it reuses the same gallery.

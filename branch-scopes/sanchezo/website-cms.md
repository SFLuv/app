# sanchezo/website-cms

**Repos touched:** `SFLuv/app` (this branch) and `SFLuv/webpage` (branch `feat/forms-and-site-editing`).
**Status:** built and tested locally. **Not pushed, not merged, not deployed.**
**Dates:** 2026-10-01.

## What it is

Lets non-technical staff edit parts of sfluv.org from the admin panel (Admin → Website) with no
code push, and lets the public sign electronic forms and waivers on the site.
Design and data model: [`docs/features/website-editing-and-forms.md`](../../docs/features/website-editing-and-forms.md).

## Time

**Not measured.** The `time-accounting` skill this repo's CLAUDE.md names was not run, so no hours
are claimed here. Add a measured figure before merging (see CLAUDE.md, "Branch Scope Documents").

## Features, largest first

| Feature | Repo |
|---|---|
| Forms and Waivers: public signing (drawn or typed signature, guardian section, email copy), admin authoring with immutable versions, open/close with auto-close, signatures list, CSV, printable record, withdrawal marking, QR code | app + webpage |
| Homepage highlights (the Spotlight carousel) editable as a list: add, reorder, on/off, photo upload, per-slide button, optional recurring-event link; append-only history with restore | app + webpage |
| Financial documents editor: upload, auto-filing by statement date, edit, soft-delete with restore; impact reports in their own block | app + webpage |
| Per-capability permissions: admin, or a private `website_banner` / `website_financials` / `website_forms` credential | app |
| Seeds: the 31 documents and 2 slides the site shows today, plus the media release and withdrawal forms (closed) | app |

## Smaller fixes and findings

| Item | Repo |
|---|---|
| The app's `useToast` has no `Toaster` mounted anywhere, so every `toast()` call in the app displays nothing. The Website tools carry their own notices instead. Not fixed app-wide. | app |
| `authFetch` is rebuilt on every provider render; the Website tools wrap it so a re-render cannot reload an editor and discard an unsaved draft | app |

## Review round (2026-10-01)

A review of the first pass found, and this round fixed: a public memory-exhaustion
crash (PNG size checked only after decoding), form choices failing to save after
deleting one, open email relay via the confirmation email, rate limits that counted
typos, a falsely reported "emailed", silent overwrites between editors, missing WebP
dimensions, never-deleted orphan uploads, every file view hitting Postgres, UTC
times in emails, backslash links, and an unbounded proxy rate-limit table.

## Migrations and routes

Migrations **1.61** (tables) and **1.62** (seed data). New public routes under `/site/*` and admin
routes under `/admin/site/*`.

## Note on migration numbers

This branch first used 1.59 and 1.60; PJ's merchant-unwrap and workflow work took those
numbers on main (2026-10-03), so these were renumbered to 1.61 and 1.62 when the branch was
brought up to date.

## Deploying

The backend (`api.sfluv.org`) deploys by hand to the VM and **must go first**: the website's forms
pages report "unavailable" until `/site/*` exists. Financials and the carousel fall back to the
copy in the website repo until then. The website auto-deploys from `main`, so merge it last.

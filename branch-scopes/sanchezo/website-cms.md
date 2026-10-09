# sanchezo/website-cms

**Repos touched:** `SFLuv/app` (this branch) and `SFLuv/webpage` (branch `feat/forms-and-site-editing`).
**Status:** built and hand-tested locally (2026-10-07 to 10-09). Merged with `pjol/vulnerability-fixes` for a joint deploy; not yet on main or deployed.
**Dates:** 2026-09-27 (scoping), 2026-10-01 to 2026-10-09. **6.3h active, measured.**

## What it is

Lets non-technical staff edit parts of sfluv.org from the app's Website page with no
code push, and lets the public sign electronic forms and waivers on the site.
Design and data model: [`docs/features/website-editing-and-forms.md`](../../docs/features/website-editing-and-forms.md).

## Time

**6.3h, measured** with the `time-accounting` skill (`measure_sittings.py`, 30-minute gap) from the
session transcript, then restricted to the sittings that were this branch's work. The same
conversation also covered unrelated sfluv.org changes (Our Team, How it Works, an old-site recovery
question, Oct 6 09:41–11:22 and 14:32–14:57), which are excluded. Corroborated by file mtimes.

| Sitting | Hours | Work |
|---|---|---|
| Sep 27 16:38–16:48 | 0.2 | Scoping the e-signable waiver |
| Oct 1 15:58–17:26 | 1.5 | Data model and first build (forms, banner, financials), review, and the review fixes |
| Oct 6 14:57–15:00, 16:11 | 0.1 | Brought up to date with main; migrations renumbered |
| Oct 7 12:34–14:33, 17:41–17:50 | 2.1 | Local test stack; hand-testing with fixes: banner items (optional button, page picker with submenus, opening a document), financials (form reset, file checks on pick), `/website` page |
| Oct 8 10:19–11:04, 13:57–14:50 | 1.6 | Withdrawal alert; credential design (per-part credentials, sub-types, grouped grant menu); past events and galleries; captions |
| Oct 9 08:22–09:15 | 0.9 | Grant menu fixes inside dialogs, activity log names, merge with `pjol/vulnerability-fixes` |

The per-feature split within a sitting is apportioned by message and file times and is approximate;
the sitting totals are measured. Work was done with Claude Code, so most of it is review, testing
and decisions rather than typing.

**Volume:** app 36 files, +9,056/−31 (4 migrations, ~20 new routes); webpage 32 files, +1,755/−109.

## Features, largest first

| Feature | Repo |
|---|---|
| Forms and Waivers: public signing (drawn or typed signature, guardian section, email copy), admin authoring with immutable versions, open/close with auto-close, signatures list, CSV, printable record, withdrawal marking, QR code | app + webpage |
| Homepage banner items (the Spotlight carousel) editable as a list: add, reorder, on/off, photo upload, optional button whose link is picked from the site's own page list (`/api/pages` on the website) or typed, optional recurring-event link; append-only history with restore | app + webpage |
| Financial documents editor: upload, auto-filing by statement date, edit, soft-delete with restore; impact reports in their own block | app + webpage |
| Past events: the volunteers page's archive (renamed from "Earlier events") moves to the backend with a photo gallery page per event (`/volunteers/past/<slug>`), outside sign-up links removed; editor to create events, set tile photos, and add/reorder/caption/remove gallery photos; 15 existing tiles seeded (migration 1.64) | app + webpage |
| Permissions: private credentials, `website_editor` for everything plus one per part (banner, financials, forms, past events), granted like any credential (improvers only) | app |
| Credential sub-types: `credential_type_definitions.parent_value` (migration 1.63), "List under" in Credential types, and a grant menu with submenus (`components/credentials/credential-picker.tsx`) | app |
| Withdrawal alert email to staff (`SITE_WITHDRAWAL_ALERT_EMAIL`, default admin@sfluv.org) | app |
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

Migrations **1.61** (website tables), **1.62** (seed: documents, slides, forms), **1.63**
(`credential_type_definitions.parent_value`, the website credentials) and **1.64** (past events tables,
the past-events credential, the 15 seeded tiles). New public routes under `/site/*` and admin routes
under `/admin/site/*`.

1.63 changes a shared table: it adds a nullable column to `credential_type_definitions` (empty for every
existing credential, so nothing about them changes) and adds five private credential types.

## Note on migration numbers

This branch first used 1.59 and 1.60; PJ's merchant-unwrap and workflow work took those
numbers on main (2026-10-03), so these were renumbered to 1.61 and 1.62 when the branch was
brought up to date.

## Deploying

This branch is merged with `pjol/vulnerability-fixes` (Go 1.25, dependency upgrades) on
`sanchezo/website-cms-with-vuln-fixes`, so both deploy together: the VM needs Go 1.25, and
`update-production-backend.sh` now runs `go mod download` first. Migrations 1.61–1.64 run on start.

The backend (`api.sfluv.org`) deploys by hand to the VM and **must go first**: the website's forms
pages report "unavailable" until `/site/*` exists. Financials and the carousel fall back to the
copy in the website repo until then. The website auto-deploys from `main`, so merge it last.

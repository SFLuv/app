# Website editing tools + Forms and Waivers

Lets non-technical staff change parts of sfluv.org from the admin panel
(app.sfluv.org → Admin → **Website**) with no code push, and lets the public
sign electronic forms and waivers on the site.

Status: built on branch `sanchezo/website-cms` (this repo) and
`feat/forms-and-site-editing` (SFLuv/webpage). **Not merged, not deployed.**

## Decisions

| Question | Decision |
|---|---|
| Who edits | Sanchez's mom, via the admin panel. Admins can do everything. |
| Scoping | Per capability, via the existing credential system (see Permissions). |
| Publishing | Instant. Every edit is undoable. |
| Forms authoring | Editor can write/paste waiver text and toggle standard fields. No drag-and-drop builder. |
| Form lifetime | Manual open/close, plus an optional automatic close date. |
| Withdrawal | A second form kind (`withdrawal`) in the same section. |

## Permissions

Three capability flags, stored as **private credential types** (so a person
cannot request them for themselves; they are not issued through the improver
flow):

| Credential type | Grants |
|---|---|
| `website_banner` | edit the homepage highlights (the rotating Spotlight carousel) |
| `website_financials` | upload/remove financial statements and impact reports |
| `website_forms` | create/edit/open/close forms; read signatures |

A request is allowed when the caller **is an admin, or holds the active
credential** for that capability. Admins bypass as everywhere else in the app.
Signatures contain personal data (and minors' guardians), so the forms
capability is deliberately separate from the other two.

The admin UI reads `GET /admin/site/me` (the caller's capabilities) and shows
only the tabs they may use.

## Tables (app database, migrations 1.61 and 1.62)

### `site_files`
Public uploads: PDFs and images used on the site. Same bytes-in-Postgres
pattern as partner logos. **Never** holds signatures.

| column | type | notes |
|---|---|---|
| id | text PK | uuid |
| filename | text | original name, sanitized; used in the public URL |
| content_type | text | sniffed from the bytes, not trusted from the browser |
| size_bytes | int | |
| data | bytea | |
| uploaded_by | text → users | |
| created_at | bigint | unix |

Allowed types: PDF, PNG, JPEG, WebP, GIF. Max 25 MB.

### `site_content` + `site_content_versions`
Small singleton documents edited as a whole. Today: key `spotlight`, the list of slides in the homepage carousel.

`site_content(key PK, value jsonb, version int, updated_by, updated_at)`
`site_content_versions(id, key, version, value jsonb, saved_by, saved_at, note)` — append only.

Every save writes a new version. **Undo = restore**: it copies an old
version's value into a *new* version, so history is never rewritten.

`spotlight` value (the order of `slides` is the order visitors see):

```json
{ "slides": [ {
  "id": "impact-report-2025-2026",      // assigned by the server
  "enabled": true,                       // off = hidden, kept for later
  "label": "Now published",              // small tag above the title
  "title": "…", "body": "plain text; [label](url) links allowed",
  "image_file_id": "uuid | null",        // an upload, or…
  "image_url": "/assets/… | null",       // …a picture already on the site (seeded slides)
  "image_width": 1050, "image_height": 817,
  "image_alt": "…", "image_position": "center 8%",   // which part of the photo the strip keeps
  "action": { "label": "Read the report", "href": "/financials-and-reports#annual-impact-reports",
              "new_tab": false, "also_open_file_id": "uuid | null", "also_open_url": "string | null" },
  "event_match": "weekly clean"          // optional: words from a recurring volunteer event's title
} ] }
```

A slide that is switched **on** must be complete (title, photo, button); one that is
switched off may be a draft. Migration 1.62 seeds the two slides the site shows today.

### `financial_documents`

| column | type | notes |
|---|---|---|
| id | text PK | uuid |
| kind | text | `activity`, `cash_flows`, `financial_position`, `activity_comparison`, `form_990n`, `form_199n`, `impact_report`, `other` |
| fiscal_year | int | the year the fiscal year **ends** — 2026 = FYE June 30, 2026 |
| period | text | `Q1` `Q2` `Q3` `Q4` `FULL` |
| as_of | text null | `YYYY-MM-DD`, for statements |
| label | text | shown on the site. Generated from kind + as_of unless overridden |
| file_id | text null → site_files | uploaded document |
| external_url | text null | legacy document already hosted on the site (relative path) |
| removed_at | bigint null | soft delete; **undo = restore** |
| created_by, created_at, updated_at | | |

`CHECK` exactly one of `file_id` / `external_url` is set. Ordering inside a
period is by kind (activity, cash flows, financial position, comparison, 199-N,
990-N, other), then creation time. `impact_report` rows are not shown in the
fiscal-year accordion; they populate the **Annual Impact Reports** block.

Migration 1.62 seeds every document currently hard-coded in the website's
`src/content/financials.ts`, so nothing is lost on cutover.

### `site_forms`

| column | type | notes |
|---|---|---|
| id | text PK | uuid |
| slug | text unique | public URL: `/forms/<slug>` |
| kind | text | `waiver` or `withdrawal` |
| is_open | bool | manual switch |
| closes_at | bigint null | optional automatic close |
| current_version | int | newest version; this is what the public sees |
| opened_at, archived_at, created_by, created_at, updated_at | | |

Effective state = **open** when `is_open` and (`closes_at` is null or in the
future); otherwise **closed**. A never-opened form is a draft.

### `site_form_versions` (immutable)
Editing a form **never** changes a version in place — it adds a new one, and
`site_forms.current_version` moves forward. This is what lets an editable
waiver stay legally meaningful: every signature points at the exact text that
person agreed to.

| column | type | notes |
|---|---|---|
| form_id, version | PK | |
| title | text | |
| summary | text | one-line blurb on the forms list |
| body | text | waiver text. `## ` starts a heading, blank line = paragraph, `- ` = bullet |
| config | jsonb | standard-field switches, below |
| saved_by, saved_at | | |

`config`:

```json
{
  "contact": "off | optional | required",
  "preferred_name": true,
  "event": "off | ask | fixed",
  "event_name": "Boundless Grant video",
  "choices_prompt": "Choose the scope of your consent",
  "choices": [ { "id": "event_only", "label": "This event/project only.", "description": "…" } ],
  "guardian_section": true,
  "confirmation_message": "Thank you — your signed form has been recorded."
}
```

`choices`, when present, are pick-exactly-one and required.

### `site_form_signatures`

| column | type | notes |
|---|---|---|
| id | text PK | uuid |
| form_id, version | FK → site_form_versions | the text that was signed |
| signer_name | text | |
| contact | text | email or phone, per `config.contact` |
| answers | jsonb | `preferred_name`, `event`, `event_date`, `choice` |
| is_minor | bool | |
| guardian_name, guardian_relationship | text | |
| signature_png | bytea | drawn signature |
| guardian_signature_png | bytea null | |
| esign_consent | bool | "I agree to sign electronically" |
| text_sha256 | text | hash of title + body + choices as signed |
| signed_at | bigint | server time, never client-supplied |
| client_ip, user_agent | text | audit trail |
| withdrawn_at | bigint null | set by an admin when a withdrawal is processed |

Signature images are excluded from list queries and only loaded for a single
record's detail/print view.

### `site_activity`
Append-only audit trail shown as "Recent changes" in the admin UI:
`(id, actor_user_id, capability, action, entity, entity_id, summary, at)`.

## API

### Public (no auth; cached ≤ 30 s on the site)

| route | returns |
|---|---|
| `GET /site/spotlight` | the enabled, complete slides, images resolved to URLs |
| `GET /site/financials` | `{ years: [...], impact_reports: [...] }`, ready to render |
| `GET /site/files/{id}/{filename}` | the file bytes, inline |
| `GET /site/forms` | open forms |
| `GET /site/forms/{slug}` | form + current version; `{ status: "closed" }` once closed; 404 if unknown |
| `POST /site/forms/{slug}/sign` | records a signature (rate-limited, honeypot) |

### Admin (`withAdmin` or the capability credential)

```
GET    /admin/site/me
GET    /admin/site/activity
POST   /admin/site/files                       multipart "file"

GET    /admin/site/spotlight                  current + versions
PUT    /admin/site/spotlight                  replaces the whole list
POST   /admin/site/spotlight/restore          { version }

GET    /admin/site/financials                  includes removed
POST   /admin/site/financials
PUT    /admin/site/financials/{id}
DELETE /admin/site/financials/{id}             soft delete
POST   /admin/site/financials/{id}/restore

GET    /admin/site/forms
POST   /admin/site/forms
GET    /admin/site/forms/{id}                  + versions
PUT    /admin/site/forms/{id}                  adds a new version
POST   /admin/site/forms/{id}/open             { closes_at? }
POST   /admin/site/forms/{id}/close
GET    /admin/site/forms/{id}/signatures       list (no images)
GET    /admin/site/forms/{id}/signatures.csv
GET    /admin/site/signatures/{sid}            full record incl. images
POST   /admin/site/signatures/{sid}/withdraw   mark withdrawn
```

## Website behaviour

* The homepage carousel, the financials page and the forms pages fetch from the API with a
  30-second revalidate, so an edit appears within about half a minute.
* If the API is unreachable, the site falls back: financials to the copy in
  `src/content/financials.ts`; the carousel to the slides in `src/content/spotlight.ts` if
  the backend predates the feature, and to nothing on any other failure; forms to a
  "temporarily unavailable" notice. It never blanks the page.
* `POST /api/forms/[slug]/sign` on the website proxies to the backend and
  forwards the visitor's IP with the shared proxy key — the same mechanism
  volunteer signups use.
* New page **Forms and Waivers** (About menu, after Financials and Reports):
  lists open forms, or says nothing needs signing right now.

## Safeguards on the public signing endpoint

* A signature image's dimensions are read from its header **before** it is
  decoded (max 2400x1200). The PNG decoder allocates the full claimed size up
  front, so skipping this lets a tiny file exhaust the server's memory.
* Per address: 60 signatures and 300 attempts per 10 minutes. Only successful
  signatures count toward the 60, so typos never lock anyone out.
* Confirmation emails: at most 3 per recipient per day, so a stranger cannot use
  the form to send SFLuv mail to someone else. The response says "emailed" only
  when the mail actually went out.
* Rate limiting is per visitor only when the website and backend share the proxy
  key (`SFLUV_VOLUNTEER_PROXY_KEY` = `VOLUNTEER_PROXY_KEY`); otherwise every web
  signer shares one bucket. **Check this is set in production before an event.**

## Concurrency and storage

* Saving the highlights carries the version the editor started from; if someone
  else saved in between, the save is refused with a plain message instead of
  silently overwriting them. Restore is a deliberate overwrite and skips this.
* Uploads nothing refers to (no financial document, no version of any content)
  are deleted after a day; the sweep runs after each upload. Files are served
  from a 64 MB in-memory cache, since they never change once stored.

## Privacy

* Signatures, contacts and guardian details are admin-only. No public listing,
  and signature images are never served from `/site/files`.
* The signer receives a confirmation email containing the exact text they
  agreed to, when they gave an email address.
* Retention policy is unset. Recommend deciding one before the first public
  use (a media release is typically kept for as long as the media is used).

## Out of scope for this branch

* Mobile-app Participate section (the API is ready for it).
* Server-generated PDFs. The admin signature view is print-friendly
  (browser "Save as PDF") instead.
* Lawyer review of the seeded media-release wording. It ships as a **draft**,
  closed, until someone opens it.

# Files stored in Postgres

Every file this platform holds is a `BYTEA` column in the app database. There is
no object storage and no CDN. This document is the complete list, written so that
the move to a bucket is a checklist rather than a rediscovery.

**Read this before adding another one.** If you are adding a file column, add a
row here in the same commit. The cost of this document is one row per feature;
the cost of not having it is finding the eleventh table during a migration that
has already started.

## Why it is like this

Bytes in Postgres is the right trade exactly once: at the start, when there is
one store to back up, no credentials to hold, no signed-URL expiry to reason
about, and the volume is small enough that nobody notices. Every one of those
advantages is an advantage of being small.

What it costs, and will keep costing:

- **Backups.** A logical dump carries every byte of every photo. The database is
  mostly images by size and mostly rows by value, and the dump time is set by the
  images.
- **Memory.** Postgres reads a row through shared buffers, so serving a 4 MB
  photo evicts index pages that something else wanted. Files compete with queries
  for the same cache.
- **Replication.** Every byte crosses the WAL. A bulk photo import is a
  replication lag event.
- **No CDN.** Every read is a round trip to the application and then to the
  database. There is no edge cache, and the `Cache-Control` headers the handlers
  set only help the client that already asked once.

None of that is urgent at current volume. All of it is why the list below exists.

## The inventory

Sizes are the CHECK constraint where there is one, and the handler's limit where
there is not.

| Table | Column | Max | Written by | Read by | Notes |
|---|---|---|---|---|---|
| `message_attachments` | `file_data` | 25 MB | `POST /conversations/{id}/attachments` | `GET /conversations/{id}/attachments/{attachmentId}` | Chat photos and files. **The largest by far** — the only one that accepts arbitrary file types, and the only one a user can add to without limit. First to move. |
| `bulletin_post_photos` | `image_data` | 4 MB | `POST /bulletin/posts/{id}/photos` | `GET /bulletin/photos/{photoId}` | Up to 6 per post. |
| `contact_avatars` | `image_data` | 4 MB | `PUT /contacts/{id}/avatar` | `GET /contacts/{id}/avatar` | One per contact. Private to the owner. |
| `profile_avatars` | `image_data` | 4 MB | `PUT /me/profile/avatar` | `GET /users/{userId}/avatar` | One per user. Visible to anyone who can see the profile. |
| `workflow_submission_photos` | `photo_data` | handler | `POST /improvers/workflows/{id}/steps/{stepId}/photos` | `GET /workflow-photos/{photoId}`, `GET /workflow-photos/public/{photoId}` | Proof-of-work photos. Has a public read route — check the authorization carefully when moving it. |
| `event_photos` | `photo_data` | handler | `POST /admin/volunteer-events/{id}/photos`, `POST /volunteer-events/staged-photos` | `GET /volunteer-events/photos/{photo_id}` | Event cover photos. Public read. |
| `event_blast_images` | `image_data` | handler | `POST /admin/volunteer-events/{id}/blast/images`, `POST /affiliates/volunteer-events/{id}/blast/images` | `GET /volunteer-events/blast-images/{image_id}` | Images in organizer blasts. Public read, because they are embedded in email. |
| `location_icons` | `image_data` | handler | `POST /locations/{id}/icon` | `GET /locations/{id}/icon` | Merchant map pins. Public read. |
| `location_photos` | `image_data` | handler | `POST /locations/{id}/photo` | `GET /locations/{id}/photo` | Merchant storefront photos. Public read. |
| `partners` | `logo_data` | handler | `POST /admin/partners/{id}/logo` | `GET /partners/{id}/logo` | Partner banner logos. Public read. |
| `site_files` | `data` | handler | website editor | `GET` by id | PDFs and images for the public website. Public read by id, so an id is the only access control. Referenced by `site_past_event_photos.file_id`. |
| `site_form_signatures` | `signature_png`, `guardian_signature_png` | handler | website form submission | **never served** | Signature images on signed waivers. Deliberately not readable over HTTP. **Do not move these to a bucket** without deciding what their access model is — they are the one entry here that is evidence rather than content. |

### Not file storage, but file-shaped

These hold a URL or a data URI in a `TEXT` column. They do not need a bucket
migration, but a data URI in them is a file in the database by another name, and
they are listed so the next audit does not have to work that out again.

| Table | Column | Notes |
|---|---|---|
| `organizations` | `logo` | `TEXT`. Served through `GET /organizers/{id}/logo`, which gates on the org having a published event. |
| `affiliates` | `affiliate_logo` | `TEXT`. |

## Moving to a bucket

The order that minimises risk, because it takes the largest and least
load-bearing things first:

1. **`message_attachments.file_data`.** Biggest, newest, fewest readers, and the
   only one with no public route. The chat beta is small, so a backfill is
   minutes.
2. **`event_photos`, `event_blast_images`, `location_photos`, `location_icons`,
   `partners.logo_data`, `site_files`.** Public reads, so these are the ones a
   CDN actually helps. They are also the ones where a signed URL is *not* needed,
   which makes them simpler than they look.
3. **`bulletin_post_photos`, `contact_avatars`, `profile_avatars`.** Private or
   semi-private, so each needs a decision about signed URLs versus proxying.
4. **`workflow_submission_photos`.** Last, because it has both a public and an
   authenticated route onto the same bytes, and that distinction has to survive
   the move.
5. **`site_form_signatures`.** Probably never. Decide deliberately.

Each step is the same four changes, and the shape is worth stating once:

- Add a nullable `storage_key TEXT` beside the bytes column.
- Write to the bucket and the column together on upload; keep writing bytes until
  the backfill is done, so a rollback loses nothing.
- Backfill, then switch reads to prefer `storage_key`.
- Only then drop the bytes column, in its own migration, after a release has run
  without it.

The reason the bytes column is dropped last and separately is that it is the only
irreversible step. Everything before it can be rolled back by deploying the
previous binary.

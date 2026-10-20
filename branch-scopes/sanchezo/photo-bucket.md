# Branch scope — `sanchezo/photo-bucket`

Oct 20 2026 · app (backend) · **~0.3h active**

Hours are wall-clock time from this session's timestamps (about 09:38 to 09:55 PDT), not measured with the
`time-accounting` script.

## What this branch is

Steps 1–3 of moving image bytes out of Postgres into a Railway bucket (plan: `handoff/sfluv-photos-to-bucket-plan.md`).
On Oct 19 the `app` database was about 3 GB, almost all of it image `BYTEA`.

**Nothing changes on deploy.** The bucket is off until `BLOB_BUCKET` is set, and copying is off until
`BLOB_BACKFILL=true` is set as well. Postgres keeps every byte; deleting them is a later, separate step.

- **`backend/blobstore`**: an S3 client (minio-go) configured from `BLOB_*` variables. Keys are append-only
  (`<table>/<row id>/<random>`) and never overwritten.
- **Migration 1.66**: a nullable `object_key` column on 8 image tables (6 in app, 2 in bot). It's a catalog-only change.
- **Reads**: each image read selects `object_key`. With a bucket configured and a key set, the bytes come from the
  bucket; if that read fails, they come from the Postgres copy. Without a bucket, nothing changes. URLs and responses are unchanged.
- **Writes**: unchanged; uploads still go to Postgres. Replacing a location icon, location photo or partner logo
  clears `object_key`, so the old bucket copy is never served. Copies of workflow uploads and cloned event covers
  carry the key with them.
- **Backfill**: a leader-only job (`bootstrap/blob_backfill.go`) runs every 2 minutes. It copies 50 rows per table per pass,
  reads each object back, checks its SHA-256, and only then sets the key. The update is guarded so a row whose bytes
  changed mid-copy is retried. It keeps running to pick up new uploads, because writes still land in Postgres.
- **Rollback**: unset `BLOB_BUCKET` and every image is served from Postgres again.

Not covered here, on purpose: waiver signatures (stay in Postgres), upload chunks (temporary), credential badges and
org/affiliate logos (base64 inlined in API responses; they need their own change). Objects for deleted rows are not
swept yet.

| Item | Hours | Repo |
|---|---|---|
| `blobstore` package, migration, read paths on 8 tables, write-path key clearing, backfill job, startup wiring; tested on fresh Postgres 18 plus an in-process fake S3: migration on all 8 tables, backfill, bucket read, fallback when the object is missing, bucket off, new rows picked up | 0.3 | app |

Volume: 15 files (4 new), 1 migration (1.66), 0 routes, 1 dependency (`github.com/minio/minio-go/v7`).

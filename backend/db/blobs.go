package db

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/SFLuv/app/backend/blobstore"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BlobTable is one image table moving from Postgres BYTEA into the bucket.
// Identifiers are constants, never request input.
type BlobTable struct {
	Database string // "app" or "bot"
	Table    string
	PK       string
	BytesCol string
	TypeCol  string
}

// BlobTables lists every image table the backfill copies. Deliberately left
// out: waiver signatures (small legal records that stay with their row),
// in-progress upload chunks (temporary), credential badges and org/affiliate
// logos (inlined as base64 in API responses; they need their own change).
var BlobTables = []BlobTable{
	{Database: "app", Table: "workflow_submission_photos", PK: "id", BytesCol: "photo_data", TypeCol: "content_type"},
	{Database: "app", Table: "workflow_step_photo_uploads", PK: "id", BytesCol: "photo_data", TypeCol: "content_type"},
	{Database: "app", Table: "site_files", PK: "id", BytesCol: "data", TypeCol: "content_type"},
	{Database: "app", Table: "location_photos", PK: "location_id", BytesCol: "image_data", TypeCol: "content_type"},
	{Database: "app", Table: "location_icons", PK: "location_id", BytesCol: "image_data", TypeCol: "content_type"},
	{Database: "app", Table: "partners", PK: "id", BytesCol: "logo_data", TypeCol: "logo_content_type"},
	{Database: "bot", Table: "event_photos", PK: "id", BytesCol: "photo_data", TypeCol: "content_type"},
	{Database: "bot", Table: "event_blast_images", PK: "id", BytesCol: "image_data", TypeCol: "content_type"},
}

func (a *AppDB) SetBlobStore(store blobstore.Store) { a.blobs = store }
func (s *BotDB) SetBlobStore(store blobstore.Store) { s.blobs = store }

// resolveBlob returns a row's image bytes: from the bucket when the row has an
// object key and the bucket is configured, otherwise from Postgres. If the
// bucket read fails, the Postgres copy is still there until the cleanup step,
// so it is served instead and the failure is returned only when no copy exists.
func resolveBlob(ctx context.Context, store blobstore.Store, key *string, fallback []byte) ([]byte, error) {
	if store == nil || key == nil || *key == "" {
		return fallback, nil
	}
	data, err := store.Get(ctx, *key)
	if err == nil {
		return data, nil
	}
	if len(fallback) > 0 {
		return fallback, nil
	}
	return nil, fmt.Errorf("reading %s from the bucket: %w", *key, err)
}

// BackfillResult reports one pass over one table.
type BackfillResult struct {
	Table   string
	Copied  int
	Failed  int
	Pending int
}

// BackfillBlobTable copies up to limit rows that have bytes but no object key
// into the bucket. Each row is read alone (photos can be several MB), uploaded
// under a fresh key, read back and compared by SHA-256, and only then given
// its key — and only if its bytes did not change in the meantime. The Postgres
// bytes are left in place. A failed row is skipped for this pass and retried
// on the next one.
func BackfillBlobTable(ctx context.Context, pool *pgxpool.Pool, store blobstore.Store, t BlobTable, limit int) (BackfillResult, error) {
	res := BackfillResult{Table: t.Table}
	table := pgx.Identifier{t.Table}.Sanitize()
	pk := pgx.Identifier{t.PK}.Sanitize()
	col := pgx.Identifier{t.BytesCol}.Sanitize()
	typ := pgx.Identifier{t.TypeCol}.Sanitize()

	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT %s::text FROM %s
		WHERE object_key IS NULL AND %s IS NOT NULL
		ORDER BY %s
		LIMIT $1
	`, pk, table, col, pk), limit)
	if err != nil {
		return res, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return res, err
	}

	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if err := backfillBlobRow(ctx, pool, store, t, table, pk, col, typ, id); err != nil {
			res.Failed++
			continue
		}
		res.Copied++
	}

	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM %s WHERE object_key IS NULL AND %s IS NOT NULL
	`, table, col)).Scan(&res.Pending); err != nil {
		return res, err
	}
	return res, nil
}

func backfillBlobRow(ctx context.Context, pool *pgxpool.Pool, store blobstore.Store, t BlobTable, table, pk, col, typ, id string) error {
	var data []byte
	var contentType string
	err := pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s, COALESCE(%s, '') FROM %s
		WHERE %s::text = $1 AND object_key IS NULL AND %s IS NOT NULL
	`, col, typ, table, pk, col), id).Scan(&data, &contentType)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // already copied or removed since the batch was listed
	}
	if err != nil {
		return err
	}

	sum := sha256.Sum256(data)
	key := blobstore.NewKey(t.Table, id)
	if err := store.Put(ctx, key, contentType, data); err != nil {
		return fmt.Errorf("put %s: %w", key, err)
	}
	back, err := store.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("read back %s: %w", key, err)
	}
	if backSum := sha256.Sum256(back); !bytes.Equal(backSum[:], sum[:]) {
		return fmt.Errorf("read back %s: checksum mismatch", key)
	}

	// Guarded on the bytes being unchanged: an image replaced mid-copy keeps
	// no key and is picked up again on the next pass.
	_, err = pool.Exec(ctx, fmt.Sprintf(`
		UPDATE %s SET object_key = $2
		WHERE %s::text = $1 AND object_key IS NULL AND sha256(%s) = $3
	`, table, pk, col), id, key, sum[:])
	return err
}

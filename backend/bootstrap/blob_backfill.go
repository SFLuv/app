package bootstrap

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/blobstore"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	blobBackfillInterval = 2 * time.Minute
	blobBackfillBatch    = 50
)

// StartBlobBackfill copies image bytes from Postgres into the bucket, a batch
// per table every couple of minutes, until every row has an object key. It
// keeps running afterwards because uploads still land in Postgres first; new
// rows are picked up on the next pass. Postgres bytes are never removed here.
//
// It runs only with a bucket configured AND BLOB_BACKFILL=true, so the bucket
// can be connected and checked before anything is copied. Leader-only.
func StartBlobBackfill(ctx context.Context, pools *DBPools, store blobstore.Store, appLogger *logger.LogCloser) {
	if store == nil || strings.TrimSpace(os.Getenv("BLOB_BACKFILL")) != "true" {
		return
	}
	go func() {
		appLogger.Logf("image backfill: started")
		complete := false
		for {
			total, pending := runBlobBackfillPass(ctx, pools, store, appLogger)
			switch {
			case pending == 0 && !complete:
				appLogger.Logf("image backfill: every image is in the bucket")
				complete = true
			case pending > 0:
				complete = false
				if total > 0 {
					appLogger.Logf("image backfill: copied %d this pass, %d still to copy", total, pending)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(blobBackfillInterval):
			}
		}
	}()
}

func runBlobBackfillPass(ctx context.Context, pools *DBPools, store blobstore.Store, appLogger *logger.LogCloser) (copied, pending int) {
	for _, t := range db.BlobTables {
		pool := pools.App
		if t.Database == "bot" {
			pool = pools.Bot
		}
		res, err := backfillTable(ctx, pool, store, t)
		if err != nil {
			if ctx.Err() == nil {
				appLogger.Logf("image backfill: %s: %s", t.Table, err)
			}
			pending++ // unknown; keep the job from declaring itself complete
			continue
		}
		if res.Failed > 0 {
			appLogger.Logf("image backfill: %s: %d rows failed this pass and will be retried", t.Table, res.Failed)
		}
		copied += res.Copied
		pending += res.Pending
	}
	return copied, pending
}

func backfillTable(ctx context.Context, pool *pgxpool.Pool, store blobstore.Store, t db.BlobTable) (db.BackfillResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return db.BackfillBlobTable(runCtx, pool, store, t, blobBackfillBatch)
}

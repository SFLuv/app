package db

import (
	"context"
	"fmt"

	"github.com/SFLuv/app/backend/logger"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PonderDB struct {
	db     *pgxpool.Pool
	logger *logger.LogCloser
}

func Ponder(db *pgxpool.Pool, logger *logger.LogCloser) *PonderDB {
	return &PonderDB{db, logger}
}

func (p *PonderDB) Ping() error {
	return p.db.Ping(context.Background())
}

// GetHookIDs returns the id of every webhook Ponder currently holds.
//
// Read-only by design: the Ponder database belongs to the indexer, and writing
// to it changes its schema out from under the running process. The backend is
// only ever allowed to look.
func (p *PonderDB) GetHookIDs(ctx context.Context) (map[int]struct{}, error) {
	rows, err := p.db.Query(ctx, `SELECT id FROM ponder_hooks;`)
	if err != nil {
		return nil, fmt.Errorf("error querying ponder hooks: %w", err)
	}
	defer rows.Close()

	hookIDs := map[int]struct{}{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("error scanning ponder hook id: %w", err)
		}
		hookIDs[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating ponder hook ids: %w", err)
	}

	return hookIDs, nil
}

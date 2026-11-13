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

// PonderHook is one webhook as Ponder stores it.
type PonderHook struct {
	ID      int
	Address string
	URL     string
}

// GetHooks returns every webhook Ponder currently holds, by id.
//
// Read-only by design: the Ponder database belongs to the indexer, and writing
// to it changes its schema out from under the running process. The backend is
// only ever allowed to look; hooks are created and deleted through Ponder's own
// API.
func (p *PonderDB) GetHooks(ctx context.Context) (map[int]PonderHook, error) {
	rows, err := p.db.Query(ctx, `SELECT id, address, url FROM ponder_hooks;`)
	if err != nil {
		return nil, fmt.Errorf("error querying ponder hooks: %w", err)
	}
	defer rows.Close()

	hooks := map[int]PonderHook{}
	for rows.Next() {
		var hook PonderHook
		if err := rows.Scan(&hook.ID, &hook.Address, &hook.URL); err != nil {
			return nil, fmt.Errorf("error scanning ponder hook: %w", err)
		}
		hooks[hook.ID] = hook
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating ponder hooks: %w", err)
	}

	return hooks, nil
}

package db

import (
	"context"

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

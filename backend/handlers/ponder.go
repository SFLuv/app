package handlers

import (
	"net/http"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
)

type PonderService struct {
	db            *db.PonderDB
	appDB         *db.AppDB
	botDB         *db.BotDB
	logger        *logger.LogCloser
	activeChainID int64
}

func NewPonderService(db *db.PonderDB, appDB *db.AppDB, botDB *db.BotDB, logger *logger.LogCloser, activeChainID int64) *PonderService {
	return &PonderService{
		db:            db,
		appDB:         appDB,
		botDB:         botDB,
		logger:        logger,
		activeChainID: activeChainID,
	}
}

func (p *PonderService) requestChainID(r *http.Request) int64 {
	if r != nil {
		if chainID := parsePositiveInt64(r.URL.Query().Get("chain_id")); chainID > 0 {
			return chainID
		}
	}
	if p != nil && p.activeChainID > 0 {
		return p.activeChainID
	}
	return 80094
}

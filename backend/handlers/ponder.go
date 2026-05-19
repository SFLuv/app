package handlers

import (
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
)

type PonderService struct {
	db     *db.PonderDB
	appDB  *db.AppDB
	botDB  *db.BotDB
	logger *logger.LogCloser
}

func NewPonderService(db *db.PonderDB, appDB *db.AppDB, botDB *db.BotDB, logger *logger.LogCloser) *PonderService {
	return &PonderService{
		db:     db,
		appDB:  appDB,
		botDB:  botDB,
		logger: logger,
	}
}

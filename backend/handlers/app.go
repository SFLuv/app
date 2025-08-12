package handlers

import (
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
)

type AppService struct {
	db     *db.AppDB
	logger *logger.LogCloser
}

func NewAppService(db *db.AppDB, logger *logger.LogCloser) *AppService {
	return &AppService{db, logger}
}

package handlers

import (
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
)

type AppService struct {
	db       *db.AppDB
	w9       *W9Service
	redeemer *RedeemerService
	logger   *logger.LogCloser
}

func NewAppService(db *db.AppDB, logger *logger.LogCloser, w9 *W9Service) *AppService {
	return &AppService{db: db, logger: logger, w9: w9}
}

func (a *AppService) SetRedeemerService(redeemer *RedeemerService) {
	a.redeemer = redeemer
}

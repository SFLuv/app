package handlers

import (
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
)

type PonderService struct {
	db     *db.PonderDB
	logger *logger.LogCloser
}

func NewPonderService(db *db.PonderDB, logger *logger.LogCloser) *PonderService {
	return &PonderService{db, logger}
}

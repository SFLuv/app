package handlers

import "github.com/SFLuv/app/backend/db"

type AppService struct {
	db *db.AppDB
}

func NewAppService(db *db.AppDB) *AppService {
	return &AppService{db}
}

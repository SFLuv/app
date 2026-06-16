package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/clientconfig"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
)

type AppService struct {
	db           *db.AppDB
	w9           *W9Service
	bot          *BotService
	redeemer     *RedeemerService
	minter       *MinterService
	logger       *logger.LogCloser
	clientConfig *clientconfig.Config
}

func NewAppService(db *db.AppDB, logger *logger.LogCloser, w9 *W9Service, clientConfig *clientconfig.Config) *AppService {
	return &AppService{db: db, logger: logger, w9: w9, clientConfig: clientConfig}
}

func (a *AppService) activeChainID() int64 {
	if a != nil && a.clientConfig != nil {
		if chainID := a.clientConfig.ActiveChainID(); chainID > 0 {
			return int64(chainID)
		}
	}
	return 80094
}

func (a *AppService) SetRedeemerService(redeemer *RedeemerService) {
	a.redeemer = redeemer
}

func (a *AppService) SetBotService(bot *BotService) {
	a.bot = bot
}

func (a *AppService) SetMinterService(minter *MinterService) {
	a.minter = minter
}

func (a *AppService) RecordAnalyticsUserActivity(ctx context.Context, userID string, r *http.Request) {
	if a == nil || a.db == nil {
		return
	}
	platform := "web"
	if r != nil {
		if header := strings.TrimSpace(r.Header.Get("X-SFLUV-Client-Platform")); header != "" {
			platform = header
		} else if strings.Contains(strings.ToLower(r.UserAgent()), "mobile") {
			platform = "mobile"
		}
	}
	if err := a.db.RecordAnalyticsUserActivity(ctx, userID, platform, time.Now().UTC()); err != nil && a.logger != nil {
		a.logger.Logf("error recording analytics user activity for %s: %s", userID, err)
	}
	a.recordClientVersionObservation(ctx, userID, "authenticated_request", r)
}

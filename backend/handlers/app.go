package handlers

import (
	"context"
	"github.com/SFLuv/app/backend/bridge"
	"net/http"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/clientconfig"
	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/realtime"
	"github.com/SFLuv/app/backend/w9provider"
)

type AppService struct {
	db           *db.AppDB
	payouts      *PayoutService
	w9Provider   w9provider.Provider
	bridge       *bridge.Client
	bot          *BotService
	redeemer     *RedeemerService
	minter       *MinterService
	logger       *logger.LogCloser
	clientConfig *clientconfig.Config
	// Read-only. Used to compare our webhook bookkeeping against what the
	// indexer actually holds; the Ponder database is never written to here.
	ponderDb *db.PonderDB
	// notifications posts internal organization alerts. Nil-safe at every
	// method, so call sites post without guarding it.
	notifications *NotificationService
	// uptime is the health poller, consulted by the acknowledge button so an
	// outage cannot be acknowledged while it is still happening.
	uptime *UptimeService
	// protocolSvc owns the backend's own Safes and the keys that sign for them.
	// Wired after construction because it needs the client config this service
	// already holds.
	protocolSvc *ProtocolService
	// realtime fans events out to foregrounded clients. Nil until wired, and
	// every use goes through the publish helpers in app_chat_realtime.go, which
	// no-op on nil — a server with no hub still works, it just falls back to
	// pushes and polling for everything.
	realtime *realtime.Hub
	// realtimeServer is the HTTP upgrade handler for the hub above. Built once
	// at startup by BuildRealtimeServer, which is also what sets `realtime`.
	realtimeServer *realtime.Server
}

// SetRealtimeHub wires the websocket fan-out in.
func (a *AppService) SetRealtimeHub(hub *realtime.Hub) {
	a.realtime = hub
}

// RealtimeHub exposes the hub for the server's own wiring and /healthz.
func (a *AppService) RealtimeHub() *realtime.Hub {
	if a == nil {
		return nil
	}
	return a.realtime
}

// SetProtocolService wires the protocol accounts in.
func (a *AppService) SetProtocolService(service *ProtocolService) {
	a.protocolSvc = service
}

// Protocol exposes the service for the boot sequence and the bot wiring.
func (a *AppService) Protocol() *ProtocolService {
	if a == nil {
		return nil
	}
	return a.protocolSvc
}

func NewAppService(db *db.AppDB, logger *logger.LogCloser, payouts *PayoutService, w9Provider w9provider.Provider, clientConfig *clientconfig.Config) *AppService {
	return &AppService{db: db, logger: logger, payouts: payouts, w9Provider: w9Provider, clientConfig: clientConfig}
}

// SetPonderDB gives the service read access to the indexer's database, which
// is what makes the boot-time hook reconciliation possible.
func (a *AppService) SetPonderDB(ponderDb *db.PonderDB) {
	a.ponderDb = ponderDb
}

func (a *AppService) activeChainID() int64 {
	if a != nil && a.clientConfig != nil {
		if chainID := a.clientConfig.ActiveChainID(); chainID > 0 {
			return int64(chainID)
		}
	}
	return 80094
}

// SetNotificationService wires the internal alert layer. Set after
// construction because the service needs the database this one already holds.
func (a *AppService) SetNotificationService(n *NotificationService) { a.notifications = n }

// Notifications exposes the alert layer to the bootstrap wiring, which hands
// it the device pusher that lives on this service.
func (a *AppService) Notifications() *NotificationService { return a.notifications }

// SetUptimeService wires the health poller.
func (a *AppService) SetUptimeService(u *UptimeService) { a.uptime = u }

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

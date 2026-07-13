// Package mcp exposes read-only SFLUV admin reporting over the Model Context
// Protocol (MCP), bolted directly onto the main backend rather than run as a
// separate server.
//
// Security model (the whole point of this package):
//   - Access is gated by an OAuth 2.1 flow whose identity step is SFLUV's own
//     Privy login (bridged through the frontend authorize page). Every issued
//     token is bound to a SFLUV user DID, and admin status is re-checked live on
//     every /mcp request, so revoking admin takes effect immediately. See
//     oauth.go / oauth_store.go.
//   - Every tool is READ ONLY: named reports only (never raw SQL), each query in
//     a read-only Postgres transaction with a statement timeout (reports.go).
//   - No secrets are ever returned: only curated, non-sensitive columns. Never
//     api keys, private keys, encrypted OAuth credentials, push tokens, W9
//     document URLs, or PIN hashes.
package mcp

import (
	"github.com/SFLuv/app/backend/handlers"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/server"
)

// Service is the bolt-on MCP endpoint plus its OAuth authorization server.
// Construct it with New and mount it with RegisterRoutes on the main router.
type Service struct {
	core  *reportCore
	oauth *oauthServer
}

// New builds the MCP service from the running server's existing DB pools and
// AppService. The report tools reuse these pools read-only; the OAuth store uses
// the app pool for its (write) bookkeeping tables.
func New(appDB, botDB, ponderDB *pgxpool.Pool, app *handlers.AppService, chainID int64) *Service {
	return &Service{
		core: &reportCore{
			appDB:    appDB,
			botDB:    botDB,
			ponderDB: ponderDB,
			chainID:  chainID,
		},
		oauth: newOAuthServer(appDB, app),
	}
}

// RegisterRoutes mounts the OAuth/discovery endpoints and the bearer-gated
// /mcp handler onto the router. The MCP tool handler is only reachable through
// oauthServer.requireBearer (valid AS token + live admin), so an unauthenticated
// or non-admin request never touches the database.
func (s *Service) RegisterRoutes(r chi.Router) {
	mcpServer := s.core.newServer()
	s.core.registerExtraTools(mcpServer)
	streamable := server.NewStreamableHTTPServer(mcpServer)
	s.oauth.registerRoutes(r, streamable)
}

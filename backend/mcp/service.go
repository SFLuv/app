// Package mcp exposes read-only SFLUV admin reporting over the Model Context
// Protocol (MCP), bolted directly onto the main backend rather than run as a
// separate server.
//
// Security model (the whole point of this package):
//   - Access is gated by the EXISTING Privy JWT + SFLUV admin check. The MCP
//     bearer token is a Privy access token; this package issues no tokens of its
//     own and stores no credentials. See auth.go.
//   - Every tool is READ ONLY: named reports only (never raw SQL), and every
//     query runs inside a read-only Postgres transaction with a statement
//     timeout (see withReadOnlyTx in reports.go).
//   - No secrets are ever returned: this package only selects curated,
//     non-sensitive columns. It never touches api keys, private keys, encrypted
//     OAuth credentials, push tokens, W9 document URLs, or PIN hashes.
package mcp

import (
	"net/http"

	"github.com/SFLuv/app/backend/handlers"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mark3labs/mcp-go/server"
)

// Service is the bolt-on MCP endpoint. Construct it with New and mount its
// Handler on the main router at /mcp.
type Service struct {
	core *reportCore
	gate *authGate
}

// New builds the MCP service from the running server's existing DB pools and
// AppService. It reuses the same pools the rest of the backend uses; the
// read-only guarantee comes from wrapping every query in a read-only
// transaction, not from a separate connection.
func New(appDB, botDB, ponderDB *pgxpool.Pool, app *handlers.AppService, chainID int64) *Service {
	return &Service{
		core: &reportCore{
			appDB:    appDB,
			botDB:    botDB,
			ponderDB: ponderDB,
			chainID:  chainID,
		},
		gate: newAuthGate(app),
	}
}

// Handler returns the auth-gated MCP HTTP handler to mount at /mcp. The auth
// middleware runs before any MCP tool is reachable, so an unauthenticated or
// non-admin request never touches the database.
func (s *Service) Handler() http.Handler {
	mcpServer := s.core.newServer()
	s.core.registerExtraTools(mcpServer)
	streamable := server.NewStreamableHTTPServer(mcpServer)
	return s.gate.middleware(streamable)
}

package mcp

import (
	"context"
	"net/http"
	"strings"

	"github.com/SFLuv/app/backend/handlers"
)

// adminAuthority is the subset of AppService the MCP endpoint needs. It is
// satisfied by *handlers.AppService and pins every access decision to the exact
// same live admin/active state the rest of the backend enforces.
type adminAuthority interface {
	UserIsActive(ctx context.Context, id string) bool
	IsAdmin(ctx context.Context, id string) bool
}

var _ adminAuthority = (*handlers.AppService)(nil)

// bearerToken extracts the token from `Authorization: Bearer <token>`. It never
// reads query strings (which leak into logs/history).
func bearerToken(r *http.Request) string {
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(authz) >= 7 && strings.EqualFold(authz[:7], "bearer ") {
		return strings.TrimSpace(authz[7:])
	}
	return ""
}

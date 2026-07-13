package mcp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/SFLuv/app/backend/handlers"
	"github.com/SFLuv/app/backend/utils/middleware"
)

// adminAuthority is the subset of AppService the MCP gate needs. It is satisfied
// by *handlers.AppService and keeps the security checks pinned to the exact same
// live admin/active state the rest of the backend enforces.
type adminAuthority interface {
	UserIsActive(ctx context.Context, id string) bool
	IsAdmin(ctx context.Context, id string) bool
}

var _ adminAuthority = (*handlers.AppService)(nil)

var (
	errNoBearerToken = errors.New("missing bearer token")
	errNotAdmin      = errors.New("not an active SFLUV admin")
)

// authGate authenticates MCP requests using the existing Privy JWT + SFLUV admin
// model. It issues no tokens and stores no credentials of its own: the bearer
// token IS a Privy access token, validated with the same key/audience/issuer/
// expiry checks as normal app traffic, and admin status is read live on every
// request so revoking admin takes effect immediately.
type authGate struct {
	authority adminAuthority
	// validate resolves a bearer token to a user DID. It defaults to
	// middleware.ValidatePrivyToken (the app's single source of truth) and is
	// only overridden in tests.
	validate func(token string) (string, error)
}

func newAuthGate(authority adminAuthority) *authGate {
	return &authGate{authority: authority, validate: middleware.ValidatePrivyToken}
}

// bearerToken extracts the token from `Authorization: Bearer <token>`. It also
// accepts the app's `Access-Token` header as a fallback for parity with the rest
// of the backend, but never reads query strings (which leak into logs/history).
func bearerToken(r *http.Request) string {
	if authz := strings.TrimSpace(r.Header.Get("Authorization")); authz != "" {
		if len(authz) >= 7 && strings.EqualFold(authz[:7], "bearer ") {
			return strings.TrimSpace(authz[7:])
		}
	}
	return strings.TrimSpace(r.Header.Get("Access-Token"))
}

// authorize validates the request and returns the admin's DID, or an error. Any
// error must be treated as a 401 by the caller; the DID is only returned when
// the caller is a validated, active SFLUV admin.
func (g *authGate) authorize(r *http.Request) (string, error) {
	token := bearerToken(r)
	if token == "" {
		return "", errNoBearerToken
	}

	// Same Privy validation (ES256 key, aud=PRIVY_APP_ID, iss=privy.io, exp) the
	// whole app uses. A forged/expired token fails here.
	userDid, err := g.validate(token)
	if err != nil {
		return "", err
	}

	ctx := r.Context()
	// Active (not soft-deleted) AND admin, checked against live DB state.
	if !g.authority.UserIsActive(ctx, userDid) || !g.authority.IsAdmin(ctx, userDid) {
		return "", errNotAdmin
	}

	return userDid, nil
}

// middleware wraps the MCP transport handler and rejects any request that is not
// a validated, active SFLUV admin before the request ever reaches a tool.
func (g *authGate) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := g.authorize(r); err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="SFLUV Admin MCP", error="invalid_token"`)
			http.Error(w, "unauthorized: SFLUV admin bearer token required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

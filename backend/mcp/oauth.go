package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/utils/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	oauthAccessTokenLifetime  = 1 * time.Hour
	oauthRefreshTokenLifetime = 30 * 24 * time.Hour
	oauthCodeLifetime         = 5 * time.Minute
	oauthStateLifetime        = 10 * time.Minute
)

// oauthServer implements the OAuth 2.1 authorization server for the admin MCP
// endpoint. Identity comes from SFLUV's own Privy login (bridged via the
// frontend authorize page), and every issued token is bound to a SFLUV user DID
// whose admin status is re-checked live on every /mcp request.
type oauthServer struct {
	store     *oauthStore
	authority adminAuthority
	// validatePrivy resolves a Privy access token to a user DID. Defaults to the
	// app's shared validator; overridden in tests.
	validatePrivy func(token string) (string, error)
	// lookupToken resolves an AS-issued access token to its owner DID. Defaults
	// to the DB-backed store; overridden in tests.
	lookupToken func(ctx context.Context, token string) (string, error)

	baseURL      string // public base URL of THIS backend (issuer / endpoints)
	authorizeURL string // frontend page that performs the Privy login step
}

func newOAuthServer(db *pgxpool.Pool, authority adminAuthority) *oauthServer {
	store := &oauthStore{db: db}
	return &oauthServer{
		store:         store,
		authority:     authority,
		validatePrivy: middleware.ValidatePrivyToken,
		lookupToken:   store.validateAccessToken,
		baseURL:       resolveMCPBaseURL(),
		authorizeURL:  resolveMCPAuthorizeURL(),
	}
}

func resolveMCPBaseURL() string {
	for _, key := range []string{"MCP_PUBLIC_BASE_URL", "PUBLIC_BACKEND_URL", "NEXT_PUBLIC_BACKEND_URL"} {
		if v := strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/"); v != "" {
			return v
		}
	}
	return "http://localhost:8080"
}

func resolveMCPAuthorizeURL() string {
	if v := strings.TrimSpace(os.Getenv("MCP_AUTHORIZE_URL")); v != "" {
		return v
	}
	base := ""
	for _, key := range []string{"APP_BASE_URL", "NEXT_PUBLIC_FRONTEND_URL"} {
		if v := strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/"); v != "" {
			base = v
			break
		}
	}
	if base == "" {
		base = "http://localhost:3000"
	}
	return base + "/mcp/authorize"
}

// registerRoutes mounts the OAuth + discovery endpoints and the bearer-gated MCP
// handler. The discovery/authorize/token/complete endpoints are intentionally
// unauthenticated at the HTTP layer; each enforces its own checks.
func (o *oauthServer) registerRoutes(r chi.Router, mcpHandler http.Handler) {
	r.Get("/.well-known/oauth-protected-resource", o.protectedResourceMetadata)
	r.Get("/.well-known/oauth-protected-resource/mcp", o.protectedResourceMetadata)
	r.Get("/.well-known/oauth-authorization-server", o.authorizationServerMetadata)
	r.Post("/oauth/register", o.registerClient)
	r.Get("/oauth/authorize", o.authorize)
	r.Post("/oauth/mcp/complete", o.complete)
	r.Post("/oauth/token", o.token)

	gated := o.requireBearer(mcpHandler)
	r.Handle("/mcp", gated)
	r.Handle("/mcp/*", gated)
}

func (o *oauthServer) protectedResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 o.baseURL + "/mcp",
		"authorization_servers":    []string{o.baseURL},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         []string{oauthScope},
		"resource_name":            "SFLUV Admin MCP",
	})
}

func (o *oauthServer) authorizationServerMetadata(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                o.baseURL,
		"authorization_endpoint":                o.baseURL + "/oauth/authorize",
		"token_endpoint":                        o.baseURL + "/oauth/token",
		"registration_endpoint":                 o.baseURL + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{oauthScope},
	})
}

// registerClient implements minimal dynamic client registration. Anyone may
// register a client, but registration alone grants nothing: a token still
// requires an interactive SFLUV admin login. Only redirect URIs are trusted, and
// they must be https or loopback.
func (o *oauthServer) registerClient(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid registration request", http.StatusBadRequest)
		return
	}
	if len(req.RedirectURIs) == 0 {
		http.Error(w, "redirect_uris is required", http.StatusBadRequest)
		return
	}
	for _, u := range req.RedirectURIs {
		if !validRedirectURI(u) {
			http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
			return
		}
	}

	clientID := "sfluv_mcp_" + randomToken(18)
	if err := o.store.registerClient(r.Context(), clientID, strings.TrimSpace(req.ClientName), req.RedirectURIs); err != nil {
		http.Error(w, "unable to register client", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  clientID,
		"client_name":                req.ClientName,
		"redirect_uris":              req.RedirectURIs,
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"client_id_issued_at":        time.Now().UTC().Unix(),
	})
}

// authorize validates the OAuth request (client, redirect_uri, PKCE, scope),
// stores a one-time login state, and redirects the user's browser to the SFLUV
// frontend login page. It performs NO authentication itself — Privy does that on
// the frontend and hands the result back via /oauth/mcp/complete.
func (o *oauthServer) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirectURI := strings.TrimSpace(q.Get("redirect_uri"))
	state := q.Get("state")

	if q.Get("response_type") != "code" {
		redirectOAuthError(w, r, redirectURI, state, "unsupported_response_type")
		return
	}
	clientID := strings.TrimSpace(q.Get("client_id"))
	codeChallenge := strings.TrimSpace(q.Get("code_challenge"))
	if clientID == "" || redirectURI == "" || codeChallenge == "" || q.Get("code_challenge_method") != "S256" {
		redirectOAuthError(w, r, redirectURI, state, "invalid_request")
		return
	}
	if !scopeAllowed(q.Get("scope")) {
		redirectOAuthError(w, r, redirectURI, state, "invalid_scope")
		return
	}
	if resource := strings.TrimSpace(q.Get("resource")); resource != "" && resource != o.baseURL+"/mcp" {
		redirectOAuthError(w, r, redirectURI, state, "invalid_target")
		return
	}
	// redirect_uri must belong to the registered client — blocks open redirects.
	if ok, err := o.store.clientAllowsRedirect(r.Context(), clientID, redirectURI); err != nil || !ok {
		redirectOAuthError(w, r, redirectURI, state, "unauthorized_client")
		return
	}

	loginState := randomToken(32)
	rec := loginStateRecord{
		ClientID:      clientID,
		RedirectURI:   redirectURI,
		ClientState:   state,
		CodeChallenge: codeChallenge,
		Scope:         q.Get("scope"),
		Resource:      q.Get("resource"),
	}
	if err := o.store.storeLoginState(r.Context(), loginState, rec, oauthStateLifetime); err != nil {
		redirectOAuthError(w, r, redirectURI, state, "server_error")
		return
	}

	dest, err := url.Parse(o.authorizeURL)
	if err != nil {
		redirectOAuthError(w, r, redirectURI, state, "server_error")
		return
	}
	dv := dest.Query()
	dv.Set("login_state", loginState)
	dest.RawQuery = dv.Encode()
	http.Redirect(w, r, dest.String(), http.StatusFound)
}

// complete is the Privy bridge, called by the frontend authorize page after the
// admin has logged in. It authenticates the Privy access token, enforces active
// + admin, consumes the one-time login state, mints an auth code bound to that
// admin's DID + the client's PKCE challenge, and returns the URL the browser
// should be redirected to (back to the MCP client with ?code=...).
func (o *oauthServer) complete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LoginState string `json:"login_state"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil || strings.TrimSpace(body.LoginState) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	// Authenticate the SFLUV admin via their Privy access token (same validation
	// as the rest of the app), then enforce active + admin.
	token := bearerToken(r)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login_required"})
		return
	}
	userDID, err := o.validatePrivy(token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login_required"})
		return
	}
	ctx := r.Context()
	if !o.authority.UserIsActive(ctx, userDID) || !o.authority.IsAdmin(ctx, userDID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not_admin"})
		return
	}

	// One-time login state → the pending OAuth request context.
	login, err := o.store.consumeLoginState(ctx, body.LoginState)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expired_login"})
		return
	}

	code := randomToken(32)
	if err := o.store.storeAuthCode(ctx, code, login, userDID, oauthCodeLifetime); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}

	redirect, err := url.Parse(login.RedirectURI)
	if err != nil || !validRedirectURI(login.RedirectURI) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_redirect"})
		return
	}
	rv := redirect.Query()
	rv.Set("code", code)
	if login.ClientState != "" {
		rv.Set("state", login.ClientState)
	}
	redirect.RawQuery = rv.Encode()

	writeJSON(w, http.StatusOK, map[string]string{"redirect_url": redirect.String()})
}

// token exchanges an authorization code (with PKCE) or a refresh token for a
// fresh access/refresh token pair. Public client (auth method "none"); security
// comes from PKCE, single-use codes, and refresh-token rotation.
func (o *oauthServer) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		o.tokenAuthCode(w, r)
	case "refresh_token":
		o.tokenRefresh(w, r)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
	}
}

func (o *oauthServer) tokenAuthCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.Form.Get("code")
	clientID := r.Form.Get("client_id")
	redirectURI := r.Form.Get("redirect_uri")
	verifier := r.Form.Get("code_verifier")

	authCode, err := o.store.consumeAuthCode(ctx, code, clientID, redirectURI)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	if !verifyPKCE(verifier, authCode.CodeChallenge) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	// Re-check admin at token issuance too.
	if !o.authority.UserIsActive(ctx, authCode.UserDID) || !o.authority.IsAdmin(ctx, authCode.UserDID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access_denied"})
		return
	}
	o.issueTokens(w, r, authCode.ClientID, authCode.UserDID, authCode.Scope, authCode.Resource)
}

func (o *oauthServer) tokenRefresh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	refreshToken := r.Form.Get("refresh_token")
	clientID := r.Form.Get("client_id")
	if refreshToken == "" || clientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	rec, err := o.store.rotateRefreshToken(ctx, refreshToken, clientID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	// Admin may have been revoked since the last refresh.
	if !o.authority.UserIsActive(ctx, rec.UserDID) || !o.authority.IsAdmin(ctx, rec.UserDID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access_denied"})
		return
	}
	o.issueTokens(w, r, rec.ClientID, rec.UserDID, rec.Scope, rec.Resource)
}

func (o *oauthServer) issueTokens(w http.ResponseWriter, _ *http.Request, clientID, userDID, scope, resource string) {
	accessToken := randomToken(32)
	refreshToken := randomToken(32)
	scope = normalizeScope(scope)
	if err := o.store.storeTokens(context.Background(), accessToken, refreshToken, clientID, userDID, scope, resource, oauthAccessTokenLifetime, oauthRefreshTokenLifetime); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"token_type":    "Bearer",
		"expires_in":    int(oauthAccessTokenLifetime.Seconds()),
		"refresh_token": refreshToken,
		"scope":         scope,
	})
}

// requireBearer gates the MCP handler: the caller must present a valid,
// non-expired AS-issued access token whose owner is STILL a live SFLUV admin.
func (o *oauthServer) requireBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			o.unauthorized(w)
			return
		}
		userDID, err := o.lookupToken(r.Context(), token)
		if err != nil {
			o.unauthorized(w)
			return
		}
		if !o.authority.UserIsActive(r.Context(), userDID) || !o.authority.IsAdmin(r.Context(), userDID) {
			o.unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (o *oauthServer) unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(
		`Bearer resource_metadata="%s/.well-known/oauth-protected-resource/mcp", scope="%s"`, o.baseURL, oauthScope))
	http.Error(w, "unauthorized: SFLUV admin OAuth token required", http.StatusUnauthorized)
}

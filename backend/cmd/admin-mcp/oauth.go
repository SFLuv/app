package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	oauthScope          = "sfluv.admin.read"
	oauthTokenLifetime  = 12 * time.Hour
	oauthCodeLifetime   = 10 * time.Minute
	oauthStateLifetime  = 10 * time.Minute
	oauthDefaultBaseURL = "http://localhost:8090"
)

type oauthServer struct {
	appDB              *pgxpool.Pool
	baseURL            string
	googleClientID     string
	googleClientSecret string
	httpClient         *http.Client
}

func newOAuthServer(appDB *pgxpool.Pool) *oauthServer {
	return &oauthServer{
		appDB:              appDB,
		baseURL:            strings.TrimRight(envOrDefault("MCP_PUBLIC_BASE_URL", oauthDefaultBaseURL), "/"),
		googleClientID:     strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_ID")),
		googleClientSecret: strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")),
		httpClient:         &http.Client{Timeout: 10 * time.Second},
	}
}

func (o *oauthServer) register(mux *http.ServeMux, mcpHandler http.Handler) {
	mux.HandleFunc("/.well-known/oauth-protected-resource", o.protectedResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", o.protectedResourceMetadata)
	mux.HandleFunc("/.well-known/oauth-authorization-server", o.authorizationServerMetadata)
	mux.HandleFunc("/oauth/register", o.registerClient)
	mux.HandleFunc("/oauth/authorize", o.authorize)
	mux.HandleFunc("/oauth/google/callback", o.googleCallback)
	mux.HandleFunc("/oauth/token", o.token)
	mux.Handle("/mcp", o.requireBearer(mcpHandler))
}

func (o *oauthServer) protectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"resource":                 o.baseURL + "/mcp",
		"authorization_servers":    []string{o.baseURL},
		"bearer_methods_supported": []string{"header"},
		"resource_documentation":   o.baseURL + "/mcp",
		"scopes_supported":         []string{oauthScope},
		"resource_name":            "SFLUV Admin MCP",
	})
}

func (o *oauthServer) authorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"issuer":                                o.baseURL,
		"authorization_endpoint":                o.baseURL + "/oauth/authorize",
		"token_endpoint":                        o.baseURL + "/oauth/token",
		"registration_endpoint":                 o.baseURL + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{oauthScope},
	})
}

func (o *oauthServer) registerClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ClientName              string   `json:"client_name"`
		RedirectURIs            []string `json:"redirect_uris"`
		TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid registration request", http.StatusBadRequest)
		return
	}
	if len(req.RedirectURIs) == 0 {
		http.Error(w, "redirect_uris is required", http.StatusBadRequest)
		return
	}
	for _, redirectURI := range req.RedirectURIs {
		if !validRedirectURI(redirectURI) {
			http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
			return
		}
	}

	clientID := "sfluv_mcp_client_" + randomToken(24)
	if err := withWriteTx(r.Context(), o.appDB, func(tx pgx.Tx) error {
		_, err := tx.Exec(r.Context(), `
			INSERT INTO admin_mcp_oauth_clients(client_id, client_name, redirect_uris)
			VALUES($1, $2, $3);
		`, clientID, strings.TrimSpace(req.ClientName), req.RedirectURIs)
		return err
	}); err != nil {
		http.Error(w, "unable to register client", http.StatusInternalServerError)
		return
	}

	writeJSONStatus(w, http.StatusCreated, map[string]any{
		"client_id":                  clientID,
		"client_name":                req.ClientName,
		"redirect_uris":              req.RedirectURIs,
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"client_id_issued_at":        time.Now().UTC().Unix(),
	})
}

func (o *oauthServer) authorize(w http.ResponseWriter, r *http.Request) {
	if o.googleClientID == "" || o.googleClientSecret == "" {
		http.Error(w, "google oauth is not configured", http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	if q.Get("response_type") != "code" {
		redirectOAuthError(w, q.Get("redirect_uri"), q.Get("state"), "unsupported_response_type")
		return
	}
	clientID := strings.TrimSpace(q.Get("client_id"))
	redirectURI := strings.TrimSpace(q.Get("redirect_uri"))
	codeChallenge := strings.TrimSpace(q.Get("code_challenge"))
	if clientID == "" || redirectURI == "" || codeChallenge == "" || q.Get("code_challenge_method") != "S256" {
		redirectOAuthError(w, redirectURI, q.Get("state"), "invalid_request")
		return
	}
	if !scopeAllowed(q.Get("scope")) {
		redirectOAuthError(w, redirectURI, q.Get("state"), "invalid_scope")
		return
	}
	if resource := strings.TrimSpace(q.Get("resource")); resource != "" && resource != o.baseURL+"/mcp" {
		redirectOAuthError(w, redirectURI, q.Get("state"), "invalid_target")
		return
	}
	if ok, err := o.clientAllowsRedirect(r.Context(), clientID, redirectURI); err != nil || !ok {
		redirectOAuthError(w, redirectURI, q.Get("state"), "unauthorized_client")
		return
	}

	loginState := randomToken(32)
	if err := o.storeLoginState(r.Context(), loginState, clientID, redirectURI, q.Get("state"), codeChallenge, q.Get("scope"), q.Get("resource")); err != nil {
		redirectOAuthError(w, redirectURI, q.Get("state"), "server_error")
		return
	}

	google := url.URL{Scheme: "https", Host: "accounts.google.com", Path: "/o/oauth2/v2/auth"}
	values := google.Query()
	values.Set("client_id", o.googleClientID)
	values.Set("redirect_uri", o.baseURL+"/oauth/google/callback")
	values.Set("response_type", "code")
	values.Set("scope", "openid email profile")
	values.Set("state", loginState)
	values.Set("prompt", "select_account")
	google.RawQuery = values.Encode()
	http.Redirect(w, r, google.String(), http.StatusFound)
}

func (o *oauthServer) googleCallback(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	login, err := o.consumeLoginState(r.Context(), state)
	if err != nil {
		http.Error(w, "expired login state", http.StatusBadRequest)
		return
	}
	if googleErr := r.URL.Query().Get("error"); googleErr != "" {
		redirectOAuthError(w, login.RedirectURI, login.ClientState, googleErr)
		return
	}

	token, err := o.exchangeGoogleCode(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		redirectOAuthError(w, login.RedirectURI, login.ClientState, "access_denied")
		return
	}
	email, err := o.googleEmail(r.Context(), token.AccessToken)
	if err != nil || email == "" {
		redirectOAuthError(w, login.RedirectURI, login.ClientState, "access_denied")
		return
	}
	if allowed, err := o.emailAllowed(r.Context(), email); err != nil || !allowed {
		redirectOAuthError(w, login.RedirectURI, login.ClientState, "access_denied")
		return
	}

	code := randomToken(32)
	if err := o.storeAuthCode(r.Context(), code, login, email); err != nil {
		redirectOAuthError(w, login.RedirectURI, login.ClientState, "server_error")
		return
	}

	redirect, _ := url.Parse(login.RedirectURI)
	values := redirect.Query()
	values.Set("code", code)
	if login.ClientState != "" {
		values.Set("state", login.ClientState)
	}
	redirect.RawQuery = values.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (o *oauthServer) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid token request", http.StatusBadRequest)
		return
	}
	if r.Form.Get("grant_type") != "authorization_code" {
		http.Error(w, "unsupported_grant_type", http.StatusBadRequest)
		return
	}

	code := r.Form.Get("code")
	clientID := r.Form.Get("client_id")
	redirectURI := r.Form.Get("redirect_uri")
	verifier := r.Form.Get("code_verifier")
	authCode, err := o.loadAuthCode(r.Context(), code, clientID, redirectURI)
	if err != nil {
		http.Error(w, "invalid_grant", http.StatusBadRequest)
		return
	}
	if !verifyPKCE(verifier, authCode.CodeChallenge) {
		http.Error(w, "invalid_grant", http.StatusBadRequest)
		return
	}
	if allowed, err := o.emailAllowed(r.Context(), authCode.Email); err != nil || !allowed {
		http.Error(w, "access_denied", http.StatusForbidden)
		return
	}

	accessToken := randomToken(32)
	if err := o.storeAccessToken(r.Context(), accessToken, authCode); err != nil {
		http.Error(w, "server_error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   int(oauthTokenLifetime.Seconds()),
		"scope":        authCode.Scope,
	})
}

func (o *oauthServer) requireBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			o.unauthorized(w)
			return
		}
		if _, err := o.validateAccessToken(r.Context(), token); err != nil {
			o.unauthorized(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (o *oauthServer) unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource/mcp", scope="%s"`, o.baseURL, oauthScope))
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

type loginStateRecord struct {
	ClientID      string
	RedirectURI   string
	ClientState   string
	CodeChallenge string
	Scope         string
	Resource      string
}

type authCodeRecord struct {
	ClientID      string
	Email         string
	RedirectURI   string
	CodeChallenge string
	Scope         string
	Resource      string
}

func (o *oauthServer) clientAllowsRedirect(ctx context.Context, clientID string, redirectURI string) (bool, error) {
	var redirects []string
	err := o.appDB.QueryRow(ctx, `SELECT redirect_uris FROM admin_mcp_oauth_clients WHERE client_id = $1;`, clientID).Scan(&redirects)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return slices.Contains(redirects, redirectURI), nil
}

func (o *oauthServer) storeLoginState(ctx context.Context, state string, clientID string, redirectURI string, clientState string, challenge string, scope string, resource string) error {
	_, err := o.appDB.Exec(ctx, `
		INSERT INTO admin_mcp_oauth_login_states(state_hash, client_id, redirect_uri, client_state, code_challenge, scope, resource, expires_at)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8);
	`, hashToken(state), clientID, redirectURI, clientState, challenge, normalizeScope(scope), strings.TrimSpace(resource), time.Now().UTC().Add(oauthStateLifetime))
	return err
}

func (o *oauthServer) consumeLoginState(ctx context.Context, state string) (loginStateRecord, error) {
	var record loginStateRecord
	tx, err := o.appDB.Begin(ctx)
	if err != nil {
		return record, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		DELETE FROM admin_mcp_oauth_login_states
		WHERE state_hash = $1
		AND expires_at > NOW()
		RETURNING client_id, redirect_uri, client_state, code_challenge, scope, resource;
	`, hashToken(state))
	if err := row.Scan(&record.ClientID, &record.RedirectURI, &record.ClientState, &record.CodeChallenge, &record.Scope, &record.Resource); err != nil {
		return record, err
	}
	return record, tx.Commit(ctx)
}

func (o *oauthServer) storeAuthCode(ctx context.Context, code string, login loginStateRecord, email string) error {
	_, err := o.appDB.Exec(ctx, `
		INSERT INTO admin_mcp_oauth_auth_codes(code_hash, client_id, email, redirect_uri, code_challenge, scope, resource, expires_at)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8);
	`, hashToken(code), login.ClientID, normalizeEmail(email), login.RedirectURI, login.CodeChallenge, normalizeScope(login.Scope), login.Resource, time.Now().UTC().Add(oauthCodeLifetime))
	return err
}

func (o *oauthServer) loadAuthCode(ctx context.Context, code string, clientID string, redirectURI string) (authCodeRecord, error) {
	var record authCodeRecord
	tx, err := o.appDB.Begin(ctx)
	if err != nil {
		return record, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		UPDATE admin_mcp_oauth_auth_codes
		SET used_at = NOW()
		WHERE code_hash = $1
		AND client_id = $2
		AND redirect_uri = $3
		AND used_at IS NULL
		AND expires_at > NOW()
		RETURNING client_id, email, redirect_uri, code_challenge, scope, resource;
	`, hashToken(code), clientID, redirectURI)
	if err := row.Scan(&record.ClientID, &record.Email, &record.RedirectURI, &record.CodeChallenge, &record.Scope, &record.Resource); err != nil {
		return record, err
	}
	return record, tx.Commit(ctx)
}

func (o *oauthServer) storeAccessToken(ctx context.Context, token string, code authCodeRecord) error {
	_, err := o.appDB.Exec(ctx, `
		INSERT INTO admin_mcp_oauth_tokens(token_hash, client_id, email, scope, resource, expires_at)
		VALUES($1, $2, $3, $4, $5, $6);
	`, hashToken(token), code.ClientID, normalizeEmail(code.Email), normalizeScope(code.Scope), code.Resource, time.Now().UTC().Add(oauthTokenLifetime))
	return err
}

func (o *oauthServer) validateAccessToken(ctx context.Context, token string) (string, error) {
	var email string
	err := o.appDB.QueryRow(ctx, `
		UPDATE admin_mcp_oauth_tokens
		SET last_used_at = NOW()
		WHERE token_hash = $1
		AND expires_at > NOW()
		AND revoked_at IS NULL
		RETURNING email;
	`, hashToken(token)).Scan(&email)
	if err != nil {
		return "", err
	}
	allowed, err := o.emailAllowed(ctx, email)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", fmt.Errorf("email not allowed")
	}
	return email, nil
}

func (o *oauthServer) emailAllowed(ctx context.Context, email string) (bool, error) {
	var allowed bool
	err := o.appDB.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM admin_mcp_allowed_emails
			WHERE email = $1
			AND revoked_at IS NULL
		);
	`, normalizeEmail(email)).Scan(&allowed)
	return allowed, err
}

type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
}

func (o *oauthServer) exchangeGoogleCode(ctx context.Context, code string) (googleTokenResponse, error) {
	var out googleTokenResponse
	values := url.Values{}
	values.Set("code", code)
	values.Set("client_id", o.googleClientID)
	values.Set("client_secret", o.googleClientSecret)
	values.Set("redirect_uri", o.baseURL+"/oauth/google/callback")
	values.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(values.Encode()))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return out, fmt.Errorf("google token exchange failed: %s", strings.TrimSpace(string(body)))
	}
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

func (o *oauthServer) googleEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("google userinfo failed")
	}
	var info struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	if !info.EmailVerified {
		return "", fmt.Errorf("google email is not verified")
	}
	return normalizeEmail(info.Email), nil
}

func withWriteTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func randomToken(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func verifyPKCE(verifier string, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:]) == challenge
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizeScope(scope string) string {
	return oauthScope
}

func scopeAllowed(scope string) bool {
	scope = strings.TrimSpace(scope)
	return scope == "" || slices.Contains(strings.Fields(scope), oauthScope)
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func validRedirectURI(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if parsed.Scheme != "http" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func redirectOAuthError(w http.ResponseWriter, redirectURI string, state string, code string) {
	if !validRedirectURI(redirectURI) {
		http.Error(w, code, http.StatusBadRequest)
		return
	}
	redirect, _ := url.Parse(redirectURI)
	values := redirect.Query()
	values.Set("error", code)
	if state != "" {
		values.Set("state", state)
	}
	redirect.RawQuery = values.Encode()
	w.Header().Set("Location", redirect.String())
	w.WriteHeader(http.StatusFound)
}

func writeJSON(w http.ResponseWriter, value any) {
	writeJSONStatus(w, http.StatusOK, value)
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

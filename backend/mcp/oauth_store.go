package mcp

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// oauthStore persists OAuth clients, one-time login states, one-time auth codes,
// and access/refresh tokens. Only hashes of secret values are stored. These are
// bookkeeping tables (writes are required), separate from the strictly
// read-only report tools.
type oauthStore struct {
	db *pgxpool.Pool
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
	UserDID       string
	RedirectURI   string
	CodeChallenge string
	Scope         string
	Resource      string
}

func (s *oauthStore) registerClient(ctx context.Context, clientID, clientName string, redirectURIs []string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO admin_mcp_oauth_clients(client_id, client_name, redirect_uris)
		VALUES ($1, $2, $3);
	`, clientID, clientName, redirectURIs)
	return err
}

func (s *oauthStore) clientAllowsRedirect(ctx context.Context, clientID, redirectURI string) (bool, error) {
	var redirects []string
	err := s.db.QueryRow(ctx, `SELECT redirect_uris FROM admin_mcp_oauth_clients WHERE client_id = $1;`, clientID).Scan(&redirects)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return slices.Contains(redirects, redirectURI), nil
}

func (s *oauthStore) storeLoginState(ctx context.Context, state string, rec loginStateRecord, ttl time.Duration) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO admin_mcp_oauth_login_states(
			state_hash, client_id, redirect_uri, client_state, code_challenge, scope, resource, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
	`, hashToken(state), rec.ClientID, rec.RedirectURI, rec.ClientState, rec.CodeChallenge, rec.Scope, rec.Resource,
		time.Now().UTC().Add(ttl))
	return err
}

// consumeLoginState atomically fetches and deletes a login state, enforcing
// single use and expiry.
func (s *oauthStore) consumeLoginState(ctx context.Context, state string) (loginStateRecord, error) {
	var rec loginStateRecord
	err := s.db.QueryRow(ctx, `
		DELETE FROM admin_mcp_oauth_login_states
		WHERE state_hash = $1 AND expires_at > NOW()
		RETURNING client_id, redirect_uri, client_state, code_challenge, scope, resource;
	`, hashToken(state)).Scan(&rec.ClientID, &rec.RedirectURI, &rec.ClientState, &rec.CodeChallenge, &rec.Scope, &rec.Resource)
	return rec, err
}

func (s *oauthStore) storeAuthCode(ctx context.Context, code string, login loginStateRecord, userDID string, ttl time.Duration) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO admin_mcp_oauth_auth_codes(
			code_hash, client_id, user_did, redirect_uri, code_challenge, scope, resource, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
	`, hashToken(code), login.ClientID, userDID, login.RedirectURI, login.CodeChallenge, normalizeScope(login.Scope), login.Resource,
		time.Now().UTC().Add(ttl))
	return err
}

// consumeAuthCode atomically marks an auth code used (single use) and returns it,
// enforcing client_id, redirect_uri, and expiry.
func (s *oauthStore) consumeAuthCode(ctx context.Context, code, clientID, redirectURI string) (authCodeRecord, error) {
	var rec authCodeRecord
	err := s.db.QueryRow(ctx, `
		UPDATE admin_mcp_oauth_auth_codes
		SET used_at = NOW()
		WHERE code_hash = $1
		AND client_id = $2
		AND redirect_uri = $3
		AND used_at IS NULL
		AND expires_at > NOW()
		RETURNING client_id, user_did, redirect_uri, code_challenge, scope, resource;
	`, hashToken(code), clientID, redirectURI).Scan(&rec.ClientID, &rec.UserDID, &rec.RedirectURI, &rec.CodeChallenge, &rec.Scope, &rec.Resource)
	return rec, err
}

// storeTokens writes an access token (linked to its refresh token) and the
// refresh token in a single transaction.
func (s *oauthStore) storeTokens(ctx context.Context, accessToken, refreshToken, clientID, userDID, scope, resource string, accessTTL, refreshTTL time.Duration) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		now := time.Now().UTC()
		if _, err := tx.Exec(ctx, `
			INSERT INTO admin_mcp_oauth_refresh_tokens(token_hash, client_id, user_did, scope, resource, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6);
		`, hashToken(refreshToken), clientID, userDID, scope, resource, now.Add(refreshTTL)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO admin_mcp_oauth_tokens(token_hash, client_id, user_did, refresh_token_hash, scope, resource, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7);
		`, hashToken(accessToken), clientID, userDID, hashToken(refreshToken), scope, resource, now.Add(accessTTL))
		return err
	})
}

// validateAccessToken looks up a non-expired, non-revoked access token, marks it
// used, and returns the owning user's DID. The caller must still re-check that
// the DID is a live admin.
func (s *oauthStore) validateAccessToken(ctx context.Context, token string) (string, error) {
	var userDID string
	err := s.db.QueryRow(ctx, `
		UPDATE admin_mcp_oauth_tokens
		SET last_used_at = NOW()
		WHERE token_hash = $1 AND expires_at > NOW() AND revoked_at IS NULL
		RETURNING user_did;
	`, hashToken(token)).Scan(&userDID)
	if err != nil {
		return "", err
	}
	return userDID, nil
}

type refreshRecord struct {
	ClientID string
	UserDID  string
	Scope    string
	Resource string
}

// rotateRefreshToken atomically consumes a refresh token (single use), revokes
// access tokens minted from it, and returns its context so a fresh pair can be
// issued. Refresh-token rotation limits the blast radius of a leaked refresh
// token.
func (s *oauthStore) rotateRefreshToken(ctx context.Context, refreshToken, clientID string) (refreshRecord, error) {
	var rec refreshRecord
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if scanErr := tx.QueryRow(ctx, `
			UPDATE admin_mcp_oauth_refresh_tokens
			SET revoked_at = NOW(), last_used_at = NOW()
			WHERE token_hash = $1 AND client_id = $2 AND expires_at > NOW() AND revoked_at IS NULL
			RETURNING client_id, user_did, scope, resource;
		`, hashToken(refreshToken), clientID).Scan(&rec.ClientID, &rec.UserDID, &rec.Scope, &rec.Resource); scanErr != nil {
			return scanErr
		}
		// Revoke the access token(s) issued from this refresh token.
		_, err := tx.Exec(ctx, `
			UPDATE admin_mcp_oauth_tokens
			SET revoked_at = NOW()
			WHERE refresh_token_hash = $1 AND revoked_at IS NULL;
		`, hashToken(refreshToken))
		return err
	})
	return rec, err
}

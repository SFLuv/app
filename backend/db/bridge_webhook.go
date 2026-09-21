package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// BridgeWebhookSchemaDDL caches what Bridge told us about our own webhook
// endpoint, so the signing key survives a restart while Bridge is unreachable.
//
// It is a cache, not a source of truth: Bridge holds the real registration, and
// every boot re-reads it and overwrites this. Keyed by URL so pointing a
// deployment at a new callback path is a new row rather than a silent overwrite
// of the old one's key.
const BridgeWebhookSchemaDDL = `
	CREATE TABLE IF NOT EXISTS bridge_webhook_endpoints (
		url         TEXT PRIMARY KEY,
		webhook_id  TEXT NOT NULL DEFAULT '',
		public_key  TEXT NOT NULL,
		created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
`

// GetBridgeWebhookPublicKey returns the cached PEM for a callback URL, or ""
// when nothing has been cached yet.
func (a *AppDB) GetBridgeWebhookPublicKey(ctx context.Context, url string) (string, error) {
	var pem string
	err := a.db.QueryRow(ctx, `
		SELECT public_key FROM bridge_webhook_endpoints WHERE url = $1;
	`, strings.TrimSpace(url)).Scan(&pem)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("error loading cached bridge webhook key: %w", err)
	}
	return pem, nil
}

// SaveBridgeWebhookPublicKey records what Bridge reported for this URL.
func (a *AppDB) SaveBridgeWebhookPublicKey(ctx context.Context, url, webhookID, pem string) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO bridge_webhook_endpoints (url, webhook_id, public_key)
		VALUES ($1, $2, $3)
		ON CONFLICT (url) DO UPDATE SET
			webhook_id = EXCLUDED.webhook_id,
			public_key = EXCLUDED.public_key,
			updated_at = NOW();
	`, strings.TrimSpace(url), strings.TrimSpace(webhookID), strings.TrimSpace(pem))
	if err != nil {
		return fmt.Errorf("error caching bridge webhook key: %w", err)
	}
	return nil
}

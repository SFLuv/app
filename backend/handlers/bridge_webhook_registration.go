package handlers

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/bridge"
	"github.com/SFLuv/app/backend/logger"
	"github.com/SFLuv/app/backend/utils"
)

// Bridge mints an RSA signing key per webhook endpoint and returns it from both
// the create and the list call. So the key does not have to be copied out of the
// dashboard into BRIDGE_WEBHOOK_PUBLIC_KEY: given BRIDGE_WEBHOOK_URL and an API
// key carrying webhook:read (plus webhook:create to register a new one), the
// backend can find or register its own endpoint and adopt the key Bridge issued.
//
// BRIDGE_WEBHOOK_PUBLIC_KEY still wins when set, so a deployment that would
// rather not grant webhook scopes keeps working exactly as before.

// resolveRetryDelays spaces out the attempts. Bridge being briefly unreachable
// at boot is ordinary; the cached key covers the gap, and the payout sweep
// covers it even when there is no cache.
var resolveRetryDelays = []time.Duration{0, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}

// StartBridgeWebhookResolution resolves the signing key in the background.
//
// Background, not at boot: this calls a third party, and a deploy must never
// wait on one to start serving. Until it lands, deliveries are refused with a
// 401 — Bridge retries them, and nothing is lost that the sweep would not pick
// up anyway.
func StartBridgeWebhookResolution(ctx context.Context, a *AppService, appLogger *logger.LogCloser) {
	if ctx == nil || a == nil || a.bridge == nil || !a.bridge.Enabled() {
		return
	}
	// An explicitly configured key is the operator's choice; do not touch it,
	// and do not spend API calls confirming it.
	if strings.TrimSpace(os.Getenv("BRIDGE_WEBHOOK_PUBLIC_KEY")) != "" {
		if appLogger != nil {
			appLogger.Logf("bridge webhooks: using BRIDGE_WEBHOOK_PUBLIC_KEY from the environment")
		}
		return
	}
	url, source := bridgeWebhookURL()
	if url == "" {
		if appLogger != nil {
			appLogger.Logf("bridge webhooks: no BRIDGE_WEBHOOK_PUBLIC_KEY, no BRIDGE_WEBHOOK_URL, and no public backend URL to derive one from — deliveries will be refused")
		}
		return
	}
	// Refuse to hand Bridge anything it cannot reach. Registering a localhost
	// URL from a developer's machine would create a dead endpoint on the shared
	// account and, worse, could claim the URL a real deployment wants.
	if !strings.HasPrefix(strings.ToLower(url), "https://") {
		if appLogger != nil {
			appLogger.Logf("bridge webhooks: the callback URL must be https, refusing to register %q (from %s)", url, source)
		}
		return
	}
	if appLogger != nil {
		appLogger.Logf("bridge webhooks: callback URL %s (from %s)", url, source)
	}

	go func() {
		// A cached key first, so verification works from the first delivery
		// even while Bridge is unreachable.
		cacheCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		cached, err := a.db.GetBridgeWebhookPublicKey(cacheCtx, url)
		cancel()
		if err != nil && appLogger != nil {
			appLogger.Logf("bridge webhooks: could not read the cached signing key: %s", err)
		}
		if cached != "" {
			a.bridge.SetWebhookPublicKey(cached)
			if appLogger != nil {
				appLogger.Logf("bridge webhooks: using the cached signing key for %s while refreshing", url)
			}
		}

		for _, delay := range resolveRetryDelays {
			if delay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
				}
			}
			attemptCtx, cancel := context.WithTimeout(ctx, bridgeAPITimeout)
			err := a.resolveBridgeWebhook(attemptCtx, url, appLogger)
			cancel()
			if err == nil {
				return
			}
			if appLogger != nil {
				appLogger.Logf("bridge webhooks: could not resolve the signing key for %s: %s", url, err)
			}
			if ctx.Err() != nil {
				return
			}
		}
		if appLogger != nil && cached == "" {
			appLogger.Logf("bridge webhooks: giving up for now; deliveries stay refused until the next restart. The payout sweep still reconciles every 10 minutes.")
		}
	}()
}

// resolveBridgeWebhook adopts the endpoint registered for url, registering one
// if Bridge has none. Idempotent: the list is checked first, so restarts reuse
// the endpoint rather than accumulating duplicates.
func (a *AppService) resolveBridgeWebhook(ctx context.Context, url string, appLogger *logger.LogCloser) error {
	existing, err := a.bridge.ListWebhooks(ctx)
	if err != nil {
		return err
	}
	var found *bridge.Webhook
	for i := range existing {
		w := &existing[i]
		if strings.EqualFold(strings.TrimRight(w.URL, "/"), strings.TrimRight(url, "/")) &&
			!strings.EqualFold(w.Status, "deleted") {
			found = w
			break
		}
	}

	if found == nil {
		created, err := a.bridge.CreateWebhook(ctx, url, bridge.EventCategories)
		if err != nil {
			return err
		}
		found = created
		if appLogger != nil {
			appLogger.Logf("bridge webhooks: registered %s (%s) for %v", url, found.ID, bridge.EventCategories)
		}
	} else if appLogger != nil {
		appLogger.Logf("bridge webhooks: adopted the existing endpoint %s (%s)", url, found.ID)
		if missing := missingCategories(found.EventCategories); len(missing) > 0 {
			// Not corrected automatically: the endpoint may be shared with
			// another deployment, and narrowing someone else's subscription
			// silently is worse than saying so.
			appLogger.Logf("bridge webhooks: WARNING endpoint %s is not subscribed to %v — those events will never arrive", found.ID, missing)
		}
	}

	if strings.TrimSpace(found.PublicKey) == "" {
		if appLogger != nil {
			appLogger.Logf("bridge webhooks: endpoint %s returned no public key; deliveries stay refused", found.ID)
		}
		return nil
	}
	a.bridge.SetWebhookPublicKey(found.PublicKey)
	if err := a.db.SaveBridgeWebhookPublicKey(ctx, url, found.ID, found.PublicKey); err != nil && appLogger != nil {
		appLogger.Logf("bridge webhooks: resolved the key but could not cache it: %s", err)
	}
	return nil
}

// missingCategories reports which of the categories we act on the endpoint is
// not subscribed to. An endpoint with no categories at all receives everything,
// so nothing is missing.
func missingCategories(subscribed []string) []string {
	if len(subscribed) == 0 {
		return nil
	}
	have := make(map[string]bool, len(subscribed))
	for _, c := range subscribed {
		have[strings.ToLower(strings.TrimSpace(c))] = true
	}
	var missing []string
	for _, want := range bridge.EventCategories {
		if !have[want] {
			missing = append(missing, want)
		}
	}
	return missing
}

// bridgeWebhookURL is where Bridge should deliver, and where that came from.
//
// BRIDGE_WEBHOOK_URL is the explicit answer. Failing that it is derived from the
// backend's own public origin — the same one the MCP OAuth endpoints and every
// other outward-facing URL are built from — because the webhook is a fixed
// path on this service and repeating the host in a second variable is how the
// two drift apart.
//
// Deliberately NOT derived from APP_BASE_URL: that is the frontend, and in
// production the two are different hosts (sfluv.org vs api.sfluv.org). A
// webhook pointed at the frontend would 404 every delivery.
//
// An unconfigured deployment yields "", and a local one yields an http URL that
// the https guard refuses — so neither can register a dead endpoint on the
// shared Bridge account.
func bridgeWebhookURL() (string, string) {
	if explicit := strings.TrimSpace(os.Getenv("BRIDGE_WEBHOOK_URL")); explicit != "" {
		return strings.TrimRight(explicit, "/"), "BRIDGE_WEBHOOK_URL"
	}
	if base := utils.PublicBackendBase(); base != "" {
		return base + bridgeWebhookPath, "the public backend URL"
	}
	return "", ""
}

// bridgeWebhookPath is the route ReceiveBridgeWebhook is mounted on.
const bridgeWebhookPath = "/bridge/webhook"

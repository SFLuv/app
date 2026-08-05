package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

func parseClientSemver(version string) ([3]int, bool) {
	var parsed [3]int
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) != 3 {
		return parsed, false
	}
	for index, part := range parts {
		if part == "" {
			return parsed, false
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return parsed, false
		}
		parsed[index] = value
	}
	return parsed, true
}

func normalizeMinimumClientVersion(version string) string {
	if _, ok := parseClientSemver(version); ok {
		return strings.TrimSpace(version)
	}
	return "1.0.0"
}

func clientVersionRequiresUpdate(version string, minimum string) bool {
	client, ok := parseClientSemver(version)
	if !ok {
		return true
	}
	required, ok := parseClientSemver(minimum)
	if !ok {
		return false
	}
	for index := range client {
		if client[index] < required[index] {
			return true
		}
		if client[index] > required[index] {
			return false
		}
	}
	return false
}

// serverFeatureFlags are feature toggles owned by this backend rather than by
// the upstream community config. They are overlaid onto the served config so
// clients can dark-launch a surface without shipping a new build.
//
// Absent env ⇒ false, which is the default both app clients rely on: an old
// build or a not-yet-deployed backend must never light up a tab that would 404.
func serverFeatureFlags() map[string]any {
	return map[string]any{
		"volunteer_events_enabled": strings.EqualFold(strings.TrimSpace(os.Getenv("VOLUNTEER_EVENTS_ENABLED")), "true"),
	}
}

// withServerFeatureFlags merges serverFeatureFlags into the config's "features"
// object, preserving everything the upstream config already provides. On any
// decode problem it returns the raw config untouched — a malformed overlay must
// never take down /config, which every client polls at startup.
func withServerFeatureFlags(raw []byte) []byte {
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		return raw
	}

	features, ok := config["features"].(map[string]any)
	if !ok || features == nil {
		features = map[string]any{}
	}
	for key, value := range serverFeatureFlags() {
		features[key] = value
	}
	config["features"] = features

	merged, err := json.Marshal(config)
	if err != nil {
		return raw
	}
	return merged
}

func (a *AppService) GetClientConfig(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.clientConfig == nil {
		http.Error(w, "client config is not loaded", http.StatusServiceUnavailable)
		return
	}
	a.recordClientPhoneHome(r.Context(), "config", r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(withServerFeatureFlags(a.clientConfig.RawJSON()))
}

func (a *AppService) GetClientVersion(w http.ResponseWriter, r *http.Request) {
	a.recordClientPhoneHome(r.Context(), "client-version", r)
	now := time.Now().UTC()
	platform := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("platform")))
	if platform == "" {
		platform = "unknown"
	}

	version := strings.TrimSpace(r.URL.Query().Get("version"))

	minimum := structs.ClientVersionInfo{
		Version: normalizeMinimumClientVersion(envString("CLIENT_MIN_VERSION", "1.0.0")),
	}

	status := "ok"
	forceUpdate := false
	maintenance := envBool("CLIENT_MAINTENANCE", false)
	message := ""

	if maintenance {
		status = "maintenance"
		message = envString("CLIENT_MAINTENANCE_MESSAGE", "SFLUV is temporarily unavailable while maintenance is in progress.")
	} else if clientVersionRequiresUpdate(version, minimum.Version) {
		status = "update_required"
		forceUpdate = true
		message = envString("CLIENT_UPDATE_REQUIRED_MESSAGE", "An SFLUV Wallet update is required.")
	}

	activeChainID := 0
	configVersion := "unknown"
	if a != nil && a.clientConfig != nil {
		activeChainID = a.clientConfig.ActiveChainID()
		configVersion = strconv.Itoa(a.clientConfig.Version)
	}

	response := structs.ClientVersionResponse{
		SchemaVersion: 1,
		ServerTime:    now.Format(time.RFC3339),
		ConfigVersion: configVersion,
		Platform:      platform,
		Status:        status,
		Minimum:       minimum,
		ForceUpdate:   forceUpdate,
		Maintenance:   maintenance,
		UpdateURL:     envString("CLIENT_UPDATE_URL_"+strings.ToUpper(platform), envString("CLIENT_UPDATE_URL", "")),
		Message:       message,
		Features: structs.ClientVersionFeatures{
			DynamicConfigRequired: envBool("CLIENT_DYNAMIC_CONFIG_REQUIRED", true),
			CeloRequired:          envBool("CLIENT_CELO_REQUIRED", activeChainID == 42220),
		},
	}

	writeJSON(w, http.StatusOK, response)
}

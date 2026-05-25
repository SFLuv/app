package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

func (a *AppService) GetClientConfig(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.clientConfig == nil {
		http.Error(w, "client config is not loaded", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(a.clientConfig.RawJSON())
}

func (a *AppService) GetClientVersion(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	platform := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("platform")))
	if platform == "" {
		platform = "unknown"
	}

	build := envInt("CLIENT_DEFAULT_BUILD", 0)
	if requestedBuild := strings.TrimSpace(r.URL.Query().Get("build")); requestedBuild != "" {
		if parsed, err := strconv.Atoi(requestedBuild); err == nil {
			build = parsed
		}
	}
	version := strings.TrimSpace(r.URL.Query().Get("version"))
	if version == "" {
		version = envString("CLIENT_DEFAULT_VERSION", "0.0.0")
	}

	minimum := structs.ClientVersionInfo{
		Version: envStringForPlatform("CLIENT_MIN_VERSION", platform, "1.0.0"),
		Build:   envIntForPlatform("CLIENT_MIN_BUILD", platform, 1),
	}
	recommended := structs.ClientVersionInfo{
		Version: envStringForPlatform("CLIENT_RECOMMENDED_VERSION", platform, minimum.Version),
		Build:   envIntForPlatform("CLIENT_RECOMMENDED_BUILD", platform, minimum.Build),
	}
	current := structs.ClientVersionInfo{
		Version: envStringForPlatform("CLIENT_CURRENT_VERSION", platform, recommended.Version),
		Build:   envIntForPlatform("CLIENT_CURRENT_BUILD", platform, recommended.Build),
	}

	status := "ok"
	forceUpdate := false
	maintenance := envBool("CLIENT_MAINTENANCE", false)
	message := envString("CLIENT_VERSION_MESSAGE", "")

	if maintenance {
		status = "maintenance"
		message = envString("CLIENT_MAINTENANCE_MESSAGE", "SFLUV is temporarily unavailable while maintenance is in progress.")
	} else if platform != "ios" && platform != "android" && platform != "web" {
		status = "unsupported_platform"
		forceUpdate = true
		message = envString("CLIENT_UNSUPPORTED_PLATFORM_MESSAGE", "This app version is not supported.")
	} else if build < minimum.Build {
		status = "update_required"
		forceUpdate = true
		message = envString("CLIENT_UPDATE_REQUIRED_MESSAGE", "An SFLUV Wallet update is required.")
	} else if build < recommended.Build {
		status = "update_recommended"
		message = envString("CLIENT_UPDATE_RECOMMENDED_MESSAGE", "A newer SFLUV Wallet update is available.")
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
		Recommended:   recommended,
		Current:       current,
		ForceUpdate:   forceUpdate,
		Maintenance:   maintenance,
		UpdateURL:     envString("CLIENT_UPDATE_URL_"+strings.ToUpper(platform), envString("CLIENT_UPDATE_URL", "")),
		Message:       message,
		Features: structs.ClientVersionFeatures{
			DynamicConfigRequired: envBool("CLIENT_DYNAMIC_CONFIG_REQUIRED", true),
			CeloRequired:          envBool("CLIENT_CELO_REQUIRED", activeChainID == 42220),
		},
	}

	_ = version
	writeJSON(w, http.StatusOK, response)
}

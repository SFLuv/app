package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/structs"
)

const (
	clientPlatformHeader = "X-SFLUV-Client-Platform"
	clientVersionHeader  = "X-SFLUV-Client-Version"
	clientBuildHeader    = "X-SFLUV-Client-Build"

	legacyMobileClientBlockEnvKey = "CLIENT_VERSION_LEGACY_BLOCK_ENABLED"
	legacyMobileClientVersion     = "1.0.0"
	legacyMobileClientBuild       = "1"
	outdatedMobileClientBody      = "Client version out of date, please update your SFLuv app."
)

func parseClientBuildNumber(build string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(build))
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func clientKeyForObservation(userID string, platform string) string {
	if strings.TrimSpace(userID) == "" {
		return ""
	}
	normalizedPlatform := strings.ToLower(strings.TrimSpace(platform))
	if normalizedPlatform == "" {
		normalizedPlatform = "unknown"
	}
	return fmt.Sprintf("user:%s:platform:%s", strings.TrimSpace(userID), normalizedPlatform)
}

func inferLegacyMobilePlatform(r *http.Request) string {
	if r == nil {
		return "mobile"
	}
	ua := strings.ToLower(r.UserAgent())
	switch {
	case strings.Contains(ua, "android"), strings.Contains(ua, "okhttp"):
		return "android"
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"), strings.Contains(ua, "cfnetwork"), strings.Contains(ua, "darwin"):
		return "ios"
	default:
		return "mobile"
	}
}

func hasClientVersionHeaders(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.TrimSpace(r.Header.Get(clientPlatformHeader)) != "" ||
		strings.TrimSpace(r.Header.Get(clientVersionHeader)) != "" ||
		strings.TrimSpace(r.Header.Get(clientBuildHeader)) != ""
}

func isBrowserLikeRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	return strings.TrimSpace(r.Header.Get("Origin")) != "" ||
		strings.TrimSpace(r.Header.Get("Referer")) != "" ||
		strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")) != "" ||
		strings.TrimSpace(r.Header.Get("Sec-Fetch-Mode")) != ""
}

func isLikelyLegacyMobileClient(r *http.Request) bool {
	if r == nil || hasClientVersionHeaders(r) || isBrowserLikeRequest(r) {
		return false
	}

	ua := strings.ToLower(r.UserAgent())
	if ua == "" {
		return false
	}
	// Browsers (and WebViews) always identify as Mozilla; the released native
	// app uses CFNetwork/Darwin (iOS) or okhttp (Android) user agents, so any
	// Mozilla UA is web traffic even when Origin/Referer/Sec-Fetch are absent.
	if strings.Contains(ua, "mozilla") {
		return false
	}
	mobileMarkers := []string{
		"android",
		"cfnetwork",
		"darwin",
		"expo",
		"iphone",
		"ipad",
		"okhttp",
		"react-native",
		"reactnative",
		"sfluv",
	}
	for _, marker := range mobileMarkers {
		if strings.Contains(ua, marker) {
			return true
		}
	}

	return false
}

func legacyMobileClientBlockEnabled() bool {
	return envBool(legacyMobileClientBlockEnvKey, true)
}

func (a *AppService) recordClientVersionObservation(ctx context.Context, userID string, source string, r *http.Request) {
	if a == nil || a.db == nil || r == nil {
		return
	}

	if strings.TrimSpace(userID) == "" {
		return
	}

	platform := strings.ToLower(strings.TrimSpace(r.Header.Get(clientPlatformHeader)))
	version := strings.TrimSpace(r.Header.Get(clientVersionHeader))
	build := strings.TrimSpace(r.Header.Get(clientBuildHeader))
	clientKey := clientKeyForObservation(userID, platform)
	if clientKey == "" {
		return
	}
	if platform == "" && version == "" && build == "" {
		return
	}

	observation := structs.ClientVersionObservation{
		UserId:      strings.TrimSpace(userID),
		ClientKey:   clientKey,
		Platform:    platform,
		Version:     strings.TrimSpace(version),
		Build:       strings.TrimSpace(build),
		BuildNumber: parseClientBuildNumber(build),
		UserAgent:   r.UserAgent(),
		Source:      strings.TrimSpace(source),
		SeenAt:      time.Now().UTC(),
	}
	if err := a.db.RecordClientVersionObservation(ctx, observation); err != nil && a.logger != nil {
		a.logger.Logf("error recording client version observation for %s: %s", userID, err)
	}
}

func (a *AppService) recordLegacyMobileClientObservation(ctx context.Context, userID string, source string, r *http.Request) {
	if a == nil || a.db == nil || strings.TrimSpace(userID) == "" {
		return
	}
	platform := inferLegacyMobilePlatform(r)
	observation := structs.ClientVersionObservation{
		UserId:         strings.TrimSpace(userID),
		ClientKey:      clientKeyForObservation(userID, platform),
		Platform:       platform,
		Version:        legacyMobileClientVersion,
		Build:          legacyMobileClientBuild,
		BuildNumber:    parseClientBuildNumber(legacyMobileClientBuild),
		UserAgent:      r.UserAgent(),
		Source:         strings.TrimSpace(source),
		LegacyInferred: true,
		SeenAt:         time.Now().UTC(),
	}
	if err := a.db.RecordClientVersionObservation(ctx, observation); err != nil && a.logger != nil {
		a.logger.Logf("error recording inferred legacy client for %s: %s", userID, err)
	}
}

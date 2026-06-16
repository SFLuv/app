package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func platformEnvSuffix(platform string) string {
	switch platform {
	case "ios":
		return "IOS"
	case "android":
		return "ANDROID"
	case "web":
		return "WEB"
	default:
		return ""
	}
}

func envStringForPlatform(baseKey string, platform string, fallback string) string {
	if suffix := platformEnvSuffix(platform); suffix != "" {
		if value := strings.TrimSpace(os.Getenv(baseKey + "_" + suffix)); value != "" {
			return value
		}
	}
	return envString(baseKey, fallback)
}

func envIntForPlatform(baseKey string, platform string, fallback int) int {
	if suffix := platformEnvSuffix(platform); suffix != "" {
		if value := strings.TrimSpace(os.Getenv(baseKey + "_" + suffix)); value != "" {
			parsed, err := strconv.Atoi(value)
			if err == nil {
				return parsed
			}
		}
	}
	return envInt(baseKey, fallback)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

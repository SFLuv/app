package utils

import (
	"os"
	"strings"
)

// PublicBackendBase is the externally reachable origin of this backend.
//
// Empty when nothing is configured, which yields root-relative URLs. That still
// resolves for a same-origin caller, and it is a better failure than emitting a
// localhost URL to a browser on someone else's machine.
func PublicBackendBase() string {
	for _, key := range []string{"PUBLIC_BACKEND_URL", "NEXT_PUBLIC_BACKEND_URL", "MCP_PUBLIC_BASE_URL"} {
		if value := strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/"); value != "" {
			return value
		}
	}
	return ""
}

// PublicBackendURL turns a root-relative API path into the URL an outside
// client should fetch.
func PublicBackendURL(path string) string {
	return PublicBackendBase() + path
}

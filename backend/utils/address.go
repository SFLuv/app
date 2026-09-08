package utils

// Address list helpers. These outlived the W9 code they were written for: the
// threshold reader that used to live here is gone, replaced by one that reads
// the token's real decimals instead of an env var whose name claimed eighteen
// when the token has six.

import (
	"strings"
	"time"
)

func NormalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

func ParseAddressList(raw string) []string {
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		addr := NormalizeAddress(part)
		if addr == "" {
			continue
		}
		if seen[addr] {
			continue
		}
		seen[addr] = true
		out = append(out, addr)
	}
	return out
}

func MergeAddressLists(base []string, extras ...string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, addr := range base {
		n := NormalizeAddress(addr)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	for _, addr := range extras {
		n := NormalizeAddress(addr)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func IsAddressInList(address string, list []string) bool {
	addr := NormalizeAddress(address)
	for _, item := range list {
		if addr == NormalizeAddress(item) {
			return true
		}
	}
	return false
}

func CurrentYearBounds() (int, int64, int64) {
	now := time.Now().UTC()
	year := now.Year()
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).Unix()
	end := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC).Unix()
	return year, start, end
}

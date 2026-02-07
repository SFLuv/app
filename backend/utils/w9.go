package utils

import (
	"fmt"
	"math/big"
	"os"
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

func W9Threshold() (*big.Int, error) {
	if weiOverride := os.Getenv("W9_LIMIT_WEI"); weiOverride != "" {
		override, ok := new(big.Int).SetString(weiOverride, 10)
		if !ok {
			return nil, fmt.Errorf("invalid W9_LIMIT_WEI value %s", weiOverride)
		}
		return override, nil
	}

	decimalString := os.Getenv("TOKEN_DECIMALS")
	if decimalString == "" {
		return nil, fmt.Errorf("TOKEN_DECIMALS not set")
	}
	decimals, ok := new(big.Int).SetString(decimalString, 10)
	if !ok {
		return nil, fmt.Errorf("invalid TOKEN_DECIMALS value %s", decimalString)
	}

	if sfluvOverride := os.Getenv("W9_LIMIT_SFLUV"); sfluvOverride != "" {
		override, ok := new(big.Int).SetString(sfluvOverride, 10)
		if !ok {
			return nil, fmt.Errorf("invalid W9_LIMIT_SFLUV value %s", sfluvOverride)
		}
		return new(big.Int).Mul(decimals, override), nil
	}

	return new(big.Int).Mul(decimals, big.NewInt(600)), nil
}

func CurrentYearBounds() (int, int64, int64) {
	now := time.Now().UTC()
	year := now.Year()
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).Unix()
	end := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC).Unix()
	return year, start, end
}

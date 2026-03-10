package handlers

import (
	"math/big"
	"testing"
)

func TestRequiresApprovedW9AtThreshold(t *testing.T) {
	limit := big.NewInt(600)

	if !requiresApprovedW9(big.NewInt(600), limit) {
		t.Fatal("expected W9 approval to be required at the threshold")
	}

	if !requiresApprovedW9(big.NewInt(601), limit) {
		t.Fatal("expected W9 approval to be required above the threshold")
	}

	if requiresApprovedW9(big.NewInt(599), limit) {
		t.Fatal("expected W9 approval to be optional below the threshold")
	}
}

func TestResolveW9AdminEmail(t *testing.T) {
	t.Setenv("W9_ADMIN_EMAIL", "")
	t.Setenv("IMPROVER_ADMIN_EMAIL", "")
	t.Setenv("AFFILIATE_ADMIN_EMAIL", "")

	if got := resolveW9AdminEmail(); got != "admin@sfluv.org" {
		t.Fatalf("expected default admin email, got %q", got)
	}

	t.Setenv("AFFILIATE_ADMIN_EMAIL", "affiliate@example.com")
	if got := resolveW9AdminEmail(); got != "affiliate@example.com" {
		t.Fatalf("expected affiliate fallback, got %q", got)
	}

	t.Setenv("IMPROVER_ADMIN_EMAIL", "improver@example.com")
	if got := resolveW9AdminEmail(); got != "improver@example.com" {
		t.Fatalf("expected improver fallback, got %q", got)
	}

	t.Setenv("W9_ADMIN_EMAIL", "w9@example.com")
	if got := resolveW9AdminEmail(); got != "w9@example.com" {
		t.Fatalf("expected W9 env override, got %q", got)
	}
}

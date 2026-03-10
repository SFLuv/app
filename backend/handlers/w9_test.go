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

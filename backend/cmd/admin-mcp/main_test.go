package main

import (
	"os"
	"testing"
)

func TestSafeLimit(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "default", in: 0, want: defaultLimit},
		{name: "negative", in: -1, want: defaultLimit},
		{name: "keeps small", in: 25, want: 25},
		{name: "caps large", in: maxLimit + 1, want: maxLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeLimit(tt.in); got != tt.want {
				t.Fatalf("safeLimit(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestEnvInt64(t *testing.T) {
	t.Setenv("SFLUV_TEST_CHAIN_ID", "123")
	if got := envInt64(1, "MISSING_CHAIN_ID", "SFLUV_TEST_CHAIN_ID"); got != 123 {
		t.Fatalf("envInt64 got %d, want 123", got)
	}

	t.Setenv("SFLUV_TEST_BAD_CHAIN_ID", "nope")
	if got := envInt64(7, "SFLUV_TEST_BAD_CHAIN_ID"); got != 7 {
		t.Fatalf("envInt64 fallback got %d, want 7", got)
	}

	_ = os.Unsetenv("SFLUV_TEST_CHAIN_ID")
}

func TestRolesFromBools(t *testing.T) {
	roles := rolesFromBools([10]bool{true, false, false, true, true})
	want := []string{"admin", "improver", "proposer"}
	if len(roles) != len(want) {
		t.Fatalf("roles length = %d, want %d (%v)", len(roles), len(want), roles)
	}
	for i := range want {
		if roles[i] != want[i] {
			t.Fatalf("roles[%d] = %q, want %q", i, roles[i], want[i])
		}
	}
}

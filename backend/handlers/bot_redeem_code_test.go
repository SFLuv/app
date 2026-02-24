package handlers

import "testing"

func TestNormalizeRedeemCode(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "exact uuid remains unchanged",
			raw:  "9f6a0d25-3a9a-4e45-a7cb-27f3efda2c57",
			want: "9f6a0d25-3a9a-4e45-a7cb-27f3efda2c57",
		},
		{
			name: "trailing 26 is removed by uuid extraction",
			raw:  "9f6a0d25-3a9a-4e45-a7cb-27f3efda2c5726",
			want: "9f6a0d25-3a9a-4e45-a7cb-27f3efda2c57",
		},
		{
			name: "encoded ampersand suffix is stripped",
			raw:  "9f6a0d25-3a9a-4e45-a7cb-27f3efda2c57%26page%3Dredeem",
			want: "9f6a0d25-3a9a-4e45-a7cb-27f3efda2c57",
		},
		{
			name: "decoded ampersand suffix is stripped",
			raw:  "9f6a0d25-3a9a-4e45-a7cb-27f3efda2c57&page=redeem",
			want: "9f6a0d25-3a9a-4e45-a7cb-27f3efda2c57",
		},
		{
			name: "unknown code falls back to lowercase",
			raw:  "ABC",
			want: "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRedeemCode(tt.raw)
			if got != tt.want {
				t.Fatalf("normalizeRedeemCode(%q) = %q; want %q", tt.raw, got, tt.want)
			}
		})
	}
}

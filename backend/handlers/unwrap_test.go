package handlers

import "testing"

func TestSmallRedemptionCeilingUsesTokenMultiplier(t *testing.T) {
	tests := []struct {
		name       string
		multiplier string
		want       string
	}{
		{
			name:       "six decimals",
			multiplier: "1000000",
			want:       "500000000",
		},
		{
			name:       "eighteen decimals",
			multiplier: "1000000000000000000",
			want:       "500000000000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TOKEN_DECIMALS", tt.multiplier)

			got, err := smallRedemptionCeilingBaseUnits()
			if err != nil {
				t.Fatalf("smallRedemptionCeilingBaseUnits() error = %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("smallRedemptionCeilingBaseUnits() = %s; want %s", got.String(), tt.want)
			}
		})
	}
}

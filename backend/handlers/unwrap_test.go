package handlers

import "testing"

func TestMinimumFollowupUnwrapAmountWeiUsesTokenMultiplier(t *testing.T) {
	tests := []struct {
		name       string
		multiplier string
		want       string
	}{
		{
			name:       "six decimals",
			multiplier: "1000000",
			want:       "100000000",
		},
		{
			name:       "eighteen decimals",
			multiplier: "1000000000000000000",
			want:       "100000000000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TOKEN_DECIMALS", tt.multiplier)

			got, err := minimumFollowupUnwrapAmountWei()
			if err != nil {
				t.Fatalf("minimumFollowupUnwrapAmountWei() error = %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("minimumFollowupUnwrapAmountWei() = %s; want %s", got.String(), tt.want)
			}
		})
	}
}

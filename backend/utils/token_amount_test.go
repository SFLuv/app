package utils

import "testing"

func TestFormatTokenAmountFromStrings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		amount         string
		multiplier     string
		fractionDigits int
		want           string
	}{
		{
			name:           "whole token",
			amount:         "1000000000000000000",
			multiplier:     "1000000000000000000",
			fractionDigits: 2,
			want:           "1.00",
		},
		{
			name:           "fractional token",
			amount:         "1050000000000000000",
			multiplier:     "1000000000000000000",
			fractionDigits: 2,
			want:           "1.05",
		},
		{
			name:           "rounds half up",
			amount:         "1049999999999999999",
			multiplier:     "1000000000000000000",
			fractionDigits: 2,
			want:           "1.05",
		},
		{
			name:           "large amount without float drift",
			amount:         "123456789123456789000000000",
			multiplier:     "1000000000000000000",
			fractionDigits: 2,
			want:           "123456789.12",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := FormatTokenAmountFromStrings(tt.amount, tt.multiplier, tt.fractionDigits)
			if err != nil {
				t.Fatalf("FormatTokenAmountFromStrings returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("FormatTokenAmountFromStrings = %q, want %q", got, tt.want)
			}
		})
	}
}

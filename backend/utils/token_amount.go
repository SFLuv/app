package utils

import (
	"fmt"
	"math/big"
	"strings"
)

func FormatTokenAmountFromStrings(amountRaw string, multiplierRaw string, fractionDigits int) (string, error) {
	amountRaw = strings.TrimSpace(amountRaw)
	multiplierRaw = strings.TrimSpace(multiplierRaw)
	if amountRaw == "" {
		return "", fmt.Errorf("amount is required")
	}
	if multiplierRaw == "" {
		return "", fmt.Errorf("token multiplier is required")
	}

	amount, ok := new(big.Int).SetString(amountRaw, 10)
	if !ok {
		return "", fmt.Errorf("invalid token amount %q", amountRaw)
	}
	multiplier, ok := new(big.Int).SetString(multiplierRaw, 10)
	if !ok {
		return "", fmt.Errorf("invalid token multiplier %q", multiplierRaw)
	}

	return FormatTokenAmount(amount, multiplier, fractionDigits)
}

func FormatTokenAmount(amount *big.Int, multiplier *big.Int, fractionDigits int) (string, error) {
	if amount == nil {
		return "", fmt.Errorf("amount is required")
	}
	if multiplier == nil || multiplier.Sign() <= 0 {
		return "", fmt.Errorf("token multiplier must be greater than 0")
	}
	if fractionDigits < 0 {
		return "", fmt.Errorf("fraction digits must be 0 or greater")
	}

	negative := amount.Sign() < 0
	absAmount := new(big.Int).Abs(new(big.Int).Set(amount))

	if fractionDigits == 0 {
		rounded, remainder := new(big.Int).QuoRem(absAmount, multiplier, new(big.Int))
		if new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(multiplier) >= 0 {
			rounded.Add(rounded, big.NewInt(1))
		}
		if negative && rounded.Sign() > 0 {
			return "-" + rounded.String(), nil
		}
		return rounded.String(), nil
	}

	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fractionDigits)), nil)
	scaled := new(big.Int).Mul(absAmount, scale)
	rounded, remainder := new(big.Int).QuoRem(scaled, multiplier, new(big.Int))
	if new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(multiplier) >= 0 {
		rounded.Add(rounded, big.NewInt(1))
	}

	whole, fractional := new(big.Int).QuoRem(rounded, scale, new(big.Int))
	fractionalText := fractional.String()
	if len(fractionalText) < fractionDigits {
		fractionalText = strings.Repeat("0", fractionDigits-len(fractionalText)) + fractionalText
	}

	formatted := whole.String() + "." + fractionalText
	if negative && rounded.Sign() > 0 {
		return "-" + formatted, nil
	}
	return formatted, nil
}

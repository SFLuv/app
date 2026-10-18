package funding

import (
	"fmt"
	"os"
	"strings"
)

// envValue reads a trimmed environment variable.
func envValue(name string) string { return strings.TrimSpace(os.Getenv(name)) }

// BuildAlertBody writes the low-faucet email.
//
// It states the problem in the first line and the consequence in the second,
// because the person reading it on a phone needs to know whether this can wait.
// The account list is included so they can see what the faucet is still holding
// up, and what it has already failed to fund.
func BuildAlertBody(snap Snapshot, result SweepResult) string {
	var b strings.Builder

	lowAccounts := []string{}
	for _, account := range snap.Accounts {
		if account.Low {
			lowAccounts = append(lowAccounts, account.Name)
		}
	}

	switch {
	case snap.Faucet.LowCelo && snap.Faucet.LowSfluv:
		fmt.Fprintf(&b, "The faucet is low on both CELO and SFLUV.\n\n")
	case snap.Faucet.LowCelo:
		fmt.Fprintf(&b, "The faucet is low on CELO.\n\n")
	case snap.Faucet.LowSfluv:
		fmt.Fprintf(&b, "The faucet is low on SFLUV.\n\n")
	case len(lowAccounts) > 0:
		// The faucet is fine, so this is an account that will not fix itself:
		// either top-ups are off, or this account is watch-only.
		fmt.Fprintf(&b, "%s is below its floor and nothing is going to top it up.\n\n",
			strings.Join(lowAccounts, ", "))
		b.WriteString("Automatic top-ups are off for it, so it needs funding by hand. " +
			"The faucet itself has enough to cover it.\n\n")
	}

	if snap.Faucet.LowCelo {
		b.WriteString("CELO is the gas every other account is funded from. When it runs out, " +
			"top-ups stop, and the accounts below drain one by one until the thing each of them " +
			"does stops working.\n\n")
	}
	if snap.Faucet.LowSfluv {
		b.WriteString("SFLUV is what volunteer rewards are paid from. When it runs out, rewards fail.\n\n")
	}

	fmt.Fprintf(&b, "Faucet %s\n", snap.Faucet.Address)
	fmt.Fprintf(&b, "  CELO   %.4f   (floor %.4f)%s\n",
		snap.Faucet.Celo, snap.Faucet.MinCelo, flag(snap.Faucet.LowCelo))
	fmt.Fprintf(&b, "  SFLUV  %.2f   (floor %.2f)%s\n\n",
		snap.Faucet.Sfluv, snap.Faucet.MinSfluv, flag(snap.Faucet.LowSfluv))

	if len(snap.Accounts) > 0 {
		b.WriteString("Accounts funded from it:\n")
		for _, account := range snap.Accounts {
			if account.Error != "" {
				fmt.Fprintf(&b, "  %-26s unknown (%s)\n", account.Name, account.Error)
				continue
			}
			fmt.Fprintf(&b, "  %-26s %.4f CELO   (floor %.4f)%s\n",
				account.Name, account.Celo, account.MinCelo, flag(account.Low))
		}
		b.WriteString("\n")
	}

	if len(result.Funded) > 0 {
		b.WriteString("Topped up on this check:\n")
		for _, line := range result.Funded {
			fmt.Fprintf(&b, "  %s\n", line)
		}
		b.WriteString("\n")
	}
	if len(result.Skipped) > 0 {
		b.WriteString("Could not top up:\n")
		for _, line := range result.Skipped {
			fmt.Fprintf(&b, "  %s\n", line)
		}
		b.WriteString("\n")
	}
	if len(result.Errors) > 0 {
		b.WriteString("Errors:\n")
		for _, line := range result.Errors {
			fmt.Fprintf(&b, "  %s\n", line)
		}
		b.WriteString("\n")
	}

	if !snap.AutoTopUpEnabled {
		b.WriteString("Automatic top-ups are switched off, so every account above needs funding by hand.\n")
	}
	return b.String()
}

func flag(low bool) string {
	if low {
		return "   LOW"
	}
	return ""
}

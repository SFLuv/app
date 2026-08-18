package handlers

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// pushW9Notice sends one W9 alert to every device a person has registered.
//
// Copied in shape from pushEventBlast: same subscription lookup, same token
// dedup, same tolerance for a device that has gone away. The data payload
// carries a type the app routes on — and unlike event pushes it carries no
// role, because owing a tax form is not tied to being a volunteer or an
// improver.
func (a *AppService) pushW9Notice(ctx context.Context, userID string, kind string, title string, body string) bool {
	subscriptions, err := a.db.GetMobilePushSubscriptionsByUser(ctx, userID)
	if err != nil {
		return false
	}

	delivered := false
	tokensSeen := map[string]struct{}{}
	for _, subscription := range subscriptions {
		if subscription == nil || !subscription.Active || !subscription.DeviceRegistered {
			continue
		}
		token := strings.TrimSpace(subscription.Token)
		if token == "" {
			continue
		}
		if _, ok := tokensSeen[token]; ok {
			continue
		}
		tokensSeen[token] = struct{}{}

		if _, err := sendExpoPushNotification(ctx, token, title, body, map[string]string{
			"type": kind,
		}); err != nil {
			a.logger.Logf("error pushing a w9 notice to %s: %s", userID, err)
			continue
		}
		delivered = true
	}
	return delivered
}

// pushW9EscrowHeld tells someone their reward has arrived but is waiting on a
// form.
//
// Worded so the money reads as theirs, because it is. "Held" invites a
// different reaction from "blocked", and the amount is stated so the size of
// the thing being asked for is obvious.
func (a *AppService) pushW9EscrowHeld(ctx context.Context, userID string, taxYear int) {
	escrowed, count, _, _, err := a.db.SumUserEscrowAndBackPay(ctx, userID, taxYear)
	if err != nil || count == 0 {
		return
	}

	window := escrowWindow()
	a.pushW9Notice(ctx, userID, "w9_escrow_held",
		fmt.Sprintf("%s SFLUV is waiting for you", formatSfluvBase(escrowed)),
		fmt.Sprintf(
			"You've earned enough this year that we need a W-9 on file. Fill one in within %d days and we'll send it straight over.",
			int(window.Hours()/24),
		),
	)
}

// pushW9Reminder nudges someone who still owes a form.
//
// The pre-expiry warning is the one that matters: after that point releasing
// stops being automatic and starts needing a person, so the deadline is stated
// plainly rather than left to be discovered.
func (a *AppService) pushW9Reminder(ctx context.Context, userID string, taxYear int, seq int, remaining time.Duration) {
	escrowed, escrowCount, backPay, backPayCount, err := a.db.SumUserEscrowAndBackPay(ctx, userID, taxYear)
	if err != nil {
		return
	}

	if seq == 1 && escrowCount > 0 {
		hours := int(remaining.Hours())
		if hours < 1 {
			hours = 1
		}
		a.pushW9Notice(ctx, userID, "w9_required",
			fmt.Sprintf("%s SFLUV — about %dh left", formatSfluvBase(escrowed), hours),
			"Finish your W-9 before the window closes and your rewards go out automatically. After that they'll need to be sent manually.",
		)
		return
	}

	if backPayCount > 0 {
		a.pushW9Notice(ctx, userID, "w9_required",
			fmt.Sprintf("%s SFLUV is still owed to you", formatSfluvBase(backPay)),
			"Complete your W-9 and we'll arrange for your rewards to be sent.",
		)
	}
}

// pushW9EscrowReleased confirms money is on its way, and is honest about the
// part that is not.
func (a *AppService) pushW9EscrowReleased(ctx context.Context, userID string, taxYear int, released *big.Int, backPayRequested int64) {
	if released != nil && released.Sign() > 0 {
		body := "Thanks — your W-9 is on file and your held rewards are on the way."
		if backPayRequested > 0 {
			body = "Thanks — your W-9 is on file. Your held rewards are on the way, and the rest is queued for us to send shortly."
		}
		a.pushW9Notice(ctx, userID, "w9_escrow_released",
			fmt.Sprintf("%s SFLUV is on the way", formatSfluvBase(released)), body)
		return
	}

	if backPayRequested > 0 {
		a.pushW9Notice(ctx, userID, "w9_escrow_released",
			"Your W-9 is on file",
			"Thanks. Your outstanding rewards are queued for us to send shortly.",
		)
	}
}

// formatSfluvBase turns base units into something a person can read, trimming
// the trailing zeros that make a whole number look like a decimal.
func formatSfluvBase(amount *big.Int) string {
	if amount == nil {
		return "0"
	}
	multiplier, err := getTokenMultiplier()
	if err != nil || multiplier == nil || multiplier.Sign() <= 0 {
		multiplier = big.NewInt(1_000_000)
	}

	whole := new(big.Int)
	remainder := new(big.Int)
	whole.QuoRem(amount, multiplier, remainder)
	if remainder.Sign() == 0 {
		return whole.String()
	}

	decimals := len(multiplier.String()) - 1
	fraction := strings.TrimRight(fmt.Sprintf("%0*d", decimals, remainder), "0")
	if fraction == "" {
		return whole.String()
	}
	return whole.String() + "." + fraction
}

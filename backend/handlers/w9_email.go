package handlers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SFLuv/app/backend/db"
	"github.com/SFLuv/app/backend/utils"
)

// W-9 email that is sent without an admin pressing anything.
//
// Tier prompts tend to fire at redemption events: somebody scans a QR code,
// the reward crosses 400 or 500 SFLUV, and the app shows a prompt while they
// are standing in a crowd with no Social Security card to hand. On 2026-10-25
// five people saw that prompt and dismissed it within seconds to minutes, and a
// week later none had filed. A push and a modal are gone once dismissed; an
// email with a link that does not expire is still there when they get home.
//
// Each tier emails once per tax year, keyed to the same "first time" signal as
// the push, so the most anybody receives in a year is four: the notice, the
// warning, the first held reward and the first refused one. The last two only
// exist while enforcement is on; off, the payout gate downgrades them to the
// warning, so the email never says money is held when it is not.

// w9SelfServeMismatchLimit is how many failed TIN matches a person may answer
// by filing again on their own. At this many failures, filing again is refused
// and they are asked to contact support: a second failure is rarely another
// typo, and TaxBandits limits how many matches it runs per person, so a loop of
// self-service retries would burn credits and attempts without fixing anything.
const w9SelfServeMismatchLimit = 2

const w9SupportEmail = "techsupport@sfluv.org"

// w9TierEmailsEnabled is the off switch for automatic W-9 email.
//
// On unless set to false. It sends mail to real people on its own, so a wrong
// call has to be revocable with a restart rather than a deploy. Admin requests
// and TIN mismatch emails are not covered: the first is somebody asking for an
// email, and the second is the only way a person learns their form failed.
func w9TierEmailsEnabled() bool {
	return envBool("W9_TIER_EMAILS_ENABLED", true)
}

// emailW9TierAsync sends the tier email off the payout's request path.
//
// Tier hooks run inline with a redemption, before the transfer, and the person
// is waiting on the scan. Mail is slow and allowed to fail, so it is never
// something they wait for.
func (a *AppService) emailW9TierAsync(userID string, taxYear int, tier string) {
	if a == nil || strings.TrimSpace(userID) == "" || !w9TierEmailsEnabled() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		a.emailW9Tier(ctx, userID, taxYear, tier)
	}()
}

// emailW9Tier mails the person their form link with wording that fits the tier
// they just reached and whether the gate is actually holding money.
func (a *AppService) emailW9Tier(ctx context.Context, userID string, taxYear int, tier string) {
	filing, err := a.db.GetW9Filing(ctx, userID, taxYear)
	if err != nil || filing == nil {
		return
	}
	// A cleared filing needs nothing. A rejected one gets the correction email
	// instead, and its link is the form that was rejected, which must never be
	// sent out again.
	if db.W9StatusClears(filing.Status) || filing.Status == db.W9StatusInvalid {
		return
	}
	formURL := strings.TrimSpace(filing.FormURL)
	if formURL == "" {
		a.logger.Logf("w9 tier email: no form link for %s at %s; skipped", userID, tier)
		return
	}

	earned, err := a.db.SumUserEarnedForYear(ctx, userID, taxYear)
	if err != nil {
		return
	}
	limitBase := w9ThresholdBase()
	amount := formatSfluvBase(earned)
	limit := formatSfluvBase(limitBase)
	enforcing := payoutEnforcementMode() == enforcementEnforce

	var subject, headline, lead string
	switch tier {
	case db.W9TierNotice:
		subject = "A W-9 for your SFLuv rewards"
		headline = fmt.Sprintf("You've earned %s SFLUV this year.", amount)
		if enforcing {
			lead = fmt.Sprintf("Once you reach %s SFLUV in a year, we need a W-9 on file before we can send more. It takes a couple of minutes, and doing it now means nothing gets held up later.", limit)
		} else {
			lead = fmt.Sprintf("Once you reach %s SFLUV in a year, we're required to have a W-9 on file for you. It takes a couple of minutes, and you can do it now.", limit)
		}
	case db.W9TierWarning:
		// Not enforcing, the warning is also what somebody past the line gets,
		// so "close to the limit" would be wrong for them.
		if earned.Cmp(limitBase) >= 0 {
			subject = "We need a W-9 from you"
			headline = fmt.Sprintf("You've earned %s SFLUV this year.", amount)
			lead = fmt.Sprintf("That's past %s SFLUV, so we're required to have a W-9 on file for you. It takes a couple of minutes.", limit)
		} else {
			subject = fmt.Sprintf("You're close to the %s SFLUV limit", limit)
			headline = fmt.Sprintf("You've earned %s SFLUV this year.", amount)
			if enforcing {
				lead = "Fill in your W-9 now and your rewards will keep arriving without interruption."
			} else {
				lead = fmt.Sprintf("Once you reach %s SFLUV we're required to have a W-9 on file, so now is a good time to fill it in.", limit)
			}
		}
	case db.W9TierEscrowed:
		subject = "Your reward is waiting on a W-9"
		headline = fmt.Sprintf("You've reached %s SFLUV for the year.", limit)
		lead = "So your latest reward is being held for you. Fill in your W-9 and we'll send it straight over."
	case db.W9TierBlocked:
		subject = "We couldn't send your reward"
		headline = fmt.Sprintf("You've earned more than %s SFLUV this year.", limit)
		lead = "We don't have a W-9 on file for you yet, so new rewards are paused. Fill in your W-9 to start receiving them again. Your QR code is still good."
	default:
		return
	}

	details := w9EmailParagraphs(a.w9EmailGreeting(ctx, userID), headline+" "+lead) +
		w9EmailButton(formURL, "Fill out your W-9") +
		w9EmailNote("This link doesn't expire. You can also fill it in from the SFLuv app.") +
		w9EmailSupportLine()
	to, sent, why := a.sendW9Email(ctx, userID, subject, "Your W-9 is ready to fill in.", details)
	a.logger.Logf("w9 tier email: %s at %s to %q: sent %t %s", userID, tier, to, sent, why)
}

// onW9TINMismatch tells somebody their W-9 failed the IRS TIN match.
//
// The first failure is nearly always a typo or a name that differs from the
// Social Security card, so it is answered with a fresh, blank form straight
// away: the email carries its link, the push and the in-app prompt open it,
// and nobody has to ask. It costs one TaxBandits credit, and the count below
// bounds it to one per person per year. If the form cannot be made right now
// (the vendor is down), the email says to use the app instead, where tapping
// the prompt makes it (see StartW9).
//
// After w9SelfServeMismatchLimit failures they are asked to contact support
// instead, no form is made, the prompt is left alone, and filing again from the
// app is refused.
//
// Called from the webhook and the sweeper with their context. The form and the
// email go to a goroutine, so a slow vendor or mail call never holds up the
// webhook's reply.
func (a *AppService) onW9TINMismatch(ctx context.Context, userID string, taxYear int) {
	filing, err := a.db.GetW9Filing(ctx, userID, taxYear)
	if err != nil || filing == nil {
		return
	}
	count := filing.TINMismatchCount
	selfServe := count < w9SelfServeMismatchLimit

	if selfServe {
		if _, err := a.armW9Prompt(ctx, userID, taxYear); err != nil {
			a.logger.Logf("w9: could not re-arm the prompt for %s after a failed TIN match: %s", userID, err)
		}
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		formURL := ""
		if selfServe && a.payouts != nil {
			reissued, err := a.payouts.ReissueInvalidW9Request(ctx, userID, taxYear)
			if err != nil {
				a.logger.Logf("w9: could not issue a fresh form to %s after a failed TIN match: %s", userID, err)
			} else if reissued != nil && reissued.Status != db.W9StatusInvalid {
				formURL = strings.TrimSpace(reissued.FormURL)
			}
		}

		if selfServe {
			a.pushW9Notice(ctx, userID, "w9_required",
				"We need a corrected W-9",
				"The name or taxpayer number on your W-9 didn't match IRS records. It's usually a typo. Tap to send a corrected one.",
			)
		} else {
			a.pushW9Notice(ctx, userID, "w9_required",
				"We still couldn't verify your W-9",
				fmt.Sprintf("Your corrected W-9 didn't match IRS records either. Please email %s and we'll help sort it out.", w9SupportEmail),
			)
		}

		to, sent, why := a.emailW9Correction(ctx, userID, taxYear, selfServe, formURL)
		a.logger.Logf("w9: TIN match failed for %s (failure %d, self-serve %t, fresh form %t); email to %q sent %t %s",
			userID, count, selfServe, formURL != "", to, sent, why)

		if !selfServe {
			a.alertW9SupportNeeded(ctx, userID, taxYear, count)
		}
	}()
}

// w9AlertTo is who hears that somebody needs a person to sort out their W-9:
// W9_ALERT_EMAIL, or admin@sfluv.org when that is unset.
func w9AlertTo() string {
	if to := strings.TrimSpace(os.Getenv("W9_ALERT_EMAIL")); to != "" {
		return to
	}
	return "admin@sfluv.org"
}

// alertW9SupportNeeded tells staff that somebody's W-9 has failed the IRS check
// too many times to retry on their own, so a person has to step in.
//
// It names the person and how to reach them, and never carries a taxpayer
// number: the backend does not have one. The form as submitted is in the
// TaxBandits console.
func (a *AppService) alertW9SupportNeeded(ctx context.Context, userID string, taxYear int, failures int) {
	sender := utils.NewEmailSender()
	if sender == nil {
		a.logger.Logf("w9 alert: email is not configured; %s needs support after %d failed TIN matches", userID, failures)
		return
	}
	name := a.w9EmailName(ctx, userID)
	if name == "" {
		name = "(no name on file)"
	}
	contact := "(no email on file)"
	if email, err := a.db.GetUserContactEmail(ctx, userID); err == nil && email != nil && strings.TrimSpace(*email) != "" {
		contact = strings.TrimSpace(*email)
	}
	earned := "unknown"
	if amount, err := a.db.SumUserEarnedForYear(ctx, userID, taxYear); err == nil {
		earned = formatSfluvBase(amount)
	}

	subject := fmt.Sprintf("W-9 needs help: %s failed the IRS check %d times", name, failures)
	details := w9EmailParagraphs(
		fmt.Sprintf("%s's W-9 for %d has failed the IRS TIN match %d times, so the app has stopped letting them retry on their own and asked them to email %s.", name, taxYear, failures, w9SupportEmail),
		"Please reach out, find out what's different between what they're entering and their Social Security card (or the business's EIN letter), then send them a new form from the admin panel's Request a W-9 card.",
	) + w9EmailList(
		"Name: "+name,
		"Email: "+contact,
		fmt.Sprintf("Earned in %d: %s SFLUV", taxYear, earned),
		"User id: "+userID,
	) + w9EmailNote("The form they submitted is in the TaxBandits console. This email deliberately carries no taxpayer number.")

	html := utils.BuildStyledEmail("A W-9 needs a person", subject, details)
	to := w9AlertTo()
	if err := sender.SendEmail(to, "SFLuv admin", subject, html, utils.NotificationFromEmail(), "SFLuv"); err != nil {
		a.logger.Logf("w9 alert to %s about %s failed: %s", to, userID, err)
		return
	}
	a.logger.Logf("w9 alert: told %s that %s needs support after %d failed TIN matches", to, userID, failures)
}

// emailW9Correction explains a failed TIN match and what to do next.
//
// formURL is the fresh form, when one could be made. The rejected form's link
// is never sent: that form cannot be submitted again.
func (a *AppService) emailW9Correction(ctx context.Context, userID string, taxYear int, selfServe bool, formURL string) (string, bool, string) {
	greeting := a.w9EmailGreeting(ctx, userID)
	var subject, subtitle, details string
	if selfServe {
		subject = "We need a corrected W-9"
		subtitle = "It's usually a small mistake, and quick to fix."
		held := ""
		if payoutEnforcementMode() == enforcementEnforce {
			held = " Until a corrected W-9 is on file, new rewards may be held."
		}
		how := "We've made you a fresh, blank form. Before you fill it in, please check:"
		if formURL == "" {
			how = "To send a corrected one, open the SFLuv app and tap the W-9 prompt. You'll get a fresh, blank form. Before you submit it, please check:"
		}
		details = w9EmailParagraphs(greeting,
			fmt.Sprintf("The name and taxpayer number on the W-9 you sent us for %d didn't match the IRS's records. That's usually a small mistake, and it's quick to fix.", taxYear),
			how,
		) + w9EmailList(
			"Your Social Security number or EIN, digit by digit.",
			"Your name exactly as it appears on your Social Security card, including your legal last name.",
			"If you're filing for a business, its name together with the business's EIN, not your personal Social Security number.",
		)
		if formURL != "" {
			details += w9EmailButton(formURL, "Fill out a corrected W-9") +
				w9EmailNote("This link doesn't expire. You can also fill it in from the SFLuv app.")
		}
		details += w9EmailParagraphs("Rewards you've already received aren't affected."+held) + w9EmailSupportLine()
	} else {
		subject = "We still couldn't verify your W-9"
		subtitle = "We'd like to help sort it out."
		details = w9EmailParagraphs(greeting,
			fmt.Sprintf("The name and taxpayer number on your corrected W-9 for %d didn't match the IRS's records either.", taxYear),
			fmt.Sprintf("Rather than have you try again on your own, we'd like to sort it out with you. Please email %s and we'll help.", w9SupportEmail),
			"Rewards you've already received aren't affected.",
		)
	}
	return a.sendW9Email(ctx, userID, subject, subtitle, details)
}

// armW9Prompt brings back the in-app W-9 prompt that fits what the person has
// earned, and returns the tier it used.
//
// Installed apps can only draw the four threshold prompts, so anything that
// needs the person's attention borrows one. Only the first two are ever
// borrowed: the escrow and blocked prompts tell somebody their money is being
// held or refused, and these are not that. Below the warning line it is the
// notice; at or past it, the warning ("close to the limit" reads a little short
// for someone already over it, but what it asks for is true, which the other
// two would not be). Newer builds change the wording using the
// flags on the status response (admin_requested, correction_needed).
func (a *AppService) armW9Prompt(ctx context.Context, userID string, taxYear int) (string, error) {
	tier := db.W9TierNotice
	earned, err := a.db.SumUserEarnedForYear(ctx, userID, taxYear)
	if err != nil {
		return "", err
	}
	if warning := w9Thresholds().Warning; warning != nil && earned.Cmp(warning) >= 0 {
		tier = db.W9TierWarning
	}
	return tier, a.db.RearmW9Tier(ctx, userID, taxYear, tier)
}

// sendW9Email sends one W-9 email to the person's contact address. Returns the
// address used, whether it went, and why not.
func (a *AppService) sendW9Email(ctx context.Context, userID string, subject string, subtitle string, detailsHTML string) (string, bool, string) {
	email, err := a.db.GetUserContactEmail(ctx, userID)
	if err != nil || email == nil || strings.TrimSpace(*email) == "" {
		return "", false, "no email address on file"
	}
	to := strings.TrimSpace(*email)
	sender := utils.NewEmailSender()
	if sender == nil {
		return to, false, "email is not configured"
	}
	html := utils.BuildStyledEmail(subject, subtitle, detailsHTML)
	if err := sender.SendEmail(to, a.w9EmailName(ctx, userID), subject, html, utils.NotificationFromEmail(), "SFLuv"); err != nil {
		a.logger.Logf("w9 email to %s failed: %s", userID, err)
		return to, false, "the email could not be sent"
	}
	return to, true, ""
}

func (a *AppService) w9EmailName(ctx context.Context, userID string) string {
	user, err := a.db.GetUserById(ctx, userID)
	if err != nil || user == nil || user.Name == nil {
		return ""
	}
	return strings.TrimSpace(*user.Name)
}

func (a *AppService) w9EmailGreeting(ctx context.Context, userID string) string {
	if name := a.w9EmailName(ctx, userID); name != "" {
		return "Hi " + name + ","
	}
	return "Hi,"
}

// The fragments below match the styling of the admin W-9 request email, so the
// automatic emails read as the same product. Text is escaped here; callers pass
// plain strings.

func w9EmailParagraphs(lines ...string) string {
	var b strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&b, `<p style="margin:0 0 12px; font-size:14px; color:#111827;">%s</p>`+"\n", utils.EscapeEmailHTML(line))
	}
	return b.String()
}

func w9EmailList(items ...string) string {
	var b strings.Builder
	b.WriteString(`<ul style="margin:0 0 12px; padding-left:20px; font-size:14px; color:#111827;">` + "\n")
	for _, item := range items {
		fmt.Fprintf(&b, `<li style="margin:0 0 6px;">%s</li>`+"\n", utils.EscapeEmailHTML(item))
	}
	b.WriteString("</ul>\n")
	return b.String()
}

func w9EmailButton(url string, label string) string {
	return fmt.Sprintf(`<p style="margin:18px 0;">
  <a href="%s" style="display:inline-block; padding:12px 20px; background:#eb6c6c; color:#ffffff; text-decoration:none; border-radius:8px; font-size:14px; font-weight:600;">%s</a>
</p>
`, utils.EscapeEmailHTML(url), utils.EscapeEmailHTML(label))
}

func w9EmailNote(text string) string {
	return fmt.Sprintf(`<p style="margin:0 0 8px; font-size:13px; color:#6b7280;">%s</p>`+"\n", utils.EscapeEmailHTML(text))
}

func w9EmailSupportLine() string {
	return fmt.Sprintf(`<p style="margin:0; font-size:12px; color:#6b7280;">Questions? Email %s.</p>`+"\n", w9SupportEmail)
}

# Branch scope: `sanchezo/admin-w9-request`

Oct 25 2026 · app (backend + frontend) · **~0.4h active** · based on `main` at `24ec8b4`

Hours are wall-clock time from this session's timestamps, not from the `time-accounting` script. It was one
sitting, about 12:11–12:33 PDT:
- about 0.2h tracing the W-9 request, push, tier-prompt and bell paths across the backend and mobile app,
  including rendering the candidate prompts for a decision;
- about 0.2h building and testing.

## What this branch is

Admins can ask anyone for a W-9 by hand, without waiting for the threshold system. It's for when enforcement is
off, ahead of a large payout, or for anyone the automatic path missed.

**Admin panel (Tax & Escrow → "Request a W-9"):**
1. Search users by name or email.
2. See their filing status and what they've earned this year against the limit.
3. Click Send W-9 request, then confirm.
4. See the outcome per channel inline: email sent to …, push sent / no device, in-app prompt waiting. It's shown
   inline rather than as a toast, because the app mounts no Toaster.

**What the person receives:**
- **An email** with a Fill out your W-9 button. It links straight to the TaxBandits form, and that link is durable.
- **A push** ("SFLuv needs a W-9 from you", type `w9_requested`). The `w9_` prefix routes it to the W-9 prompt on
  installed apps.
- **The in-app prompt.** Installed apps can only draw the four threshold prompts, so the request borrows one:
  `notice_400` below the warning line, `warning_500` at or above it. It never borrows escrow or blocked, because
  those say money is held or refused. The prompt is re-armed even if it was dismissed before.
- **A bell entry** that says "SFLuv has asked you for a W-9". An `admin_request` marker row next to the borrowed
  tier changes the wording. The marker ranks below every real tier, so the prompt is unaffected.

**Routes, both behind `withAdmin`:**
- `GET /admin/w9/{user_id}/status`
- `POST /admin/w9/{user_id}/request`

The request reuses `EnsureW9Request`, so pressing again keeps the same form. It's refused (409) once a W-9 is on
file.

| Item | Hours | Repo |
|---|---|---|
| Tracing the request/push/prompt/bell paths and the installed-app tier handling; rendering the prompt options | 0.2 | app |
| Endpoint, re-arm helper, bell wording, admin card. Throwaway end-to-end test, not committed per the repo rule, run against real Postgres 18 (`cmd/init` schema) with the fake W-9 provider and email/push test mode, all passing: earned 680 with a phone → `warning_500` re-armed despite an earlier dismissal, bell un-dismissed with the new wording, form created, email with the link captured, push captured; a second press reuses the same form; earned 120 with no phone → `notice_400`, email yes, push honestly no; no email on file → reported; filing completed → 409; unknown user → 404 | 0.2 | app |

Volume: 7 files, +517 / −12. 0 migrations (the tier column is plain text), 2 routes.

## Known limits

- **Shadow mode can outrank the request.** It records every tier a payout crosses, including `escrow_600`. A
  person who already has an escrow or blocked tier row will see that prompt ("your reward is being held"), not the
  borrowed one, because the app always shows the highest tier. Whether any such rows exist in production is being
  checked.
- **The next mobile release** should add a dedicated prompt for `admin_request`. The backend already writes the
  marker.
- **The admin card was type-checked but not clicked through in a browser.** Local login needs Privy on port 3000.

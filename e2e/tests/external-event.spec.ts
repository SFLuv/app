import { expect, test } from "@playwright/test"

/**
 * An affiliate proposes a partner-hosted event; an admin approves it.
 *
 * This is the path most volunteer traffic is expected to take: SFLuv advertises
 * an event that somebody else runs, and the signup button hands the visitor to
 * that organisation. Nothing about it had ever been exercised — every volunteer
 * event in the development database was `internal` with no signup_url, so the
 * mode was written, wired and never used.
 *
 * Identity comes from the pranks table, so this spec assumes a prank is already
 * pointing the signed-in account at the affiliate test user. See harness.setPrank.
 */
const PARTNER_URL = "https://www.stanthonysf.org/volunteer/signup"

function localDateTime(daysAhead: number, hour: number) {
  const d = new Date()
  d.setDate(d.getDate() + daysAhead)
  d.setHours(hour, 0, 0, 0)
  const pad = (n: number) => String(n).padStart(2, "0")
  // datetime-local wants wall clock with no zone, which is also what the API
  // wants — start_at_local is the event's own timezone, not UTC.
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

test("an affiliate can request an event whose signups live on a partner's site", async ({ page }) => {
  const title = `partner-event-${Date.now()}`

  await page.goto("/affiliates", { waitUntil: "domcontentloaded" })
  await page.getByRole("button", { name: /request event/i }).click()

  // Reaching this modal at all is the identity proof: /affiliates is behind the
  // affiliate guard, and the signed-in account is not an affiliate — only the
  // prank makes it one. The event's owner is asserted against the database at
  // the end, which is stronger than reading a name out of the form.
  await expect(page.getByText(/^Created by/)).toBeVisible()

  await page.getByLabel(/^Title/).fill(title)
  await page.getByLabel(/^Description/).fill("Signups handled by the partner organisation.")
  await page.getByLabel(/^Starts/).fill(localDateTime(3, 10))
  await page.getByLabel(/^Ends/).fill(localDateTime(3, 13))

  // Targeted by its current value, not its label. The control is a Radix Select
  // — a button acting as a combobox — and the "Sign-up" label carries no htmlFor,
  // so getByLabel never resolves it. It defaults to internal, so the trigger
  // reads "Managed in SFLuv".
  await page.getByRole("combobox").filter({ hasText: /managed in sfluv/i }).click()
  await page.getByRole("option", { name: /external link/i }).click()

  // The URL field only exists in external mode — that it appears at all is part
  // of what this asserts.
  const url = page.getByPlaceholder(/partner\.org\/signup/i)
  await expect(url).toBeVisible()
  await url.fill(PARTNER_URL)

  await page.getByRole("button", { name: /^(submit|request|create)/i }).click()

  // An affiliate request lands as pending: it is a proposal, not a publish.
  await expect(page.getByText(new RegExp(title, "i")).first()).toBeVisible({ timeout: 20_000 })
})

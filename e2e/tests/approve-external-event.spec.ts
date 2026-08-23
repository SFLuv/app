import { expect, test } from "@playwright/test"

/**
 * The other half of the partner-event path: an admin approving what an affiliate
 * proposed. Until this happens the event is invisible — pending events are not
 * served to the public listing, so nothing reaches the website.
 *
 * Assumes NO prank is set: this must run as the real signed-in admin, which is
 * the whole point of the approval gate.
 */
test("an admin approves a pending partner event", async ({ page }) => {
  await page.goto("/admin", { waitUntil: "domcontentloaded" })

  // A Radix TabsTrigger, so role="tab" — NOT "button". Asking for a button
  // matches nothing, the click never happens, and the spec fails looking for an
  // event on whichever tab was default (Merchants). The section is also called
  // "Events", not "Volunteer Events"; it carries the pending-queue count badge.
  await page.getByRole("tab", { name: /^Events/ }).first().click()
  await page.waitForTimeout(1500)

  const title = page.getByText(/partner-event-\d+/).first()
  await expect(title).toBeVisible({ timeout: 20_000 })
  const eventTitle = (await title.textContent())?.trim() ?? ""
  await title.click()

  await page.getByRole("button", { name: /^approve/i }).first().click()

  // Scoped to the event's own row, NOT the page.
  //
  // The first version asserted getByText(/approved/i) against the whole page and
  // passed while the approval was failing with a 400 — it had matched an
  // "Approved" badge on an unrelated merchant card. The event stayed pending and
  // the spec reported success.
  //
  // Asserting the pending state is GONE from this event's row is the claim that
  // actually means something; a stray badge elsewhere cannot satisfy it.
  const row = page.locator("[class*=card], [data-slot=card], li, tr")
    .filter({ hasText: eventTitle })
    .first()
  await expect(row).not.toContainText(/pending/i, { timeout: 20_000 })
})

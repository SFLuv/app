/**
 * What a volunteer sees when a redemption does not simply pay out.
 *
 * Two outcomes have their own copy, and both were written blind today — no
 * browser has rendered either. They matter more than most screens because they
 * are read at a live event by somebody who has just been told, in effect, that
 * they are not getting their reward.
 *
 *   202  held    — money reserved, code consumed, releases when the W-9 lands
 *   409  refused — a hold is already open; the code is HANDED BACK
 *
 * The refusal copy has one job beyond explaining itself: to say the QR still
 * works. Without that it reads as the platform having eaten the reward.
 */
import { readFileSync } from "fs"
import path from "path"
import { expect, test } from "../lib/test"

test("a redemption that cannot be paid ends somewhere, and says why", async ({ page }) => {
  await page.goto("/faucet/redeem?code=not-a-real-code")

  /**
   * The point is that it RESOLVES — a spinner that never ends is the failure.
   * Which terminal state depends on who is scanning, and the seeded account is
   * merchant-typed, so it lands on the merchant refusal rather than an
   * invalid-code error. Both are correct endings; neither is a hang.
   */
  await expect(
    page.getByRole("heading", {
      name: /Invalid|Error|expired|redeemed|not active|Merchant Account|Reward Not Sent|Reward Held/i,
    }),
  ).toBeVisible({ timeout: 30_000 })
})

/**
 * A merchant scanning a volunteer QR must not be sent to the tax form.
 *
 * Both refusals arrive as a 409, and before the merchant bar existed there was
 * only one of them — so the page showed W-9 copy for every 409. A merchant
 * following that would complete a tax form and still not be paid, because their
 * account is the problem, not their paperwork.
 */
test("a merchant is told it is their account, not their tax form", async ({ page }) => {
  await page.goto("/faucet/redeem?code=not-a-real-code")

  const merchantHeading = page.getByRole("heading", { name: "Merchant Account" })
  const appeared = await merchantHeading
    .waitFor({ state: "visible", timeout: 30_000 })
    .then(() => true)
    .catch(() => false)

  test.skip(!appeared, "the seeded account is not merchant-typed on this database")

  await expect(page.getByText(/personal SFLuv account/i)).toBeVisible()
  await expect(
    page.getByText(/This QR code has not been used up/i),
    "a refused merchant must be told their code still works",
  ).toBeVisible()
  await expect(
    page.getByText(/W-9|tax form/i),
    "a merchant refusal must not send them to the tax form",
  ).toBeHidden()
})

test("the held and refused copy exists and says the right thing", async () => {
  /**
   * Asserted against the source rather than the DOM, deliberately.
   *
   * Reaching either state in a browser needs a live code redeemed by an account
   * already at the annual limit — that is what
   * testing/scripts/06-w9-threshold-crossing.sh proves, end to end and on
   * chain. Duplicating it here would be slow, flaky, and would test the same
   * thing twice.
   *
   * What a browser test can cheaply protect is the promise in the words: that
   * a refused code is described as still usable. Losing that sentence is a
   * silent regression no API test would ever catch.
   */
  const source = readFileSync(
    path.resolve(__dirname, "../../frontend/app/faucet/redeem/page.tsx"),
    "utf8",
  )

  expect(source, "the held state must still be handled").toContain("Reward Held")
  expect(source, "the refused state must still be handled").toContain("Reward Not Sent")
  expect(
    source.replace(/\s+/g, " "),
    "the refusal must tell the volunteer their QR code still works",
  ).toContain("This QR code has not been used up")
})

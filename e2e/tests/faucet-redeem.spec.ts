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

test("an invalid code is refused without claiming anything happened", async ({ page }) => {
  await page.goto("/faucet/redeem?code=not-a-real-code")

  // Any terminal state is fine here; a spinner that never resolves is not.
  await expect(
    page.getByRole("heading", { name: /Invalid|Error|expired|redeemed|not active/i }),
  ).toBeVisible({ timeout: 30_000 })
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

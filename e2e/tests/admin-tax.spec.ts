/**
 * The admin Tax & Escrow panel.
 *
 * Worth a spec because this panel lost a working button today. Back pay was
 * retired with the tier redesign — escrow can no longer accumulate, so nothing
 * can lapse into being owed-but-unreserved — and the "Send back pay" control
 * plus the faucet-coverage arithmetic that supported it were removed. The route
 * behind that button is gone from the router, so if the control ever comes back
 * it will 404 in a user's face.
 */
import { clearPranks } from "../lib/harness"
import { expect, test } from "../lib/test"

test.afterEach(() => clearPranks())

test("the tax panel renders, and offers no back-pay control", async ({ page }) => {
  await page.goto("/admin")

  // The sidebar nav is buttons, not links (dashboard/sidebar.tsx:280).
  await expect(page.getByRole("button", { name: "Admin Panel" })).toBeVisible()

  await page.getByRole("tab", { name: /Tax & Escrow/ }).click()

  // The faucet arithmetic that survived the redesign.
  await expect(page.getByText("Faucet on-chain")).toBeVisible()
  await expect(page.getByText("Available to allocate")).toBeVisible()

  /**
   * The removals. "Outstanding back pay" was the coverage line, and the button
   * called POST /admin/w9/{user}/back-pay, which no longer exists.
   */
  await expect(page.getByText("Outstanding back pay")).toBeHidden()
  await expect(page.getByRole("button", { name: /back pay/i })).toBeHidden()
})

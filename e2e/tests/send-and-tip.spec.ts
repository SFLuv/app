/**
 * An actual payment to a merchant, on chain, followed by the tip prompt.
 *
 * This is the only spec in the suite that moves money. It is slower and more
 * fragile than the rest — a real account-abstraction round trip through the
 * bundler and paymaster — and that is the point: it is the one that proves the
 * money arrives in the right till, which is the whole product.
 *
 * Requirements it will tell you about rather than fail mysteriously on:
 *
 *   - the sending wallet needs SFLUV     → testing/scripts/fund-wallet.sh
 *   - the chain clock must match reality → testing/scripts/sync-chain-time.sh
 *     (drift makes every UserOperation fail with "AA32 expired or not due")
 */
import { chainClockDriftSeconds, psql, tokenBalance, waitForBalance } from "../lib/harness"
import { readSession } from "../lib/session"
import { expect, test } from "../lib/test"

/**
 * A real transfer through the bundler is not a two-second affair, and the
 * settlement window is wider than the UI's. The screen says "broadcast to the
 * network" as soon as the UserOperation is submitted; the chain can take
 * appreciably longer, especially if the fork's clock has drifted and the
 * paymaster's validity window forces a retry.
 */
const SEND_TIMEOUT_MS = 180_000
const SETTLE_TIMEOUT_MS = 240_000

test.describe.configure({ timeout: SEND_TIMEOUT_MS + SETTLE_TIMEOUT_MS * 2 + 60_000 })

type Merchant = { id: string; name: string; payTo: string; tipTo: string }

function merchantWithTillAndTip(): Merchant | null {
  const row = psql(
    `SELECT l.id || '|' || l.name || '|' || w.wallet_address || '|' || TRIM(l.tipping_wallet_address)
       FROM locations l
       JOIN location_payment_wallets w
         ON w.location_id = l.id AND w.active = TRUE AND w.is_default = TRUE
      WHERE l.approval = TRUE
        AND NULLIF(TRIM(l.tipping_wallet_address), '') IS NOT NULL
        AND l.name IS NOT NULL AND TRIM(l.name) <> ''
      ORDER BY l.id DESC LIMIT 1;`,
  )
  if (!row) return null
  const [id, name, payTo, tipTo] = row.split("|")
  return { id, name, payTo, tipTo }
}

test("paying a merchant moves SFLUV to their till, and offers a tip", async ({ page }) => {
  const merchant = merchantWithTillAndTip()
  test.skip(!merchant, "no approved merchant has both a payment and a tipping wallet")

  /**
   * Check the clock before doing anything expensive. Drift does not fail
   * cleanly — it makes every transfer retry, so the spec times out minutes
   * later pointing at the wrong thing entirely.
   */
  const drift = chainClockDriftSeconds()
  if (drift !== null && Math.abs(drift) > 300) {
    throw new Error(
      `the fork's clock is ${drift}s out, which makes every UserOperation retry ` +
        `(AA32 expired or not due). Run testing/scripts/sync-chain-time.sh first.`,
    )
  }

  const session = readSession()
  const from = psql(`SELECT primary_wallet_address FROM users WHERE id = '${session.userId}';`)
  test.skip(!from, "the seeded account has no primary wallet")

  await page.goto(`/redirect?mode=send&to=${merchant!.payTo}&tipTo=${merchant!.tipTo}&l=${merchant!.id}`)
  await page.waitForURL(/\/wallets\//, { timeout: 90_000 })

  const modal = page.getByRole("dialog")
  await expect(modal).toBeVisible({ timeout: 60_000 })

  /**
   * Fail loudly and early on an empty wallet. Without this the Send button
   * simply does nothing useful and the spec times out somewhere less obvious.
   */
  const available = modal.getByText(/Available:\s*[\d.]+\s*SFLUV/)
  await expect(available).toBeVisible({ timeout: 30_000 })
  const availableText = (await available.innerText()).trim()
  const availableAmount = Number(availableText.replace(/[^\d.]/g, ""))
  expect(
    availableAmount,
    `the sending wallet is empty (${availableText}) — run testing/scripts/fund-wallet.sh ${from} 500`,
  ).toBeGreaterThan(1)

  /**
   * Read the till before sending. A success screen is what the app believes;
   * the chain is what happened, and reconciling those two is the entire reason
   * this spec is worth its slowness.
   */
  const tillBefore = tokenBalance(merchant!.payTo)

  // The confirm step has its own amount field, distinct from the manual form's.
  await modal.locator("#confirm-amount").fill("1")
  await modal.getByRole("button", { name: "Send", exact: true }).click()

  /**
   * The tip prompt only renders after the send actually succeeds, so its
   * appearance IS the success assertion — and it is the one that matters, since
   * tipping is invisible to any test that stops at "no error was shown".
   */
  await expect(
    modal.getByText("Optional Tip"),
    "a successful payment to a merchant with a tipping wallet must offer a tip",
  ).toBeVisible({ timeout: SEND_TIMEOUT_MS })

  await expect(modal.getByText(`Tip ${merchant!.name}`)).toBeVisible()
  await expect(modal.locator("#post-send-tip-amount")).toBeVisible()
  await expect(modal.getByRole("button", { name: "Send Tip" })).toBeVisible()

  /**
   * And the money is actually there. The token has 6 decimals, so one whole
   * SFLUV is 1_000_000 base units.
   */
  if (tillBefore !== null) {
    const tillAfter = await waitForBalance(merchant!.payTo, tillBefore + 1_000_000n, SETTLE_TIMEOUT_MS)
    expect(
      tillAfter,
      "one whole SFLUV should have arrived in the merchant's till",
    ).toBe(tillBefore + 1_000_000n)
    console.log(`  till ${tillBefore} → ${tillAfter}`)
  } else {
    console.log("  (skipped the on-chain check: could not resolve the token address)")
  }

  console.log(`  paid 1 SFLUV to ${merchant!.name}, tip prompt offered`)

  /**
   * Now actually tip, and check where it landed.
   *
   * This is the assertion the whole tipping feature rests on: a tip is a
   * SEPARATE transfer to a DIFFERENT wallet. If it goes to the till instead,
   * nothing errors, the customer sees success, and the shop's tips become
   * indistinguishable from its takings — which is the one thing splitting the
   * wallets was for.
   */
  const tipBefore = tokenBalance(merchant!.tipTo)
  const tillBeforeTip = tokenBalance(merchant!.payTo)

  await modal.locator("#post-send-tip-amount").fill("1")
  await modal.getByRole("button", { name: "Send Tip" }).click()

  /**
   * Assert a POSITIVE outcome, not the absence of the prompt. toBeHidden also
   * passes if the whole dialog closed, which would let a tip that never sent
   * look like a success.
   */
  await expect(
    modal.getByText("Tip Sent!"),
    "the tip should report as sent",
  ).toBeVisible({ timeout: SEND_TIMEOUT_MS })

  if (tipBefore !== null && tillBeforeTip !== null) {
    const tipAfter = await waitForBalance(merchant!.tipTo, tipBefore + 1_000_000n, SETTLE_TIMEOUT_MS)
    expect(
      tipAfter,
      "the tip should arrive in the merchant's TIPPING wallet",
    ).toBe(tipBefore + 1_000_000n)

    /**
     * Read the till only AFTER the tip has landed. Checking it earlier would
     * pass for the wrong reason — the tip simply not having been mined yet.
     */
    expect(
      tokenBalance(merchant!.payTo)! - tillBeforeTip,
      "a tip must not land in the till — that is what separate wallets are for",
    ).toBe(0n)

    console.log(`  tip wallet ${tipBefore} → ${tipAfter}, till unchanged`)
  }
})

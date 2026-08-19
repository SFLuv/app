/**
 * Paying a merchant, from the map a customer actually uses.
 *
 * The journey is: map → merchant row → location modal → Pay → /redirect, which
 * resolves the wallet to send to and offers "Pay with SFLuv Wallet". This spec
 * covers up to that point and stops short of moving money — the transfer itself
 * needs a funded smart account and a real account-abstraction round trip, and
 * is worth its own spec rather than being smuggled into this one.
 *
 * What is being protected here is the wiring: that a merchant on the map leads
 * to a payment screen addressed to THAT merchant's till. Getting the address
 * wrong sends a customer's money to the wrong shop, and nothing downstream
 * would notice.
 */
import { psql } from "../lib/harness"
import { expect, test } from "../lib/test"

type Merchant = { id: string; name: string; payTo: string; tipTo: string }

/**
 * An approved location with a default payment wallet. The API exposes these as
 * pay_to_address / tip_to_address, but they are not columns: the payment wallet
 * lives in location_payment_wallets and only the tipping one is on the location
 * row.
 */
function merchantWithTill(requireTip: boolean): Merchant | null {
  const row = psql(
    `SELECT l.id || '|' || l.name || '|' || w.wallet_address || '|' ||
            COALESCE(NULLIF(TRIM(l.tipping_wallet_address), ''), '')
       FROM locations l
       JOIN location_payment_wallets w
         ON w.location_id = l.id AND w.active = TRUE AND w.is_default = TRUE
      WHERE l.approval = TRUE
        AND l.name IS NOT NULL AND TRIM(l.name) <> ''
        ${requireTip ? "AND NULLIF(TRIM(l.tipping_wallet_address), '') IS NOT NULL" : ""}
      ORDER BY l.id DESC LIMIT 1;`,
  )
  if (!row) return null
  const [id, name, payTo, tipTo] = row.split("|")
  return { id, name, payTo, tipTo }
}

/** Find a merchant in the map's own list and open it. */
async function openMerchant(page: import("@playwright/test").Page, name: string) {
  await page.goto("/map")
  await expect(page.getByRole("heading", { name: "Merchant Map" })).toBeVisible()

  // The panel can start collapsed; opening it is a no-op when it is already out.
  const expand = page.getByRole("button", { name: "Show merchant list" })
  if (await expand.isVisible().catch(() => false)) await expand.click()

  await page.getByPlaceholder("Search merchants").fill(name.slice(0, 24))

  // Rows are <button type="button"> whose click selects the location
  // (map-merchant-panel.tsx:145).
  await page.getByRole("button", { name: new RegExp(escapeRe(name)) }).first().click()
}

function escapeRe(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}

test("a merchant on the map leads to a payment screen addressed to that merchant", async ({
  page,
}) => {
  const merchant = merchantWithTill(false)
  test.skip(!merchant, "no approved location has a default payment wallet")

  await openMerchant(page, merchant!.name)

  const dialog = page.getByRole("dialog")
  await expect(dialog).toBeVisible()
  await expect(dialog.getByText(merchant!.name, { exact: false }).first()).toBeVisible()

  await dialog.getByRole("button", { name: "Pay", exact: true }).click()

  /**
   * The Pay button builds /redirect?mode=send&to=…&tipTo=…&l=…
   * (lib/redeem-link.ts:198). The `to` is the assertion that matters: it is the
   * till the customer's money is about to go to.
   */
  await page.waitForURL(/\/redirect\?/)
  const params = new URL(page.url()).searchParams
  expect(params.get("mode")).toBe("send")
  expect(
    (params.get("to") || "").toLowerCase(),
    "the payment must be addressed to this merchant's own till",
  ).toBe(merchant!.payTo.toLowerCase())
  expect(params.get("l")).toBe(merchant!.id)

  /**
   * An AUTHENTICATED customer never sees the "Send SFLUV / Pay with SFLuv
   * Wallet" chooser — that is the logged-out path. /redirect goes
   * checking → ensuring-wallet → redirecting and hands off to the wallet send
   * screen (app/redirect/page.tsx:263).
   */
  await page.waitForURL(/\/wallets\//, { timeout: 60_000 })

  /**
   * The wallet page opens the send modal and then strips the query with
   * router.replace (app/wallets/[address]/page.tsx:456), so the handoff cannot
   * be asserted from the URL — only from the modal it opened.
   */
  const sendModal = page.getByRole("dialog")
  await expect(sendModal).toBeVisible({ timeout: 30_000 })

  /**
   * The recipient must be prefilled with THIS merchant's till. It is the one
   * value a customer cannot sanity-check by eye, and getting it wrong pays the
   * wrong shop with no error anywhere.
   */
  /**
   * With a recipient already known, the modal skips the camera and opens
   * straight on the confirm step — "the camera step has nothing left to do"
   * (send-crypto-modal.tsx:676). So there is no #recipient input to read; the
   * address is rendered on the confirmation instead.
   */
  /**
   * The address is rendered shortened — shortenAddress(recipient, 8, 6) at
   * send-crypto-modal.tsx:1239 — so "0x83Ba8a64B3…" shows as "0x83Ba8a...83D9F1".
   * Asserting the full string finds nothing.
   */
  const shortened = `${merchant!.payTo.slice(0, 8)}...${merchant!.payTo.slice(-6)}`
  await expect(
    sendModal.getByText(shortened, { exact: false }).first(),
    "the confirmation should be addressed to this merchant's till",
  ).toBeVisible({ timeout: 20_000 })

  /**
   * And it should name the merchant, not only show hex. A customer cannot
   * verify an address by eye, so the name is what makes the screen checkable.
   */
  await expect(
    sendModal.getByText(merchant!.name, { exact: false }).first(),
    "the confirmation should name the merchant, not just show an address",
  ).toBeVisible({ timeout: 20_000 })

  // The confirm step, reached without ever showing a camera.
  await expect(sendModal.getByText("Send Cryptocurrency")).toBeVisible()

  console.log(`  ${merchant!.name} → confirm screen for ${merchant!.payTo}`)
})

test("a merchant with a tipping wallet carries it through to the payment screen", async ({
  page,
}) => {
  const merchant = merchantWithTill(true)
  test.skip(!merchant, "no approved location has both a payment and a tipping wallet")

  await openMerchant(page, merchant!.name)
  await page.getByRole("dialog").getByRole("button", { name: "Pay", exact: true }).click()
  await page.waitForURL(/\/redirect\?/)

  const params = new URL(page.url()).searchParams

  /**
   * tipTo is what makes the tip prompt appear after a successful send. Lose it
   * here and tipping silently stops existing for this merchant — no error, no
   * failed request, just a prompt that never shows.
   *
   * It must also differ from the payment wallet: a shared address would make
   * tips indistinguishable from takings.
   */
  expect(
    (params.get("tipTo") || "").toLowerCase(),
    "the tipping wallet must survive the trip to the payment screen",
  ).toBe(merchant!.tipTo.toLowerCase())

  expect(
    params.get("tipTo")!.toLowerCase(),
    "tips must not land in the same wallet as takings",
  ).not.toBe(params.get("to")!.toLowerCase())

  console.log(`  ${merchant!.name} pay=${merchant!.payTo} tip=${merchant!.tipTo}`)
})

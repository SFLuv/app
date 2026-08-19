/**
 * The suite's floor. If this is red, nothing else is worth reading.
 *
 * Two things get proved here. That the seeded session actually authenticates,
 * and that prank forwarding changes who the backend thinks is calling — the one
 * link in the harness that the shell-level round trip could not reach, because
 * prankForwardingMiddleware returns early unless a valid Privy token has already
 * put a userDid in the context (backend/router/router.go:43).
 */
import { clearPranks, findUserByAdminFlag, getUser, setPrank } from "../lib/harness"
import { readSession } from "../lib/session"
import { expect, test } from "../lib/test"

/**
 * Sidebar nav entries render as <Button> with an onClick, not as anchors
 * (dashboard/sidebar.tsx:280), so they carry the button role. Asking for a link
 * role here matches nothing.
 */
/**
 * Present for every authenticated user, absent otherwise (dashboard/sidebar.tsx:110).
 *
 * tests/auth.setup.ts deliberately does NOT wait on this, and the difference is
 * worth keeping straight. Here it is an assertion: a regular authenticated user
 * gets a sidebar, and if that stops being true this spec should go red and say
 * so. There it was a dependency: it gated the credential the entire suite runs
 * on, so an onboarding gate that hides the sidebar from one account type — a
 * merchant mid-onboarding, say — would take the whole suite down with it rather
 * than fail one assertion. Seeding waits on the Privy token instead.
 */
const AUTHED_NAV = "Contacts"

/** Present only when user.isAdmin (dashboard/sidebar.tsx:192). */
const ADMIN_NAV = "Admin Panel"

test.afterEach(() => {
  // Always, including after a failure. A leftover prank would silently change
  // what every later spec — and the developer's own browser — sees.
  clearPranks()
})

test("the seeded session is authenticated", async ({ page }) => {
  const session = readSession()

  await page.goto("/")

  await expect(page.getByRole("button", { name: AUTHED_NAV })).toBeVisible()
  /**
   * exact: true matters. getByRole matches the accessible name as a SUBSTRING
   * by default, and the sidebar renders a "Connected Wallets" entry
   * (dashboard/sidebar.tsx:53) which contains "Connect" — so the loose form
   * matches an authenticated user's own nav and the assertion fails on a
   * working app.
   */
  await expect(page.getByRole("button", { name: "Connect", exact: true })).toBeHidden()

  console.log(`  authenticated as ${session.email} (${session.userId})`)
})

test("a prank forwards identity to another user", async ({ page }) => {
  const session = readSession()
  const pranker = getUser(session.userId)

  /**
   * Pick a prankee whose admin flag is the opposite of the seeded account's, so
   * the assertion has something to observe either way. Which account a hand
   * login lands on is not knowable in advance — three dev accounts share the
   * email sanchez@oleary.com — so the pair is chosen at runtime rather than
   * hardcoded.
   */
  const prankee = findUserByAdminFlag(!pranker.isAdmin, pranker.id)

  await page.goto("/")
  const adminNav = page.getByRole("button", { name: ADMIN_NAV })
  await expect(page.getByRole("button", { name: AUTHED_NAV })).toBeVisible()

  if (pranker.isAdmin) {
    await expect(adminNav).toBeVisible()
  } else {
    await expect(adminNav).toBeHidden()
  }

  setPrank(pranker.id, prankee.id)
  await page.reload()
  await expect(page.getByRole("button", { name: AUTHED_NAV })).toBeVisible()

  // The backend now answers as the prankee, so the role-gated nav flips.
  if (prankee.isAdmin) {
    await expect(adminNav).toBeVisible()
  } else {
    await expect(adminNav).toBeHidden()
  }

  clearPranks()
  await page.reload()
  await expect(page.getByRole("button", { name: AUTHED_NAV })).toBeVisible()

  if (pranker.isAdmin) {
    await expect(adminNav).toBeVisible()
  } else {
    await expect(adminNav).toBeHidden()
  }

  console.log(
    `  ${pranker.email} (admin=${pranker.isAdmin}) → ` +
      `${prankee.email} (admin=${prankee.isAdmin}) and back`,
  )
})

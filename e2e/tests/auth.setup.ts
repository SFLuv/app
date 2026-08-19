/**
 * Seed the Privy session. Run this by hand, once:
 *
 *     npm run auth
 *
 * A browser opens. Log in through Privy as a dev account. The spec waits, saves
 * the session, and exits.
 *
 * Why a human does this: Privy runs with captchaEnabled: true
 * (frontend/context/Providers.tsx:42), and captchas are not automatable. That
 * turns out to be the right gate anyway — one deliberate human login is a
 * cleaner boundary than any automated credential would be.
 *
 * NOTE: this imports @playwright/test directly, not lib/test. The network
 * sandbox there would block the captcha and OAuth hosts a real login needs. It
 * is the only spec in the suite allowed to do this.
 */
import { expect, test } from "@playwright/test"
import { mkdirSync } from "fs"
import path from "path"
import { AUTH_DIR, AUTH_STATE, SESSION_FILE } from "../lib/env"
import { getUser } from "../lib/harness"
import { findPrivyUserIdInPage, findUserIdInStorageState, writeSession } from "../lib/session"

const LOGIN_WINDOW_MS = 10 * 60 * 1000

test("seed a Privy session", async ({ page }) => {
  test.setTimeout(LOGIN_WINDOW_MS + 60_000)
  mkdirSync(AUTH_DIR, { recursive: true })

  await page.goto("/")

  console.log(
    "\n" +
      "  ────────────────────────────────────────────────────────────\n" +
      "  Log in through Privy in the browser window that just opened.\n" +
      "  This spec waits up to 10 minutes, then saves the session.\n" +
      "  ────────────────────────────────────────────────────────────\n",
  )

  /**
   * Wait for the token itself, and poll for it INSIDE the page.
   *
   * Two separate mistakes are being avoided here, and both of them cost a
   * working suite.
   *
   * Not a UI landmark. This used to wait on the sidebar's "Contacts" button.
   * That is a fine assertion in a spec, but it is the wrong thing to seed on:
   * the account someone logs in with owns approved locations, so it is
   * merchant-typed, and a merchant part-way through onboarding can be shown a
   * form with no sidebar at all. The landmark would then never appear, seeding
   * would fail, and every spec in the suite fails with it — over a screen
   * change that broke nothing. Seeding must depend only on what makes the
   * session a session.
   *
   * Not context.storageState(). An earlier version called it once a second
   * until a token appeared. Playwright reads localStorage by opening a
   * transient page per origin, so that loop spawned and closed a tab every
   * second — visible as the Privy window flickering open and shut, and very
   * likely stealing focus from the login it was waiting for. Snapshot storage
   * once, when the thing being waited for has already happened.
   *
   * waitForFunction polls in the page's own context, which costs nothing and
   * disturbs nothing. findPrivyUserIdInPage is the same rule
   * findUserIdInStorageState applies to the saved file, asked of the live page —
   * a JWT issued by privy.io, whose subject is the claim the backend
   * authenticates on (token.Claims.GetSubject() in
   * backend/utils/middleware/auth.go).
   */
  const seenInPage = await page
    .waitForFunction(
      findPrivyUserIdInPage,
      undefined,
      // A second between attempts, rather than the default animation frame:
      // the login window belongs to a human, and the page should stay smooth.
      { timeout: LOGIN_WINDOW_MS, polling: 1000 },
    )
    .then((handle) => handle.jsonValue())

  /**
   * Snapshot once, now that the token is known to be there.
   *
   * The identity that matters is the one in the SAVED state, because that file
   * is what every other spec replays as storageState — so read it back rather
   * than trusting what the page reported. If a token reached the page but not
   * the file, the seed is worthless, and saying so here is far cheaper than a
   * whole suite failing as "not logged in".
   */
  await page.context().storageState({ path: AUTH_STATE })
  const userId = findUserIdInStorageState(AUTH_STATE)

  expect(
    userId,
    `logged in as ${seenInPage}, but no privy.io JWT reached ` +
      `${path.relative(process.cwd(), AUTH_STATE)} — the saved session would authenticate nobody`,
  ).toBeTruthy()

  const user = getUser(userId!)
  writeSession({ userId: user.id, email: user.email, capturedAt: new Date().toISOString() })

  console.log(
    `\n  session saved for ${user.email} (${user.id}), admin=${user.isAdmin}\n` +
      `    ${path.relative(process.cwd(), AUTH_STATE)}\n` +
      `    ${path.relative(process.cwd(), SESSION_FILE)}\n`,
  )
})

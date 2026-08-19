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
import { findUserIdInStorageState, writeSession } from "../lib/session"

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
   * Wait on a POSITIVE UI landmark, and do not poll storageState.
   *
   * An earlier version called context.storageState() once a second until a
   * token appeared. Playwright reads localStorage by opening a transient page
   * per origin, so that loop spawned and closed a tab every second — visible as
   * the Privy window flickering open and shut, and very likely stealing focus
   * from the login it was waiting for. Never poll storageState; snapshot it
   * once, when the thing you were waiting for has happened.
   *
   * "Contacts is visible" is the right signal because it is present only for an
   * authenticated user (dashboard/sidebar.tsx:110). The tempting inverse —
   * "Connect went away" — is wrong: while AppProvider resolves, the sidebar
   * renders a spinner and no header at all (frontend/app/sidebar.tsx:44), so
   * Connect is absent before login as well as after.
   */
  await expect(
    page.getByRole("button", { name: "Contacts" }),
    "no login completed within the login window",
  ).toBeVisible({ timeout: LOGIN_WINDOW_MS })

  /**
   * Now snapshot. The token can land a moment after the UI does, so allow a few
   * attempts — but seconds apart and only a handful, not once a second forever.
   */
  let userId: string | null = null
  for (let attempt = 0; attempt < 10; attempt++) {
    await page.context().storageState({ path: AUTH_STATE })
    userId = findUserIdInStorageState(AUTH_STATE)
    if (userId) break
    await page.waitForTimeout(2000)
  }

  expect(
    userId,
    "logged in, but no privy.io JWT appeared in cookies or local storage",
  ).toBeTruthy()

  const user = getUser(userId!)
  writeSession({ userId: user.id, email: user.email, capturedAt: new Date().toISOString() })

  console.log(
    `\n  session saved for ${user.email} (${user.id}), admin=${user.isAdmin}\n` +
      `    ${path.relative(process.cwd(), AUTH_STATE)}\n` +
      `    ${path.relative(process.cwd(), SESSION_FILE)}\n`,
  )
})

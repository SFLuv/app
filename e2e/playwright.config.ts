import { defineConfig, devices } from "@playwright/test"
import { AUTH_STATE, BASE_URL } from "./lib/env"

export default defineConfig({
  testDir: "./tests",
  globalSetup: "./global-setup.ts",

  /**
   * Serial, one worker, on purpose.
   *
   * Identity switching goes through the pranks table, which has a primary key
   * on the pranker. One seeded account can therefore act as one user at a time,
   * and two parallel specs would silently fight over the same row.
   *
   * If runs ever get slow enough that this hurts, the fix is more seeded
   * sessions (one per worker), not more workers.
   */
  fullyParallel: false,
  workers: 1,

  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [["list"], ["html", { open: "never" }]],

  use: {
    baseURL: BASE_URL,

    // The frontend serves HTTPS locally from the self-signed pair in
    // frontend/certificates, so every navigation fails without this.
    ignoreHTTPSErrors: true,

    trace: "retain-on-failure",
    video: "retain-on-failure",
    screenshot: "only-on-failure",
  },

  projects: [
    {
      /**
       * The hand login. Headed, because a human completes it — Privy has
       * captchaEnabled: true (frontend/context/Providers.tsx:42) and captchas
       * are not automatable.
       *
       * Deliberately NOT wired as a dependency of the chromium project: it is a
       * once-a-session human step, not something to re-run before every suite.
       */
      name: "setup",
      testMatch: /auth\.setup\.ts/,
      use: { ...devices["Desktop Chrome"], headless: false },
    },
    {
      name: "chromium",
      testIgnore: /auth\.setup\.ts/,
      use: { ...devices["Desktop Chrome"], storageState: AUTH_STATE },
    },
  ],
})

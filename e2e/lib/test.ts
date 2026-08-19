import { test as base, expect } from "@playwright/test"

/**
 * The suite's `test`, with the network sandbox attached.
 *
 * Every spec must import from here rather than from @playwright/test directly.
 * The one deliberate exception is tests/auth.setup.ts — see the note there.
 */

/**
 * Hosts a test run may reach. Default-deny: anything not listed is aborted.
 *
 * The point is that a misconfigured env cannot reach production. api.sfluv.org
 * and app.sfluv.org are not on this list and must never be added — if a spec
 * appears to need them, the environment is wrong, not the list.
 *
 * The non-local entries are here because the app genuinely cannot work without
 * them:
 *   - privy.io   the seeded session refreshes its token against Privy's API
 *   - Google     map tiles and Places on the merchant map
 */
const ALLOWED_HOSTS = [
  "localhost",
  "127.0.0.1",
  "privy.io",
  "googleapis.com",
  "gstatic.com",
]

function isAllowed(hostname: string): boolean {
  return ALLOWED_HOSTS.some((allowed) => hostname === allowed || hostname.endsWith(`.${allowed}`))
}

/**
 * Blocked hosts are reported once each at the end of a run rather than silently
 * dropped. A spec that fails because of the sandbox should say so plainly —
 * otherwise it reads as an application bug.
 */
const blocked = new Set<string>()

export const test = base.extend({
  context: async ({ context }, use) => {
    await context.route("**/*", (route) => {
      let hostname: string
      try {
        hostname = new URL(route.request().url()).hostname
      } catch {
        return route.continue()
      }
      if (isAllowed(hostname)) return route.continue()
      blocked.add(hostname)
      return route.abort("blockedbyclient")
    })
    await use(context)
  },
})

test.afterAll(async () => {
  if (blocked.size > 0) {
    console.warn(
      `\n  network sandbox blocked ${blocked.size} host(s): ${[...blocked].join(", ")}\n` +
        `  If a failure looks unexplained, this is the first thing to check.\n`,
    )
    blocked.clear()
  }
})

export { expect }

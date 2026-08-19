import { spawnSync } from "child_process"
import path from "path"
import { ANVIL_RPC, APP_DB, BACKEND_URL, DB_HOST, DB_PORT, DB_USER, REPO_ROOT } from "./env"

/**
 * Control over the local dev stack, driven entirely through what already
 * exists: dev-up.sh's menu and psql. Nothing here adds tooling to the repo.
 *
 * The menu reads stdin and has no tty check, so it drives from a pipe. Two
 * rules keep that deterministic:
 *
 *   1. Search users by full user id, never by email. pick_user matches
 *      `contact_email ILIKE '%'||q||'%' OR id = q`, so a full did never matches
 *      an email and exactly one row comes back — the selection is always "1".
 *      Emails genuinely collide here, so searching by email is also ambiguous.
 *   2. End every sequence with "q". The menu redraws after each action, and the
 *      option list changes once a prank exists.
 *
 * `./dev-up.sh menu` boots nothing and installs no shutdown trap (dev-up.sh:649),
 * so calling it never disturbs a running stack.
 */

const DEV_UP = path.join(REPO_ROOT, "dev-up.sh")
const USER_ID_PATTERN = /^did:privy:[a-z0-9]+$/i

function assertUserId(value: string, label: string): void {
  if (!USER_ID_PATTERN.test(value)) {
    throw new Error(`${label} is not a privy user id: ${JSON.stringify(value)}`)
  }
}

/** Run a read-only query against the local app database. */
export function psql(sql: string): string {
  const result = spawnSync(
    "psql",
    ["-h", DB_HOST, "-p", DB_PORT, "-U", DB_USER, "-d", APP_DB, "-tAc", sql],
    { encoding: "utf8" },
  )
  if (result.status !== 0) {
    throw new Error(`psql failed: ${(result.stderr || "").trim()}`)
  }
  return result.stdout.trim()
}

/**
 * Feed a menu sequence to dev-up.sh over stdin.
 *
 * Input goes through spawnSync's `input` rather than a `printf | ...` shell
 * pipe, so user ids never pass through a shell and there is nothing to quote.
 */
function runMenu(lines: string[], expectPattern: RegExp, action: string): string {
  const result = spawnSync(DEV_UP, ["menu"], {
    input: lines.concat("").join("\n"),
    encoding: "utf8",
    cwd: REPO_ROOT,
  })

  if (result.status !== 0) {
    throw new Error(`dev-up.sh menu exited ${result.status} during ${action}:\n${result.stderr}`)
  }
  if (!expectPattern.test(result.stdout)) {
    throw new Error(`${action} did not confirm. Menu output:\n${result.stdout}`)
  }
  return result.stdout
}

/**
 * On-chain SFLUV balance, in base units.
 *
 * Exists so a spec that claims money moved can prove it rather than infer it
 * from the UI. A success screen is what the app believes; the chain is what
 * actually happened, and those are the two things a payment test exists to
 * reconcile.
 *
 * Returns null when the chain or token cannot be reached, so a spec can skip
 * rather than fail on an environment problem.
 */
let cachedToken: string | null | undefined

/**
 * The SFLUV contract address, read from the backend's own config rather than
 * hardcoded — it differs per environment and the dev env file does not carry
 * it. Resolved once per run.
 */
function tokenAddress(): string | null {
  if (cachedToken !== undefined) return cachedToken
  const result = spawnSync("curl", ["-s", "--max-time", "8", `${BACKEND_URL}/config`], {
    encoding: "utf8",
  })
  cachedToken = null
  if (result.status === 0 && result.stdout) {
    const match = result.stdout.match(/"primary_token"\s*:\s*\{[^}]*"address"\s*:\s*"(0x[a-fA-F0-9]{40})"/)
    if (match) cachedToken = match[1]
  }
  return cachedToken
}

export function tokenBalance(address: string): bigint | null {
  const token = tokenAddress()
  if (!token) return null
  if (!/^0x[a-fA-F0-9]{40}$/.test(address)) return null

  const result = spawnSync(
    "cast",
    ["call", token, "balanceOf(address)(uint256)", address, "--rpc-url", ANVIL_RPC],
    { encoding: "utf8" },
  )
  if (result.status !== 0) return null
  const raw = (result.stdout || "").trim().split(/\s+/)[0]
  try {
    return BigInt(raw)
  } catch {
    return null
  }
}

/**
 * Wait for an on-chain balance to reach an expected value.
 *
 * ASYNC on purpose, and it must stay that way. The UI reports a transfer as
 * "broadcast to the network", not confirmed, so reading the chain the instant
 * the success screen appears finds the old balance and looks exactly like the
 * money went somewhere wrong.
 *
 * The first version of this polled with a synchronous sleep, which was much
 * worse than merely inelegant: lib/test.ts installs a page.route() handler that
 * runs in THIS node process, so blocking the event loop stalls the browser's
 * network entirely. The transaction being waited for could not be submitted
 * while the wait was in progress — a deadlock that presented as "tips do not
 * arrive".
 */
export async function waitForBalance(
  address: string,
  expected: bigint,
  timeoutMs = 90_000,
): Promise<bigint | null> {
  const deadline = Date.now() + timeoutMs
  let last: bigint | null = null
  while (Date.now() < deadline) {
    last = tokenBalance(address)
    if (last !== null && last === expected) return last
    await new Promise((resolve) => setTimeout(resolve, 1000))
  }
  return last
}

/**
 * How far the fork's clock is behind wall time, in seconds.
 *
 * Drift is the single most expensive thing to misdiagnose here. The paymaster
 * signs each UserOperation with a real-time validity window that the chain
 * judges against block.timestamp, so once they part company every operation
 * fails with "AA32 expired or not due" — and anvil only advances time when
 * blocks are mined, so an idle fork drifts on its own.
 *
 * Mild drift does not break transfers outright; it makes them retry, turning a
 * seven-second spec into a four-minute timeout. Severe drift hangs the whole
 * app on its loading spinner. Fix with testing/scripts/sync-chain-time.sh.
 */
export function chainClockDriftSeconds(): number | null {
  const result = spawnSync("cast", ["block", "latest", "--rpc-url", ANVIL_RPC], {
    encoding: "utf8",
  })
  if (result.status !== 0) return null
  const match = (result.stdout || "").match(/^timestamp\s+(\d+)/m)
  if (!match) return null
  return Math.floor(Date.now() / 1000) - Number(match[1])
}

export function pranksActive(): boolean {
  return psql("SELECT to_regclass('public.pranks') IS NOT NULL;") === "t"
}

/**
 * Forward every request from `prankerUserId` to `prankeeUserId`.
 *
 * The running backend picks this up on the next request — no restart. It takes
 * effect only because the dev backend runs with IN_PRODUCTION=false; the
 * middleware is not even mounted otherwise (backend/router/router.go:142).
 */
export function setPrank(prankerUserId: string, prankeeUserId: string): void {
  assertUserId(prankerUserId, "pranker")
  assertUserId(prankeeUserId, "prankee")
  if (prankerUserId === prankeeUserId) {
    throw new Error("pranker and prankee are the same user — the menu refuses this")
  }
  runMenu(
    ["2", prankerUserId, "1", prankeeUserId, "1", "q"],
    /prank set/,
    "set prank",
  )
}

/** Drop all pranks. Safe to call when none exist — option 3 is hidden then. */
export function clearPranks(): void {
  if (!pranksActive()) return
  runMenu(["3", "y", "q"], /pranks cleared/, "clear pranks")
}

export type DevUser = {
  id: string
  email: string
  isAdmin: boolean
}

export function getUser(userId: string): DevUser {
  assertUserId(userId, "user id")
  const row = psql(
    `SELECT id || '|' || COALESCE(NULLIF(TRIM(contact_email), ''), '(no email)') || '|' || is_admin
       FROM users WHERE id = '${userId}';`,
  )
  if (!row) throw new Error(`no user with id ${userId} in the local database`)
  const [id, email, isAdmin] = row.split("|")
  /**
   * "true", not "t". A bare boolean column under psql -tA prints t/f, but this
   * one is concatenated into text with ||, and that cast yields true/false.
   * Checking only for "t" made every account look non-admin, which sent the
   * smoke spec down the wrong branch and failed it against a perfectly correct
   * app. Accept both rather than depend on which form the query happens to
   * produce.
   */
  return { id, email, isAdmin: isAdmin === "t" || isAdmin === "true" }
}

/**
 * Find an active user whose admin flag differs from `isAdmin`.
 *
 * The smoke spec needs a prankee whose role is visibly different from the
 * seeded account's, and it cannot hardcode one: which account a hand login
 * lands on depends on the Privy account used.
 */
export function findUserByAdminFlag(isAdmin: boolean, excludeUserId: string): DevUser {
  assertUserId(excludeUserId, "excluded user id")
  /**
   * accepted_privacy_policy is not optional here.
   *
   * A user who has not accepted it never reaches the dashboard at all:
   * AppProvider diverts to the policy gate and returns before setting
   * authenticated (context/AppProvider.tsx:744), so the sidebar never renders
   * and every nav assertion fails against an app that is behaving correctly.
   *
   * "A row exists" and "a user the app can render" are different questions, and
   * a fixture picker has to ask the second one.
   */
  const row = psql(
    `SELECT id FROM users
      WHERE is_admin = ${isAdmin ? "TRUE" : "FALSE"}
        AND active = TRUE
        AND accepted_privacy_policy = TRUE
        AND id <> '${excludeUserId}'
      ORDER BY id LIMIT 1;`,
  )
  if (!row) throw new Error(`no active user with is_admin = ${isAdmin} to prank into`)
  return getUser(row)
}

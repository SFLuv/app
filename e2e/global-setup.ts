import { ANVIL_RPC, BACKEND_ENV_FILE, BACKEND_URL, BASE_URL, DB_HOST, backendEnv } from "./lib/env"

/**
 * Preflight. Fails closed.
 *
 * The suite moves money, approves merchants and drains a faucet. Every one of
 * those is harmless against a local anvil fork and catastrophic against
 * production, and the difference between the two is a handful of env values. So
 * the run refuses to start unless it can prove it is local, rather than
 * assuming it because that is the usual case.
 *
 * This is the same reasoning as the network sandbox in lib/test.ts, one layer
 * earlier: that one stops a request reaching production, this one stops the run
 * beginning at all.
 */

const LOCAL_HOSTS = new Set(["localhost", "127.0.0.1", "::1", "0.0.0.0"])

function fail(reason: string): never {
  throw new Error(
    `\n\npreflight failed: ${reason}\n\n` +
      `The e2e suite refuses to run unless it is pointed at a local dev stack.\n` +
      `Start one with ./dev-up.sh, then try again.\n`,
  )
}

function hostOf(url: string, label: string): string {
  try {
    return new URL(url).hostname
  } catch {
    fail(`${label} is not a URL: ${url}`)
  }
}

async function reachable(url: string, init?: RequestInit): Promise<boolean> {
  try {
    const response = await fetch(url, { ...init, signal: AbortSignal.timeout(5000) })
    return response.status > 0
  } catch {
    return false
  }
}

export default async function globalSetup(): Promise<void> {
  if (Object.keys(backendEnv).length === 0) {
    fail(`no dev backend env at ${BACKEND_ENV_FILE} — has ./dev-up.sh ever run?`)
  }

  // 1. Never point the browser at anything but a local app.
  const appHost = hostOf(BASE_URL, "base URL")
  if (!LOCAL_HOSTS.has(appHost)) fail(`base URL is not local: ${BASE_URL}`)

  // 2. The prank middleware is only mounted when this is false. A run against a
  //    production-mode backend could not switch identity anyway, and should not
  //    be attempted.
  if ((backendEnv.IN_PRODUCTION || "").trim().toLowerCase() === "true") {
    fail("the dev backend has IN_PRODUCTION=true")
  }

  // 3. Chain writes must land on the fork, never on real Celo.
  const rpcHost = hostOf(ANVIL_RPC, "RPC URL")
  if (!LOCAL_HOSTS.has(rpcHost)) fail(`RPC_URL is not local: ${ANVIL_RPC}`)

  // 4. Same for the database.
  if (!LOCAL_HOSTS.has(DB_HOST)) fail(`database host is not local: ${DB_HOST}`)

  // 5. The stack has to actually be up, or every spec fails with a confusing
  //    navigation error instead of one clear message here.
  const backendHost = hostOf(BACKEND_URL, "backend URL")
  if (!LOCAL_HOSTS.has(backendHost)) fail(`backend URL is not local: ${BACKEND_URL}`)
  if (!(await reachable(`${BACKEND_URL}/locations`))) {
    fail(`backend is not answering on ${BACKEND_URL}`)
  }

  const chainUp = await reachable(ANVIL_RPC, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "eth_chainId", params: [] }),
  })
  if (!chainUp) fail(`anvil is not answering on ${ANVIL_RPC}`)

  console.log(`  preflight ok — app ${BASE_URL}, backend ${BACKEND_URL}, chain ${ANVIL_RPC}`)
}

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs"
import path from "path"
import { AUTH_DIR, SESSION_FILE } from "./env"

/**
 * Work out which account a seeded Privy session belongs to.
 *
 * The user id is not something a spec can hardcode: which account a hand login
 * lands on depends on the Privy account used, and three dev accounts share the
 * email sanchez@oleary.com.
 *
 * Rather than depend on a Privy storage key name — which is internal and will
 * change — this scans every stored value for anything that parses as a JWT
 * issued by privy.io, and takes its subject. That is the same claim the backend
 * authenticates on (`token.Claims.GetSubject()` in
 * backend/utils/middleware/auth.go), so it is the right identity by
 * construction.
 */

type StorageState = {
  cookies?: Array<{ value?: string }>
  origins?: Array<{ localStorage?: Array<{ name?: string; value?: string }> }>
}

export type SeededSession = {
  userId: string
  email: string
  capturedAt: string
}

function decodeJwtPayload(candidate: string): Record<string, unknown> | null {
  const parts = candidate.split(".")
  if (parts.length !== 3) return null
  try {
    const base64 = parts[1].replace(/-/g, "+").replace(/_/g, "/")
    const json = Buffer.from(base64, "base64").toString("utf8")
    const parsed = JSON.parse(json)
    return typeof parsed === "object" && parsed !== null ? parsed : null
  } catch {
    return null
  }
}

/**
 * Pull every string worth testing out of a stored value. Privy wraps some
 * values in JSON, so the raw string alone is not enough.
 */
function candidateStrings(value: string): string[] {
  const found = [value]
  try {
    const parsed = JSON.parse(value)
    if (typeof parsed === "string") found.push(parsed)
    else if (parsed && typeof parsed === "object") {
      for (const nested of Object.values(parsed)) {
        if (typeof nested === "string") found.push(nested)
      }
    }
  } catch {
    // Not JSON. The raw string is the only candidate, which is the common case.
  }
  return found
}

export function findUserIdInStorageState(statePath: string): string | null {
  if (!existsSync(statePath)) return null
  const state: StorageState = JSON.parse(readFileSync(statePath, "utf8"))

  const stored: string[] = []
  for (const cookie of state.cookies || []) {
    if (cookie.value) stored.push(cookie.value)
  }
  for (const origin of state.origins || []) {
    for (const entry of origin.localStorage || []) {
      if (entry.value) stored.push(entry.value)
    }
  }

  for (const value of stored) {
    for (const candidate of candidateStrings(value)) {
      const payload = decodeJwtPayload(candidate)
      if (!payload) continue
      if (payload.iss !== "privy.io") continue
      const subject = payload.sub
      if (typeof subject === "string" && subject.startsWith("did:privy:")) {
        return subject
      }
    }
  }
  return null
}

export function writeSession(session: SeededSession): void {
  if (!existsSync(AUTH_DIR)) mkdirSync(AUTH_DIR, { recursive: true })
  writeFileSync(SESSION_FILE, JSON.stringify(session, null, 2) + "\n")
}

export function readSession(): SeededSession {
  if (!existsSync(SESSION_FILE)) {
    throw new Error(
      `no seeded session at ${path.relative(process.cwd(), SESSION_FILE)}.\n` +
        `Run: npm run auth  (opens a browser; log in through Privy once)`,
    )
  }
  return JSON.parse(readFileSync(SESSION_FILE, "utf8"))
}

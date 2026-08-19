import { existsSync, readFileSync } from "fs"
import path from "path"

export const E2E_ROOT = path.resolve(__dirname, "..")
export const REPO_ROOT = path.resolve(E2E_ROOT, "..")

/**
 * The env file dev-up.sh writes for the backend it boots.
 *
 * This is deliberately NOT backend/.env. dev-up.sh generates its own file and
 * the running dev backend reads only that one, so backend/.env can hold
 * anything without affecting a test run.
 */
export const BACKEND_ENV_FILE = path.join(REPO_ROOT, "tmp", "backend.dev.env")

/**
 * Parse a KEY=value env file. Values are never logged anywhere in this suite —
 * the file holds live keys, and a test run should not be able to leak them into
 * a report or a terminal scrollback.
 */
function parseEnvFile(file: string): Record<string, string> {
  const out: Record<string, string> = {}
  if (!existsSync(file)) return out

  for (const line of readFileSync(file, "utf8").split("\n")) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith("#")) continue
    const eq = trimmed.indexOf("=")
    if (eq === -1) continue
    out[trimmed.slice(0, eq).trim()] = trimmed.slice(eq + 1).trim()
  }
  return out
}

export const backendEnv = parseEnvFile(BACKEND_ENV_FILE)

/** The web app. Serves HTTPS locally from the self-signed pair in frontend/certificates. */
export const BASE_URL =
  process.env.PLAYWRIGHT_BASE_URL || backendEnv.APP_BASE_URL || "https://localhost:3000"

export const BACKEND_URL =
  backendEnv.PUBLIC_BACKEND_URL || `http://localhost:${backendEnv.PORT || "8080"}`

export const ANVIL_RPC = backendEnv.RPC_URL || "http://127.0.0.1:8545"

const [dbHost, dbPort] = (backendEnv.DB_BASE_URL || "localhost:5432").split(":")
export const DB_HOST = dbHost || "localhost"
export const DB_PORT = dbPort || "5432"
export const DB_USER = backendEnv.DB_USER || process.env.USER || ""
export const APP_DB = backendEnv.APP_DB_NAME || "app"

/** Where the hand-seeded Privy session lives. Git-ignored; it is a live credential. */
export const AUTH_DIR = path.join(E2E_ROOT, ".auth")
export const AUTH_STATE = path.join(AUTH_DIR, "user.json")

/**
 * A sidecar recording which account the seeded session belongs to. Emails
 * collide across dev accounts (three share sanchez@oleary.com), so specs must
 * key off the user id, and they cannot know it until someone has logged in.
 */
export const SESSION_FILE = path.join(AUTH_DIR, "session.json")

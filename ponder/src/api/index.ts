import { db } from "ponder:api";
import schema from "ponder:schema";
import { Hono } from "hono";
import { client, graphql } from "ponder";
import { timingSafeEqual } from "node:crypto";
import pg from "pg";
import { addHook, deleteHook, PonderHook } from "../db";

// Everything in this file is the backend's alone, and all of it sits behind the
// admin key. Only files under src/api change here: Ponder hashes every other
// file under src into its build id, so touching the indexing code or db.ts
// would reindex from the start block.
//
// The guard used to be app.use("/"), which in Hono matches the root path and
// nothing else. POST /hooks therefore answered anybody, and it pings the URL it
// is handed with X-Admin-Key attached before storing it: anyone could have the
// key sent to a server of their choosing. Every stored hook is also sent the key
// on every transfer touching its address, so a hook planted that way keeps
// receiving it — including after the key is rotated, until it is deleted.

// The admin key, compared in constant time. An unset key refuses everything:
// an empty header must never match an empty key.
const hasAdminKey = (provided: string | undefined): boolean => {
  const expected = process.env.ADMIN_KEY
  if (!expected || !provided) return false
  const a = Buffer.from(expected)
  const b = Buffer.from(provided)
  return a.length === b.length && timingSafeEqual(a, b)
}

// Origins a hook may point at, from HOOK_ALLOWED_ORIGINS (comma-separated, for
// example https://api.sfluv.org — the origin of the backend's
// PONDER_CALLBACK_URL). Unset means any origin, as before, and says so at boot.
const allowedOrigins = (process.env.HOOK_ALLOWED_ORIGINS || "")
  .split(",")
  .map((origin) => origin.trim().replace(/\/+$/, "").toLowerCase())
  .filter(Boolean)

const hookOriginAllowed = (url: URL): boolean =>
  allowedOrigins.length === 0 || allowedOrigins.includes(url.origin.toLowerCase())

const parseHookUrl = (raw: unknown): URL | null => {
  if (typeof raw !== "string") return null
  try {
    const url = new URL(raw)
    if (url.protocol !== "https:" && url.protocol !== "http:") return null
    if (url.username || url.password) return null
    return url
  } catch {
    return null
  }
}

const isAddress = (raw: unknown): raw is string =>
  typeof raw === "string" && /^0x[0-9a-fA-F]{40}$/.test(raw)

// Read-only, for listing hooks. db.ts keeps its pool to itself and cannot gain
// an export without changing the build id.
const hooksPool = new pg.Pool({ connectionString: process.env.DATABASE_URL, max: 2 })

const listHooks = async (): Promise<PonderHook[]> =>
  (await hooksPool.query(`SELECT id, address, url FROM ponder_hooks ORDER BY id;`)).rows

const describeHook = (hook: PonderHook) => {
  const url = parseHookUrl(hook.url)
  return { ...hook, allowed: url !== null && hookOriginAllowed(url) }
}

// At boot, name any stored hook that points outside the allowed origins. They
// are reported, not deleted: a wrong HOOK_ALLOWED_ORIGINS must not wipe the
// backend's own hooks. DELETE /hooks?id= removes one.
void (async () => {
  if (allowedOrigins.length === 0) {
    console.warn("ponder api: HOOK_ALLOWED_ORIGINS is not set; hooks may point at any origin")
    return
  }
  try {
    const foreign = (await listHooks()).map(describeHook).filter((hook) => !hook.allowed)
    if (foreign.length > 0) {
      console.warn(
        `ponder api: ${foreign.length} stored hook(s) point outside HOOK_ALLOWED_ORIGINS and are sent the admin key on every matching transfer; delete them:`,
        foreign.map(({ id, address, url }) => ({ id, address, url })),
      )
    }
  } catch (error) {
    console.warn("ponder api: could not audit stored hooks:", error)
  }
})()

const app = new Hono();

app.use("*", async (c, next) => {
  if (!hasAdminKey(c.req.header("X-Admin-Key"))) {
    return c.text("unauthorized", 401)
  }
  await next()
})

app.use("/sql/*", client({ db, schema }));

app.use("/", graphql({ db, schema }));
app.use("/graphql", graphql({ db, schema }));

app.get("/hooks", async (c) => {
  try {
    return c.json((await listHooks()).map(describeHook))
  }
  catch(error) {
    console.log(error)
    return c.text("internal server error", 500)
  }
})

app.post("/hooks", async (c) => {
  let hookRequest: Partial<PonderHook>
  try {
    hookRequest = (await c.req.json()) as Partial<PonderHook>
  } catch {
    return c.text("bad request", 400)
  }

  const url = parseHookUrl(hookRequest.url)
  if (!url || !isAddress(hookRequest.address)) {
    return c.text("bad request", 400)
  }
  if (!hookOriginAllowed(url)) {
    return c.text("hook url origin is not allowed", 400)
  }

  try {
    // The ping carries the key, so it goes nowhere it was not sent: no
    // following a redirect to another host, and no waiting forever.
    const ping = await fetch(url, {
      headers: {
        "X-Admin-Key": process.env.ADMIN_KEY as string
      },
      redirect: "manual",
      signal: AbortSignal.timeout(10_000),
    })
    if(!ping.ok) {
      return c.text("bad request", 400)
    }
    const hookResponse = await addHook({ id: 0, address: hookRequest.address, url: String(hookRequest.url) })
    return c.json(hookResponse, 201)
  }
  catch(error) {
    console.log(error)
    return c.text("internal server error", 500)
  }
})

app.delete("/hooks", async (c) => {
  const hookId = Number(c.req.query("id"))
  if (!Number.isInteger(hookId) || hookId <= 0) {
    return c.text("bad request", 400)
  }

  try {
    await deleteHook(hookId)
    return c.text("ok", 200)
  }
  catch(error) {
    console.log(error)
    return c.text("internal server error", 500)
  }
})

export default app;

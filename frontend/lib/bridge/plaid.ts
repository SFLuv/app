// Plaid Link, the way a merchant connects a bank account.
//
// The token comes from our backend (which asked Bridge for it); the merchant
// picks their bank and logs in inside Plaid's own iframe; the public token
// that comes back goes straight to our backend, which hands it to Bridge.
// Account numbers never touch this page or ours.

const PLAID_LINK_SCRIPT = "https://cdn.plaid.com/link/v2/stable/link-initialize.js"

export interface PlaidLinkResult {
  publicToken: string
  metadata: unknown
}

interface PlaidHandler {
  open: () => void
  exit: (opts?: { force?: boolean }) => void
  destroy: () => void
}

interface PlaidGlobal {
  create: (config: {
    token: string
    onSuccess: (public_token: string, metadata: unknown) => void
    onExit: (err: unknown, metadata: unknown) => void
    onEvent?: (eventName: string, metadata: unknown) => void
  }) => PlaidHandler
}

declare global {
  interface Window {
    Plaid?: PlaidGlobal
  }
}

let scriptPromise: Promise<PlaidGlobal> | null = null

function loadPlaid(): Promise<PlaidGlobal> {
  if (typeof window === "undefined") {
    return Promise.reject(new Error("Plaid Link is only available in the browser"))
  }
  if (window.Plaid) return Promise.resolve(window.Plaid)
  if (scriptPromise) return scriptPromise

  scriptPromise = new Promise<PlaidGlobal>((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(`script[src="${PLAID_LINK_SCRIPT}"]`)
    const script = existing ?? document.createElement("script")
    const done = () => {
      if (window.Plaid) resolve(window.Plaid)
      else reject(new Error("Plaid Link failed to initialise"))
    }
    if (existing && window.Plaid) {
      resolve(window.Plaid)
      return
    }
    script.addEventListener("load", done, { once: true })
    script.addEventListener(
      "error",
      () => {
        scriptPromise = null
        reject(new Error("Could not load Plaid Link"))
      },
      { once: true },
    )
    if (!existing) {
      script.src = PLAID_LINK_SCRIPT
      script.async = true
      document.head.appendChild(script)
    }
  })
  return scriptPromise
}

// PlaidExitError is thrown when the merchant closes Plaid without linking.
// Callers treat it as "cancelled", not as a failure to report.
export class PlaidExitError extends Error {
  constructor(public readonly detail: unknown) {
    super("Bank connection was cancelled")
    this.name = "PlaidExitError"
  }
}

// openPlaidLink resolves with the public token once the merchant has linked
// a bank, rejects with PlaidExitError if they close the modal, and rejects
// with a plain Error if Plaid reports a problem.
export async function openPlaidLink(linkToken: string): Promise<PlaidLinkResult> {
  const plaid = await loadPlaid()
  return new Promise<PlaidLinkResult>((resolve, reject) => {
    let settled = false
    const handler = plaid.create({
      token: linkToken,
      onSuccess: (publicToken, metadata) => {
        settled = true
        resolve({ publicToken, metadata })
        handler.destroy()
      },
      onExit: (err, metadata) => {
        if (settled) return
        settled = true
        handler.destroy()
        if (err) reject(new Error(describePlaidError(err)))
        else reject(new PlaidExitError(metadata))
      },
    })
    handler.open()
  })
}

function describePlaidError(err: unknown): string {
  if (err && typeof err === "object") {
    const e = err as { display_message?: string; error_message?: string; error_code?: string }
    return e.display_message || e.error_message || e.error_code || "Plaid reported an error"
  }
  return "Plaid reported an error"
}

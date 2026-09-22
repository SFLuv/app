"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { getAddress, type Address, type Hex } from "viem"

import type { AppWallet } from "@/lib/wallets/wallets"
import { hasSignetNodes, SIGNET_REGISTRY } from "@/lib/signet/config"
import {
  buildBindAuthorization,
  encodeBindWithSignature,
  type SignTypedData,
} from "@/lib/signet/bind"
import {
  enrol,
  type EnrolProgress,
  type ExecuteFromSafe,
} from "@/lib/signet/enrol"
import {
  celoClient,
  getBlockPin,
  readEnrolmentState,
  type EnrolmentState,
} from "@/lib/signet/state"
import {
  isSessionExpired,
  openSession,
  ResolverAuthError,
  type SignetSession,
} from "@/lib/signet/session"
import { SignetSigner, type UserOpSigner } from "@/lib/signet/signer"

/**
 * Where the "sign with Signet" preference lives.
 *
 * Per (account, wallet): one login has several smart wallets and only one is
 * bound at a time, so a preference that outlived a wallet switch would silently
 * apply to the wrong one.
 *
 * localStorage is right for this and only this: it is a per-browser convenience
 * with no security weight. Turning it on grants nothing — the Signet key is
 * already a Safe owner from enrolment — and turning it off revokes nothing.
 * Every access is wrapped because private windows and blocked site data make
 * these throw rather than return empty.
 */
const preferenceKey = (eoa: Address, safe: Address) =>
  `signet:prefer:${eoa.toLowerCase()}:${safe.toLowerCase()}`

function readPreference(eoa: Address, safe: Address): boolean {
  try {
    return window.localStorage.getItem(preferenceKey(eoa, safe)) === "1"
  } catch {
    return false
  }
}

function writePreference(eoa: Address, safe: Address, on: boolean) {
  try {
    if (on) window.localStorage.setItem(preferenceKey(eoa, safe), "1")
    else window.localStorage.removeItem(preferenceKey(eoa, safe))
  } catch {
    // A browser that will not store it still honours the toggle for this tab.
  }
}

/**
 * User-facing copy for a failed session or enrolment.
 *
 * The SDK classifies node refusals by matching on prose — "no auth resolver
 * configured", "did not authorize" — but the fleet answers a plain 401 whose
 * whole body is {"error":"unauthorized"}. That matches nothing, so it arrives
 * as `unknown` with `isNodeMisconfiguration` false. The rich classification is
 * therefore real but mostly unreachable, and the default branch has to carry
 * the common case honestly: it names both plausible causes rather than
 * asserting one we cannot distinguish.
 *
 * Never passes the node's own text through. `e.message` reads
 * "https://oll1.nodes.oleary.com: unknown — {"error":"unauthorized"}" — JSON,
 * wrapped around a host the user has no relationship with. That belongs in the
 * console, where it is ours to act on.
 */
function describeSignetFailure(e: unknown): string {
  if (!(e instanceof ResolverAuthError)) {
    return e instanceof Error && e.message
      ? e.message
      : "Unable to set up Signet signing."
  }
  // Ordered by how much it narrows things: a node we never reached says
  // nothing about authorization, so transport is checked before any verdict.
  if (e.transport) {
    return "Could not reach the signing service. Please try again."
  }
  if (e.isNodeMisconfiguration) {
    return "The signing service is not ready yet — this is not a problem with your wallet."
  }
  switch (e.code) {
    case "not_authorized":
      return "This wallet is not authorized to use Signet yet."
    case "siwe_verification_failed":
      return "That signature could not be verified. Please try again."
    case "block_pin_stale":
    case "block_pin_ahead":
    case "block_pin_hash_mismatch":
      return "The network moved while we were setting up. Please try again."
    case "chain_id_mismatch":
    case "missing_session_resource":
    case "missing_expiration":
      // Malformed requests we built. Nothing the user can do, and nothing they
      // should be invited to retry — it would fail identically.
      return "Something went wrong preparing the request. Please report this."
    default:
      return "The signing service turned down the request. If you have just linked this wallet, wait a moment and try again."
  }
}

/** Everything we want in the console when the above hides the detail. */
function logSignetFailure(scope: string, e: unknown) {
  if (e instanceof ResolverAuthError) {
    console.error(`[signet] ${scope}`, {
      code: e.code,
      node: e.nodeUrl,
      status: e.status,
      transport: e.transport,
      detail: e.detail,
    })
  } else {
    console.error(`[signet] ${scope}`, e)
  }
}

/**
 * Signet enrolment state for one smart wallet, plus the one irreversible action
 * that starts it.
 *
 * Scoped to a single wallet on purpose. One login has several smart wallets but
 * only one binding, so at most one of them is Signet-signed at a time.
 */
export function useSignetEnrolment(wallet: AppWallet | null | undefined) {
  const [state, setState] = useState<EnrolmentState | null>(null)
  const [loading, setLoading] = useState(false)
  const [binding, setBinding] = useState(false)
  const [error, setError] = useState<string | null>(null)

  /**
   * Held in memory only, never localStorage.
   *
   * The session private key is a bearer credential for /v1/sign for as long as
   * it lives, and its one-hour TTL is the whole mitigation for holding it at
   * all. Persisting it across tabs would widen that window for no gain: the
   * only thing enrolment needs a session for is keygen, which happens once.
   */
  const [session, setSession] = useState<SignetSession | null>(null)
  const [authenticating, setAuthenticating] = useState(false)
  const [enrolling, setEnrolling] = useState(false)
  const [progress, setProgress] = useState<EnrolProgress[]>([])
  /** Whether this wallet signs with Signet. Restored from localStorage below. */
  const [preferSignet, setPreferSignetState] = useState(false)

  const client = useMemo(() => celoClient(), [])

  const eoa = wallet?.owner?.address
    ? (getAddress(wallet.owner.address) as Address)
    : null
  const safe = wallet?.address ? (getAddress(wallet.address) as Address) : null
  const eligibleShape = wallet?.type === "smartwallet" && !!eoa && !!safe

  const refresh = useCallback(async () => {
    if (!eligibleShape || !eoa || !safe) {
      setState(null)
      return
    }
    setLoading(true)
    try {
      setState(await readEnrolmentState(client, eoa, safe))
      setError(null)
    } catch (e) {
      // Detection failing must never break the settings page: a Celo RPC hiccup
      // should hide the section, not surface an error the user cannot act on.
      console.error("[signet] enrolment read failed", e)
      setState(null)
    } finally {
      setLoading(false)
    }
  }, [client, eligibleShape, eoa, safe])

  useEffect(() => {
    void refresh()
  }, [refresh])

  /**
   * Re-read chain state until it reflects a write we just made.
   *
   * `execSponsored` resolves when the bundler ACCEPTS the operation, which is
   * before it is mined and well before a public RPC will serve the result. A
   * single read at that moment returns the state we just changed, so the card
   * sits unchanged and the write looks like it did nothing until the user
   * reloads the page by hand.
   *
   * Gives up quietly after the window: the write has usually landed by then
   * anyway, and a stuck spinner would be a worse lie than a stale card that
   * the next refresh corrects.
   */
  const refreshUntil = useCallback(
    async (
      satisfied: (s: EnrolmentState) => boolean,
      attempts = 10,
      delayMs = 1500,
    ) => {
      if (!eoa || !safe) return
      for (let i = 0; i < attempts; i++) {
        try {
          const next = await readEnrolmentState(client, eoa, safe)
          setState(next)
          if (satisfied(next)) return
        } catch (e) {
          console.error("[signet] refresh failed", e)
        }
        await new Promise((resolve) => setTimeout(resolve, delayMs))
      }
    },
    [client, eoa, safe],
  )

  // A session speaks for exactly one account. If the wallet changes under us,
  // the old one is not merely stale, it is the wrong identity.
  useEffect(() => {
    setSession((current) => (current && current.eoa !== eoa ? null : current))
  }, [eoa])

  /**
   * Mint a Signet session: one `personal_sign` from the Privy EOA over an
   * ERC-4361 message that binds an ephemeral session key.
   *
   * ORDERING. This cannot run before the bind. `resolve()` answers a zero
   * subject for an unbound account, and the group is configured with
   * `requireCanonicalSubject`, so a node rejects that outright rather than
   * namespacing anything under zero. Bind first, always.
   *
   * A live session is reused. Keygen is idempotent and may be retried, and
   * there is no reason to ask for a second signature inside the same hour.
   */
  const ensureSession = useCallback(async (): Promise<SignetSession | null> => {
    if (!wallet || !eoa) return null
    if (!hasSignetNodes()) {
      setError("Signing service is not configured yet.")
      return null
    }
    if (session && session.eoa === eoa && !isSessionExpired(session)) {
      return session
    }

    setAuthenticating(true)
    setError(null)
    try {
      const fresh = await openSession({
        eoa,
        signMessage: wallet.signMessage,
        getBlockPin: () => getBlockPin(client),
      })
      setSession(fresh)
      return fresh
    } catch (e) {
      logSignetFailure("session failed", e)
      setError(describeSignetFailure(e))
      return null
    } finally {
      setAuthenticating(false)
    }
  }, [wallet, eoa, session, client])

  /**
   * Step 2. Two user-visible actions in one: the EOA signs an EIP-712
   * authorization, then the Safe relays it and pays. Rebindable afterwards, but
   * re-pointing moves the Signet subject, so callers confirm first.
   */
  const bind = useCallback(async () => {
    if (!wallet || !eoa || !safe) return false
    if (state && !state.allowed) {
      setError("This wallet is not part of the trial.")
      return false
    }
    if (state?.boundSafe && !state.boundElsewhere) {
      // Already bound to this wallet; treat as success so the flow is resumable.
      await refresh()
      return true
    }

    setBinding(true)
    setError(null)
    try {
      // The Privy EOA authorizes; only the account may bind itself.
      const auth = await buildBindAuthorization(client, eoa, safe)
      const signature = (await wallet.signTypedData(
        auth.domain,
        auth.types as unknown as Record<string, { name: string; type: string }[]>,
        auth.message as unknown as Record<string, unknown>,
      )) as Hex
      const receipt = await wallet.execSponsored(
        SIGNET_REGISTRY,
        encodeBindWithSignature(auth, signature),
      )
      if (receipt.error) {
        setError(receipt.error)
        return false
      }
      await refreshUntil((next) => !!next.boundSafe && !next.boundElsewhere)
      return true
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unable to enable Signet.")
      return false
    } finally {
      setBinding(false)
    }
  }, [wallet, eoa, safe, state, refresh, refreshUntil, client])

  /**
   * Steps 4 and 5: keygen, then make the threshold key a Safe owner.
   *
   * Resumable by construction — `enrol` re-reads chain state rather than
   * trusting a cursor, and keygen treats a 409 as success — so this is safe to
   * call again after a closed tab, a declined signature or a failed relay.
   */
  const completeEnrolment = useCallback(async () => {
    if (!wallet || !eoa || !safe) return false
    if (state && !state.allowed) {
      setError("This wallet is not part of the trial.")
      return false
    }
    // Checked here so the failure names the missing step. Without a binding
    // `resolve()` answers a zero subject, and the node turns that into a flat
    // "unauthorized" that tells the user nothing about what to do next.
    if (!state?.boundSafe) {
      setError("Link this wallet to Signet first.")
      return false
    }

    const active = await ensureSession()
    if (!active) return false // ensureSession has already set a readable error

    setEnrolling(true)
    setProgress([])
    try {
      /** `enrol` wants a hash; `execSponsored` reports failure in-band. */
      const execute: ExecuteFromSafe = async (to, data) => {
        const receipt = await wallet.execSponsored(to, data)
        if (receipt.error) throw new Error(receipt.error)
        if (!receipt.hash) throw new Error("sponsored call returned no hash")
        return receipt.hash
      }
      const signTypedData: SignTypedData = (auth) =>
        wallet.signTypedData(
          auth.domain,
          auth.types as unknown as Record<string, { name: string; type: string }[]>,
          auth.message as unknown as Record<string, unknown>,
        ) as Promise<Hex>

      const result = await enrol({
        client,
        session: active,
        eoa,
        safe,
        execute,
        signTypedData,
        onProgress: (p) => setProgress((prev) => [...prev, p]),
      })
      await refreshUntil((next) => next.signetIsOwner)
      return result.finalState.signetIsOwner
    } catch (e) {
      logSignetFailure("enrolment failed", e)
      setError(describeSignetFailure(e))
      return false
    } finally {
      setEnrolling(false)
    }
  }, [wallet, eoa, safe, state, ensureSession, client, refreshUntil])

  // Restore the stored preference whenever the wallet changes. Runs on mount,
  // so a reload keeps signing with whatever the user last chose.
  useEffect(() => {
    if (!eoa || !safe) {
      setPreferSignetState(false)
      return
    }
    setPreferSignetState(readPreference(eoa, safe))
  }, [eoa, safe])

  /**
   * A signer that authenticates on first use rather than at install time.
   *
   * The bundler calls getAddress() while building the operation and
   * signMessage() only when it signs. Answering getAddress() from the enrolment
   * state means restoring the preference on page load costs no signature — the
   * SIWE prompt arrives when the user actually sends something, which is a
   * moment they are already expecting to confirm.
   */
  const buildLazySigner = useCallback(
    (signetAddress: Address, account: Address): UserOpSigner => {
      let signer: SignetSigner | null = null
      return {
        getAddress: async () => signetAddress,
        signMessage: async (message) => {
          if (!signer) {
            const session = await ensureSession()
            if (!session) throw new Error("Signet session unavailable")
            signer = await SignetSigner.create(client, session, signetAddress, account, {
              reauthenticate: async () => {
                const fresh = await ensureSession()
                if (!fresh) throw new Error("Signet re-authentication failed")
                return fresh
              },
            })
          }
          return signer.signMessage(message)
        },
      }
    },
    [client, ensureSession],
  )

  /**
   * Install or clear the signer on the wallet itself.
   *
   * Conditioned on `signetIsOwner`, not on the preference alone: a key the Safe
   * does not recognise would sign operations the module rejects on chain, so an
   * un-enrolled wallet must keep using the Privy key whatever is stored.
   */
  useEffect(() => {
    if (!wallet) return
    const signetAddress = state?.signetAddress
    if (preferSignet && state?.signetIsOwner && signetAddress && safe) {
      wallet.setSignetSigner(buildLazySigner(signetAddress, safe))
    } else {
      wallet.setSignetSigner(null)
    }
    return () => {
      wallet.setSignetSigner(null)
    }
  }, [wallet, preferSignet, state?.signetIsOwner, state?.signetAddress, safe, buildLazySigner])

  /** Persist and apply. Takes effect on the next transaction, not this one. */
  const setPreferSignet = useCallback(
    (on: boolean) => {
      setPreferSignetState(on)
      if (eoa && safe) writePreference(eoa, safe, on)
    },
    [eoa, safe],
  )

  return {
    /** null while loading, or when this wallet cannot participate at all. */
    state,
    loading,
    binding,
    error,
    refresh,
    bind,
    /** True while the Privy EOA is being asked for the SIWE signature. */
    authenticating,
    session,
    ensureSession,
    /** True while keygen / add-owner are running. */
    enrolling,
    /** Live step log from `enrol`, for driving the card's checklist. */
    progress,
    completeEnrolment,
    /** Whether this wallet signs with Signet. Persisted per (account, wallet). */
    preferSignet,
    setPreferSignet,
    /** Hide the whole section unless the gate says this user is in the trial. */
    visible: !!state?.allowed,
  }
}

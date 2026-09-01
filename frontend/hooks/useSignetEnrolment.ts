"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { getAddress, type Address, type Hex } from "viem"

import type { AppWallet } from "@/lib/wallets/wallets"
import { SIGNET_REGISTRY } from "@/lib/signet/config"
import { buildBindAuthorization, encodeBindWithSignature } from "@/lib/signet/bind"
import { celoClient, readEnrolmentState, type EnrolmentState } from "@/lib/signet/state"

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
      await refresh()
      return true
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unable to enable Signet.")
      return false
    } finally {
      setBinding(false)
    }
  }, [wallet, eoa, safe, state, refresh, client])

  return {
    /** null while loading, or when this wallet cannot participate at all. */
    state,
    loading,
    binding,
    error,
    refresh,
    bind,
    /** Hide the whole section unless the gate says this user is in the trial. */
    visible: !!state?.allowed,
  }
}

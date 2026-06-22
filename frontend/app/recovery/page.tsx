"use client"

import { useSearchParams, useRouter } from "next/navigation"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { usePrivy } from "@privy-io/react-auth"
import { Button } from "@/components/ui/button"
import { BACKEND } from "@/lib/constants"
import { useApp } from "@/context/AppProvider"
import { useChainConfig } from "@/context/ChainConfigProvider"

const loginRedirectPendingKey = "recovery_login_redirect_pending"

// Format a base-unit token amount to a human string without pulling in ethers
// (whose formatUnits API differs across v5/v6).
function formatBaseUnits(amount: string, decimals: number): string {
  try {
    const n = BigInt(amount)
    const base = 10n ** BigInt(decimals)
    const whole = n / base
    const frac = n % base
    if (frac === 0n) return whole.toString()
    const fracStr = frac.toString().padStart(decimals, "0").replace(/0+$/, "")
    return fracStr ? `${whole.toString()}.${fracStr}` : whole.toString()
  } catch {
    return amount
  }
}

const shortAddress = (address: string): string =>
  address.length > 12 ? `${address.slice(0, 6)}…${address.slice(-4)}` : address

type PreviewState = "loading" | "ready" | "invalid" | "none" | "claimed"

const Page = () => {
  const searchParams = useSearchParams()
  const router = useRouter()
  const { authFetch, user, status: appStatus, ensurePrimarySmartWallet } = useApp()
  const { login, authenticated, ready: privyReady } = usePrivy()
  const config = useChainConfig()

  const sigAuthAccount = searchParams.get("sigAuthAccount")
  const sigAuthExpiry = searchParams.get("sigAuthExpiry")
  const sigAuthSignature = searchParams.get("sigAuthSignature")
  const sigAuthRedirect = searchParams.get("sigAuthRedirect")
  const hasSigAuth = Boolean(sigAuthAccount && sigAuthExpiry && sigAuthSignature)

  const sigAuthBody = useMemo(() => {
    const body: Record<string, string> = {
      sigAuthAccount: sigAuthAccount || "",
      sigAuthExpiry: sigAuthExpiry || "",
      sigAuthSignature: sigAuthSignature || "",
    }
    if (sigAuthRedirect) body.sigAuthRedirect = sigAuthRedirect
    return body
  }, [sigAuthAccount, sigAuthExpiry, sigAuthSignature, sigAuthRedirect])

  const [previewState, setPreviewState] = useState<PreviewState>("loading")
  const [account, setAccount] = useState<string | null>(null)
  const [amountBase, setAmountBase] = useState<string>("0")
  const [claiming, setClaiming] = useState(false)
  const [claimed, setClaimed] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [loginPending, setLoginPending] = useState<boolean>(() => {
    if (typeof window === "undefined") return false
    return window.sessionStorage.getItem(loginRedirectPendingKey) === "1"
  })
  const claimAttemptedRef = useRef(false)

  const sessionReady = authenticated && privyReady && appStatus === "authenticated" && Boolean(user)
  const formattedAmount = useMemo(() => formatBaseUnits(amountBase, config.tokenDecimals), [amountBase, config.tokenDecimals])
  const tokenSymbol = config.tokenSymbol || "SFLuv"

  const markLoginPending = useCallback(() => {
    setLoginPending(true)
    if (typeof window !== "undefined") {
      try { window.sessionStorage.setItem(loginRedirectPendingKey, "1") } catch { /* ignore */ }
    }
  }, [])
  const clearLoginPending = useCallback(() => {
    setLoginPending(false)
    if (typeof window !== "undefined") {
      try { window.sessionStorage.removeItem(loginRedirectPendingKey) } catch { /* ignore */ }
    }
  }, [])

  // Sanity check + claimable balance: the backend verifies the sigAuth signature
  // (incl. smart-account EIP-1271 / Safe ownership) and returns the balance.
  useEffect(() => {
    if (!hasSigAuth) {
      setPreviewState("invalid")
      return
    }
    let cancelled = false
    ;(async () => {
      try {
        const res = await fetch(BACKEND + "/recovery/balance", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(sigAuthBody),
        })
        if (cancelled) return
        if (!res.ok) {
          setPreviewState("invalid")
          return
        }
        const data = await res.json()
        setAccount(typeof data.account === "string" ? data.account : sigAuthAccount)
        setAmountBase(String(data.amount || "0"))
        if (data.claimed) {
          setPreviewState("claimed")
        } else if (!data.amount || data.amount === "0") {
          setPreviewState("none")
        } else {
          setPreviewState("ready")
        }
      } catch {
        if (!cancelled) setPreviewState("invalid")
      }
    })()
    return () => {
      cancelled = true
    }
  }, [hasSigAuth, sigAuthBody, sigAuthAccount])

  const claim = useCallback(async () => {
    if (claimAttemptedRef.current) return
    claimAttemptedRef.current = true
    setClaiming(true)
    setError(null)
    try {
      // Make sure the logged-in user has a primary smart wallet to receive into.
      await ensurePrimarySmartWallet().catch(() => undefined)
      const res = await authFetch("/recovery/claim", {
        method: "POST",
        body: JSON.stringify(sigAuthBody),
      })
      const data = await res.json().catch(() => ({} as Record<string, unknown>))
      if (!res.ok) {
        setError((data && (data as { message?: string }).message) || "Unable to claim your balance.")
        claimAttemptedRef.current = false
        setClaiming(false)
        return
      }
      setClaimed(true)
      const recipient = typeof (data as { recipient?: string }).recipient === "string" ? (data as { recipient: string }).recipient : ""
      const destination = recipient ? `/wallets/${recipient}` : "/wallets"
      setTimeout(() => router.replace(destination), 1800)
    } catch {
      setError("Unable to claim your balance. Please try again.")
      claimAttemptedRef.current = false
      setClaiming(false)
    }
  }, [authFetch, ensurePrimarySmartWallet, router, sigAuthBody])

  // After login completes and the session is ready, auto-claim.
  useEffect(() => {
    if (!loginPending) return
    if (sessionReady && previewState === "ready") {
      clearLoginPending()
      void claim()
    }
  }, [loginPending, sessionReady, previewState, claim, clearLoginPending])

  const onClaim = useCallback(async () => {
    if (sessionReady) {
      void claim()
      return
    }
    if (!privyReady) return
    markLoginPending()
    try {
      await login()
    } catch {
      clearLoginPending()
    }
  }, [sessionReady, privyReady, claim, login, markLoginPending, clearLoginPending])

  const Card = ({ children }: { children: React.ReactNode }) => (
    <div className="mx-auto w-full max-w-md px-4 text-center">
      <div className="rounded-lg border bg-card/95 p-6 shadow-sm">
        <img src="/icon.png" alt="SFLUV" className="mx-auto h-16 w-16 object-contain" />
        {children}
      </div>
    </div>
  )

  const ReturnToMap = () => (
    <Button variant="outline" className="mt-5 w-full" onClick={() => router.push("/map")}>
      Return to map
    </Button>
  )

  return (
    <div className="min-h-screen flex items-center justify-center">
      {previewState === "loading" ? (
        <div className="text-center space-y-6">
          <h2 className="text-3xl font-bold text-black dark:text-white">Verifying…</h2>
          <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c] m-auto" />
        </div>
      ) : previewState === "invalid" ? (
        <Card>
          <h2 className="mt-4 text-2xl font-bold text-black dark:text-white">Recovery link invalid</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            This recovery link is missing or its signature could not be verified. Please reopen it from your Citizen
            Wallet app.
          </p>
          <ReturnToMap />
        </Card>
      ) : previewState === "none" ? (
        <Card>
          <h2 className="mt-4 text-2xl font-bold text-black dark:text-white">Nothing to recover</h2>
          {account && <p className="mt-2 text-xs text-muted-foreground">{shortAddress(account)}</p>}
          <p className="mt-2 text-sm text-muted-foreground">
            This account has no recoverable SFLuv balance from the migration.
          </p>
          <ReturnToMap />
        </Card>
      ) : previewState === "claimed" && !claimed ? (
        <Card>
          <h2 className="mt-4 text-2xl font-bold text-black dark:text-white">Already claimed</h2>
          {account && <p className="mt-2 text-xs text-muted-foreground">{shortAddress(account)}</p>}
          <p className="mt-2 text-sm text-muted-foreground">
            This balance has already been recovered.
          </p>
          <ReturnToMap />
        </Card>
      ) : claimed ? (
        <Card>
          <h2 className="mt-4 text-2xl font-bold text-black dark:text-white">Recovered!</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            {formattedAmount} {tokenSymbol} is on its way to your wallet. Redirecting…
          </p>
        </Card>
      ) : (
        <Card>
          <h2 className="mt-4 text-2xl font-bold text-black dark:text-white">Claimable Balance</h2>
          {account && <p className="mt-1 text-xs text-muted-foreground">{shortAddress(account)}</p>}
          <p className="mt-3 text-3xl font-bold text-[#eb6c6c]">
            {formattedAmount} {tokenSymbol}
          </p>
          {error && <p className="mt-4 text-sm text-[#b42318] dark:text-[#ffb4a8]">{error}</p>}
          <Button onClick={onClaim} disabled={claiming || loginPending || !privyReady} className="mt-5 w-full">
            {claiming || loginPending ? "Claiming…" : sessionReady ? "Claim" : "Log in to claim"}
          </Button>
        </Card>
      )}
    </div>
  )
}

export default Page

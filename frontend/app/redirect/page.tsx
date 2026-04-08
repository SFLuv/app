"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { isAddress } from "viem"
import { useApp } from "@/context/AppProvider"
import { COMMUNITY } from "@/lib/constants"
import { Button } from "@/components/ui/button"
import { AlertTriangle, Loader2 } from "lucide-react"
import { decodeBase64UrlAddress } from "@/lib/redeem-link"

type RedirectStage =
  | "checking"
  | "needs-login"
  | "ensuring-wallet"
  | "redirecting"
  | "error"

// Stub: SFLuv app deep-link probe. Returns true if the app caught the link.
// The app has not launched yet, so this currently no-ops and returns false.
const tryOpenSfluvApp = async (_to: string, _tipTo: string): Promise<boolean> => {
  return false
}

// Build the query-string body shared by both Citizen Wallet target URLs
// (custom scheme + universal link). CW's router accepts `sendto`, `tipTo`,
// and `alias` at the top-level path; Flutter's `parseSendtoUrl` lifts these
// straight into the native send screen.
const buildCwSendQuery = (to: string, tipTo: string): string => {
  const alias = COMMUNITY.community.alias
  const params = new URLSearchParams()
  params.set("alias", alias)
  params.set("sendto", `${to}@${alias}`)
  if (tipTo && isAddress(tipTo)) {
    params.set("tipTo", tipTo)
  }
  return params.toString()
}

const buildCwUniversalLink = (to: string, tipTo: string): string =>
  `https://app.citizenwallet.xyz/?${buildCwSendQuery(to, tipTo)}`

// Probe for the Citizen Wallet app via its custom URL scheme. Resolves true
// if the OS handed off to the app (detected via the page becoming hidden or
// pagehide firing within the probe window), false otherwise.
//
// We use the `citizenwallet://` scheme rather than the `app.citizenwallet.xyz`
// universal link because the universal link, when the app is NOT installed,
// would navigate the browser away from this page to the CW web wallet — which
// would short-circuit the SFLuv login fallback. The custom scheme either
// opens the app or fails silently, leaving us on /redirect.
const tryOpenCitizenWalletApp = (to: string, tipTo: string): Promise<boolean> => {
  return new Promise((resolve) => {
    if (typeof window === "undefined" || typeof document === "undefined") {
      resolve(false)
      return
    }

    const userAgent = typeof navigator !== "undefined" ? navigator.userAgent || "" : ""
    const isMobile = /Android|iPhone|iPad|iPod/i.test(userAgent)
    if (!isMobile) {
      resolve(false)
      return
    }

    if (document.hidden) {
      // Tab is already backgrounded; visibility transitions can't be trusted.
      resolve(false)
      return
    }

    const cwDeepLink = `citizenwallet:///?${buildCwSendQuery(to, tipTo)}`

    let resolved = false
    const finish = (opened: boolean) => {
      if (resolved) return
      resolved = true
      document.removeEventListener("visibilitychange", handleVisibilityChange)
      window.removeEventListener("pagehide", handlePageHide)
      window.removeEventListener("blur", handleBlur)
      resolve(opened)
    }

    const handleVisibilityChange = () => {
      if (document.hidden) finish(true)
    }
    const handlePageHide = () => finish(true)
    const handleBlur = () => finish(true)

    document.addEventListener("visibilitychange", handleVisibilityChange)
    window.addEventListener("pagehide", handlePageHide)
    window.addEventListener("blur", handleBlur)

    try {
      window.location.href = cwDeepLink
    } catch {
      // Navigation throw — treat as the app not being installed.
    }

    // 1500ms is the standard "app handoff" detection window: long enough for
    // iOS/Android to switch context, short enough that users don't notice
    // when CW isn't installed.
    setTimeout(() => finish(false), 1500)
  })
}

export default function RedirectPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { status, login, user, walletsStatus, ensurePrimarySmartWallet } = useApp()

  // Parameter sources, in priority order:
  //   1. Native CW sendtoUrl format: `sendto=<hex>@<alias>`, `tipTo=<hex>`.
  //      This is the canonical format for merchant QRs — CW's Dart scanner
  //      parses it natively; we also accept it here so non-CW users landing
  //      on our domain reach the same send flow.
  //   2. Aliased short form: `m=s`, `t=<base64url>`, `tt=<base64url>`.
  //   3. Legacy long form: `mode=send`, `to=<hex>`, `tipTo=<hex>`.

  const sendtoParam = searchParams.get("sendto") || ""
  const sendtoAddress = useMemo(() => {
    if (!sendtoParam) return ""
    const [addr] = sendtoParam.split("@")
    return addr || ""
  }, [sendtoParam])

  const rawMode = searchParams.get("m") || searchParams.get("mode")
  const mode = useMemo(() => {
    if (rawMode) {
      if (rawMode === "s" || rawMode === "send") return "send"
      return rawMode
    }
    // When a sendto= param is present, default mode to "send" so the native
    // CW sendtoUrl format works without an explicit mode param.
    if (sendtoAddress) return "send"
    return null
  }, [rawMode, sendtoAddress])

  const to = useMemo(() => {
    if (sendtoAddress && isAddress(sendtoAddress)) return sendtoAddress
    const aliasTo = searchParams.get("t")
    if (aliasTo) {
      const decoded = decodeBase64UrlAddress(aliasTo)
      if (decoded) return decoded
      // Fall through: allow raw 0x addresses passed under the alias too.
      if (isAddress(aliasTo)) return aliasTo
    }
    return searchParams.get("to") || ""
  }, [sendtoAddress, searchParams])

  const tipTo = useMemo(() => {
    const rawTipTo = searchParams.get("tipTo")
    if (rawTipTo && isAddress(rawTipTo)) return rawTipTo
    const aliasTipTo = searchParams.get("tt")
    if (aliasTipTo) {
      const decoded = decodeBase64UrlAddress(aliasTipTo)
      if (decoded) return decoded
      if (isAddress(aliasTipTo)) return aliasTipTo
    }
    return rawTipTo || ""
  }, [searchParams])

  const sigAuthAccount = searchParams.get("sigAuthAccount")

  const [stage, setStage] = useState<RedirectStage>("checking")
  const [error, setError] = useState<string | null>(null)
  const handledInitialRef = useRef(false)
  const ensureInFlightRef = useRef(false)

  // Initial dispatch: validate params, try app deep link, then CW deep link
  useEffect(() => {
    if (handledInitialRef.current) return

    if (mode !== "send") {
      setError("Unsupported redirect mode.")
      setStage("error")
      handledInitialRef.current = true
      return
    }
    if (!to) {
      setError("Missing recipient address.")
      setStage("error")
      handledInitialRef.current = true
      return
    }
    if (!isAddress(to)) {
      setError("Invalid recipient address.")
      setStage("error")
      handledInitialRef.current = true
      return
    }

    handledInitialRef.current = true

    const run = async () => {
      // 1. Try the SFLuv-native app deep link (currently a stub).
      const sfluvOpened = await tryOpenSfluvApp(to, tipTo)
      if (sfluvOpened) return

      // 2. If the user came from CW's in-app browser (sigAuthAccount present),
      // bounce them back into CW directly via the universal link — this is
      // safe because they're already in the CW context.
      if (sigAuthAccount) {
        window.location.replace(buildCwUniversalLink(to, tipTo))
        return
      }

      // 3. Probe for the Citizen Wallet app via its custom scheme. If
      // installed, the OS will hand off and this page will be backgrounded.
      const cwOpened = await tryOpenCitizenWalletApp(to, tipTo)
      if (cwOpened) return

      // 4. Fall through: wait for auth status to settle before showing login UI
      setStage("checking")
    }
    void run()
  }, [mode, to, tipTo, sigAuthAccount])

  // Once auth status is known, route to login prompt or wallet ensure
  useEffect(() => {
    if (stage !== "checking") return
    if (status === "loading") return
    if (status === "authenticated") {
      setStage("ensuring-wallet")
    } else {
      setStage("needs-login")
    }
  }, [stage, status])

  // Once authenticated, ensure a primary wallet exists, then push to wallet send
  useEffect(() => {
    if (stage !== "needs-login" && stage !== "ensuring-wallet") {
      return
    }
    if (status !== "authenticated") return
    if (walletsStatus === "loading") return
    if (ensureInFlightRef.current) return

    ensureInFlightRef.current = true
    setStage("ensuring-wallet")

    let cancelled = false
    const ensureAndRedirect = async () => {
      try {
        let primary = user?.primaryWalletAddress?.trim() || ""
        if (!primary) {
          const ok = await ensurePrimarySmartWallet()
          if (!ok) {
            if (!cancelled) {
              setError("Could not initialize your primary wallet.")
              setStage("error")
            }
            return
          }
          primary = user?.primaryWalletAddress?.trim() || ""
        }
        if (!primary) {
          if (!cancelled) {
            setError("Primary wallet is not yet available. Please try again.")
            setStage("error")
          }
          return
        }
        if (cancelled) return
        setStage("redirecting")
        const walletQuery = new URLSearchParams()
        walletQuery.set("send", "1")
        walletQuery.set("to", to)
        if (tipTo && isAddress(tipTo)) {
          walletQuery.set("tipTo", tipTo)
        }
        router.replace(`/wallets/${primary}?${walletQuery.toString()}`)
      } catch {
        if (!cancelled) {
          setError("Failed to redirect to your wallet.")
          setStage("error")
        }
      } finally {
        ensureInFlightRef.current = false
      }
    }
    void ensureAndRedirect()
    return () => {
      cancelled = true
    }
  }, [
    stage,
    status,
    walletsStatus,
    user?.primaryWalletAddress,
    ensurePrimarySmartWallet,
    router,
    to,
    tipTo,
  ])

  const handleLogin = async () => {
    setError(null)
    // Stage stays at "needs-login" while the Privy modal is open. If login
    // succeeds, the auth-watch effect picks up status="authenticated" and
    // advances; if the user cancels, they remain on this screen and can retry.
    await login()
  }

  const renderBody = () => {
    if (stage === "error") {
      return (
        <div className="flex flex-col items-center gap-3 text-center">
          <div className="flex items-center gap-2 text-red-600">
            <AlertTriangle className="h-5 w-5" />
            <span className="font-medium">Unable to continue</span>
          </div>
          <p className="text-sm text-muted-foreground">{error}</p>
        </div>
      )
    }

    if (stage === "needs-login") {
      const recipientPreview = to ? `${to.slice(0, 6)}...${to.slice(-4)}` : ""
      return (
        <div className="flex flex-col items-center gap-4 text-center">
          <h1 className="text-xl font-semibold">Send SFLUV</h1>
          <p className="text-sm text-muted-foreground">
            Log in to send to{" "}
            <span className="font-mono">{recipientPreview}</span>.
          </p>
          <Button onClick={handleLogin}>Log In to Continue</Button>
        </div>
      )
    }

    const label =
      stage === "ensuring-wallet"
        ? "Preparing your wallet..."
        : stage === "redirecting"
          ? "Redirecting..."
          : "Loading..."

    return (
      <div className="flex flex-col items-center gap-3 text-center">
        <Loader2 className="h-8 w-8 animate-spin text-[#eb6c6c]" />
        <p className="text-sm text-muted-foreground">{label}</p>
      </div>
    )
  }

  return (
    <div className="flex min-h-[60vh] w-full items-center justify-center p-6">
      <div className="w-full max-w-sm rounded-lg border bg-card p-6 shadow-sm">
        {renderBody()}
      </div>
    </div>
  )
}

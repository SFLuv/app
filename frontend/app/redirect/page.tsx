"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { isAddress } from "viem"
import { useApp } from "@/context/AppProvider"
import { useLocation } from "@/context/LocationProvider"
import { COMMUNITY } from "@/lib/constants"
import { Button } from "@/components/ui/button"
import { AlertTriangle, Loader2 } from "lucide-react"
import { decodeBase64UrlAddress } from "@/lib/redeem-link"

type RedirectStage =
  | "checking"
  | "choose-wallet"
  | "ensuring-wallet"
  | "redirecting"
  | "error"

// Build the query-string body shared by Citizen Wallet handoff URLs. CW's
// router accepts `sendto`, `tipTo`, and `alias` at the top-level path;
// Flutter's `parseSendtoUrl` lifts these straight into the native send screen.
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

export default function RedirectPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { status, login, user, walletsStatus, ensurePrimarySmartWallet } = useApp()
  const { mapLocations, mapLocationsStatus } = useLocation()

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

  // Optional location id appended to merchant QR URLs (l=<id>). Used as a
  // fast path for resolving the merchant name without scanning all locations.
  const locationIdParam = searchParams.get("l") || ""

  const sigAuthAccount = searchParams.get("sigAuthAccount")

  const [stage, setStage] = useState<RedirectStage>("checking")
  const [error, setError] = useState<string | null>(null)
  const handledInitialRef = useRef(false)
  const ensureInFlightRef = useRef(false)

  // Resolve the merchant location name from the public locations list. If
  // `l` is in the URL we use it directly; otherwise we fall back to matching
  // by pay_to_address (case-insensitive).
  const resolvedLocationName = useMemo(() => {
    if (mapLocationsStatus !== "available") return ""
    if (mapLocations.length === 0) return ""

    if (locationIdParam) {
      const numericId = Number(locationIdParam)
      if (Number.isFinite(numericId)) {
        const byId = mapLocations.find((location) => location.id === numericId)
        if (byId?.name) return byId.name
      }
    }

    if (to) {
      const lowered = to.toLowerCase()
      const byAddress = mapLocations.find(
        (location) => (location.pay_to_address || "").toLowerCase() === lowered,
      )
      if (byAddress?.name) return byAddress.name
    }

    return ""
  }, [mapLocations, mapLocationsStatus, locationIdParam, to])

  // Initial dispatch: validate params, then either bounce sigAuth users
  // straight into CW or move into the auth-status check.
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

    // If the user came from CW's in-app browser (sigAuthAccount present),
    // bounce them back into CW directly via the universal link — they're
    // already in the CW context, so no choice screen is needed.
    if (sigAuthAccount) {
      window.location.replace(buildCwUniversalLink(to, tipTo))
      return
    }

    setStage("checking")
  }, [mode, to, tipTo, sigAuthAccount])

  // Auth-status gate: while we're in "checking" wait for Privy to resolve,
  // then either skip the chooser entirely (already authenticated) or surface
  // the SFLuv wallet continuation button.
  useEffect(() => {
    if (stage !== "checking") return
    if (status === "loading") return
    if (status === "authenticated") {
      setStage("ensuring-wallet")
    } else {
      setStage("choose-wallet")
    }
  }, [stage, status])

  // Once authenticated (after the user picks "Pay with SFLuv Wallet"),
  // ensure a primary wallet exists and push to the wallet send screen.
  useEffect(() => {
    if (stage !== "ensuring-wallet") return
    if (status !== "authenticated") return
    if (walletsStatus === "loading") return
    if (ensureInFlightRef.current) return

    ensureInFlightRef.current = true

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

  const handlePayWithSfluv = async () => {
    setError(null)
    if (status === "authenticated") {
      setStage("ensuring-wallet")
      return
    }
    // Privy login modal: if the user completes it, the auth-watch effect
    // above takes over once status flips to "authenticated". Pre-flip the
    // stage so the spinner shows immediately.
    setStage("ensuring-wallet")
    try {
      await login()
    } catch {
      // If login throws or the modal is dismissed, drop back to the choice
      // screen so the user can retry.
      setStage("choose-wallet")
    }
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

    if (stage === "choose-wallet") {
      const recipientPreview = to ? `${to.slice(0, 6)}...${to.slice(-4)}` : ""
      const recipientLabel = resolvedLocationName || recipientPreview
      const recipientIsName = !!resolvedLocationName
      return (
        <div className="flex flex-col items-center gap-4 text-center">
          <h1 className="text-xl font-semibold">Send SFLUV</h1>
          <p className="text-sm text-muted-foreground">
            Send to{" "}
            <span className={recipientIsName ? "font-semibold" : "font-mono"}>
              {recipientLabel}
            </span>
            .
          </p>
          <div className="flex w-full flex-col gap-2">
            <Button
              size="lg"
              className="h-12 w-full text-base"
              onClick={handlePayWithSfluv}
            >
              Pay with SFLuv Wallet
            </Button>
          </div>
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

"use client"

import { useEffect, useRef, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { Address, isAddress } from "viem"
import { generateReceiveLink } from "@citizenwallet/sdk"
import { useApp } from "@/context/AppProvider"
import { CW_APP_BASE_URL, COMMUNITY } from "@/lib/constants"
import { Button } from "@/components/ui/button"
import { AlertTriangle, Loader2 } from "lucide-react"

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

export default function RedirectPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { status, login, user, walletsStatus, ensurePrimarySmartWallet } = useApp()

  const mode = searchParams.get("mode")
  const to = searchParams.get("to") || ""
  const tipTo = searchParams.get("tipTo") || ""
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
      const opened = await tryOpenSfluvApp(to, tipTo)
      if (opened) return

      if (sigAuthAccount) {
        let cwLink = generateReceiveLink(
          CW_APP_BASE_URL,
          COMMUNITY,
          to as Address,
        )
        if (tipTo && isAddress(tipTo)) {
          cwLink += `&tipTo=${tipTo}`
        }
        window.location.replace(cwLink)
        return
      }

      // Fall through: wait for auth status to settle before showing login UI
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
        router.replace(
          `/wallets/${primary}?send=1&to=${encodeURIComponent(to)}`,
        )
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

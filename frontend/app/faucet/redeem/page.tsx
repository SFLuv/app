"use client"

import { useSearchParams } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { usePrivy, useWallets } from "@privy-io/react-auth";
import { AppWallet } from "@/lib/wallets/wallets";
import { BACKEND, CHAIN_ID } from "@/lib/constants";
import { normalizeRedeemCode } from "@/lib/redeem-link";

const normalizeReturnTo = (rawValue: string | null): string | null => {
  if (!rawValue) return null
  const trimmed = rawValue.trim()
  if (!trimmed.startsWith("/")) return null
  if (trimmed.startsWith("//")) return null
  return trimmed
}

const Page = () => {
  const missingSigAuthMessage = "Please download the CitizenWallet app, then scan your QR code again."
  const searchParams = useSearchParams();
  const router = useRouter();
  const { login, authenticated, ready: privyReady } = usePrivy()
  const { wallets, ready: walletsReady } = useWallets()

  const [error, setError] = useState<string | null>();
  const [success, setSuccess] = useState<boolean>(false);
  const [w9Url, setW9Url] = useState<string | null>(null);
  const [w9Email, setW9Email] = useState<string | null>(null);
  const [redeemAccount, setRedeemAccount] = useState<string | null>(null)
  const [continueWithWebWalletRequested, setContinueWithWebWalletRequested] = useState<boolean>(false)
  const [continuingWithWebWallet, setContinuingWithWebWallet] = useState<boolean>(false)
  const [webWalletError, setWebWalletError] = useState<string | null>(null)
  const [successRedirectTo, setSuccessRedirectTo] = useState<string | null>(null)
  const directRedeemAttemptedRef = useRef<boolean>(false)
  const webWalletRedeemAttemptedRef = useRef<boolean>(false)
  const redirectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const sigAuthAccount = searchParams.get("sigAuthAccount")
  const sigAuthSignature = searchParams.get("sigAuthSignature")
  const sigAuthRedirect = searchParams.get("sigAuthRedirect")
  const hasSigAuth = Boolean(sigAuthAccount && sigAuthSignature)
  const shouldUseWebWalletFlow = !hasSigAuth
  const returnTo = normalizeReturnTo(searchParams.get("returnTo"))
  const code = normalizeRedeemCode(searchParams.get("code"))


  const buildW9Url = (baseUrl: string, walletAddress: string, email?: string | null) => {
    if (!baseUrl) return baseUrl
    if (!walletAddress) return baseUrl
    try {
      const url = new URL(baseUrl)
      url.searchParams.set("wallet", walletAddress)
      if (email) {
        url.searchParams.set("email", email)
      }
      return url.toString()
    } catch {
      const encoded = encodeURIComponent(walletAddress)
      const withWallet = baseUrl.includes("?") ? `${baseUrl}&wallet=${encoded}` : `${baseUrl}?wallet=${encoded}`
      if (!email) return withWallet
      const encodedEmail = encodeURIComponent(email)
      return `${withWallet}&email=${encodedEmail}`
    }
  }

  const sendBotRequest = async (address: string, overrideReturnTo?: string) => {
    // let verified = verifyAccountOwnership()
    //implement real verification
    try {
      setSuccessRedirectTo(overrideReturnTo || null)
      setRedeemAccount(address)
      const controller = new AbortController()
      const timeoutId = setTimeout(() => controller.abort(), 20000)
      let res: Response
      try {
        res = await fetch(BACKEND + "/redeem", {
          method: "POST",
          signal: controller.signal,
          body: JSON.stringify({
            code,
            address
          })
        });
      } finally {
        clearTimeout(timeoutId)
      }

      if (res.status != 200) {
        let text = await res.text()
        try {
          const data = JSON.parse(text)
          if (data?.reason === "w9_required" || data?.error === "w9_required") {
            setW9Url(data?.w9_url || null)
            setW9Email(data?.email || null)
            setError("W9 Required")
            return
          }
          if (data?.reason === "w9_pending" || data?.error === "w9_pending") {
            setW9Url(null)
            setW9Email(data?.email || null)
            setError("W9 Pending")
            return
          }
        } catch {
          // ignore json parse error
        }
        switch (text) {
        case "code not started":
          setError("Code not active yet.")
          break;
        case "code expired":
          setError("Code expired.")
          break;
        case "code redeemed":
          setError("Code already redeemed.")
          break;
        case "user redeemed":
          setError("User already redeemed for this event.")
          break;
        default:
          setError("Error redeeming code.")
        }
        return
      }

      setSuccess(true)
    } catch (err) {
      if (err instanceof Error && err.name === "AbortError") {
        setError("Redeem request timed out. Please try again.")
        return
      }
      setError("Internal server error.")
      return
    }

    //redirect back to app
  }

  const ensureWebWalletQueryParams = useCallback((fallbackReturnTo?: string) => {
    const params = new URLSearchParams(searchParams.toString())
    let shouldReplace = false

    if (params.get("webWallet") !== "1") {
      params.set("webWallet", "1")
      shouldReplace = true
    }
    if (params.get("source") !== "wallet") {
      params.set("source", "wallet")
      shouldReplace = true
    }

    const existingReturnTo = normalizeReturnTo(params.get("returnTo"))
    if (fallbackReturnTo) {
      const shouldSetReturnTo = !existingReturnTo || existingReturnTo === "/wallets"
      if (shouldSetReturnTo && existingReturnTo !== fallbackReturnTo) {
        params.set("returnTo", fallbackReturnTo)
        shouldReplace = true
      }
    }

    if (shouldReplace) {
      router.replace(`/faucet/redeem?${params.toString()}`)
    }
  }, [router, searchParams])

  const redeemWithWebWallet = useCallback(async () => {
    if (webWalletRedeemAttemptedRef.current || success) return
    if (!authenticated || !privyReady || !walletsReady) return
    if (!code) {
      setError("Invalid redeem code.")
      setWebWalletError("Invalid redeem code.")
      setContinueWithWebWalletRequested(false)
      return
    }

    webWalletRedeemAttemptedRef.current = true
    setContinuingWithWebWallet(true)
    setWebWalletError(null)
    setError(null)
    try {
      const primaryWallet = wallets[0]
      if (!primaryWallet) {
        throw new Error("No web wallet found. Connect a wallet and try again.")
      }

      await Promise.race([
        primaryWallet.switchChain(CHAIN_ID),
        new Promise<never>((_, reject) => {
          setTimeout(() => reject(new Error("Wallet network switch timed out. Please try again.")), 15000)
        }),
      ])

      const smartWallet = new AppWallet(primaryWallet, "SW-1", { index: 0n })
      await Promise.race([
        smartWallet.init(),
        new Promise<never>((_, reject) => {
          setTimeout(() => reject(new Error("Smart wallet initialization timed out. Please try again.")), 15000)
        }),
      ])

      if (!smartWallet.address) {
        throw new Error("Unable to resolve Smart Wallet 1 address.")
      }

      const smartWalletReturnTo = `/wallets/${smartWallet.address}`
      if (!returnTo || returnTo === "/wallets") {
        ensureWebWalletQueryParams(smartWalletReturnTo)
      }

      await sendBotRequest(
        smartWallet.address,
        !returnTo || returnTo === "/wallets" ? smartWalletReturnTo : undefined
      )
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unable to continue with web wallet."
      setError(message)
      setWebWalletError(message)
    } finally {
      setContinuingWithWebWallet(false)
      setContinueWithWebWalletRequested(false)
    }
  }, [authenticated, code, ensureWebWalletQueryParams, privyReady, returnTo, success, wallets, walletsReady])

  const continueWithWebWallet = useCallback(async () => {
    if (!code) {
      setWebWalletError("Invalid redeem code.")
      return
    }
    ensureWebWalletQueryParams("/wallets")
    setWebWalletError(null)
    setContinueWithWebWalletRequested(true)

    if (!privyReady) {
      setWebWalletError("Wallet login is still initializing. Please try again.")
      setContinueWithWebWalletRequested(false)
      return
    }

    if (!authenticated) {
      try {
        await login()
      } catch {
        setWebWalletError("Login was cancelled. Please try again.")
        setContinueWithWebWalletRequested(false)
        setError(missingSigAuthMessage)
      }
      return
    }
    void redeemWithWebWallet()
  }, [authenticated, code, ensureWebWalletQueryParams, login, privyReady, redeemWithWebWallet])

  useEffect(() => {
    directRedeemAttemptedRef.current = false
    webWalletRedeemAttemptedRef.current = false
    setContinueWithWebWalletRequested(false)
    setContinuingWithWebWallet(false)
    setSuccessRedirectTo(null)
  }, [code, sigAuthAccount, sigAuthSignature])

  useEffect(() => {
    if (!code) {
      setError("Invalid redeem code.")
      return
    }

    if (sigAuthAccount && sigAuthSignature) {
      if (directRedeemAttemptedRef.current) return
      directRedeemAttemptedRef.current = true
      void sendBotRequest(sigAuthAccount)
      return
    }

    ensureWebWalletQueryParams("/wallets")

    if (authenticated) {
      setError(null)
      return
    }

    setError(missingSigAuthMessage)
  }, [authenticated, code, ensureWebWalletQueryParams, sigAuthAccount, sigAuthSignature])

  useEffect(() => {
    if (!shouldUseWebWalletFlow) return
    if (!code) return
    if (hasSigAuth) return
    if (!authenticated || !privyReady || !walletsReady) return
    void redeemWithWebWallet()
  }, [
    authenticated,
    code,
    hasSigAuth,
    privyReady,
    shouldUseWebWalletFlow,
    walletsReady,
    redeemWithWebWallet,
  ])

  useEffect(() => {
    if (!continueWithWebWalletRequested) return
    if (!authenticated || !privyReady || !walletsReady) return
    void redeemWithWebWallet()
  }, [continueWithWebWalletRequested, authenticated, privyReady, walletsReady, redeemWithWebWallet])

  const showDownloadPrompt = error === missingSigAuthMessage && privyReady && !authenticated
  const isFinalErrorState = Boolean(
    error &&
    error !== missingSigAuthMessage &&
    error !== "W9 Required" &&
    error !== "W9 Pending"
  )
  const shouldAutoRedirect = success || isFinalErrorState

  useEffect(() => {
    if (!shouldAutoRedirect) return
    if (redirectTimeoutRef.current) return

    redirectTimeoutRef.current = setTimeout(() => {
      const destination = hasSigAuth && sigAuthRedirect
        ? `${sigAuthRedirect}/close`
        : (success ? (successRedirectTo || returnTo || "/wallets") : (returnTo || "/wallets"))
      router.replace(destination)
    }, 2000)
  }, [hasSigAuth, returnTo, router, shouldAutoRedirect, sigAuthRedirect, success, successRedirectTo])

  useEffect(() => {
    return () => {
      if (redirectTimeoutRef.current) {
        clearTimeout(redirectTimeoutRef.current)
      }
    }
  }, [])

  return (
    <div className="min-h-screen flex items-center justify-center">
      {
        success ?
        <div style={{textAlign: "center"}}>
          <h2 className="text-3xl font-bold text-black dark:text-white">
            Code redeemed!
          </h2>
        </div>
        : (!error || (error === missingSigAuthMessage && !showDownloadPrompt)) ?
        <div className="text-center space-y-6 justify-center items-center">
          <h2 className="text-3xl font-bold text-black dark:text-white">Redeeming...</h2>
          <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c] m-auto"></div>
        </div>
        : error ?
        <div className="mx-auto w-full max-w-lg px-4 text-center">
          <h2 className={`font-bold text-black dark:text-white ${error === missingSigAuthMessage ? "text-xl sm:text-2xl" : "text-3xl"}`}>
            {error}
          </h2>
          {error === "W9 Required" && (
            <div className="mt-4 space-y-3 text-sm text-muted-foreground">
              <p>
                You have reached the $600 annual SFLuv earnings threshold. We are required to collect a W9 before any
                additional payouts can be issued.
              </p>
              {w9Url && (
                <a
                  href={buildW9Url(w9Url, redeemAccount || sigAuthAccount || "", w9Email)}
                  className="inline-flex w-full items-center justify-center rounded-md bg-[#eb6c6c] px-16 py-10 text-2xl font-semibold text-white sm:w-auto sm:px-12 sm:py-8 sm:text-xl"
                  target="_blank"
                  rel="noreferrer"
                >
                  Submit W9
                </a>
              )}
            </div>
          )}
          {error === "W9 Pending" && (
            <div className="mt-4 space-y-3 text-sm text-muted-foreground">
              <p>
                Your W9 form is being processed. Once approved by an admin, scan this QR code again to receive your
                SFLuv.
              </p>
            </div>
          )}
          {showDownloadPrompt &&
            <div className="mx-auto mt-4 w-full max-w-md space-y-4 rounded-2xl border bg-card/95 p-4 shadow-sm">
              <p className="text-sm text-muted-foreground">
                Redeem with CitizenWallet or continue here using your web wallet.
              </p>

              <div className="grid grid-cols-2 gap-3">
                <a
                  href="https://apps.apple.com/us/app/citizen-wallet/id6460822891"
                  className="inline-flex items-center justify-center rounded-lg border bg-background/60 p-2 transition-colors hover:bg-background"
                >
                  <img
                    className="h-auto w-full max-w-[180px]"
                    src="/appstore.svg"
                    alt="Download on the App Store"
                    />
                </a>
                <a
                  href="https://play.google.com/store/apps/details?id=xyz.citizenwallet.wallet&hl=en&pli=1"
                  className="inline-flex items-center justify-center rounded-lg border bg-background/60 p-2 transition-colors hover:bg-background"
                >
                  <img
                    className="h-auto w-full max-w-[180px]"
                    src="/googleplaystore.svg"
                    alt="Get it on Google Play"
                    />
                </a>
              </div>

              <div className="flex items-center gap-3 text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
                <span className="h-px flex-1 bg-border" />
                <span>or</span>
                <span className="h-px flex-1 bg-border" />
              </div>

              <div className="space-y-2">
                <Button
                  onClick={continueWithWebWallet}
                  disabled={continuingWithWebWallet}
                  className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]"
                >
                  {continuingWithWebWallet ? "Continuing..." : "Continue with Web Wallet"}
                </Button>
                {webWalletError && (
                  <p className="text-xs text-red-600">{webWalletError}</p>
                )}
              </div>
            </div>
            }
        </div>
        :
        <div className="text-center space-y-6 justify-center items-center">
          <h2 className="text-3xl font-bold text-black dark:text-white">Redeeming...</h2>
          <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c] m-auto"></div>
        </div>
      }
    </div>
  )
}

export default Page;

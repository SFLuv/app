"use client"

import { useSearchParams } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { usePrivy } from "@privy-io/react-auth";
import { BACKEND } from "@/lib/constants";
import { normalizeRedeemCode } from "@/lib/redeem-link";
import { SFLUV_GOOGLE_PLAY_URL, SFLUV_IOS_APP_STORE_URL } from "@/lib/app-download-links";
import { useApp } from "@/context/AppProvider";
import { GetUserResponse, WalletResponse } from "@/types/server";

const normalizeReturnTo = (rawValue: string | null): string | null => {
  if (!rawValue) return null
  const trimmed = rawValue.trim()
  if (!trimmed.startsWith("/")) return null
  if (trimmed.startsWith("//")) return null
  return trimmed
}

const isHexAddress = (value: string) => /^0x[a-fA-F0-9]{40}$/.test(value)
const loginRedirectPendingKey = "faucet_redeem_login_redirect_pending"
const loginRedirectPendingTimeoutMs = 15000

const Page = () => {
  const missingPrimarySmartWalletMessage = "Your default wallet is still initializing. Please wait a few seconds and try again."
  const searchParams = useSearchParams();
  const router = useRouter();
  const { ensurePrimarySmartWallet, authFetch, status: appStatus, walletsStatus: appWalletsStatus, user, wallets: appWallets } = useApp()
  const { login, authenticated, ready: privyReady } = usePrivy()

  const [error, setError] = useState<string | null>();
  const [success, setSuccess] = useState<boolean>(false);
  const [w9Email, setW9Email] = useState<string | null>(null);
  const [redeemAccount, setRedeemAccount] = useState<string | null>(null)
  const [continueWithWebWalletRequested, setContinueWithWebWalletRequested] = useState<boolean>(false)
  const [continuingWithWebWallet, setContinuingWithWebWallet] = useState<boolean>(false)
  const [loginRedirectPending, setLoginRedirectPending] = useState<boolean>(() => {
    if (typeof window === "undefined") return false
    return window.sessionStorage.getItem(loginRedirectPendingKey) === "1"
  })
  const [webWalletError, setWebWalletError] = useState<string | null>(null)
  const [successRedirectTo, setSuccessRedirectTo] = useState<string | null>(null)
  const directRedeemAttemptedRef = useRef<boolean>(false)
  const webWalletRedeemAttemptedRef = useRef<boolean>(false)
  const webWalletRedeemInFlightRef = useRef<boolean>(false)
  const redirectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const sigAuthAccount = searchParams.get("sigAuthAccount")
  const sigAuthSignature = searchParams.get("sigAuthSignature")
  const sigAuthRedirect = searchParams.get("sigAuthRedirect")
  const hasSigAuth = Boolean(sigAuthAccount && sigAuthSignature)
  const shouldUseWebWalletFlow = !hasSigAuth
  const returnTo = normalizeReturnTo(searchParams.get("returnTo"))
  const code = normalizeRedeemCode(searchParams.get("code"))
  const isWebWalletSessionReady =
    authenticated &&
    privyReady &&
    appStatus === "authenticated" &&
    appWalletsStatus === "available" &&
    Boolean(user)
  const isFinalErrorState = Boolean(
    error &&
    error !== "Reward Held" &&
    error !== "Reward Not Sent" &&
    error !== "Merchant Account"
  )
  const shouldAutoRedirect = success || isFinalErrorState
  const markLoginRedirectPending = useCallback(() => {
    setLoginRedirectPending(true)
    if (typeof window === "undefined") return
    try {
      window.sessionStorage.setItem(loginRedirectPendingKey, "1")
    } catch {
      // ignore storage errors
    }
  }, [])
  const clearLoginRedirectPending = useCallback(() => {
    if (typeof window !== "undefined") {
      try {
        window.sessionStorage.removeItem(loginRedirectPendingKey)
      } catch {
        // ignore storage errors
      }
    }
    setLoginRedirectPending(false)
  }, [])
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

      // A held reward is a success with a condition, not a failure: the code
      // was consumed and the money is the volunteer's, waiting on a W-9.
      if (res.status === 202) {
        setError("Reward Held")
        return
      }

      // Refused rather than held: past the limit with a hold already open, so
      // nothing more can be sent until the form is in. The backend hands the
      // code back, which is the part that has to be said out loud — otherwise
      // this reads as the reward having been lost.
      if (res.status === 409) {
        // A 409 now has two quite different causes, and telling them apart
        // matters: a merchant sent down the W-9 path would fill in a tax form
        // and still not get paid, because their account is the problem, not
        // their paperwork.
        let reason = ""
        try {
          reason = ((await res.clone().json()) as { reason?: string })?.reason ?? ""
        } catch {
          // An older backend answers 409 without a body. The W-9 hold was the
          // only refusal then, so that is the safe reading.
        }
        setError(reason === "merchant_account" ? "Merchant Account" : "Reward Not Sent")
        return
      }

      if (res.status != 200) {
        let text = await res.text()
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

  const getStoredWalletAddress = (wallet: WalletResponse | undefined): string => {
    if (!wallet) return ""
    const rawAddress = wallet.is_eoa ? wallet.eoa_address : wallet.smart_address
    return typeof rawAddress === "string" ? rawAddress.trim() : ""
  }

  const getWalletRouteForAddress = useCallback((walletAddress: string, backendWallets?: WalletResponse[]) => {
    const normalizedAddress = walletAddress.trim().toLowerCase()
    if (!normalizedAddress) return "/wallets"

    const backendMatch = (backendWallets || []).some((wallet) => getStoredWalletAddress(wallet).toLowerCase() === normalizedAddress)
    const frontendMatch = appWallets.some((wallet) => wallet.address?.toLowerCase() === normalizedAddress)

    return backendMatch || frontendMatch ? `/wallets/${walletAddress}` : "/wallets"
  }, [appWallets])

  const loadAuthedUserRecord = useCallback(async (): Promise<GetUserResponse> => {
    const res = await authFetch("/users")
    if (!res.ok) {
      throw new Error("Unable to load your account details. Please try again.")
    }
    return await res.json() as GetUserResponse
  }, [authFetch])

  const resolveDefaultRedeemWallet = useCallback(async (): Promise<{ address: string; walletPath: string }> => {
    const normalizeWalletAddress = (value?: string | null) => {
      const trimmed = (value || "").trim()
      return isHexAddress(trimmed) ? trimmed.toLowerCase() : ""
    }

    const currentPrimaryAddress = normalizeWalletAddress(user?.primaryWalletAddress)
    if (currentPrimaryAddress) {
      return {
        address: currentPrimaryAddress,
        walletPath: getWalletRouteForAddress(currentPrimaryAddress),
      }
    }

    let userResponse = await loadAuthedUserRecord()
    let resolvedAddress = normalizeWalletAddress(userResponse.user.primary_wallet_address)
    if (resolvedAddress) {
      return {
        address: resolvedAddress,
        walletPath: getWalletRouteForAddress(resolvedAddress, userResponse.wallets),
      }
    }

    const hasPrimarySmartWallet = await ensurePrimarySmartWallet()
    if (!hasPrimarySmartWallet) {
      throw new Error(missingPrimarySmartWalletMessage)
    }

    userResponse = await loadAuthedUserRecord()
    resolvedAddress = normalizeWalletAddress(userResponse.user.primary_wallet_address)
    if (resolvedAddress) {
      return {
        address: resolvedAddress,
        walletPath: getWalletRouteForAddress(resolvedAddress, userResponse.wallets),
      }
    }

    const fallbackSmartWallet = userResponse.wallets.find((wallet) =>
      wallet.is_eoa === false &&
      wallet.smart_index === 0 &&
      typeof wallet.smart_address === "string" &&
      isHexAddress(wallet.smart_address.trim())
    )
    const fallbackAddress = normalizeWalletAddress(getStoredWalletAddress(fallbackSmartWallet))
    if (fallbackAddress) {
      return {
        address: fallbackAddress,
        walletPath: getWalletRouteForAddress(fallbackAddress, userResponse.wallets),
      }
    }

    throw new Error(missingPrimarySmartWalletMessage)
  }, [ensurePrimarySmartWallet, getWalletRouteForAddress, loadAuthedUserRecord, user?.primaryWalletAddress])

  const redeemWithWebWallet = useCallback(async () => {
    if (webWalletRedeemAttemptedRef.current || webWalletRedeemInFlightRef.current || success) return
    if (!isWebWalletSessionReady) return
    if (!code) {
      setError("Invalid redeem code.")
      setWebWalletError("Invalid redeem code.")
      setContinueWithWebWalletRequested(false)
      return
    }

    webWalletRedeemInFlightRef.current = true
    setContinuingWithWebWallet(true)
    setWebWalletError(null)
    setError(null)
    try {
      const { address: defaultWalletAddress, walletPath } = await resolveDefaultRedeemWallet()
      if (!returnTo || returnTo === "/wallets") {
        ensureWebWalletQueryParams(walletPath)
      }

      webWalletRedeemAttemptedRef.current = true
      await sendBotRequest(
        defaultWalletAddress,
        !returnTo || returnTo === "/wallets" ? walletPath : undefined
      )
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unable to continue with web wallet."
      if (message === missingPrimarySmartWalletMessage || message === "Unable to load your account details. Please try again.") {
        webWalletRedeemAttemptedRef.current = false
      }
      setError(message)
      setWebWalletError(message)
    } finally {
      webWalletRedeemInFlightRef.current = false
      setContinuingWithWebWallet(false)
      setContinueWithWebWalletRequested(false)
    }
  }, [code, ensureWebWalletQueryParams, isWebWalletSessionReady, missingPrimarySmartWalletMessage, resolveDefaultRedeemWallet, returnTo, success])

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
        markLoginRedirectPending()
        await login()
      } catch {
        clearLoginRedirectPending()
        setWebWalletError("Login was cancelled. Please try again.")
        setContinueWithWebWalletRequested(false)
        setError(null)
      }
      return
    }
    if (isWebWalletSessionReady) {
      void redeemWithWebWallet()
    }
  }, [authenticated, clearLoginRedirectPending, code, ensureWebWalletQueryParams, isWebWalletSessionReady, login, markLoginRedirectPending, privyReady, redeemWithWebWallet])

  useEffect(() => {
    directRedeemAttemptedRef.current = false
    webWalletRedeemAttemptedRef.current = false
    setContinueWithWebWalletRequested(false)
    setContinuingWithWebWallet(false)
    setSuccessRedirectTo(null)
    setWebWalletError(null)
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
      return
    }
  }, [
    authenticated,
    code,
    ensureWebWalletQueryParams,
    sigAuthAccount,
    sigAuthSignature,
  ])

  useEffect(() => {
    if (!shouldUseWebWalletFlow) return
    if (!code) return
    if (hasSigAuth) return
    if (!isWebWalletSessionReady) return
    void redeemWithWebWallet()
  }, [
    isWebWalletSessionReady,
    code,
    hasSigAuth,
    shouldUseWebWalletFlow,
    redeemWithWebWallet,
  ])

  useEffect(() => {
    if (!continueWithWebWalletRequested) return
    if (!isWebWalletSessionReady) return
    void redeemWithWebWallet()
  }, [continueWithWebWalletRequested, isWebWalletSessionReady, redeemWithWebWallet])

  useEffect(() => {
    if (!loginRedirectPending) return
    const timeoutId = setTimeout(() => {
      clearLoginRedirectPending()
    }, loginRedirectPendingTimeoutMs)

    return () => {
      clearTimeout(timeoutId)
    }
  }, [clearLoginRedirectPending, loginRedirectPending])

  useEffect(() => {
    if (!loginRedirectPending) return
    if (isWebWalletSessionReady || isFinalErrorState || success || !shouldUseWebWalletFlow) {
      clearLoginRedirectPending()
    }
  }, [clearLoginRedirectPending, isFinalErrorState, isWebWalletSessionReady, loginRedirectPending, shouldUseWebWalletFlow, success])

  const showAppDownloadPrompt =
    shouldUseWebWalletFlow &&
    Boolean(code) &&
    !authenticated &&
    !success &&
    error !== "Reward Held"

  useEffect(() => {
    if (!shouldAutoRedirect) {
      if (redirectTimeoutRef.current) {
        clearTimeout(redirectTimeoutRef.current)
        redirectTimeoutRef.current = null
      }
      return
    }

    if (redirectTimeoutRef.current) {
      clearTimeout(redirectTimeoutRef.current)
      redirectTimeoutRef.current = null
    }

    redirectTimeoutRef.current = setTimeout(() => {
      const destination = hasSigAuth && sigAuthRedirect
        ? `${sigAuthRedirect}/close`
        : (success ? (successRedirectTo || returnTo || "/wallets") : (returnTo || "/wallets"))
      redirectTimeoutRef.current = null
      router.replace(destination)
    }, 2000)

    return () => {
      if (redirectTimeoutRef.current) {
        clearTimeout(redirectTimeoutRef.current)
        redirectTimeoutRef.current = null
      }
    }
  }, [hasSigAuth, returnTo, router, shouldAutoRedirect, sigAuthRedirect, success, successRedirectTo])

  return (
    <div className="min-h-screen flex items-center justify-center">
      {
        success ?
        <div style={{textAlign: "center"}}>
          <h2 className="text-3xl font-bold text-black dark:text-white">
            Code redeemed!
          </h2>
        </div>
        : showAppDownloadPrompt ?
        <div className="mx-auto w-full max-w-md px-4 text-center">
          <div className="rounded-lg border bg-card/95 p-6 shadow-sm">
            <img
              src="/icon.png"
              alt="SFLUV"
              className="mx-auto h-16 w-16 object-contain"
            />
            <h2 className="mt-4 text-2xl font-bold text-black dark:text-white">
              Redeem in the SFLUV app
            </h2>
            <p className="mt-2 text-sm text-muted-foreground">
              Download the app, then open its scanner to redeem this QR code into your SFLUV wallet.
            </p>
            <div className="mt-5 grid gap-3 sm:grid-cols-2">
              <a
                href={SFLUV_IOS_APP_STORE_URL}
                target="_blank"
                rel="noreferrer"
                className="flex min-h-14 items-center gap-3 rounded-lg border border-border bg-white px-3 py-2 text-left shadow-sm transition hover:border-[#eb6c6c]/50 hover:bg-[#fff7f7] dark:bg-black"
              >
                <img
                  src="/appstore.svg"
                  alt=""
                  className="h-9 w-9 shrink-0 object-contain"
                />
                <span>
                  <span className="block text-xs text-muted-foreground">Download on</span>
                  <span className="block text-sm font-semibold text-foreground">App Store</span>
                </span>
              </a>
              <a
                href={SFLUV_GOOGLE_PLAY_URL}
                target="_blank"
                rel="noreferrer"
                className="flex min-h-14 items-center gap-3 rounded-lg border border-border bg-white px-3 py-2 text-left shadow-sm transition hover:border-[#eb6c6c]/50 hover:bg-[#fff7f7] dark:bg-black"
              >
                <img
                  src="/googleplaystore.svg"
                  alt=""
                  className="h-9 w-9 shrink-0 object-contain"
                />
                <span>
                  <span className="block text-xs text-muted-foreground">Get it on</span>
                  <span className="block text-sm font-semibold text-foreground">Google Play</span>
                </span>
              </a>
            </div>
            {webWalletError && (
              <p className="mt-4 text-sm text-[#b42318] dark:text-[#ffb4a8]">
                {webWalletError}
              </p>
            )}
            <Button
              onClick={continueWithWebWallet}
              disabled={!privyReady || continuingWithWebWallet || loginRedirectPending}
              variant="outline"
              className="mt-5 w-full"
            >
              {continuingWithWebWallet || loginRedirectPending ? "Continuing..." : "Continue with web app"}
            </Button>
          </div>
        </div>
        : !error ?
        <div className="text-center space-y-6 justify-center items-center">
          <h2 className="text-3xl font-bold text-black dark:text-white">Redeeming...</h2>
          <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c] m-auto"></div>
        </div>
        : error ?
        <div className="mx-auto w-full max-w-lg px-4 text-center">
          <h2 className="text-3xl font-bold text-black dark:text-white">
            {error}
          </h2>
          {error === "Reward Held" && (
            <div className="mt-4 space-y-3 text-sm text-muted-foreground">
              <p>
                Your reward has been saved — it is yours, and no one else can claim it. You have earned enough
                this year that we need a W-9 on file before we can send it.
              </p>
              <p>
                Open the SFLuv app to complete the form. Your rewards go out as soon as it is in.
              </p>
            </div>
          )}
          {error === "Merchant Account" && (
            <div className="mt-4 space-y-3 text-sm text-muted-foreground">
              <p>
                Volunteer rewards go to a personal SFLuv account, so that a shop&apos;s takings and its
                owner&apos;s rewards never end up in the same wallet.
              </p>
              <p>
                <span className="font-semibold">This QR code has not been used up.</span> Sign in with a
                personal account and scan it again.
              </p>
            </div>
          )}
          {error === "Reward Not Sent" && (
            <div className="mt-4 space-y-3 text-sm text-muted-foreground">
              <p>
                You have already earned more than the yearly reporting limit, so we cannot send anything else
                until a W-9 is on file.
              </p>
              <p>
                <span className="font-semibold">This QR code has not been used up.</span> Complete the form in
                the SFLuv app, then scan it again and the reward will go straight through.
              </p>
            </div>
          )}
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

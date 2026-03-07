"use client"

import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { usePrivy, useWallets } from "@privy-io/react-auth";
import { AppWallet } from "@/lib/wallets/wallets";
import { BACKEND, CHAIN_ID } from "@/lib/constants";
import { normalizeRedeemCode } from "@/lib/redeem-link";


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

  const sigAuthAccount = searchParams.get("sigAuthAccount")
  const sigAuthSignature = searchParams.get("sigAuthSignature")
  const sigAuthRedirect = searchParams.get("sigAuthRedirect")
  const sigAuthExpiry = searchParams.get("sigAuthExpiry")
  const code = normalizeRedeemCode(searchParams.get("code"))


  const closeModal = (delay: number) => {
    setTimeout(() => {
        if (sigAuthRedirect) {
          router.push(sigAuthRedirect + "/close")
        }
      }, delay)
  }

  useEffect(() => {
    if (!code) {
      setError("Invalid redeem code.")
      return
    }
    if (!sigAuthAccount || !sigAuthSignature) {
      setError(missingSigAuthMessage)
      return
    }
    sendBotRequest(sigAuthAccount)
  }, [sigAuthAccount, sigAuthSignature, code])

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

  const sendBotRequest = async (address: string) => {
    // let verified = verifyAccountOwnership()
    //implement real verification
    try {
      setRedeemAccount(address)
      let res = await fetch(BACKEND + "/redeem", {
        method: "POST",
        body: JSON.stringify({
          code,
          address
        })
      });

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

      setTimeout(() => {
        router.replace("/map?sidebar=false")
      }, 2000)
    } catch {
      setError("Internal server error.")
      closeModal(2000)
      return
    }

    //redirect back to app
  }

  const continueWithWebWallet = async () => {
    if (!code) {
      setWebWalletError("Invalid redeem code.")
      return
    }
    setWebWalletError(null)
    setContinueWithWebWalletRequested(true)

    if (!privyReady) {
      setWebWalletError("Wallet login is still initializing. Please try again.")
      setContinueWithWebWalletRequested(false)
      return
    }

    if (!authenticated) {
      await login()
      return
    }
  }

  useEffect(() => {
    const redeemWithWebWallet = async () => {
      if (!continueWithWebWalletRequested || !authenticated) return
      if (!walletsReady) return
      if (!code) {
        setWebWalletError("Invalid redeem code.")
        setContinueWithWebWalletRequested(false)
        return
      }

      setContinuingWithWebWallet(true)
      setError(null)
      setWebWalletError(null)
      try {
        const primaryWallet = wallets[0]
        if (!primaryWallet) {
          throw new Error("No web wallet found. Connect a wallet and try again.")
        }

        await primaryWallet.switchChain(CHAIN_ID)
        const smartWallet = new AppWallet(primaryWallet, "SW-1", { index: 0n })
        await smartWallet.init()
        if (!smartWallet.address) {
          throw new Error("Unable to resolve Smart Wallet 1 address.")
        }

        await sendBotRequest(smartWallet.address)
      } catch (err) {
        const message = err instanceof Error ? err.message : "Unable to continue with web wallet."
        setError(missingSigAuthMessage)
        setWebWalletError(message)
      } finally {
        setContinuingWithWebWallet(false)
        setContinueWithWebWalletRequested(false)
      }
    }

    redeemWithWebWallet()
  }, [continueWithWebWalletRequested, authenticated, walletsReady, code, wallets])

  return (
    <div className="min-h-screen flex items-center justify-center">
      {
        error ?
        <div className="text-center">
          <h2 className="text-3xl font-bold text-black dark:text-white">
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
          {error === missingSigAuthMessage &&
            <div className="space-y-4">
              <div className="columns-2 m-auto max-w-80">
                <a href="https://apps.apple.com/us/app/citizen-wallet/id6460822891">
                  <img
                    className="cursor-pointer max-w-36 m-auto"
                    src="/appstore.svg"
                    />
                </a>
                <a href="https://play.google.com/store/apps/details?id=xyz.citizenwallet.wallet&hl=en&pli=1">
                  <img
                    className="cursor-pointer max-w-36 m-auto"
                    src="/googleplaystore.svg"
                    />
                </a>
              </div>

              <div className="space-y-2">
                <Button
                  onClick={continueWithWebWallet}
                  disabled={continuingWithWebWallet}
                  className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
                >
                  {continuingWithWebWallet ? "Continuing..." : "Continue with Web Wallet"}
                </Button>
                <p className="text-xs text-muted-foreground">
                  This uses your Smart Wallet 1 account (index 0) to redeem.
                </p>
                {webWalletError && (
                  <p className="text-xs text-red-600">{webWalletError}</p>
                )}
              </div>
            </div>
            }
        </div>
        : success ?
        <div style={{textAlign: "center"}}>
          <h2 className="text-3xl font-bold text-black dark:text-white">
            Code redeemed!
          </h2>
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

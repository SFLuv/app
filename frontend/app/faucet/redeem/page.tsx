"use client"

import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { BACKEND } from "@/lib/constants";


const Page = () => {
  const searchParams = useSearchParams();
  const router = useRouter();

  const [error, setError] = useState<string | null>();
  const [success, setSuccess] = useState<boolean>(false);

  const sigAuthAccount = searchParams.get("sigAuthAccount")
  const sigAuthSignature = searchParams.get("sigAuthSignature")
  const sigAuthRedirect = searchParams.get("sigAuthRedirect")
  const sigAuthExpiry = searchParams.get("sigAuthExpiry")
  const code = searchParams.get("code")


  const closeModal = (delay: number) => {
    setTimeout(() => {
        if (sigAuthRedirect) {
          router.push(sigAuthRedirect + "/close")
        }
      }, delay)
  }

  useEffect(() => {
    if (!sigAuthAccount || !sigAuthSignature || !code) {
      setError("Please download the CitizenWallet app, then scan your QR code again.")
      return
    }
    sendBotRequest()
  }, [])

  const sendBotRequest = async () => {
    // let verified = verifyAccountOwnership()
    //implement real verification
    try {
      let res = await fetch(BACKEND + "/redeem", {
        method: "POST",
        body: JSON.stringify({
          code,
          address: sigAuthAccount
        })
      });

      if (res.status != 200) {
        let text = await res.text()
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

  return (
    <div className="min-h-screen flex items-center justify-center">
      {
        error ?
        <div className="text-center">
          <h2 className="text-3xl font-bold text-black dark:text-white">
            {error}
          </h2>
          {error === "Please download the CitizenWallet app, then scan your QR code again." &&
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

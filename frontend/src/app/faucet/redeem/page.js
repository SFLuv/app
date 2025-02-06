"use client"

import CircularProgress from '@mui/material/CircularProgress';
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { verifyAccountOwnership } from "@citizenwallet/sdk";

const closeModal = (sigAuthRedirect, delay) => {
  setTimeout(() => {
      if (sigAuthRedirect) {
        router.push(decodeUri(sigAuthRedirect) + "/close")
      }
    }, [delay])
}


const Page = () => {
  const searchParams = useSearchParams();
  const router = useRouter();

  const [error, setError] = useState();
  const [success, setSuccess] = useState();

  const sigAuthAccount = searchParams.get("sigAuthAccount")
  const sigAuthSignature = searchParams.get("sigAuthSignature")
  const sigAuthRedirect = searchParams.get("sigAuthRedirect")
  const sigAuthExpiry = searchParams.get("sigAuthExpiry")
  const code = searchParams.get("code")

  useEffect(() => {
    sendBotRequest()
  }, [])

  const sendBotRequest = async () => {
    if (!sigAuthAccount || !sigAuthSignature || !code) {
      setError("Invalid request.")
      closeModal(sigAuthRedirect, 2000)
      return
    }

    // let verified = verifyAccountOwnership()
    //implement real verification

    let res = await fetch(process.env.NEXT_PUBLIC_BACKEND_SERVER + "/redeem", {
      method: "POST",
      body: JSON.stringify({
        code,
        address: sigAuthAccount
      })
    });

    if (res.status != 200) {
      setError("Error redeeming code. Please close this page.")
      return
    }

    setSuccess(true)
    closeModal(sigAuthRedirect, 2000)


    //redirect back to app
  }




  return (
    <div style={{ display: "flex", height: "100vh", width: "100vw", color: "#eb6c6c" }}>
      <div style={{ display: "flex", margin: "auto" }}>
      {
          error ?
          <div style={{textAlign: "center"}}>
            <h2 style={{color: "black", size: "4vh"}}>
              {error}
            </h2>
          </div>
          : success ?
          <div style={{textAlign: "center"}}>
            <h2 style={{color: "black", size: "4vh"}}>
              Code redeemed!
            </h2>
          </div>
          :
          <div style={{textAlign: "center"}}>
            <h2 style={{color: "black", size: "4vh"}}>Redeeming...</h2>
            <CircularProgress color="inherit" size="8vh"/>
          </div>
        }
      </div>
    </div>
  )
}

export default Page;
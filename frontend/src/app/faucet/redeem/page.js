"use client"

import CircularProgress from '@mui/material/CircularProgress';
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { verifyAccountOwnership } from "@citizenwallet/sdk";


const Page = () => {
  const searchParams = useSearchParams();
  const router = useRouter();

  const [error, setError] = useState();
  const [success, setSuccess] = useState();

  const sigAuthAccount = searchParams.get("sigAuthAccount")
  const sigAuthSignature = searchParams.get("sigAuthSignature") || "t"
  const sigAuthRedirect = searchParams.get("sigAuthRedirect")
  const sigAuthExpiry = searchParams.get("sigAuthExpiry")
  const code = searchParams.get("code")

  useEffect(() => {
    console.log('sending')
    sendBotRequest()
  }, [])

  const sendBotRequest = async () => {
    if (!sigAuthAccount || !sigAuthSignature || !code) {
      console.log("missing param")
      setError("Invalid request. Please close this page.")
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
      console.log(res.status)
      setError("Error redeeming code. Please close this page.")
      setTimeout(() => {}, [2000])
      console.log("error redirect")
      return
    }

    setSuccess(true)
    console.log("success redirect")

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
              Code redeemed. You may now exit this page.
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
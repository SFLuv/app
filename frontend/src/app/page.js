"use client"

import Image from "next/image";
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { useSearchParams } from "next/navigation";

export default function Home() {
  const router = useRouter()

  const searchParams = useSearchParams()
  const sigAuthAccount = searchParams.get("sigAuthAccount")
  const sigAuthSignature = searchParams.get("sigAuthSignature")
  const sigAuthRedirect = searchParams.get("sigAuthRedirect")
  const sigAuthExpiry = searchParams.get("sigAuthExpiry")
  const code = searchParams.get("code")

  useEffect(() => {
    router.replace("/redeem"
      + "?address=" + sigAuthAccount
      + "&signature=" + sigAuthSignature
      + "&code=" + code
    )
  }, [])

  return (
    <div style={{ display: "flex", height: "100vh", width: "100vw" }}>
      <div style={{ display: "flex", margin: "auto" }}>
        Redirecting to redeem page...
      </div>
    </div>
  )
}
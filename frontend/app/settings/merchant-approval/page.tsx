"use client"

import { useEffect, useState } from "react"
import { useApp } from "@/context/AppProvider"
import { MerchantApprovalForm } from "@/components/merchant/merchant-approval-form"
import { GOOGLE_MAPS_API_KEY } from "@/lib/constants"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"

export default function MerchantApprovalPage() {
  const { status, login } = useApp()
  const [googleReady, setGoogleReady] = useState(false)
  const [googleLoadError, setGoogleLoadError] = useState<string | null>(null)

  const hasGoogleMaps = () => {
    return typeof window !== "undefined" && !!(window as any).google?.maps?.importLibrary
  }

  const waitForGoogle = async (timeoutMs = 15000): Promise<void> => {
    if (typeof window === "undefined") return

    const started = Date.now()
    while (Date.now() - started < timeoutMs) {
      if (hasGoogleMaps()) return
      await new Promise((resolve) => setTimeout(resolve, 50))
    }

    throw new Error("Google Maps script timed out")
  }

  const ensureGoogleScript = async () => {
    if (typeof window === "undefined") return
    if (hasGoogleMaps()) {
      setGoogleReady(true)
      return
    }

    const src = `https://maps.googleapis.com/maps/api/js?key=${GOOGLE_MAPS_API_KEY}&libraries=places`
    const existingScript = document.querySelector<HTMLScriptElement>(`script[src^="https://maps.googleapis.com/maps/api/js"]`)

    if (!existingScript) {
      const script = document.createElement("script")
      script.src = src
      script.async = true
      script.defer = true
      document.head.appendChild(script)
    }

    await waitForGoogle()
    setGoogleReady(true)
  }

  useEffect(() => {
    let mounted = true

    login()
    ensureGoogleScript().catch((error) => {
      if (!mounted) return
      console.error(error)
      setGoogleLoadError("Failed to load Google Places search. Please refresh and try again.")
    })

    return () => {
      mounted = false
    }
  }, [])

  if (status === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  if (status === "unauthenticated") {
    return (<Card className="border-[#eb6c6c]/40 bg-[#eb6c6c]/5">
        <CardContent className="p-6 space-y-4 text-center">
          <p className="text-sm text-[#8f2e2e]">
            You must have an <span className="font-semibold">SFLUV account</span>{" "}
            in order to submit a merchant approval form.
          </p>

          <p className="text-sm text-[#8f2e2e]">
            Click the button below to create an account or log in if you already
            have one.
          </p>

          <Button
            variant="default"
            size="lg"
            className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
            onClick={login}
          >
            Create Account / Log In
          </Button>
        </CardContent>
      </Card>)
  }

  if (googleLoadError) {
    return (
      <Card className="border-[#eb6c6c]/40 bg-[#eb6c6c]/5">
        <CardContent className="p-6 text-sm text-[#8f2e2e]">
          {googleLoadError}
        </CardContent>
      </Card>
    )
  }

  if (!googleReady) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <div className="max-w-3xl mx-auto">
      <h1 className="text-3xl font-bold text-black dark:text-white mb-6">Become a Merchant</h1>
      <p className="text-gray-600 dark:text-gray-400 mb-6">
        Complete the form below to apply for merchant status. Once approved, you'll be able to accept SFLuv as payment
        and access merchant-specific features.
      </p>

      <MerchantApprovalForm />
    </div>
  )
}

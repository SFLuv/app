"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { MerchantApprovalForm } from "@/components/merchant/merchant-approval-form"
import { GOOGLE_MAPS_API_KEY } from "@/lib/constants"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"

export default function MerchantApprovalPage() {
  const { status, login } = useApp()
  const router = useRouter()

  const addGoogleScript = async () => {
    const existingScript = document.querySelector<HTMLScriptElement>(
        `script[src^="https://maps.googleapis.com/maps/api/js"]`);
    if (!existingScript) {
    const script = document.createElement("script");
    script.src = `https://maps.googleapis.com/maps/api/js?key=${GOOGLE_MAPS_API_KEY}&libraries=places`;
    script.async = true;
    script.defer = true;
    document.head.appendChild(script);
    }
  }

  useEffect(() => {
    login()
    router.replace("/settings/merchant-approval")
    addGoogleScript()
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

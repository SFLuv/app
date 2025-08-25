"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { MerchantApprovalForm } from "@/components/merchant/merchant-approval-form"
import { GOOGLE_MAPS_API_KEY } from "@/lib/constants"

export default function MerchantApprovalPage() {
  const { status } = useApp()

  const addGoogleScript = async () => {
          const existingScript = document.querySelector<HTMLScriptElement>(
              `script[src^="https://maps.googleapis.com/maps/api/js"]`);
          if (!existingScript) {
          const script = document.createElement("script");
          script.src = `https://maps.googleapis.com/maps/api/js?key=${GOOGLE_MAPS_API_KEY}&libraries=places`;
          script.async = true;
          script.defer = true;
          document.head.appendChild(script);
          console.log("Script appended")
          }
      }

  useEffect(() => {
    addGoogleScript()
  }, [])

  if (status === "loading") {
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

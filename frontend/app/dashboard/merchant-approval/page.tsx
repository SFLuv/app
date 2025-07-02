"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { MerchantApprovalForm } from "@/components/merchant/merchant-approval-form"

export default function MerchantApprovalPage() {
  const { user } = useApp()
  const router = useRouter()

  // Redirect if user already has a merchant status
  useEffect(() => {
    if (user?.merchantStatus) {
      router.push("/dashboard/merchant-status")
    }
  }, [user, router])

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

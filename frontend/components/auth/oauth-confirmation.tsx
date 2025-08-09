"use client"

import { useState, useEffect } from "react"
import { Button } from "@/components/ui/button"
import { ArrowLeft, CheckCircle, Loader2 } from "lucide-react"

interface OAuthConfirmationProps {
  provider: "google" | "github"
  onBack: () => void
  onSuccess: () => void
}

export function OAuthConfirmation({ provider, onBack, onSuccess }: OAuthConfirmationProps) {
  const [status, setStatus] = useState<"loading" | "success">("loading")

  useEffect(() => {
    // Simulate OAuth authentication process
    const timer = setTimeout(() => {
      setStatus("success")
      // Auto-proceed after showing success for a moment
      const successTimer = setTimeout(() => {
        onSuccess()
      }, 1500)

      return () => clearTimeout(successTimer)
    }, 2000)

    return () => clearTimeout(timer)
  }, [onSuccess])

  const providerName = provider === "google" ? "Google" : "GitHub"

  return (
    <div className="flex flex-col items-center justify-center py-4">
      {status === "loading" ? (
        <>
          <Loader2 className="h-16 w-16 text-[#eb6c6c] animate-spin mb-6" />
          <h3 className="text-xl font-medium text-black dark:text-white mb-2">Connecting to {providerName}</h3>
          <p className="text-gray-600 dark:text-gray-300 text-center mb-6">
            Please wait while we authenticate your account...
          </p>
          <Button variant="ghost" className="hover:bg-[#eb6c6c] hover:text-white" onClick={onBack}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Cancel
          </Button>
        </>
      ) : (
        <>
          <CheckCircle className="h-16 w-16 text-green-500 mb-6" />
          <h3 className="text-xl font-medium text-black dark:text-white mb-2">Successfully connected</h3>
          <p className="text-gray-600 dark:text-gray-300 text-center">Your {providerName} account has been verified.</p>
        </>
      )}
    </div>
  )
}

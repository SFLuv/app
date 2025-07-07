"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { AuthLayout } from "@/components/auth/auth-layout"
import { AuthOptions } from "@/components/auth/auth-options"
import { LoginForm } from "@/components/auth/login-form"
import { EmailForm } from "@/components/auth/email-form"
import { OAuthConfirmation } from "@/components/auth/oauth-confirmation"

// Import the mock auth hook
import { useMockAuth } from "@/components/auth/mock-auth"

type AuthStep = "options" | "auth"
type AuthMethod = "email" | "google" | "github"

export default function LoginPage() {
  const router = useRouter()
  const [step, setStep] = useState<AuthStep>("options")
  const [method, setMethod] = useState<AuthMethod | null>(null)
  const [useEmailOtp, setUseEmailOtp] = useState(false)

  const handleSelectMethod = (selectedMethod: AuthMethod) => {
    setMethod(selectedMethod)
    setStep("auth")
  }

  // Update the handleLoginSubmit function
  const { mockLogin } = useMockAuth()

  const handleLoginSubmit = (email: string, password: string) => {
    console.log(`Logged in with email: ${email} and password`)
    mockLogin(email, password)
  }

  const handleEmailSubmit = (email: string, password: string, otp?: string) => {
    console.log(`Logged in with email: ${email}, Password: [HIDDEN], OTP: ${otp}`)
    mockLogin(email, password)
  }

  const handleAuthSuccess = () => {
    console.log(`Logged in with ${method}`)
    mockLogin(`${method}@example.com`, "password123")
  }

  const getTitle = () => {
    if (step === "options") return "Login to your account"
    if (method === "email") {
      return useEmailOtp ? "Login with email" : "Enter your credentials"
    }
    return `Login with ${method === "google" ? "Google" : "GitHub"}`
  }

  const getDescription = () => {
    if (step === "options") return "Welcome back to SFLuv"
    if (method === "email" && useEmailOtp) return "We'll send you a verification code"
    return ""
  }

  return (
    <AuthLayout title={getTitle()} description={getDescription()}>
      {step === "options" && <AuthOptions onSelectMethod={handleSelectMethod} isLogin />}

      {step === "auth" && method === "email" && !useEmailOtp && (
        <LoginForm onBack={() => setStep("options")} onSubmit={handleLoginSubmit} />
      )}

      {step === "auth" && method === "email" && useEmailOtp && (
        <EmailForm isLogin onBack={() => setStep("options")} onSubmit={handleEmailSubmit} />
      )}

      {step === "auth" && (method === "google" || method === "github") && (
        <OAuthConfirmation provider={method} onBack={() => setStep("options")} onSuccess={handleAuthSuccess} />
      )}
    </AuthLayout>
  )
}

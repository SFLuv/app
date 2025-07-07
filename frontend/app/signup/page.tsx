"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { AuthLayout } from "@/components/auth/auth-layout"
import { AuthOptions } from "@/components/auth/auth-options"
import { RoleSelection } from "@/components/auth/role-selection"
import { EmailForm } from "@/components/auth/email-form"
import { OAuthConfirmation } from "@/components/auth/oauth-confirmation"

// Import the mock auth hook
import { useMockAuth } from "@/components/auth/mock-auth"

type AuthStep = "options" | "auth" | "role"
type AuthMethod = "email" | "google" | "github"
type UserRole = "user" | "merchant" | "admin" | null

export default function SignupPage() {
  const router = useRouter()
  const [step, setStep] = useState<AuthStep>("options")
  const [method, setMethod] = useState<AuthMethod | null>(null)
  const [role, setRole] = useState<UserRole>(null)
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [userData, setUserData] = useState({
    email: "",
    password: "",
  })

  const handleSelectMethod = (selectedMethod: AuthMethod) => {
    setMethod(selectedMethod)
    setStep("auth")
  }

  const handleAuthSuccess = () => {
    setIsAuthenticated(true)
    setStep("role")
  }

  // Update the handleSelectRole function
  const { mockLogin } = useMockAuth()

  const handleSelectRole = (selectedRole: "user" | "merchant" | "admin") => {
    setRole(selectedRole)
    console.log(`Signing up with ${method} as a ${selectedRole}`)
    console.log(`User data:`, userData)

    // For merchant role, we'll set them as a user first, then redirect to merchant approval
    const initialRole = selectedRole === "merchant" ? "user" : selectedRole

    mockLogin(userData.email || `${method}@example.com`, userData.password || "password123", initialRole)

    // Redirect to merchant approval if merchant role was selected
    if (selectedRole === "merchant") {
      router.push("/dashboard/merchant-approval")
    } else {
      router.push("/dashboard")
    }
  }

  const handleEmailSubmit = (email: string, password: string, otp?: string) => {
    console.log(`Verified email: ${email}, Password: [HIDDEN], OTP: ${otp}`)
    setUserData({
      email,
      password,
    })
    handleAuthSuccess()
  }

  const getTitle = () => {
    if (step === "options") return "Create your account"
    if (step === "auth") {
      if (method === "email") return "Verify your email"
      return `Connect with ${method === "google" ? "Google" : "GitHub"}`
    }
    if (step === "role") return "Choose your role"
    return "Sign up"
  }

  const getDescription = () => {
    if (step === "options") return "Sign up to start using SFLuv"
    if (step === "auth") {
      if (method === "email") return "Create your account and verify your email"
      return `Authenticate with your ${method === "google" ? "Google" : "GitHub"} account`
    }
    if (step === "role") return "Select how you want to use SFLuv"
    return ""
  }

  return (
    <AuthLayout title={getTitle()} description={getDescription()}>
      {step === "options" && <AuthOptions onSelectMethod={handleSelectMethod} />}

      {step === "auth" && method === "email" && (
        <EmailForm onBack={() => setStep("options")} onSubmit={handleEmailSubmit} />
      )}

      {step === "auth" && (method === "google" || method === "github") && (
        <OAuthConfirmation provider={method} onBack={() => setStep("options")} onSuccess={handleAuthSuccess} />
      )}

      {step === "role" && isAuthenticated && (
        <RoleSelection onSelectRole={handleSelectRole} onBack={() => setStep("auth")} />
      )}
    </AuthLayout>
  )
}

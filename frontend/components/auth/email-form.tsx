"use client"

import type React from "react"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ArrowLeft, CheckCircle, Loader2, Eye, EyeOff } from "lucide-react"

interface EmailFormProps {
  isLogin?: boolean
  onBack: () => void
  onSubmit: (email: string, password: string, otp?: string) => void
}

export function EmailForm({ isLogin = false, onBack, onSubmit }: EmailFormProps) {
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [otp, setOtp] = useState("")
  const [step, setStep] = useState<"email" | "otp" | "success">("email")
  const [isLoading, setIsLoading] = useState(false)
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmPassword, setShowConfirmPassword] = useState(false)
  const [passwordError, setPasswordError] = useState("")

  const validatePassword = () => {
    if (password.length < 8) {
      setPasswordError("Password must be at least 8 characters long")
      return false
    }

    if (password !== confirmPassword) {
      setPasswordError("Passwords do not match")
      return false
    }

    setPasswordError("")
    return true
  }

  const handleSendOtp = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!isLogin && !validatePassword()) {
      return
    }

    setIsLoading(true)

    // Simulate OTP sending
    setTimeout(() => {
      setIsLoading(false)
      setStep("otp")
    }, 1500)
  }

  const handleVerifyOtp = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsLoading(true)

    // Simulate OTP verification
    setTimeout(() => {
      setIsLoading(false)
      setStep("success")

      // Auto-proceed after showing success for a moment
      setTimeout(() => {
        onSubmit(email, password, otp)
      }, 1500)
    }, 1500)
  }

  return (
    <div>
      {step === "email" ? (
        <form onSubmit={handleSendOtp} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="email" className="text-black dark:text-white">
              Email address
            </Label>
            <Input
              id="email"
              type="email"
              placeholder="Enter your email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              className="text-black dark:text-white placeholder:text-gray-500 dark:placeholder:text-gray-400"
            />
          </div>

          {!isLogin && (
            <>
              <div className="space-y-2">
                <Label htmlFor="password" className="text-black dark:text-white">
                  Password
                </Label>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? "text" : "password"}
                    placeholder="Create a password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                    className="text-black dark:text-white placeholder:text-gray-500 dark:placeholder:text-gray-400"
                  />
                  <button
                    type="button"
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500"
                    onClick={() => setShowPassword(!showPassword)}
                  >
                    {showPassword ? <EyeOff size={18} /> : <Eye size={18} />}
                  </button>
                </div>
                <p className="text-xs text-gray-500 dark:text-gray-400">Password must be at least 8 characters long</p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="confirmPassword" className="text-black dark:text-white">
                  Confirm Password
                </Label>
                <div className="relative">
                  <Input
                    id="confirmPassword"
                    type={showConfirmPassword ? "text" : "password"}
                    placeholder="Confirm your password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    required
                    className="text-black dark:text-white placeholder:text-gray-500 dark:placeholder:text-gray-400"
                  />
                  <button
                    type="button"
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500"
                    onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                  >
                    {showConfirmPassword ? <EyeOff size={18} /> : <Eye size={18} />}
                  </button>
                </div>
              </div>

              {passwordError && <p className="text-sm text-red-500 dark:text-red-400">{passwordError}</p>}
            </>
          )}

          <Button type="submit" className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]" disabled={isLoading}>
            {isLoading ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Sending code...
              </>
            ) : (
              "Send verification code"
            )}
          </Button>

          <Button type="button" variant="ghost" className="w-full hover:bg-[#eb6c6c] hover:text-white" onClick={onBack}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back
          </Button>
        </form>
      ) : step === "otp" ? (
        <form onSubmit={handleVerifyOtp} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="otp" className="text-black dark:text-white">
              Verification code
            </Label>
            <Input
              id="otp"
              type="text"
              placeholder="Enter the 6-digit code"
              value={otp}
              onChange={(e) => setOtp(e.target.value)}
              required
              maxLength={6}
              className="text-black dark:text-white placeholder:text-gray-500 dark:placeholder:text-gray-400"
            />
            <p className="text-sm text-gray-800 dark:text-gray-200">We sent a code to {email}</p>
          </div>

          <Button type="submit" className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c]" disabled={isLoading}>
            {isLoading ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Verifying...
              </>
            ) : (
              "Verify code"
            )}
          </Button>

          <Button
            type="button"
            variant="ghost"
            className="w-full hover:bg-[#eb6c6c] hover:text-white"
            onClick={() => setStep("email")}
          >
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back
          </Button>
        </form>
      ) : (
        <div className="flex flex-col items-center justify-center py-4">
          <CheckCircle className="h-16 w-16 text-green-500 mb-6" />
          <h3 className="text-xl font-medium text-black dark:text-white mb-2">Email verified</h3>
          <p className="text-gray-600 dark:text-gray-300 text-center">Your email has been successfully verified.</p>
        </div>
      )}
    </div>
  )
}

"use client"

import { useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardContent } from "@/components/ui/card"
import { ArrowLeft, ArrowRight, Wallet, Key, Check, AlertTriangle } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import { useImportWallet, usePrivy } from "@privy-io/react-auth"

interface ConnectWalletModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface StepIndicatorProps {
  currentStep: number
  totalSteps: number
}

function StepIndicator({ currentStep, totalSteps }: StepIndicatorProps) {
  return (
    <div className="flex items-center justify-center space-x-1 sm:space-x-2 mb-4 sm:mb-6">
      {Array.from({ length: totalSteps }, (_, index) => {
        const stepNumber = index + 1
        const isActive = stepNumber === currentStep
        const isCompleted = stepNumber < currentStep

        return (
          <div key={stepNumber} className="flex items-center">
            <div
              className={`
                flex items-center justify-center w-6 h-6 sm:w-8 sm:h-8 rounded-full text-xs sm:text-sm font-medium
                ${
                  isCompleted
                    ? "bg-green-500 text-white"
                    : isActive
                      ? "bg-[#eb6c6c] text-white"
                      : "bg-gray-200 text-gray-600 dark:bg-gray-700 dark:text-gray-400"
                }
              `}
            >
              {isCompleted ? <Check className="w-3 h-3 sm:w-4 sm:h-4" /> : stepNumber}
            </div>
            {stepNumber < totalSteps && (
              <div
                className={`
                  w-4 sm:w-8 h-0.5 mx-1 sm:mx-2
                  ${stepNumber < currentStep ? "bg-green-500" : "bg-gray-200 dark:bg-gray-700"}
                `}
              />
            )}
          </div>
        )
      })}
    </div>
  )
}

export function ConnectWalletModal({ open, onOpenChange }: ConnectWalletModalProps) {
  const [currentStep, setCurrentStep] = useState(1)
  const [walletName, setWalletName] = useState("")
  const [privateKey, setPrivateKey] = useState("")
  const [isLoading, setIsLoading] = useState(false)
  const [errors, setErrors] = useState<{ name?: string; privateKey?: string }>({})
  const { toast } = useToast()
  const { importWallet } = useImportWallet()

  const totalSteps = 2

  const validateStep1 = () => {
    const newErrors: { name?: string } = {}

    if (!walletName.trim()) {
      newErrors.name = "Wallet name is required"
    } else if (walletName.trim().length < 2) {
      newErrors.name = "Wallet name must be at least 2 characters"
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const validateStep2 = () => {
    const newErrors: { privateKey?: string } = {}

    if (!privateKey.trim()) {
      newErrors.privateKey = "Private key is required"
    } else if (!privateKey.startsWith("0x") && privateKey.length !== 64 && privateKey.length !== 66) {
      newErrors.privateKey = "Please enter a valid private key"
    }

    setErrors(newErrors)
    return Object.keys(newErrors).length === 0
  }

  const handleNext = () => {
    if (currentStep === 1 && validateStep1()) {
      setCurrentStep(2)
    }
  }

  const handleBack = () => {
    if (currentStep > 1) {
      setCurrentStep(currentStep - 1)
      setErrors({})
    }
  }

  const handleImportWallet = async () => {
    if (!validateStep2()) return

    setIsLoading(true)
    try {
      // Use Privy's importWallet function
      await importWallet({
        privateKey: privateKey.startsWith("0x") ? privateKey : `0x${privateKey}`,
      })

      toast({
        title: "Wallet Connected",
        description: `${walletName} has been successfully connected and imported.`,
      })

      // Reset form and close modal
      resetForm()
      onOpenChange(false)
    } catch (error) {
      console.error("Failed to import wallet:", error)
      toast({
        title: "Import Failed",
        description: "Failed to import wallet. Please check your private key and try again.",
        variant: "destructive",
      })
    } finally {
      setIsLoading(false)
    }
  }

  const resetForm = () => {
    setCurrentStep(1)
    setWalletName("")
    setPrivateKey("")
    setErrors({})
    setIsLoading(false)
  }

  const handleModalClose = (open: boolean) => {
    if (!open) {
      resetForm()
    }
    onOpenChange(open)
  }

  return (
    <Dialog open={open} onOpenChange={handleModalClose}>
      <DialogContent className="w-[95vw] max-w-md mx-auto max-h-[90vh] overflow-y-auto rounded-lg">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="flex items-center gap-2 text-lg sm:text-xl">
            <Wallet className="h-4 w-4 sm:h-5 sm:w-5" />
            Connect Wallet
          </DialogTitle>
          <DialogDescription className="text-sm">
            Import your existing wallet by providing a name and private key
          </DialogDescription>
        </DialogHeader>

        <StepIndicator currentStep={currentStep} totalSteps={totalSteps} />

        <div className="space-y-4 sm:space-y-6">
          {currentStep === 1 && (
            <div className="space-y-4">
              <Card className="border-blue-200 dark:border-blue-800">
                <CardContent className="p-3 sm:p-4">
                  <div className="flex items-start gap-2 sm:gap-3">
                    <div className="h-6 w-6 sm:h-8 sm:w-8 rounded-full bg-blue-100 dark:bg-blue-900/20 flex items-center justify-center flex-shrink-0">
                      <Wallet className="h-3 w-3 sm:h-4 sm:w-4 text-blue-600 dark:text-blue-400" />
                    </div>
                    <div className="space-y-1">
                      <p className="font-medium text-sm">Wallet Name</p>
                      <p className="text-xs text-muted-foreground">
                        Choose a memorable name for your wallet to easily identify it later.
                      </p>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <div className="space-y-2">
                <Label htmlFor="wallet-name" className="text-sm font-medium">
                  Wallet Name
                </Label>
                <Input
                  id="wallet-name"
                  placeholder="My Main Wallet"
                  value={walletName}
                  onChange={(e) => {
                    setWalletName(e.target.value)
                    if (errors.name) {
                      setErrors({ ...errors, name: undefined })
                    }
                  }}
                  className={`h-11 ${errors.name ? "border-red-500" : ""}`}
                />
                {errors.name && <p className="text-sm text-red-500">{errors.name}</p>}
              </div>
            </div>
          )}

          {currentStep === 2 && (
            <div className="space-y-4">
              <Card className="border-amber-200 dark:border-amber-800">
                <CardContent className="p-3 sm:p-4">
                  <div className="flex items-start gap-2 sm:gap-3">
                    <div className="h-6 w-6 sm:h-8 sm:w-8 rounded-full bg-amber-100 dark:bg-amber-900/20 flex items-center justify-center flex-shrink-0">
                      <AlertTriangle className="h-3 w-3 sm:h-4 sm:w-4 text-amber-600 dark:text-amber-400" />
                    </div>
                    <div className="space-y-1">
                      <p className="font-medium text-sm">Security Warning</p>
                      <p className="text-xs text-muted-foreground">
                        Never share your private key with anyone. We encrypt and securely store this information.
                      </p>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <div className="space-y-2">
                <Label htmlFor="private-key" className="flex items-center gap-2 text-sm font-medium">
                  <Key className="h-3 w-3 sm:h-4 sm:w-4" />
                  Private Key
                </Label>
                <Input
                  id="private-key"
                  type="password"
                  placeholder="0x... or 64-character hex string"
                  value={privateKey}
                  onChange={(e) => {
                    setPrivateKey(e.target.value)
                    if (errors.privateKey) {
                      setErrors({ ...errors, privateKey: undefined })
                    }
                  }}
                  className={`h-11 font-mono text-sm ${errors.privateKey ? "border-red-500" : ""}`}
                />
                {errors.privateKey && <p className="text-sm text-red-500">{errors.privateKey}</p>}
                <p className="text-xs text-muted-foreground">
                  Enter your wallet's private key (with or without 0x prefix)
                </p>
              </div>

              <div className="bg-gray-50 dark:bg-gray-900/50 p-3 rounded-lg">
                <p className="text-sm font-medium mb-1">Wallet Summary:</p>
                <p className="text-sm text-muted-foreground">Name: {walletName}</p>
              </div>
            </div>
          )}
        </div>

        <div className="flex flex-col sm:flex-row gap-3 sm:justify-between pt-4 border-t">
          <Button
            variant="outline"
            onClick={handleBack}
            disabled={currentStep === 1 || isLoading}
            className="flex items-center justify-center gap-2 bg-transparent h-11 order-2 sm:order-1"
          >
            <ArrowLeft className="h-4 w-4" />
            Back
          </Button>

          {currentStep < totalSteps ? (
            <Button
              onClick={handleNext}
              disabled={isLoading}
              className="flex items-center justify-center gap-2 h-11 order-1 sm:order-2"
            >
              Next
              <ArrowRight className="h-4 w-4" />
            </Button>
          ) : (
            <Button
              onClick={handleImportWallet}
              disabled={isLoading}
              className="flex items-center justify-center gap-2 h-11 order-1 sm:order-2"
            >
              {isLoading ? (
                <>
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
                  Importing...
                </>
              ) : (
                <>
                  <Wallet className="h-4 w-4" />
                  Connect Wallet
                </>
              )}
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

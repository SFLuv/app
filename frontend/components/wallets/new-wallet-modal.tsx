"use client"

import { useEffect, useMemo, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardContent } from "@/components/ui/card"
import { ArrowLeft, ArrowRight, Wallet, Key, Check, AlertTriangle } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import { useImportWallet, usePrivy } from "@privy-io/react-auth"
import { parseKeyFromCWLink } from "@/lib/wallets/parse"
import { useApp } from "@/context/AppProvider"

interface AddWalletModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  addWalletFunction: (walletName: string) => Promise<void>
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

export function NewWalletModal({ open, onOpenChange, addWalletFunction: addWallet }: AddWalletModalProps) {
  const [currentStep, setCurrentStep] = useState(1)
  const [walletName, setWalletName] = useState("")
  const [isLoading, setIsLoading] = useState(false)
  const [errors, setErrors] = useState<{ name?: string }>({})
  const { toast } = useToast()


  const totalSteps = 1

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


  const handleAddWallet = async () => {
    if (!validateStep1()) return

    setIsLoading(true)
    try {
      await addWallet(walletName)

      toast({
        title: "Wallet Added",
        description: `${walletName} has been successfully added.`,
      })

      // Reset form and close modal
      resetForm()
      onOpenChange(false)
    } catch (error) {
      console.error("Failed to import wallet:", error)
      toast({
        title: "Import Failed",
        description: "Failed to add wallet.",
        variant: "destructive",
      })
    } finally {
      setIsLoading(false)
    }
  }

  const resetForm = () => {
    setCurrentStep(1)
    setWalletName("")
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
        </div>

        <div className="flex flex-col sm:flex-row gap-3 sm:justify-between pt-4 border-t">
          <Button
            onClick={handleAddWallet}
            disabled={isLoading}
            className="flex items-center justify-center gap-2 h-11 order-1 sm:order-2"
          >
            {isLoading ? (
              <>
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
                Adding...
              </>
            ) : (
              <>
                <Wallet className="h-4 w-4" />
                Add Wallet
              </>
            )}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

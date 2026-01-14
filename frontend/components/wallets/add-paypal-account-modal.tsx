"use client"

import { useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

interface UpdatePayPalAccountModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  updatePayPalAddressFunction: (address: string) => Promise<void>
}

export function UpdatePayPalAccountModal({
  open,
  onOpenChange,
  updatePayPalAddressFunction: updatePayPalAddress,
}: UpdatePayPalAccountModalProps) {
  const [walletAddress, setWalletAddress] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState(false)
  const [isLoading, setIsLoading] = useState(false)

  const isValidAddress = (address: string) => {
    return address.startsWith("0x") && address.length === 42
  }

  const resetModal = () => {
    setWalletAddress("")
    setError(null)
    setSuccessMessage(false)
    setIsLoading(false)
  }

  const handleSubmit = async () => {
    if (!isValidAddress(walletAddress)) {
      setError(
        "Please enter a valid wallet address (must start with 0x and be 42 characters long)."
      )
      return
    }

    setError(null)
    setIsLoading(true)

    try {
      await updatePayPalAddress(walletAddress)
      setSuccessMessage(true)
      setWalletAddress("")

      setTimeout(() => {
        setSuccessMessage(false)
        onOpenChange(false)
      }, 3000)
    } catch (err) {
      setError("Failed to save PayPal address. Please try again.")
      console.error(err)
    } finally {
      setIsLoading(false)
    }
  }

  const handleOpenChange = (open: boolean) => {
    if (!open) resetModal()
    onOpenChange(open)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="w-[95vw] max-w-lg rounded-lg p-6 sm:p-8">
        <DialogHeader className="mb-4">
          <DialogTitle className="text-xl sm:text-2xl font-semibold mb-2">
            Add PayPal Account
          </DialogTitle>
          <DialogDescription className="text-sm sm:text-base text-gray-700 dark:text-gray-300">
            SFLUV currently supports cashing out your SFLUV balance via PayPal.
            Please enter your PayPal wallet address below. Once you have entered a
            PayPal wallet address, the "Cash Out SFLUV" feature on your connected
            wallets will route money to the PayPal account associated with this
            address.
          </DialogDescription>
        </DialogHeader>

        {successMessage ? (
          <div className="py-8 text-center text-green-600 font-medium text-lg sm:text-xl">
            Your PayPal wallet address has been recorded.
          </div>
        ) : (
          <div className="space-y-6">
            <div className="space-y-3">
              <Label
                htmlFor="paypal-wallet"
                className="text-base sm:text-lg font-medium"
              >
                PayPal Wallet Address
              </Label>
              <Input
                id="paypal-wallet"
                placeholder="Enter your PayPal wallet address here"
                value={walletAddress}
                onChange={(e) => {
                  setWalletAddress(e.target.value)
                  if (error) setError(null)
                }}
                className={`h-12 sm:h-14 text-base sm:text-lg ${
                  error ? "border-red-500" : ""
                }`}
                disabled={isLoading}
              />
              {error && (
                <p className="text-sm sm:text-base text-red-500">{error}</p>
              )}
            </div>

            <Button
              onClick={handleSubmit}
              className="w-full h-12 sm:h-14 text-base sm:text-lg bg-[#eb6c6c] hover:bg-[#d55c5c]"
              disabled={isLoading}
            >
              {isLoading ? "Submitting..." : "Submit"}
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

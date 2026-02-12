"use client"

import { useEffect, useState } from "react"
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
import { Card, CardContent } from "@/components/ui/card"
import { CheckCircle } from "lucide-react"
import type { AppWallet } from "@/lib/wallets/wallets"
import { useApp } from "@/context/AppProvider"

interface CashoutCryptoModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wallet: AppWallet
}

export function CashOutCryptoModal({
  open,
  onOpenChange,
  wallet,
}: CashoutCryptoModalProps) {
  const [cashOutValue, setCashOutValue] = useState<string>("")
  const [confirmed, setConfirmed] = useState(false)
  const [cashOutError, setCashOutError] = useState<string | null>(null)
  const [isProcessing, setIsProcessing] = useState(false)
  const [txHash, setTxHash] = useState<string | null>(null)
  const [walletSFLUV, setWalletSFLUV] = useState(0)
  const [payPalEthAddress, setPayPalEthAddress] = useState<string>("")
  const [payPalInputAddress, setPayPalInputAddress] = useState<string>("")
  const [payPalInputError, setPayPalInputError] = useState<string | null>(null)
  const [isSavingPayPalAddress, setIsSavingPayPalAddress] = useState(false)
  const { user, updatePayPalAddress } = useApp()

  useEffect(() => {
    if(!open) return
    walletBalanceChange()
    const userPayPalEthAddress = user?.paypalEthAddress || ""
    setPayPalEthAddress(userPayPalEthAddress)
    setPayPalInputAddress(userPayPalEthAddress)
    setPayPalInputError(null)
    setCashOutError(null)
    setConfirmed(false)
    setTxHash(null)
    setCashOutValue("0.00")
    setIsProcessing(false)
  }, [open, user, wallet])

  const walletBalanceChange = async () => {
    const balance = await wallet.getSFLUVBalanceFormatted()
    if (balance != null) {
      setWalletSFLUV(balance)
    }
  }

  const isValidPayPalEthAddress = (address: string) => {
    return address.startsWith("0x") && address.length === 42
  }

  const handleSavePayPalAddress = async () => {
    const normalizedAddress = payPalInputAddress.trim()

    if(!isValidPayPalEthAddress(normalizedAddress)) {
      setPayPalInputError(
        "Please enter a valid wallet address (must start with 0x and be 42 characters long)."
      )
      return
    }

    setIsSavingPayPalAddress(true)
    setPayPalInputError(null)
    setCashOutError(null)

    try {
      await updatePayPalAddress(normalizedAddress)
      setPayPalEthAddress(normalizedAddress)
      setPayPalInputAddress(normalizedAddress)
    } catch {
      setPayPalInputError("Failed to save PayPal address. Please try again.")
    } finally {
      setIsSavingPayPalAddress(false)
    }
  }

  /* ---------------- USD INPUT LOGIC ---------------- */

  const handleAmountChange = (value: string) => {
    if (value === "") {
      setCashOutValue("")
      return
    }

    // Immediately normalize "." → "0."
    if (value === ".") {
      setCashOutValue("0.")
      return
    }

    const dollarRegex = /^(0|[1-9]\d*)(\.\d{0,2})?$/

    // Covers ".1" → "0.1" as well
    const normalizedValue = value.startsWith(".") ? `0${value}` : value

    if (dollarRegex.test(normalizedValue)) {
      setCashOutValue(normalizedValue)
    }
  }

  const handleBlur = () => {
    if (cashOutValue === "" || cashOutValue === ".") {
      setCashOutValue("0.00")
      return
    }

    const num = Number(cashOutValue)
    if (!isNaN(num)) {
      setCashOutValue(num.toFixed(2))
    }
  }

  /* ---------------- UNWRAP ---------------- */

  const handleCashOut = async () => {
    const amount = Number(cashOutValue)
    if (!Number.isFinite(amount) || amount <= 0) return
    if (!payPalEthAddress) {
      setCashOutError("Please provide a PayPal address before unwrapping.")
      return
    }

    setCashOutError(null)
    setIsProcessing(true)
    setTxHash(null)

    try {
      if (!user) {
        throw new Error("user not found")
      }

      const tx = await wallet.unwrapAndBridge(cashOutValue, payPalEthAddress)
      if (!tx || tx.error || !tx.hash) {
        if (tx?.hash) {
          setCashOutError("Transaction reverted at hash:")
          setTxHash(tx.hash)
        } else {
          setCashOutError(tx?.error ?? "Unwrap failed. Please try again.")
        }
        return
      }

      setCashOutValue("0.00")
      setTxHash(tx.hash)
      setConfirmed(true)
      await walletBalanceChange()
    } catch {
      setCashOutError("Unwrap failed. Please try again.")
    } finally {
      setIsProcessing(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(open) => {
        if (!open) {
          setCashOutValue("0.00")
          setConfirmed(false)
          setCashOutError(null)
          setTxHash(null)
          setPayPalInputError(null)
          setIsProcessing(false)
        }
        onOpenChange(open)
      }}
    >
      <DialogContent className="w-[95vw] max-w-md mx-auto rounded-lg">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">
            Unwrap SFLUV
          </DialogTitle>
          <DialogDescription className="text-sm">
            Convert your SFLUV into USD and send it to PayPal
          </DialogDescription>
        </DialogHeader>

        {!confirmed ? (
          <div className="space-y-4">
            {!payPalEthAddress ? (
              <Card>
                <CardContent className="p-4 sm:p-6 space-y-3">
                  <p className="text-sm">
                    You do not currently have a PayPal address to unwrap to. Please provide one here:
                  </p>
                  <Label htmlFor="paypal-eth-input" className="text-sm font-medium">
                    PayPal ETH Address
                  </Label>
                  <Input
                    id="paypal-eth-input"
                    placeholder="0x..."
                    value={payPalInputAddress}
                    onChange={(e) => {
                      setPayPalInputAddress(e.target.value)
                      if(payPalInputError) {
                        setPayPalInputError(null)
                      }
                    }}
                    className={`h-12 text-base ${payPalInputError ? "border-red-500" : ""}`}
                    disabled={isSavingPayPalAddress}
                  />
                  {payPalInputError && (
                    <p className="text-sm text-red-600">{payPalInputError}</p>
                  )}
                  <Button
                    variant="destructive"
                    className="w-full h-12 text-base"
                    onClick={handleSavePayPalAddress}
                    disabled={isSavingPayPalAddress}
                  >
                    {isSavingPayPalAddress ? "Saving..." : "Save PayPal Address"}
                  </Button>
                </CardContent>
              </Card>
            ) : (
              <>
                <Card>
                  <CardContent className="p-4 sm:p-6 space-y-3">
                    <Label className="text-sm font-medium">Amount (USD)</Label>
                    <Input
                      inputMode="decimal"
                      placeholder="0.00"
                      value={cashOutValue}
                      onChange={(e) => handleAmountChange(e.target.value)}
                      onBlur={handleBlur}
                      className="h-12 text-lg"
                    />
                    <p className="text-xs text-muted-foreground">
                      Available balance: {walletSFLUV}
                    </p>
                  </CardContent>
                </Card>

                <Button
                  variant="destructive"
                  className="w-full h-12 text-base"
                  disabled={Number(cashOutValue) <= 0 || isProcessing}
                  onClick={handleCashOut}
                >
                  {isProcessing ? "Processing..." : "Unwrap SFLUV"}
                </Button>
              </>
            )}
            {cashOutError && (
              <div className="space-y-2 max-w-full">
                <p className="text-sm text-red-600 break-words [overflow-wrap:anywhere]">{cashOutError}</p>
                {txHash && (
                  <p className="text-xs break-all [overflow-wrap:anywhere] font-mono text-muted-foreground">
                    {txHash}
                  </p>
                )}
              </div>
            )}
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center py-10 text-center space-y-3">
            <CheckCircle className="h-10 w-10 text-green-600" />
            <p className="text-base font-medium">Transaction completed with hash:</p>
            {txHash && (
              <p className="text-xs break-all font-mono text-muted-foreground">
                {txHash}
              </p>
            )}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

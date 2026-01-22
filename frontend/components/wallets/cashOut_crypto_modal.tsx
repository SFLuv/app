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
  const [walletSFLUV, setWalletSFLUV] = useState(0)
  const [payPalEthAddress, setPayPalEthAddress] = useState<string>("")
  const { user } = useApp()

  useEffect(() => {
    walletBalanceChange()
    if (user) {
    setPayPalEthAddress(user?.paypalEthAddress)
    } else {
      console.log("no user found")
    }
    }, [])

  const walletBalanceChange = async () => {
    const balance = await wallet.getSFLUVBalanceFormatted()
    if (balance != null) {
    setWalletSFLUV(balance)
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

  /* ---------------- CASH OUT ---------------- */

  const handleCashOut = async () => {
    const amount = Number(cashOutValue)
    if (amount <= 0) return

    if (user) {
    await wallet.unwrap(amount, payPalEthAddress)
    } else {
      throw new Error("user not found")
    }

    setCashOutValue("0.00")
    setConfirmed(true)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(open) => {
        if (!open) {
          setCashOutValue("0.00")
          setConfirmed(false)
        }
        onOpenChange(open)
      }}
    >
      <DialogContent className="w-[95vw] max-w-md mx-auto rounded-lg">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">
            Cash Out Your SFLUV
          </DialogTitle>
          <DialogDescription className="text-sm">
            Convert your SFLUV into USD and send it to PayPal
          </DialogDescription>
        </DialogHeader>

        {!confirmed ? (
          <div className="space-y-4">
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
              disabled={Number(cashOutValue) <= 0}
              onClick={() => {
                handleCashOut()
                walletBalanceChange()
              }}
            >
              Cash Out
            </Button>
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center py-10 text-center space-y-3">
            <CheckCircle className="h-10 w-10 text-green-600" />
            <p className="text-base font-medium">
              SFLUV has been cashed!
            </p>
            <p className="text-sm text-muted-foreground">
              Check your PayPal account.
            </p>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

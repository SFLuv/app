"use client"

import { useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent } from "@/components/ui/card"
import { Copy, QrCode, CheckCircle } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import type { ConnectedWallet } from "@/types/privy-wallet"
import { AppWallet } from "@/lib/wallets/wallets"
import { CHAIN, SYMBOL } from "@/lib/constants"

interface ReceiveCryptoModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wallet: AppWallet
}

export function ReceiveCryptoModal({ open, onOpenChange, wallet }: ReceiveCryptoModalProps) {
  const [amount, setAmount] = useState("")
  const [memo, setMemo] = useState("")
  const [copied, setCopied] = useState(false)
  const { toast } = useToast()

  const copyAddress = async () => {
    try {
      await navigator.clipboard.writeText(wallet.address || "0x")
      setCopied(true)
      toast({
        title: "Address Copied",
        description: "Wallet address has been copied to clipboard",
      })
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      toast({
        title: "Copy Failed",
        description: "Failed to copy address to clipboard",
        variant: "destructive",
      })
    }
  }

  const generatePaymentRequest = () => {
    const params = new URLSearchParams()
    if (amount) params.append("amount", amount)
    if (memo) params.append("message", memo)

    const currencySymbol = SYMBOL
    const paymentUrl = `${currencySymbol.toLowerCase()}:${wallet.address}${params.toString() ? `?${params.toString()}` : ""}`

    toast({
      title: "Payment Request Generated",
      description: "Share this address or QR code to receive payments",
    })

    return paymentUrl
  }

  const currencySymbol = SYMBOL
  const networkName = CHAIN.name

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[95vw] max-w-md mx-auto max-h-[90vh] rounded-lg overflow-y-auto">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">Receive Cryptocurrency</DialogTitle>
          <DialogDescription className="text-sm">
            Share your wallet address to receive {currencySymbol} on {networkName}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 sm:space-y-6">
          {/* QR Code Section */}
          <Card>
            <CardContent className="p-4 sm:p-6">
              <div className="text-center space-y-3 sm:space-y-4">
                <div className="h-40 w-40 sm:h-48 sm:w-48 mx-auto bg-muted rounded-lg flex items-center justify-center">
                  <div className="text-center">
                    <QrCode className="h-12 w-12 sm:h-16 sm:w-16 mx-auto text-muted-foreground mb-2" />
                    <p className="text-sm text-muted-foreground">QR Code</p>
                    <p className="text-xs text-muted-foreground">{wallet?.address?.slice(0, 8) || "0x"}...</p>
                  </div>
                </div>
                <p className="text-sm text-muted-foreground">
                  Scan this QR code to send {currencySymbol} to this wallet
                </p>
              </div>
            </CardContent>
          </Card>

          {/* Wallet Address */}
          <div className="space-y-2">
            <Label className="text-sm font-medium">Wallet Address</Label>
            <div className="flex gap-2">
              <Input value={wallet.address} readOnly className="font-mono text-xs sm:text-sm h-11" />
              <Button
                variant="outline"
                size="sm"
                onClick={copyAddress}
                className="px-3 bg-transparent h-11 flex-shrink-0"
              >
                {copied ? <CheckCircle className="h-4 w-4 text-green-600" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
            <div className="flex gap-2">
              <p className="text-xs text-muted-foreground flex-1">
                Only send {currencySymbol} on {networkName} to this address
              </p>
            </div>
          </div>

          {/* Optional Payment Request */}
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="request-amount" className="text-sm font-medium">
                Request Amount (Optional)
              </Label>
              <div className="relative">
                <Input
                  id="request-amount"
                  type="number"
                  step="0.00000001"
                  placeholder="0.00"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                  className="h-11 pr-16"
                />
                <div className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground font-medium">
                  {currencySymbol}
                </div>
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="request-memo" className="text-sm font-medium">
                Message (Optional)
              </Label>
              <Textarea
                id="request-memo"
                placeholder="Add a message for the sender"
                value={memo}
                onChange={(e) => setMemo(e.target.value)}
                rows={2}
                className="resize-none"
              />
            </div>

            {(amount || memo) && (
              <Button variant="outline" onClick={generatePaymentRequest} className="w-full bg-transparent h-11">
                Generate Payment Request
              </Button>
            )}
          </div>

          {/* Security Notice */}
          <Card className="border-amber-200 dark:border-amber-800">
            <CardContent className="p-3 sm:p-4">
              <div className="space-y-2">
                <p className="font-medium text-sm">Security Tips</p>
                <ul className="text-xs text-muted-foreground space-y-1">
                  <li>• Only share this address with trusted senders</li>
                  <li>• Verify the sender before sharing payment requests</li>
                  <li>• Double-check the network matches ({networkName})</li>
                  <li>• Never share your private keys or seed phrase</li>
                </ul>
              </div>
            </CardContent>
          </Card>
        </div>
      </DialogContent>
    </Dialog>
  )
}

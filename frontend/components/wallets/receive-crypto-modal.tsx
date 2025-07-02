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

interface ReceiveCryptoModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wallet: ConnectedWallet
}

export function ReceiveCryptoModal({ open, onOpenChange, wallet }: ReceiveCryptoModalProps) {
  const [amount, setAmount] = useState("")
  const [memo, setMemo] = useState("")
  const [copied, setCopied] = useState(false)
  const { toast } = useToast()

  const getCurrencySymbol = (chainType: string) => {
    switch (chainType) {
      case "ethereum":
        return "ETH"
      case "polygon":
        return "MATIC"
      case "arbitrum":
        return "ETH"
      case "optimism":
        return "ETH"
      case "base":
        return "ETH"
      default:
        return "ETH"
    }
  }

  const getNetworkDisplayName = (chainType: string) => {
    switch (chainType) {
      case "ethereum":
        return "Ethereum"
      case "polygon":
        return "Polygon"
      case "arbitrum":
        return "Arbitrum"
      case "optimism":
        return "Optimism"
      case "base":
        return "Base"
      default:
        return chainType.charAt(0).toUpperCase() + chainType.slice(1)
    }
  }

  const copyAddress = async () => {
    try {
      await navigator.clipboard.writeText(wallet.address)
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

    const currencySymbol = getCurrencySymbol(wallet.type)
    const paymentUrl = `${currencySymbol.toLowerCase()}:${wallet.address}${params.toString() ? `?${params.toString()}` : ""}`

    toast({
      title: "Payment Request Generated",
      description: "Share this address or QR code to receive payments",
    })

    return paymentUrl
  }

  const currencySymbol = getCurrencySymbol(wallet.type)
  const networkName = getNetworkDisplayName(wallet.type)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Receive Cryptocurrency</DialogTitle>
          <DialogDescription>
            Share your wallet address to receive {currencySymbol} on {networkName}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6">
          {/* QR Code Section */}
          <Card>
            <CardContent className="p-6">
              <div className="text-center space-y-4">
                <div className="h-48 w-48 mx-auto bg-muted rounded-lg flex items-center justify-center">
                  <div className="text-center">
                    <QrCode className="h-16 w-16 mx-auto text-muted-foreground mb-2" />
                    <p className="text-sm text-muted-foreground">QR Code</p>
                    <p className="text-xs text-muted-foreground">{wallet.address.slice(0, 8)}...</p>
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
            <Label>Wallet Address</Label>
            <div className="flex gap-2">
              <Input value={wallet.address} readOnly className="font-mono text-sm" />
              <Button variant="outline" size="sm" onClick={copyAddress} className="px-3 bg-transparent">
                {copied ? <CheckCircle className="h-4 w-4 text-green-600" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Only send {currencySymbol} on {networkName} to this address
            </p>
          </div>

          {/* Optional Payment Request */}
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="request-amount">Request Amount (Optional)</Label>
              <div className="relative">
                <Input
                  id="request-amount"
                  type="number"
                  step="0.00000001"
                  placeholder="0.00"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                />
                <div className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
                  {currencySymbol}
                </div>
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="request-memo">Message (Optional)</Label>
              <Textarea
                id="request-memo"
                placeholder="Add a message for the sender"
                value={memo}
                onChange={(e) => setMemo(e.target.value)}
                rows={2}
              />
            </div>

            {(amount || memo) && (
              <Button variant="outline" onClick={generatePaymentRequest} className="w-full bg-transparent">
                Generate Payment Request
              </Button>
            )}
          </div>

          {/* Security Notice */}
          <Card className="border-amber-200 dark:border-amber-800">
            <CardContent className="p-4">
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

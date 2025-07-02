"use client"

import type React from "react"

import { useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Send, AlertTriangle, CheckCircle, X } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import type { ConnectedWallet } from "@/types/privy-wallet"

interface SendCryptoModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wallet: ConnectedWallet
}

export function SendCryptoModal({ open, onOpenChange, wallet }: SendCryptoModalProps) {
  const [step, setStep] = useState<"form" | "confirm" | "sending" | "success" | "error">("form")
  const [formData, setFormData] = useState({
    recipient: "",
    amount: "",
    memo: "",
  })
  const [error, setError] = useState("")
  const { toast } = useToast()

  // Mock balance for validation
  const mockBalance = 3.75

  const getWalletDisplayName = (walletType: string) => {
    switch (walletType) {
      case "metamask":
        return "MetaMask"
      case "coinbase_wallet":
        return "Coinbase Wallet"
      case "wallet_connect":
        return "WalletConnect"
      case "rainbow":
        return "Rainbow"
      case "trust":
        return "Trust Wallet"
      default:
        return walletType.charAt(0).toUpperCase() + walletType.slice(1)
    }
  }

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

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setError("")

    // Basic validation
    if (!formData.recipient || !formData.amount) {
      setError("Please fill in all required fields")
      return
    }

    if (Number.parseFloat(formData.amount) <= 0) {
      setError("Amount must be greater than 0")
      return
    }

    if (Number.parseFloat(formData.amount) > mockBalance) {
      setError("Insufficient balance")
      return
    }

    // Basic address validation
    if (!formData.recipient.startsWith("0x") || formData.recipient.length !== 42) {
      setError("Please enter a valid Ethereum address")
      return
    }

    setStep("confirm")
  }

  const handleConfirm = async () => {
    setStep("sending")

    // Mock sending process
    await new Promise((resolve) => setTimeout(resolve, 2000))

    // Simulate random success/failure
    if (Math.random() > 0.2) {
      setStep("success")
      toast({
        title: "Transaction Sent",
        description: `Successfully sent ${formData.amount} ${getCurrencySymbol(wallet.type)} to ${formData.recipient.slice(0, 6)}...${formData.recipient.slice(-4)}`,
      })
    } else {
      setStep("error")
      setError("Transaction failed. Please try again.")
    }
  }

  const handleClose = () => {
    setStep("form")
    setFormData({ recipient: "", amount: "", memo: "" })
    setError("")
    onOpenChange(false)
  }

  const renderContent = () => {
    const currencySymbol = getCurrencySymbol(wallet.type)

    switch (step) {
      case "form":
        return (
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="recipient">Recipient Address *</Label>
              <Input
                id="recipient"
                placeholder="0x..."
                value={formData.recipient}
                onChange={(e) => setFormData({ ...formData, recipient: e.target.value })}
                className="font-mono text-sm"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="amount">Amount *</Label>
              <div className="relative">
                <Input
                  id="amount"
                  type="number"
                  step="0.00000001"
                  placeholder="0.00"
                  value={formData.amount}
                  onChange={(e) => setFormData({ ...formData, amount: e.target.value })}
                />
                <div className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
                  {currencySymbol}
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                Available: {mockBalance.toLocaleString()} {currencySymbol}
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="memo">Memo (Optional)</Label>
              <Textarea
                id="memo"
                placeholder="Add a note for this transaction"
                value={formData.memo}
                onChange={(e) => setFormData({ ...formData, memo: e.target.value })}
                rows={3}
              />
            </div>

            {error && (
              <div className="flex items-center gap-2 text-red-600 text-sm">
                <AlertTriangle className="h-4 w-4" />
                {error}
              </div>
            )}

            <div className="flex gap-2 pt-4">
              <Button type="button" variant="outline" onClick={handleClose} className="flex-1 bg-transparent">
                Cancel
              </Button>
              <Button type="submit" className="flex-1">
                Review Transaction
              </Button>
            </div>
          </form>
        )

      case "confirm":
        return (
          <div className="space-y-4">
            <div className="text-center">
              <h3 className="text-lg font-semibold mb-2">Confirm Transaction</h3>
              <p className="text-muted-foreground text-sm">Please review the details before sending</p>
            </div>

            <Card>
              <CardContent className="p-4 space-y-4">
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground">From</span>
                  <div className="flex items-center gap-2">
                    <Avatar className="h-6 w-6">
                      <AvatarImage src={`/placeholder.svg?height=24&width=24&text=${wallet.walletClientType}`} />
                      <AvatarFallback>{wallet.walletClientType.slice(0, 2).toUpperCase()}</AvatarFallback>
                    </Avatar>
                    <span className="font-medium">{getWalletDisplayName(wallet.walletClientType)}</span>
                  </div>
                </div>

                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground">To</span>
                  <span className="font-mono text-sm">
                    {formData.recipient.slice(0, 6)}...{formData.recipient.slice(-4)}
                  </span>
                </div>

                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground">Amount</span>
                  <span className="font-semibold">
                    {formData.amount} {currencySymbol}
                  </span>
                </div>

                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground">Network Fee</span>
                  <span className="text-sm">~0.0001 {currencySymbol}</span>
                </div>

                {formData.memo && (
                  <div className="flex items-start justify-between">
                    <span className="text-muted-foreground">Memo</span>
                    <span className="text-sm text-right max-w-[200px] break-words">{formData.memo}</span>
                  </div>
                )}
              </CardContent>
            </Card>

            <div className="flex gap-2">
              <Button variant="outline" onClick={() => setStep("form")} className="flex-1">
                Back
              </Button>
              <Button onClick={handleConfirm} className="flex-1">
                <Send className="h-4 w-4 mr-2" />
                Send Transaction
              </Button>
            </div>
          </div>
        )

      case "sending":
        return (
          <div className="text-center space-y-4">
            <div className="h-16 w-16 mx-auto rounded-full bg-primary/10 flex items-center justify-center">
              <Send className="h-8 w-8 text-primary animate-pulse" />
            </div>
            <div>
              <h3 className="text-lg font-semibold">Sending Transaction</h3>
              <p className="text-muted-foreground">Please wait while we process your transaction...</p>
            </div>
          </div>
        )

      case "success":
        return (
          <div className="text-center space-y-4">
            <div className="h-16 w-16 mx-auto rounded-full bg-green-100 dark:bg-green-900/20 flex items-center justify-center">
              <CheckCircle className="h-8 w-8 text-green-600 dark:text-green-400" />
            </div>
            <div>
              <h3 className="text-lg font-semibold">Transaction Sent!</h3>
              <p className="text-muted-foreground">Your transaction has been broadcast to the network</p>
            </div>
            <Badge variant="secondary" className="font-mono text-xs">
              TX: 0x1234...5678
            </Badge>
            <Button onClick={handleClose} className="w-full">
              Done
            </Button>
          </div>
        )

      case "error":
        return (
          <div className="text-center space-y-4">
            <div className="h-16 w-16 mx-auto rounded-full bg-red-100 dark:bg-red-900/20 flex items-center justify-center">
              <X className="h-8 w-8 text-red-600 dark:text-red-400" />
            </div>
            <div>
              <h3 className="text-lg font-semibold">Transaction Failed</h3>
              <p className="text-muted-foreground">{error}</p>
            </div>
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => setStep("form")} className="flex-1">
                Try Again
              </Button>
              <Button onClick={handleClose} className="flex-1">
                Close
              </Button>
            </div>
          </div>
        )

      default:
        return null
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Send Cryptocurrency</DialogTitle>
          <DialogDescription>
            Send {getCurrencySymbol(wallet.type)} from your {getWalletDisplayName(wallet.walletClientType)} wallet
          </DialogDescription>
        </DialogHeader>
        {renderContent()}
      </DialogContent>
    </Dialog>
  )
}

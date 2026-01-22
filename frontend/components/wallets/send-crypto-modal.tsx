"use client"

import type React from "react"

import { useEffect, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Send, AlertTriangle, CheckCircle, X, Copy, ArrowLeft } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import type { ConnectedWallet } from "@/types/privy-wallet"
import { AppWallet } from "@/lib/wallets/wallets"
import { BYUSD_DECIMALS, SFLUV_DECIMALS, SYMBOL } from "@/lib/constants"
import { Address, Hash } from "viem"
import { useContacts } from "@/context/ContactsProvider";
import ContactOrAddressInput from "../contacts/contact-or-address-input"

interface SendCryptoModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wallet: AppWallet
  balance: number
}

export function SendCryptoModal({ open, onOpenChange, wallet, balance }: SendCryptoModalProps) {
  const [step, setStep] = useState<"form" | "confirm" | "sending" | "success" | "error">("form")
  const [hash, setHash] = useState<Hash | null>(null)
  const [copied, setCopied] = useState<boolean>(false)
  const [formData, setFormData] = useState({
    recipient: "",
    amount: "",
    memo: "",
  })
  const [error, setError] = useState("")
  const { toast } = useToast()
  const { contacts } = useContacts()

  const copyHash = async () => {
    try {
    if(!hash) throw new Error("no hash to copy")
      await navigator.clipboard.writeText(hash)
      setCopied(true)
      toast({
        title: "Hash Copied",
        description: "Tx hash has been copied to clipboard",
      })
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      toast({
        title: "Copy Failed",
        description: "Failed to copy hash to clipboard",
        variant: "destructive",
      })
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

    if (Number.parseFloat(formData.amount) > balance) {
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

    let receipt = await wallet.send(BigInt(Number(formData.amount) * (10 ** SFLUV_DECIMALS)), formData.recipient as Address)
    if(!receipt) {
      setStep("error")
      setError("Error creating transaction. Please try again.")
      return
    }

    if (receipt.hash) {
      setStep("success")
      setHash(receipt.hash as Hash)
      toast({
        title: "Transaction Sent",
        description: `Successfully sent ${formData.amount} ${SYMBOL} to ${formData.recipient.slice(0, 6)}...${formData.recipient.slice(-4)}`,
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
    setHash(null)
    onOpenChange(false)
  }

  const renderContent = () => {
     switch (step) {
      case "form":
        return (
          <div className="space-y-4">
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="recipient" className="text-sm font-medium">
                  Recipient Address *
                </Label>
                <ContactOrAddressInput
                  id="recipient"
                  onChange={(value) => setFormData({ ...formData, recipient: value })}
                  className="font-mono text-sm h-11"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="amount" className="text-sm font-medium">
                  Amount *
                </Label>
                <div className="relative">
                  <Input
                    id="amount"
                    type="number"
                    step="0.00000001"
                    placeholder="0.00"
                    value={formData.amount}
                    onChange={(e) => setFormData({ ...formData, amount: e.target.value })}
                    className="h-11 pr-16"
                  />
                  <div className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground font-medium">
                    {SYMBOL}
                  </div>
                </div>
                <p className="text-xs text-muted-foreground">
                  Available: {balance} {SYMBOL}
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="memo" className="text-sm font-medium">
                  Memo (Optional)
                </Label>
                <Textarea
                  id="memo"
                  placeholder="Add a note for this transaction"
                  value={formData.memo}
                  onChange={(e) => setFormData({ ...formData, memo: e.target.value })}
                  rows={3}
                  className="resize-none"
                />
              </div>

              {error && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{error}</span>
                </div>
              )}

              <div className="flex flex-col gap-3 pt-4 border-t">
                <Button type="submit" className="h-11 w-full">
                  Review Transaction
                </Button>
                <Button type="button" variant="outline" onClick={handleClose} className="h-11 w-full bg-transparent">
                  Cancel
                </Button>
              </div>
            </form>
          </div>
        )

      case "confirm":
        return (
          <div className="space-y-4">
            <div className="text-center pb-2">
              <h3 className="text-lg font-semibold mb-2">Confirm Transaction</h3>
              <p className="text-muted-foreground text-sm">Please review the details before sending</p>
            </div>

            <Card>
              <CardContent className="p-4 space-y-4">
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground text-sm">From</span>
                  <div className="flex items-center gap-2">
                    <Avatar className="h-5 w-5">
                      <AvatarImage src={`/placeholder.svg?height=20&width=20&text=${wallet.name}`} />
                      <AvatarFallback className="text-xs">
                        {wallet.name.slice(0, 2).toUpperCase()}
                      </AvatarFallback>
                    </Avatar>
                    <span className="font-medium text-sm">{wallet.name.toUpperCase()}</span>
                  </div>
                </div>

                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground text-sm">To</span>
                  <span className="font-mono text-sm">
                    {contacts.find((contact) => contact.address === formData.recipient)?.name || formData.recipient.slice(0, 6) + "..." + formData.recipient.slice(-4)}
                  </span>
                </div>

                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground text-sm">Amount</span>
                  <span className="font-semibold">
                    {formData.amount} {SYMBOL}
                  </span>
                </div>

                {formData.memo && (
                  <div className="flex items-start justify-between">
                    <span className="text-muted-foreground text-sm">Memo</span>
                    <span className="text-sm text-right max-w-[200px] break-words">{formData.memo}</span>
                  </div>
                )}
              </CardContent>
            </Card>

            <div className="flex flex-col gap-3 pt-4 border-t">
              <Button onClick={handleConfirm} className="h-11 w-full">
                <Send className="h-4 w-4 mr-2" />
                Send Transaction
              </Button>
              <Button variant="outline" onClick={() => setStep("form")} className="h-11 w-full bg-transparent">
                <ArrowLeft className="h-4 w-4 mr-2" />
                Back
              </Button>
            </div>
          </div>
        )

      case "sending":
        return (
          <div className="text-center space-y-6 py-8">
            <div className="h-16 w-16 mx-auto rounded-full bg-primary/10 flex items-center justify-center">
              <Send className="h-8 w-8 text-primary animate-pulse" />
            </div>
            <div>
              <h3 className="text-lg font-semibold mb-2">Sending Transaction</h3>
              <p className="text-muted-foreground text-sm">Please wait while we process your transaction...</p>
            </div>
          </div>
        )

      case "success":
        return (
          <div className="text-center space-y-6 py-4">
            <div className="h-16 w-16 mx-auto rounded-full bg-green-100 dark:bg-green-900/20 flex items-center justify-center">
              <CheckCircle className="h-8 w-8 text-green-600 dark:text-green-400" />
            </div>
            <div>
              <h3 className="text-lg font-semibold mb-2">Transaction Sent!</h3>
              <p className="text-muted-foreground text-sm mb-4">Your transaction has been broadcast to the network</p>
              <div className="space-y-2">
                <Label className="text-sm font-medium">Tranaction ID</Label>
                <div className="flex gap-2">
                  <Input value={`${hash?.slice(0, 6)}...${hash?.slice(-4)}`} readOnly className="font-mono text-xs sm:text-sm h-11" />
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={copyHash}
                    className="px-3 bg-transparent h-11 flex-shrink-0"
                  >
                    {copied ? <CheckCircle className="h-4 w-4 text-green-600" /> : <Copy className="h-4 w-4" />}
                  </Button>
                </div>
                </div>
            </div>
            <Button onClick={handleClose} className="w-full h-11">
              Done
            </Button>
          </div>
        )

      case "error":
        return (
          <div className="text-center space-y-6 py-4">
            <div className="h-16 w-16 mx-auto rounded-full bg-red-100 dark:bg-red-900/20 flex items-center justify-center">
              <X className="h-8 w-8 text-red-600 dark:text-red-400" />
            </div>
            <div>
              <h3 className="text-lg font-semibold mb-2">Transaction Failed</h3>
              <p className="text-muted-foreground text-sm">{error}</p>
            </div>
            <div className="flex flex-col gap-3">
              <Button onClick={() => setStep("form")} className="w-full h-11">
                Try Again
              </Button>
              <Button variant="outline" onClick={handleClose} className="w-full h-11 bg-transparent">
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
      <DialogContent className="w-[95vw] max-w-md mx-auto max-h-[90vh] rounded-lg overflow-y-auto">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">Send Cryptocurrency</DialogTitle>
          <DialogDescription className="text-sm">
            Send {SYMBOL} from your {wallet.name.toUpperCase()} wallet
          </DialogDescription>
        </DialogHeader>
        {renderContent()}
      </DialogContent>
    </Dialog>
  )
}

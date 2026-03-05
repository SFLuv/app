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
import { useApp } from "@/context/AppProvider"
import type { ConnectedWallet } from "@/types/privy-wallet"
import { AppWallet } from "@/lib/wallets/wallets"
import { BYUSD_DECIMALS, SFLUV_DECIMALS, SYMBOL } from "@/lib/constants"
import { Address, Hash } from "viem"
import { useContacts } from "@/context/ContactsProvider";
import ContactOrAddressInput from "../contacts/contact-or-address-input"
import type { W9Submission } from "@/types/w9"

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
  const [w9Email, setW9Email] = useState<string | null>(null)
  const [w9Reason, setW9Reason] = useState<"w9_required" | "w9_pending" | null>(null)
  const [w9Year, setW9Year] = useState<number | null>(null)
  const [w9EmailInput, setW9EmailInput] = useState<string>("")
  const [w9Submitting, setW9Submitting] = useState<boolean>(false)
  const [formData, setFormData] = useState({
    recipient: "",
    amount: "",
    memo: "",
  })
  const [error, setError] = useState("")
  const { toast } = useToast()
  const { contacts } = useContacts()
  const { user, authFetch } = useApp()

  const isValidEmail = (email: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)

  const toAmountWei = () => BigInt(Number(formData.amount) * (10 ** SFLUV_DECIMALS))

  const executeSend = async () => {
    try {
      const amountWei = toAmountWei()
      const receipt = await wallet.send(amountWei, formData.recipient as Address)

      if (!receipt) {
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
        return
      }

      setStep("error")
      setError("Transaction failed. Please try again.")
    } catch {
      setStep("error")
      setError("Transaction failed. Please try again.")
    }
  }

  const findPendingSubmissionId = async (walletAddress: string, year: number): Promise<number | null> => {
    const res = await authFetch("/admin/w9/pending")
    if (res.status !== 200) {
      throw new Error("Unable to fetch pending W9 submissions.")
    }
    const data = await res.json()
    const submissions: W9Submission[] = Array.isArray(data?.submissions) ? data.submissions : []
    const normalizedWallet = walletAddress.toLowerCase()
    const matches = submissions.filter((submission) => {
      return (
        submission.pending_approval &&
        submission.wallet_address.toLowerCase() === normalizedWallet &&
        submission.year === year
      )
    })
    if (matches.length === 0) return null
    matches.sort((a, b) => b.id - a.id)
    return matches[0].id
  }

  const handleApproveAndSend = async () => {
    if (!user?.isAdmin) {
      setError("Only admins can approve W9 submissions.")
      return
    }

    const email = w9EmailInput.trim()
    if (!email) {
      setError("Recipient email is required to approve W9. Enter an email to continue.")
      return
    }
    if (!isValidEmail(email)) {
      setError("Please enter a valid recipient email.")
      return
    }

    setW9Submitting(true)
    try {
      const year = w9Year ?? new Date().getUTCFullYear()
      let submissionId: number | null = null
      let alreadyApproved = false

      const submitRes = await authFetch("/w9/submit", {
        method: "POST",
        body: JSON.stringify({
          wallet_address: formData.recipient,
          email,
          year,
        }),
      })

      if (submitRes.status === 201) {
        const data = await submitRes.json()
        submissionId = data?.submission?.id ?? null
      } else if (submitRes.status === 409) {
        const data = await submitRes.json().catch(() => null)
        const submitError = data?.error
        if (submitError === "w9_approved") {
          alreadyApproved = true
        } else if (submitError === "w9_pending") {
          submissionId = await findPendingSubmissionId(formData.recipient, year)
          if (!submissionId) {
            throw new Error("Pending W9 submission not found for this wallet.")
          }
        } else {
          throw new Error("Unable to submit W9 for approval.")
        }
      } else {
        throw new Error("Unable to submit W9 for approval.")
      }

      if (!alreadyApproved) {
        if (!submissionId) {
          submissionId = await findPendingSubmissionId(formData.recipient, year)
        }
        if (!submissionId) {
          throw new Error("W9 submission could not be identified for approval.")
        }

        const approveRes = await authFetch("/admin/w9/approve", {
          method: "PUT",
          body: JSON.stringify({ id: submissionId }),
        })
        if (approveRes.status === 409) {
          const approveData = await approveRes.json().catch(() => null)
          if (approveData?.error !== "w9_not_pending") {
            throw new Error("Unable to approve W9 submission.")
          }
        } else if (approveRes.status !== 200) {
          throw new Error("Unable to approve W9 submission.")
        }
      }

      toast({
        title: "W9 Approved",
        description: "Recipient W9 is approved. Continuing transfer.",
      })

      setW9Reason(null)
      setW9Year(null)
      setW9Email(email)
      setError("")
      setStep("sending")
      await executeSend()
    } catch (err) {
      setStep("error")
      setError(err instanceof Error ? err.message : "Failed to approve W9. Please try again.")
    } finally {
      setW9Submitting(false)
    }
  }

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
    setW9Reason(null)
    setW9Year(null)
    setW9Email(null)
    setW9EmailInput("")
    setError("")

    const amountWei = toAmountWei()

    if (user?.isAdmin) {
      try {
        const res = await authFetch("/w9/check", {
          method: "POST",
          body: JSON.stringify({
            from_address: wallet.address,
            to_address: formData.recipient,
            amount: amountWei.toString(),
          }),
        })

        if (res.status === 403) {
          const data = await res.json().catch(() => null)
          const reason: "w9_required" | "w9_pending" = data?.reason === "w9_pending" ? "w9_pending" : "w9_required"
          const email = typeof data?.email === "string" && data.email.trim() ? data.email.trim() : null

          setW9Reason(reason)
          setW9Year(typeof data?.year === "number" ? data.year : null)
          setW9Email(email)
          setW9EmailInput(email || "")
          setError(
            reason === "w9_pending"
              ? "W9 submission is pending approval. Transfers are blocked until approved."
              : "W9 required before sending to this wallet."
          )
          setStep("error")
          return
        }

        if (res.status !== 200) {
          setError("Unable to validate W9 compliance. Please try again.")
          setStep("error")
          return
        }
      } catch {
        setError("Unable to validate W9 compliance. Please try again.")
        setStep("error")
        return
      }
    }

    await executeSend()
  }

  const handleClose = () => {
    setStep("form")
    setFormData({ recipient: "", amount: "", memo: "" })
    setError("")
    setHash(null)
    setW9Email(null)
    setW9Reason(null)
    setW9Year(null)
    setW9EmailInput("")
    setW9Submitting(false)
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
        if (w9Reason) {
          return (
            <div className="space-y-5 py-2">
              <div className="h-16 w-16 mx-auto rounded-full bg-amber-100 dark:bg-amber-900/20 flex items-center justify-center">
                <AlertTriangle className="h-8 w-8 text-amber-600 dark:text-amber-400" />
              </div>
              <div className="text-center space-y-2">
                <h3 className="text-lg font-semibold">W9 Approval Required</h3>
                <p className="text-sm text-muted-foreground break-all">
                  <span className="font-mono">{formData.recipient}</span> needs to have an approved W9 form in order to receive more {SYMBOL}.
                </p>
                <p className="text-sm text-muted-foreground">
                  To pre-approve this user&apos;s W9 form, click approve below.
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="w9-email" className="text-sm font-medium">
                  Recipient Email
                </Label>
                <Input
                  id="w9-email"
                  type="email"
                  value={w9EmailInput}
                  onChange={(e) => setW9EmailInput(e.target.value)}
                  placeholder="user@example.com"
                  className="h-11"
                />
                <p className="text-xs text-muted-foreground">
                  {w9Email
                    ? "Prefilled from existing records. You can edit it before approving."
                    : "No email found in W9 records. Enter recipient email to continue."}
                </p>
              </div>

              {error && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{error}</span>
                </div>
              )}

              <div className="flex flex-col gap-3 pt-2">
                <Button onClick={handleApproveAndSend} className="w-full h-11" disabled={w9Submitting}>
                  {w9Submitting ? "Approving..." : "Approve & Send"}
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    setStep("form")
                    setError("")
                    setW9Reason(null)
                    setW9Year(null)
                    setW9EmailInput("")
                  }}
                  className="w-full h-11 bg-transparent"
                  disabled={w9Submitting}
                >
                  Cancel
                </Button>
              </div>
            </div>
          )
        }

        return (
          <div className="text-center space-y-6 py-4">
            <div className="h-16 w-16 mx-auto rounded-full bg-red-100 dark:bg-red-900/20 flex items-center justify-center">
              <X className="h-8 w-8 text-red-600 dark:text-red-400" />
            </div>
            <div>
              <h3 className="text-lg font-semibold mb-2">Transaction Failed</h3>
              <p className="text-muted-foreground text-sm">{error}</p>
              {w9Email ? (
                <p className="text-sm mt-2">
                  Recipient email on file: <span className="font-medium">{w9Email}</span>
                </p>
              ) : (
                <p className="text-sm mt-2 text-muted-foreground">No recipient email on file.</p>
              )}
            </div>
            <div className="flex flex-col gap-3">
              <Button
                onClick={() => {
                  setStep("form")
                  setW9Reason(null)
                  setW9Year(null)
                  setW9EmailInput("")
                }}
                className="w-full h-11"
              >
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

"use client"

import { useMemo, useState } from "react"
import { Address, isAddress, getAddress, parseUnits } from "viem"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Loader2, Send } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import { AppWallet } from "@/lib/wallets/wallets"
import ContactOrAddressInput from "../contacts/contact-or-address-input"
import { USDC, USDC_SYMBOL } from "./usdc-theme"

interface SendUsdcModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wallet: AppWallet
  balance: number | null
  decimals: number
  onSuccess: () => void | Promise<void>
}

// Focused USDC transfer, following the SFLUV send modal's structure (amount +
// recipient + status) but scoped to a plain ERC-20 transfer of the underlying.
export function SendUsdcModal({ open, onOpenChange, wallet, balance, decimals, onSuccess }: SendUsdcModalProps) {
  const { toast } = useToast()
  const [amount, setAmount] = useState("")
  const [recipient, setRecipient] = useState("")
  const [processing, setProcessing] = useState(false)
  const [error, setError] = useState("")

  const recipientAddress = useMemo<Address | null>(() => {
    const trimmed = recipient.trim()
    return isAddress(trimmed) ? getAddress(trimmed) : null
  }, [recipient])

  const amountNumber = Number.parseFloat(amount)
  const amountValid = Number.isFinite(amountNumber) && amountNumber > 0
  const overBalance = balance !== null && amountValid && amountNumber > balance
  const canSubmit = amountValid && !overBalance && !!recipientAddress && !processing

  const reset = () => {
    setAmount("")
    setRecipient("")
    setError("")
    setProcessing(false)
  }

  const handleClose = (next: boolean) => {
    if (!next) reset()
    onOpenChange(next)
  }

  const submit = async () => {
    if (!recipientAddress) {
      setError("Enter a valid recipient address.")
      return
    }
    if (!amountValid) {
      setError("Enter an amount greater than 0.")
      return
    }
    if (overBalance) {
      setError("Amount exceeds your USDC balance.")
      return
    }

    setProcessing(true)
    setError("")
    try {
      const amountWei = parseUnits(amount, decimals)
      const receipt = await wallet.sendUSDC(amountWei, recipientAddress)
      if (!receipt || receipt.error || !receipt.hash) {
        setError(receipt?.error ?? "The USDC transfer did not complete. Please try again.")
        setProcessing(false)
        return
      }
      toast({ title: "USDC sent", description: `Sent ${amount} ${USDC_SYMBOL}.` })
      await onSuccess()
      reset()
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to send USDC.")
      setProcessing(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[460px]">
        <DialogHeader>
          <DialogTitle className={USDC.accentText}>Send {USDC_SYMBOL}</DialogTitle>
          <DialogDescription>Transfer USDC on Celo to another address.</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="usdc-send-amount">Amount ({USDC_SYMBOL})</Label>
              <span className="text-xs text-muted-foreground">
                Available: {balance === null ? "—" : balance.toLocaleString("en-US", { maximumFractionDigits: 2 })}
              </span>
            </div>
            <Input
              id="usdc-send-amount"
              type="number"
              min="0"
              step="0.01"
              placeholder="0.00"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              disabled={processing}
            />
            {overBalance && <p className="text-xs text-[#b42318] dark:text-[#ffb4a8]">Amount exceeds your USDC balance.</p>}
          </div>

          <div className="space-y-2">
            <Label>Recipient</Label>
            <ContactOrAddressInput
              id="usdc-send-recipient"
              onChange={(value, resolved) => setRecipient(resolved?.address ?? value)}
            />
          </div>

          {error && <p className="text-sm text-[#b42318] dark:text-[#ffb4a8]">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleClose(false)} disabled={processing}>
            Cancel
          </Button>
          <Button className={USDC.button} onClick={submit} disabled={!canSubmit}>
            {processing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Send className="mr-2 h-4 w-4" />}
            {processing ? "Sending…" : `Send ${USDC_SYMBOL}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

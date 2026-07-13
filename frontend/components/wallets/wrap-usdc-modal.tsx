"use client"

import { useState } from "react"
import { parseUnits } from "viem"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ArrowRight, Loader2 } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import { AppWallet } from "@/lib/wallets/wallets"
import { useChainConfig } from "@/context/ChainConfigProvider"
import { USDC, USDC_SYMBOL } from "./usdc-theme"

interface WrapUsdcModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wallet: AppWallet
  balance: number | null
  decimals: number
  onSuccess: () => void | Promise<void>
}

// Wrap USDC into SFLUV (1:1) via the wrapper's depositFor. The wallet handles the
// USDC approval before depositing, so this UI just collects the amount and shows
// the conversion + status.
export function WrapUsdcModal({ open, onOpenChange, wallet, balance, decimals, onSuccess }: WrapUsdcModalProps) {
  const { toast } = useToast()
  const chainConfig = useChainConfig()
  const tokenSymbol = chainConfig.tokenSymbol
  const [amount, setAmount] = useState("")
  const [processing, setProcessing] = useState(false)
  const [error, setError] = useState("")

  const amountNumber = Number.parseFloat(amount)
  const amountValid = Number.isFinite(amountNumber) && amountNumber > 0
  const overBalance = balance !== null && amountValid && amountNumber > balance
  const canSubmit = amountValid && !overBalance && !processing

  const reset = () => {
    setAmount("")
    setError("")
    setProcessing(false)
  }

  const handleClose = (next: boolean) => {
    if (!next) reset()
    onOpenChange(next)
  }

  const submit = async () => {
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
      const receipt = await wallet.wrap(amountWei)
      if (!receipt || receipt.error || !receipt.hash) {
        setError(receipt?.error ?? "The wrap did not complete. Please try again.")
        setProcessing(false)
        return
      }
      toast({ title: "Wrapped", description: `Wrapped ${amount} ${USDC_SYMBOL} into ${tokenSymbol}.` })
      await onSuccess()
      reset()
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to wrap USDC.")
      setProcessing(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[460px]">
        <DialogHeader>
          <DialogTitle className={USDC.accentText}>Wrap {USDC_SYMBOL} into {tokenSymbol}</DialogTitle>
          <DialogDescription>
            Deposit USDC to mint {tokenSymbol} 1:1 to this wallet. You may be asked to approve USDC first.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="usdc-wrap-amount">Amount ({USDC_SYMBOL})</Label>
              <span className="text-xs text-muted-foreground">
                Available: {balance === null ? "—" : balance.toLocaleString("en-US", { maximumFractionDigits: 2 })}
              </span>
            </div>
            <Input
              id="usdc-wrap-amount"
              type="number"
              min="0"
              step="0.01"
              placeholder="0.00"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              disabled={processing}
            />
          </div>

          <div className="flex items-center justify-center gap-3 rounded-lg border border-border bg-muted/30 px-4 py-3 text-sm">
            <span className="font-medium text-[#2775ca] dark:text-[#6aa8e0]">{amountValid ? amount : "0"} {USDC_SYMBOL}</span>
            <ArrowRight className="h-4 w-4 text-muted-foreground" />
            <span className="font-medium text-[#eb6c6c]">{amountValid ? amount : "0"} {tokenSymbol}</span>
          </div>

          {overBalance && <p className="text-xs text-[#b42318] dark:text-[#ffb4a8]">Amount exceeds your USDC balance.</p>}
          {error && <p className="text-sm text-[#b42318] dark:text-[#ffb4a8]">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleClose(false)} disabled={processing}>
            Cancel
          </Button>
          <Button className={USDC.button} onClick={submit} disabled={!canSubmit}>
            {processing && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {processing ? "Wrapping…" : `Wrap into ${tokenSymbol}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

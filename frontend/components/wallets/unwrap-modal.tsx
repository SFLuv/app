"use client"

import { useState } from "react"
import { isAddress } from "viem"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Loader2 } from "lucide-react"
import { useToast } from "@/hooks/use-toast"
import { AppWallet } from "@/lib/wallets/wallets"
import { useChainConfig } from "@/context/ChainConfigProvider"

interface UnwrapModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wallet: AppWallet
  balance: number | null
  /** Where the proceeds land. Prefilled from the account's cash-out address when it has one. */
  defaultDestination?: string
  onSuccess: () => void | Promise<void>
}

/**
 * Cash a till out of SFLUV and into the backing asset, at the destination
 * address the merchant nominates.
 *
 * Two things about this are worth knowing before reading the code. The caller
 * decides whether this exists at all — it is behind the server's unwrap_enabled
 * flag — and even switched on, the wallet must hold REDEEMER_ROLE on the token
 * for the call to go through. That role is granted per address on chain, so a
 * freshly minted till will be refused; unwrapAndBridge says so in its own words
 * and we show them rather than inventing a friendlier untruth about why it
 * failed.
 *
 * The destination is typed, not picked from contacts. Contacts are SFLuv
 * accounts on this chain and the proceeds are not staying on it, so offering
 * that list would only ever suggest a wrong answer.
 */
export function UnwrapModal({
  open,
  onOpenChange,
  wallet,
  balance,
  defaultDestination,
  onSuccess,
}: UnwrapModalProps) {
  const { toast } = useToast()
  const chainConfig = useChainConfig()
  const tokenSymbol = chainConfig.tokenSymbol
  const [amount, setAmount] = useState("")
  const [destination, setDestination] = useState(defaultDestination ?? "")
  const [processing, setProcessing] = useState(false)
  const [error, setError] = useState("")

  const amountNumber = Number.parseFloat(amount)
  const amountValid = Number.isFinite(amountNumber) && amountNumber > 0
  const overBalance = balance !== null && amountValid && amountNumber > balance
  const trimmedDestination = destination.trim()
  const destinationValid = isAddress(trimmedDestination)
  const canSubmit = amountValid && !overBalance && destinationValid && !processing

  const reset = () => {
    setAmount("")
    setDestination(defaultDestination ?? "")
    setError("")
    setProcessing(false)
  }

  const handleClose = (next: boolean) => {
    if (!next) reset()
    onOpenChange(next)
  }

  const submit = async () => {
    setProcessing(true)
    setError("")
    try {
      const receipt = await wallet.unwrapAndBridge(amount, trimmedDestination)
      if (!receipt || receipt.error || !receipt.hash) {
        setError(receipt?.error ?? "The unwrap did not complete. Please try again.")
        setProcessing(false)
        return
      }
      toast({ title: "Unwrapped", description: `Cashed out ${amount} ${tokenSymbol}.` })
      await onSuccess()
      reset()
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to unwrap.")
      setProcessing(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[460px]">
        <DialogHeader>
          <DialogTitle className="text-black dark:text-white">Unwrap {tokenSymbol}</DialogTitle>
          <DialogDescription>
            Move {tokenSymbol} out of this till and into the backing asset at an address you
            control. This leaves SFLuv and cannot be undone from here.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="unwrap-amount">Amount ({tokenSymbol})</Label>
              <span className="text-xs text-muted-foreground">
                Available: {balance === null ? "—" : balance.toLocaleString("en-US", { maximumFractionDigits: 2 })}
              </span>
            </div>
            <Input
              id="unwrap-amount"
              type="number"
              min="0"
              step="0.01"
              placeholder="0.00"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              disabled={processing}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="unwrap-destination">Destination address</Label>
            <Input
              id="unwrap-destination"
              placeholder="0x..."
              value={destination}
              onChange={(e) => setDestination(e.target.value)}
              disabled={processing}
              className="font-mono text-sm"
            />
          </div>

          {overBalance && (
            <p className="text-xs text-[#b42318] dark:text-[#ffb4a8]">
              Amount exceeds this till&apos;s balance.
            </p>
          )}
          {trimmedDestination !== "" && !destinationValid && (
            <p className="text-xs text-[#b42318] dark:text-[#ffb4a8]">
              That is not a valid wallet address.
            </p>
          )}
          {error && <p className="text-sm text-[#b42318] dark:text-[#ffb4a8]">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleClose(false)} disabled={processing}>
            Cancel
          </Button>
          <Button className="bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={submit} disabled={!canSubmit}>
            {processing && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {processing ? "Unwrapping…" : "Unwrap"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

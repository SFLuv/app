"use client"

import { useEffect, useMemo, useState } from "react"
import { formatUnits, parseUnits, type Address } from "viem"
import { ArrowLeft, Loader2, Undo2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { useApp } from "@/context/AppProvider"
import { useChainConfig } from "@/context/ChainConfigProvider"
import { useToast } from "@/hooks/use-toast"
import type { AppWallet } from "@/lib/wallets/wallets"
import type { LocationTransaction } from "@/types/location-transactions"

type RefundModalProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  locationId: number
  transaction: LocationTransaction | null
  tillWallet: AppWallet | null
  onRefunded?: () => void | Promise<void>
}

// Three steps, because a refund is three separate decisions and collapsing them
// is how the wrong amount goes out: what is being refunded, how much of it, and
// a last look before the money moves.
type Step = "details" | "amount" | "confirm"

export function RefundModal({
  open,
  onOpenChange,
  locationId,
  transaction,
  tillWallet,
  onRefunded,
}: RefundModalProps) {
  const { authFetch } = useApp()
  const chainConfig = useChainConfig()
  const { toast } = useToast()

  const [step, setStep] = useState<Step>("details")
  const [mode, setMode] = useState<"full" | "partial">("full")
  const [partialInput, setPartialInput] = useState("")
  const [percent, setPercent] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)

  const decimals = chainConfig.tokenDecimals
  const symbol = chainConfig.tokenSymbol

  // The remainder the backend published with this row. The submission is
  // checked against a freshly computed remainder too — this only shapes the
  // form, it is not what authorises the amount.
  const remaining = useMemo(() => {
    const raw = transaction?.refund?.remaining_base
    try {
      return raw ? BigInt(raw) : null
    } catch {
      return null
    }
  }, [transaction])

  const originalAmount = useMemo(() => {
    try {
      return transaction ? BigInt(transaction.amount_base) : null
    } catch {
      return null
    }
  }, [transaction])

  useEffect(() => {
    if (!open) return
    setStep("details")
    setMode("full")
    setPartialInput("")
    setPercent(null)
    setBusy(false)
  }, [open, transaction?.hash])

  const fmt = (value: bigint | null) =>
    value === null
      ? "—"
      : `${Number(formatUnits(value, decimals)).toLocaleString(undefined, { maximumFractionDigits: 2 })} ${symbol}`

  // Rounded to the cent, because a percentage of an odd amount is not a payable
  // figure and the merchant is agreeing to an exact number on the next step.
  const roundToCent = (value: bigint) => {
    if (decimals <= 2) return value
    const factor = 10n ** BigInt(decimals - 2)
    return ((value + factor / 2n) / factor) * factor
  }

  const amountToRefund = useMemo(() => {
    if (!remaining) return null
    if (mode === "full") return remaining
    if (percent !== null) return roundToCent((remaining * BigInt(percent)) / 100n)
    try {
      const parsed = parseUnits(partialInput || "0", decimals)
      return parsed > 0n ? parsed : null
    } catch {
      return null
    }
  }, [mode, percent, partialInput, remaining, decimals])

  const overRemaining = Boolean(amountToRefund && remaining && amountToRefund > remaining)

  const submit = async () => {
    if (!transaction || !tillWallet || !amountToRefund || !remaining) return
    if (amountToRefund > remaining) return
    setBusy(true)
    try {
      const receipt = await tillWallet.send(amountToRefund, transaction.from as Address)
      if (!receipt || receipt.error || !receipt.hash) {
        throw new Error(receipt?.error || "The refund could not be submitted")
      }

      const res = await authFetch(`/locations/${locationId}/refunds`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          original_tx_hash: transaction.hash,
          refund_tx_hash: receipt.hash,
          wallet_address: tillWallet.address,
          amount: amountToRefund.toString(),
          location_id: locationId,
        }),
      })
      const body = (await res.json().catch(() => ({}))) as { error?: string }
      if (!res.ok) {
        // The transfer already happened. Say so rather than implying it did
        // not, or the merchant will send the money a second time.
        throw new Error(
          `${body.error || "The refund was sent but could not be recorded."} The money has already left your till — don't send it again.`,
        )
      }

      toast({ title: "Refund issued", description: `${fmt(amountToRefund)} sent back to the customer.` })
      onOpenChange(false)
      await onRefunded?.()
    } catch (error) {
      toast({
        title: "Refund failed",
        description: error instanceof Error ? error.message : undefined,
        variant: "destructive",
      })
    } finally {
      setBusy(false)
    }
  }

  if (!transaction) return null

  const alreadyRefunded = transaction.refund?.refunded_base
    ? BigInt(transaction.refund.refunded_base)
    : 0n

  return (
    <Dialog open={open} onOpenChange={(next) => (busy ? null : onOpenChange(next))}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {step === "details" && "Refund this payment"}
            {step === "amount" && "How much?"}
            {step === "confirm" && "Confirm refund"}
          </DialogTitle>
          <DialogDescription>
            {step === "details" && "Check this is the right payment before refunding it."}
            {step === "amount" && "Refund all of what's left, or part of it."}
            {step === "confirm" && "This sends tokens back to the customer straight away."}
          </DialogDescription>
        </DialogHeader>

        {step === "details" && (
          <div className="space-y-3 text-sm">
            <Row label="Paid" value={fmt(originalAmount)} />
            {alreadyRefunded > 0n && <Row label="Already refunded" value={fmt(alreadyRefunded)} />}
            <Row label="Left to refund" value={fmt(remaining)} strong />
            <Row label="Customer" value={shorten(transaction.from)} mono />
            <Row label="When" value={new Date(transaction.timestamp * 1000).toLocaleString()} />
            <Row label="Transaction" value={shorten(transaction.hash)} mono />
          </div>
        )}

        {step === "amount" && (
          <div className="space-y-4">
            <div className="flex gap-2">
              <Button
                type="button"
                variant={mode === "full" ? "default" : "outline"}
                size="sm"
                onClick={() => {
                  setMode("full")
                  setPercent(null)
                }}
              >
                Refund all {fmt(remaining)}
              </Button>
              <Button
                type="button"
                variant={mode === "partial" ? "default" : "outline"}
                size="sm"
                onClick={() => setMode("partial")}
              >
                Refund part
              </Button>
            </div>

            {mode === "partial" && (
              <div className="space-y-3">
                <div className="flex flex-wrap gap-2">
                  {[25, 50, 75].map((p) => (
                    <Button
                      key={p}
                      type="button"
                      size="sm"
                      variant={percent === p ? "secondary" : "outline"}
                      onClick={() => {
                        setPercent(p)
                        setPartialInput("")
                      }}
                    >
                      {p}%
                    </Button>
                  ))}
                </div>
                <div className="space-y-1">
                  <Label htmlFor="refund-amount" className="text-xs text-muted-foreground">
                    Or an exact amount
                  </Label>
                  <Input
                    id="refund-amount"
                    inputMode="decimal"
                    placeholder="0.00"
                    value={partialInput}
                    onChange={(e) => {
                      setPartialInput(e.target.value)
                      setPercent(null)
                    }}
                  />
                </div>
                {overRemaining && (
                  <p className="text-xs text-red-600">
                    That&apos;s more than the {fmt(remaining)} left on this payment.
                  </p>
                )}
              </div>
            )}

            <div className="rounded-lg border p-3 text-sm">
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Refunding</span>
                <span className="font-medium">{fmt(amountToRefund)}</span>
              </div>
            </div>
          </div>
        )}

        {step === "confirm" && (
          <div className="space-y-3 text-sm">
            <Row label="Refund" value={fmt(amountToRefund)} strong />
            <Row label="To" value={shorten(transaction.from)} mono />
            <Row
              label="Leaves on this payment"
              value={fmt(remaining && amountToRefund ? remaining - amountToRefund : remaining)}
            />
            <p className="text-xs text-muted-foreground">
              Sent from this location&apos;s till wallet. It can&apos;t be undone from here.
            </p>
          </div>
        )}

        <DialogFooter className="gap-2 sm:gap-2">
          {step !== "details" && (
            <Button
              type="button"
              variant="ghost"
              onClick={() => setStep(step === "confirm" ? "amount" : "details")}
              disabled={busy}
            >
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back
            </Button>
          )}
          {step === "details" && (
            <Button type="button" onClick={() => setStep("amount")} disabled={!remaining || remaining === 0n}>
              Continue
            </Button>
          )}
          {step === "amount" && (
            <Button
              type="button"
              onClick={() => setStep("confirm")}
              disabled={!amountToRefund || amountToRefund === 0n || overRemaining}
            >
              Continue
            </Button>
          )}
          {step === "confirm" && (
            <Button type="button" onClick={() => void submit()} disabled={busy || !amountToRefund}>
              {busy ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Undo2 className="mr-2 h-4 w-4" />}
              Issue refund
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Row({ label, value, mono, strong }: { label: string; value: string; mono?: boolean; strong?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className={`${mono ? "font-mono text-xs" : ""} ${strong ? "font-semibold" : ""}`}>{value}</span>
    </div>
  )
}

function shorten(value: string) {
  return value.length > 14 ? `${value.slice(0, 8)}…${value.slice(-6)}` : value
}

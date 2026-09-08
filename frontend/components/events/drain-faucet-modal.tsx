"use client"

import { useEffect, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { AlertTriangle, Loader2 } from "lucide-react"

/** Typed exactly to enable the action — case-sensitive on purpose. */
const CONFIRM_WORD = "DRAIN"

interface DrainFaucetModalProps {
  open: boolean
  onOpenChange: () => void
  handleDrainFaucet: () => Promise<void>
  drainFaucetError: boolean
  /** Shown so the admin sees the amount they are about to move. */
  faucetBalance?: string
}

/**
 * Confirmation for draining the faucet.
 *
 * This transfers the entire faucet balance on-chain to the admin address and
 * cannot be undone, so it is gated behind typing the word rather than a single
 * click — the same treatment any irreversible, value-moving action deserves.
 */
export function DrainFaucetModal({
  open,
  onOpenChange,
  handleDrainFaucet,
  drainFaucetError,
  faucetBalance,
}: DrainFaucetModalProps) {
  const [drainError, setDrainError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false)
  const [confirmation, setConfirmation] = useState<string>("")

  useEffect(() => {
    setDrainError(null)
    setIsSubmitting(false)
    setConfirmation("")
  }, [open])

  const confirmed = confirmation.trim() === CONFIRM_WORD

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!confirmed || isSubmitting) return

    // Previously never set, so the pending state never showed and the button
    // stayed clickable through the request.
    setIsSubmitting(true)
    setDrainError(null)
    try {
      await handleDrainFaucet()
      if (!drainFaucetError) {
        onOpenChange()
        return
      }
      setDrainError("Encountered a server error while draining the faucet. Please try again later.")
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="mx-auto max-h-[90vh] w-[95vw] max-w-md overflow-y-auto">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="flex items-center gap-2 text-lg text-destructive sm:text-xl">
            <AlertTriangle className="h-5 w-5" />
            Drain faucet
          </DialogTitle>
          <DialogDescription className="text-sm">
            This sends the faucet&apos;s entire balance to the admin wallet. It happens on-chain and
            <strong> cannot be undone.</strong>
          </DialogDescription>
        </DialogHeader>

        <form className="space-y-4" onSubmit={handleSubmit}>
          <div className="space-y-2 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm">
            {faucetBalance && (
              <div className="flex items-baseline justify-between gap-3">
                <span className="text-muted-foreground">Amount to be moved</span>
                <span className="font-semibold">{faucetBalance} SFLUV</span>
              </div>
            )}
            <p className="text-muted-foreground">
              Any unredeemed QR codes will stop working once the faucet is empty, including codes already
              printed and handed out.
            </p>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="drain-confirm">
              Type <span className="font-mono font-semibold text-foreground">{CONFIRM_WORD}</span> to confirm
            </Label>
            <Input
              id="drain-confirm"
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
              placeholder={CONFIRM_WORD}
              autoComplete="off"
              spellCheck={false}
              disabled={isSubmitting}
            />
          </div>

          {drainError && (
            <div className="flex items-center gap-2 rounded-lg bg-red-50 p-3 text-sm text-red-600 dark:bg-red-900/20">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              <span>{drainError}</span>
            </div>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onOpenChange} disabled={isSubmitting}>
              Cancel
            </Button>
            <Button type="submit" variant="destructive" disabled={!confirmed || isSubmitting}>
              {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Drain faucet
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

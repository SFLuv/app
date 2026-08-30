"use client"

import { AlertTriangle } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

/**
 * The one-way-door warning, shown wherever somebody can start becoming a
 * merchant.
 *
 * It exists because the consequence is invisible at the moment of the click.
 * Choosing a merchant account is reversible right up until a location of yours
 * is approved, and permanent from that moment on — an approved listing is on
 * the public map with a wallet taking payments behind it, and there is no
 * honest way to unwind that into a personal account.
 *
 * Rendered from one component so every entry point into the flow says the same
 * thing. Two of them saying it differently would be worse than one of them not
 * saying it at all.
 */
export function MerchantAccountPermanenceNotice({
  open,
  onOpenChange,
  onConfirm,
  busy = false,
  confirmLabel = "Continue",
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
  busy?: boolean
  confirmLabel?: string
}) {
  return (
    <Dialog open={open} onOpenChange={(next) => (busy ? undefined : onOpenChange(next))}>
      <DialogContent>
        <DialogHeader>
          <div className="mb-2 flex h-10 w-10 items-center justify-center rounded-full bg-amber-100 dark:bg-amber-500/15">
            <AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400" />
          </div>
          <DialogTitle>A merchant account becomes permanent</DialogTitle>
          <DialogDescription asChild>
            <div className="space-y-3 text-left">
              <p>
                You can switch back to a personal account at any time while you have no locations
                on the SFLuv map and nothing waiting to be reviewed.
              </p>
              <p>
                Once any location of yours is approved, this stays a merchant account for good.
                An approved location is public and takes payments, so it cannot be unwound into a
                personal account.
              </p>
              <p>
                You can still cancel an application yourself while it is waiting to be reviewed.
              </p>
            </div>
          </DialogDescription>
        </DialogHeader>

        <DialogFooter className="gap-2 sm:gap-2">
          <Button variant="outline" disabled={busy} onClick={() => onOpenChange(false)}>
            Not now
          </Button>
          <Button
            className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
            disabled={busy}
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

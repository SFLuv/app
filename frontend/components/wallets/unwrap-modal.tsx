"use client"

import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { LocationPayoutCard } from "@/components/merchant/location-payout-card"
import type { AuthedLocation } from "@/types/location"

interface UnwrapModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  location: AuthedLocation
  onSuccess: () => void | Promise<void>
}

/**
 * Unwrap from the merchant wallet page. This is the same bank-backed flow as
 * the location's settings card, opened in a dialog: verify the business,
 * connect a bank, then unwrap. The merchant never types a crypto address —
 * the destination is the Bridge payout address provisioned for this location.
 */
export function UnwrapModal({ open, onOpenChange, location, onSuccess }: UnwrapModalProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-[560px]">
        <DialogHeader>
          <DialogTitle className="text-black dark:text-white">Unwrap to your bank</DialogTitle>
          <DialogDescription>
            Turns SFLUV from this location back into dollars in the business bank account. Your first unwrap each
            month is any amount; further unwraps that month must be at least $500.
          </DialogDescription>
        </DialogHeader>
        <LocationPayoutCard location={location} onUnwrapped={onSuccess} />
      </DialogContent>
    </Dialog>
  )
}

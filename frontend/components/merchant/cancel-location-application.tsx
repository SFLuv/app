"use client"

import { useState } from "react"
import { Loader2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { useLocation } from "@/context/LocationProvider"
import { useApp } from "@/context/AppProvider"
import type { AuthedLocation } from "@/types/location"

/**
 * Withdraws an application that is still waiting to be reviewed.
 *
 * Offered only while `approval` is null. An approved location is on the map
 * with money landing in a wallet behind it and is not something a button
 * should be able to take away; a rejected one has already been decided.
 *
 * Confirmed rather than immediate, because it cannot be undone: the listing is
 * retired, and getting back into the queue means filling the form in again.
 */
export function CancelLocationApplication({
  location,
  onCancelled,
}: {
  location: AuthedLocation
  onCancelled?: () => void
}) {
  const { cancelLocationApplication } = useLocation()
  const { refreshUserRecord } = useApp()
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  // Anything not yet approved. Rejected included: it blocks a revert to a
  // personal account the same way a pending one does, so it needs the same way
  // out. An approved location is on the map with money behind it and is not
  // something a button should be able to take away.
  if (location.approval === true) return null

  const confirm = async () => {
    setBusy(true)
    setError("")
    try {
      await cancelLocationApplication(location.id)
      setOpen(false)
      // The account's own record has to be re-read, not just the location list:
      // cancelling the last pending application is what reopens the way back to
      // a personal account, and settings reads that from the server.
      await refreshUserRecord()
      onCancelled?.()
    } catch (cancelError) {
      setError(
        cancelError instanceof Error
          ? cancelError.message
          : "Something went wrong cancelling this application.",
      )
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <Button
        variant="outline"
        className="border-red-300 text-red-700 hover:bg-red-50 dark:border-red-800 dark:text-red-300 dark:hover:bg-red-950/40"
        onClick={() => setOpen(true)}
      >
        Cancel this application
      </Button>

      <Dialog open={open} onOpenChange={(next) => (busy ? undefined : setOpen(next))}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Cancel this application?</DialogTitle>
            <DialogDescription>
              {location.name || "This location"} will be withdrawn. This cannot be undone.
            </DialogDescription>
          </DialogHeader>

          {error && (
            <p className="rounded-md border border-red-400/40 bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-500/10 dark:text-red-200">
              {error}
            </p>
          )}

          <DialogFooter className="grid grid-cols-2 gap-2 sm:flex-row sm:justify-end sm:gap-2">
            <Button
              variant="outline"
              className="w-full sm:w-40"
              disabled={busy}
              onClick={() => setOpen(false)}
            >
              Keep it
            </Button>
            <Button
              className="w-full bg-red-600 text-white hover:bg-red-700 sm:w-40"
              disabled={busy}
              onClick={() => void confirm()}
            >
              {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Cancel it
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

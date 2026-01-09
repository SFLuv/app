"use client"

import { useEffect, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { AlertTriangle } from "lucide-react"
import { Event } from "@/types/event"

interface DrainFaucetModalProps {
  open: boolean
  onOpenChange: () => void
  handleDrainFaucet: () => Promise<void>
  drainFaucetError: boolean
}

export function DrainFaucetModal({ open, onOpenChange, handleDrainFaucet, drainFaucetError }: DrainFaucetModalProps) {
  const [drainError, setDrainError]= useState<string | null>()
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false)

  useEffect(() => {
    setDrainError(null)
    setIsSubmitting(false)
  }, [open])

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {

    e.preventDefault()

    await handleDrainFaucet()
    if(!drainFaucetError) {
      onOpenChange()
    } else {
      setDrainError("Encountered a server error while draining faucet. Please try again later.")
    }

    setIsSubmitting(false)
  }


  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[95vw] max-w-md mx-auto max-h-[90vh] rounded-lg overflow-y-auto">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">Drain Faucet</DialogTitle>
          <DialogDescription className="text-sm">
            Are you sure you want to drain the faucet?
          </DialogDescription>
        </DialogHeader>
        {isSubmitting ?
          <div className="min-h-screen flex items-center justify-center">
            <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
          </div>
        :
          <form
          className="space-y-4 sm:space-y-6"
          onSubmit={handleSubmit}
          >

            {/* Details */}
            <div className="space-y-2">

              {/* Submit Button */}
              <div className="pt-2 text-center">
                <Button type="submit">
                  Drain
                </Button>
              </div>

              {drainError && (
                <div className="flex items-center gap-2 text-red-600 text-sm p-3 bg-red-50 dark:bg-red-900/20 rounded-lg">
                  <AlertTriangle className="h-4 w-4 flex-shrink-0" />
                  <span>{drainError}</span>
                </div>
              )}

            </div>
          </form>
        }
      </DialogContent>
    </Dialog>
  )
}

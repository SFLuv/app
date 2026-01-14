"use client"

import { useEffect, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { AlertTriangle } from "lucide-react"
import { Event } from "@/types/event"
import { useApp } from "@/context/AppProvider"

interface IdleModalProps {
  open: boolean
  onOpenChange: () => void
  getRemainingTime: () => number
}

export function IdleModal({ open, onOpenChange, getRemainingTime }: IdleModalProps) {
  const getTime = (): number => {
    return Math.floor(getRemainingTime() / 1000)
  }

  const [seconds, setSeconds] = useState<number>(getTime())
  const [timerInterval, setTimerInterval] = useState<NodeJS.Timeout | undefined>();

  const { status } = useApp()

  useEffect(() => {
    if(open) {
      const i = setInterval(() => {
        setSeconds(getTime())
      }, 1000)

      setTimerInterval(i)
    }
    else {
      clearInterval(timerInterval)
      setTimerInterval(undefined)
      setSeconds(getTime())
    }

    return () => {
      clearInterval(timerInterval)
      setTimerInterval(undefined)
      setSeconds(getTime())
    }
  }, [open])

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    onOpenChange()
  }


  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[95vw] max-w-md mx-auto max-h-[90vh] rounded-lg overflow-y-auto">
        <DialogHeader className="space-y-2 pb-2">
          <DialogTitle className="text-lg sm:text-xl">User Idle</DialogTitle>
          {status === "authenticated" ?
            <DialogDescription className="text-sm">
              You will be automatically logged out in {seconds} {seconds === 1 ? "second" : "seconds"}.
            </DialogDescription> :
            <DialogDescription className="text-sm">
              You have been automatically logged out.
            </DialogDescription>
          }
        </DialogHeader>
        <form
          className="space-y-4 sm:space-y-6"
          onSubmit={handleSubmit}
        >

          {/* Details */}
          <div className="space-y-2">

            {/* Submit Button */}
            <div className="pt-2 text-center">
              <Button type="submit">
                Close
              </Button>
            </div>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

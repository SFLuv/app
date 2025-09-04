"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Calendar, CheckCircle, Loader2 } from "lucide-react"

interface GoogleCalendarSyncProps {
  opportunities: Array<{
    id: string
    title: string
    date: string
    organizer: string
    location: {
      address: string
      city: string
      state: string
      zip: string
    }
  }>
}

export function GoogleCalendarSync({ opportunities }: GoogleCalendarSyncProps) {
  const [isSyncing, setIsSyncing] = useState(false)
  const [syncSuccess, setSyncSuccess] = useState(false)

  const handleSync = async () => {
    setIsSyncing(true)

    // Simulate API call to sync with Google Calendar
    try {
      // In a real app, this would call an API to perform OAuth and calendar sync
      await new Promise((resolve) => setTimeout(resolve, 2000))

      setSyncSuccess(true)
      setTimeout(() => setSyncSuccess(false), 5000) // Reset success state after 5 seconds
    } catch (error) {
      console.error("Failed to sync with Google Calendar:", error)
    } finally {
      setIsSyncing(false)
    }
  }

  return (
    <Button
      onClick={handleSync}
      disabled={isSyncing}
      className={`flex items-center space-x-2 ${
        syncSuccess ? "bg-green-600 hover:bg-green-700" : "bg-[#eb6c6c] hover:bg-[#d55c5c]"
      }`}
    >
      {isSyncing ? (
        <>
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Syncing...</span>
        </>
      ) : syncSuccess ? (
        <>
          <CheckCircle className="h-4 w-4" />
          <span>Synced!</span>
        </>
      ) : (
        <>
          <Calendar className="h-4 w-4" />
          <span>Sync to Google Calendar</span>
        </>
      )}
    </Button>
  )
}

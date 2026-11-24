"use client"

import { Loader2 } from "lucide-react"

import { NotificationHub } from "@/components/organization/notification-hub"
import { useApp } from "@/context/AppProvider"

/**
 * The Notification Hub: for the house organization's admins, who decide which
 * team hears about what, and for team leads and team admins, who decide where
 * each of their team's notifications goes. The server checks every request;
 * the menu only links here for people it would let in.
 */
export default function NotificationHubPage() {
  const { status } = useApp()

  if (status === "loading") {
    return (
      <div className="flex min-h-[70vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (status !== "authenticated") {
    return <p className="container mx-auto text-sm text-muted-foreground">Sign in to use the Notification Hub.</p>
  }

  return (
    <div className="container mx-auto">
      <NotificationHub />
    </div>
  )
}

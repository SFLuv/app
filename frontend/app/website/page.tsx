"use client"

import { Loader2 } from "lucide-react"

import { WebsitePanel } from "@/components/admin/website/website-panel"
import { useApp } from "@/context/AppProvider"

/**
 * The website tools, for admins and for anyone holding a website credential.
 * The panel shows only the tools the person may use, and the server checks
 * every request. Granting access lives in the Admin Panel (Users → Website
 * access), since only admins may do it.
 */
export default function WebsitePage() {
  const { status } = useApp()

  if (status === "loading") {
    return (
      <div className="flex min-h-[70vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (status !== "authenticated") {
    return <p className="container mx-auto p-4 text-sm text-muted-foreground">Sign in to use the website tools.</p>
  }

  return (
    <div className="container mx-auto px-3 py-4 sm:p-4">
      <WebsitePanel />
    </div>
  )
}

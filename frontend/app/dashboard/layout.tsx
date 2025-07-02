"use client"

import type React from "react"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { DashboardSidebar } from "@/components/dashboard/sidebar"
import { SidebarProvider, SidebarInset, SidebarTrigger } from "@/components/ui/sidebar"

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode
}) {
  const router = useRouter()
  const { status, user } = useApp()

  // Redirect to login if not authenticated
  useEffect(() => {
    if (status === "unauthenticated") {
      router.push("/")
    }
  }, [status, router])

  if (status === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  if (!user) {
    return null
  }

  return (
    <SidebarProvider>
      <div className="flex h-screen w-full overflow-hidden">
        <DashboardSidebar />
        <SidebarInset className="flex flex-col w-full">
          <header className="h-16 border-b flex items-center px-4 bg-secondary w-full">
            <SidebarTrigger className="mr-4" />
            <h1 className="text-xl font-semibold text-black dark:text-white">Dashboard</h1>
          </header>
          <main className="flex-1 overflow-auto p-6 bg-[#d3d3d3] dark:bg-[#1a1a1a] w-full">{children}</main>
        </SidebarInset>
      </div>
    </SidebarProvider>
  )
}

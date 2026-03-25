"use client"

import type React from "react"
import type { Metadata } from "next"
import { Inter } from "next/font/google"
import "./globals.css"
import { ThemeProvider } from "@/components/theme-provider"
import Providers from "@/context/Providers"
import { useApp } from "@/context/AppProvider"
import { DashboardSidebar } from "@/components/dashboard/sidebar"
import { SidebarProvider, SidebarInset, SidebarTrigger } from "@/components/ui/sidebar"
import { Button } from "@/components/ui/button"
import { useEffect, useState } from "react"
import { usePathname, useSearchParams } from "next/navigation"


export default function Sidebar({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {

  const { status, login } = useApp();
  const [open, setOpen] = useState(false);
  const pathname = usePathname()
  const search = useSearchParams()
  const shouldHideSidebar = pathname == "/faucet/redeem" || pathname.startsWith("/photos/") || search.get("sidebar") === "false"

  useEffect(() => {
    if(status == "authenticated") setOpen(true)
  }, [status])

  if (shouldHideSidebar) return children

  if (status === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <SidebarProvider open={open} onOpenChange={setOpen} defaultOpen={status === "authenticated"}>
      <div className="flex h-screen w-full overflow-hidden">
        <DashboardSidebar />
        <SidebarInset className="flex flex-col w-full">
          <header className="h-16 border-b border-border/70 flex flex-row items-center px-4 bg-card/85 backdrop-blur-md w-full">
            <SidebarTrigger className="mr-4" />
            <h1 className="text-xl font-semibold mr-auto text-black dark:text-white">Dashboard</h1>
            {status !== "authenticated" && <Button
              variant="default"
              size="lg"
              className="margin-left-auto"
              onClick={() => login()}
            >
              Connect
            </Button>}
          </header>
          <main className="flex-1 overflow-auto p-3 sm:p-6 w-full">{children}</main>
        </SidebarInset>
      </div>
    </SidebarProvider>
  )
}

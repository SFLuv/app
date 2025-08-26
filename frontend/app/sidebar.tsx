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


export default function Sidebar({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {

  const { status, login } = useApp();
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if(status == "authenticated") setOpen(true)
  }, [status])

  if (status === "loading") return children
  return (
    <SidebarProvider open={open} onOpenChange={setOpen} defaultOpen={status === "authenticated"}>
      <div className="flex h-screen w-full overflow-hidden">
        <DashboardSidebar />
        <SidebarInset className="flex flex-col w-full">
          <header className="h-16 border-b flex flex-row items-center items-end px-4 bg-secondary w-full">
            <SidebarTrigger className="mr-4" />
            <h1 className="text-xl font-semibold mr-auto text-black dark:text-white">Dashboard</h1>
            {status !== "authenticated" && <Button
              variant="default"
              size="lg"
              className="bg-[#eb6c6c] hover:bg-[#d55c5c] margin-left-auto"
              onClick={() => login()}
            >
              Connect
            </Button>}
          </header>
          <main className="flex-1 overflow-auto p-6 bg-[#d3d3d3] dark:bg-[#1a1a1a] w-full">{children}</main>
        </SidebarInset>
      </div>
    </SidebarProvider>
  )
}

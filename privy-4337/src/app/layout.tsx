import type React from "react"
import type { Metadata } from "next"
import { Inter } from "next/font/google"
import "./globals.css"
import { Toaster } from "sonner"
import AppProvider from "@/providers/AppProvider"
import { CSSProperties } from "react"

const inter = Inter({ subsets: ["latin"] })

export const metadata: Metadata = {
  title: "SFLUV 4337 Wallet",
  description: "A web hosted wallet with 4337 integration.",
}


export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className={inter.className}>
        <AppProvider>{children}</AppProvider>
        <Toaster richColors position="top-right" />
      </body>
    </html>
  )
}

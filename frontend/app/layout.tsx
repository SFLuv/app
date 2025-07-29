import type React from "react"
import type { Metadata } from "next"
import { Inter } from "next/font/google"
import "./globals.css"
import { ThemeProvider } from "@/components/theme-provider"
import Providers from "@/context/Providers"
import Sidebar from "./sidebar"

const inter = Inter({ subsets: ["latin"] })

export const metadata: Metadata = {
  title: "SFLuv - Local Economy Management",
  description:
    "A central app for managing payments, users, and merchant discovery for a local economy management tool.",
    generator: 'v0.dev',
  icons: "/icon.png"
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {

  return (
    <html lang="en" suppressHydrationWarning>
      <body className={inter.className}>
        <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
          <Providers>
            <Sidebar>{children}</Sidebar>
          </Providers>
        </ThemeProvider>
      </body>
    </html>
  )
}

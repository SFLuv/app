import type React from "react"
import type { Metadata } from "next"
import { Inter } from "next/font/google"
import "./globals.css"
import { Toaster } from "sonner"
import AppProvider from "@/providers/AppProvider"
import { CSSProperties } from "react"
import { PrivyProvider } from "@privy-io/react-auth"

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
      <PrivyProvider
        appId={process.env.NEXT_PUBLIC_PRIVY_APP_ID || ""}
        config={{
          loginMethods: ["wallet", "email"],
          appearance: {
            theme: "dark",
            accentColor: "#daa520",
            logo: "/images/logo.png",
          },
          embeddedWallets: {
            ethereum: {
                createOnLogin: 'users-without-wallets',
            },
          }
        }}
      >
        {children}
      </PrivyProvider>
      </body>
    </html>
  )
}

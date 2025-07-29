"use client"

import { ReactNode, useMemo } from "react"
import AppProvider from "./AppProvider"
import { PrivyProvider } from "@privy-io/react-auth"
import { PRIVY_ID } from "@/lib/constants"
import { useTheme } from "next-themes"

const Providers = ({ children }: { children: ReactNode }) => {
  const { resolvedTheme } = useTheme()
  return (
    <PrivyProvider
      appId={PRIVY_ID}
      config={{
        loginMethods: ["wallet", "email", "google"],
        appearance: {
          theme: resolvedTheme as any || "light",
          accentColor: "#eb6c6c",
          logo: "/icon.png",
          loginMessage: "Connect to the SFLuv Dashboard!"
        },
        embeddedWallets: {
          ethereum: {
              createOnLogin: 'users-without-wallets',
          },
          showWalletUIs: false
        }
      }}
    >
      <AppProvider>
        {children}
      </AppProvider>
    </PrivyProvider>
  )
}

export default Providers
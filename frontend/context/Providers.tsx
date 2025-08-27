"use client"

import { ReactNode, Suspense, useMemo } from "react"
import AppProvider from "./AppProvider"
import { PrivyProvider } from "@privy-io/react-auth"
import { CHAIN, PRIVY_ID } from "@/lib/constants"
import { useTheme } from "next-themes"
import LocationProvider from "./LocationProvider"
import ContactsProvider from "./ContactsProvider"

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
        },
        defaultChain: CHAIN,
        supportedChains: [CHAIN]
      }}
    >
      <AppProvider>
        <ContactsProvider>
        <LocationProvider>
        <Suspense fallback={
          <div className="min-h-screen flex items-center justify-center">
            <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
          </div>
        }>
        {children}
        </Suspense>
        </LocationProvider>
        </ContactsProvider>
      </AppProvider>
    </PrivyProvider>
  )
}

export default Providers
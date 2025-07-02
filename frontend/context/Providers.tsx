"use client"

import { ReactNode } from "react"
import AppProvider from "./AppProvider"
import { PrivyProvider } from "@privy-io/react-auth"
import { PRIVY_ID } from "@/lib/constants"

const Providers = ({ children }: { children: ReactNode }) => {
  return (
    <PrivyProvider
      appId={PRIVY_ID}
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
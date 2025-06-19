"use client"

import { PrivyProvider } from "@privy-io/react-auth"

export default function AppProvider({ children }: any) {
  return (
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
  )
}
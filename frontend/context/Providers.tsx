"use client"

import { ReactNode, Suspense, useMemo } from "react"
import AppProvider from "./AppProvider"
import { PrivyProvider } from "@privy-io/react-auth"
import { CHAIN, PRIVY_CLIENT_ID, PRIVY_ID } from "@/lib/constants"
import { useTheme } from "next-themes"
import LocationProvider from "./LocationProvider"
import ContactsProvider from "./ContactsProvider"
import TransactionProvider from "./TransactionProvider"

const Providers = ({ children }: { children: ReactNode }) => {
  const { resolvedTheme } = useTheme()
  const customOAuthRedirectUrl =
    process.env.NEXT_PUBLIC_PRIVY_CUSTOM_OAUTH_REDIRECT_URL?.trim() || undefined
  const loginMethods = useMemo(() => {
    const methods = ["email", "google", "apple"] as Array<"email" | "google" | "apple" | "passkey">
    if (process.env.NEXT_PUBLIC_PRIVY_ENABLE_PASSKEY_LOGIN?.trim().toLowerCase() === "true") {
      methods.push("passkey")
    }
    return methods
  }, [])

  const privyConfig = useMemo(() => ({
    loginMethods,
    appearance: {
      theme: resolvedTheme as any || "light",
      accentColor: "#eb6c6c" as `#${string}`,
      logo: "/icon.png",
      loginMessage: "Connect to the SFLuv Dashboard!"
    },
    captchaEnabled: true,
    mfa: {
      noPromptOnMfaRequired: false,
    },
    customOAuthRedirectUrl,
    externalWallets: {
      coinbaseWallet: {
        connectionOptions: "eoaOnly" as const
      }
    },
    embeddedWallets: {
      ethereum: {
          createOnLogin: 'users-without-wallets' as const,
      },
      showWalletUIs: false
    },
    defaultChain: CHAIN,
    supportedChains: [CHAIN]
  }), [customOAuthRedirectUrl, loginMethods, resolvedTheme])

  return (
    <PrivyProvider
      appId={PRIVY_ID}
      clientId={PRIVY_CLIENT_ID}
      config={privyConfig}
    >
      <AppProvider>
        <ContactsProvider>
        <LocationProvider>
        <TransactionProvider>
        <Suspense fallback={
          <div className="min-h-screen flex items-center justify-center">
            <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
          </div>
        }>
        {children}
        </Suspense>
        </TransactionProvider>
        </LocationProvider>
        </ContactsProvider>
      </AppProvider>
    </PrivyProvider>
  )
}

export default Providers

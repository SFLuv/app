"use client"

import type { ReactNode } from "react"
import { useApp } from "@/context/AppProvider"
import MerchantOnboardingPage from "@/app/merchant-onboarding/page"

/**
 * The wall for a merchant who has not listed a shop yet: while the flag holds,
 * the onboarding screen IS the app — rendered in place of whatever route they
 * are on, not navigated to. An earlier design redirected instead, and a
 * navigation can lose a race against the other replaces that fire during
 * sign-in; a render cannot lose anything. The URL is left alone on purpose —
 * AppProvider still nudges it toward /merchant-onboarding when the router
 * cooperates, but nothing depends on that succeeding.
 *
 * Mounted in Providers below Location/Contacts/Transaction, because the
 * onboarding form reads those contexts.
 */
export function MerchantOnboardingWall({ children }: { children: ReactNode }) {
  const { merchantOnboardingWalled } = useApp()
  if (merchantOnboardingWalled) {
    return <MerchantOnboardingPage />
  }
  return <>{children}</>
}

"use client"

import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { Store } from "lucide-react"
import { Button } from "@/components/ui/button"
import { isChromeFreeRoute } from "@/lib/app-chrome"
import { MERCHANT_ONBOARDING_PATH } from "@/lib/merchant-onboarding"

/**
 * The way back to onboarding, kept in front of a gated merchant on every
 * screen.
 *
 * A merchant who has not listed a shop is free to look around — that was the
 * product owner's call — but the backend refuses their writes, so the app owes
 * them a standing explanation of why the buttons they can see will not work.
 * It does not cover the page: this is a notice, not a second gate.
 */
export function MerchantOnboardingBanner() {
  const pathname = usePathname()
  const search = useSearchParams()
  const router = useRouter()

  // Redundant on the screen it points at, and unwelcome on the screens that
  // drop the app chrome entirely — see isChromeFreeRoute.
  if (pathname.startsWith(MERCHANT_ONBOARDING_PATH) || isChromeFreeRoute(pathname, search)) {
    return null
  }

  return (
    <div className="pointer-events-none fixed inset-x-0 bottom-0 z-[70] px-3 pb-3 sm:px-6 sm:pb-6">
      <div className="pointer-events-auto mx-auto flex w-full max-w-3xl flex-col gap-3 rounded-2xl border border-[#eb6c6c]/50 bg-card/95 p-4 shadow-[0_1px_3px_hsl(var(--foreground)/0.08),0_16px_40px_hsl(var(--foreground)/0.18)] backdrop-blur-md sm:flex-row sm:items-center sm:gap-4">
        <Store className="hidden h-6 w-6 shrink-0 text-[#eb6c6c] sm:block" />
        <div className="min-w-0 flex-1 space-y-1">
          <p className="text-sm font-semibold text-foreground">
            Finish setting up your merchant account
          </p>
          <p className="text-sm leading-6 text-muted-foreground">
            You can look around, but sending, receiving and everything else stays
            switched off until your business is listed.
          </p>
        </div>
        <Button
          type="button"
          className="w-full shrink-0 bg-[#eb6c6c] hover:bg-[#d55c5c] sm:w-auto"
          onClick={() => router.push(MERCHANT_ONBOARDING_PATH)}
        >
          List your business
        </Button>
      </div>
    </div>
  )
}

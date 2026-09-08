"use client"

import { useEffect, useRef, useState } from "react"
import { useRouter } from "next/navigation"
import { Store } from "lucide-react"

import { Button } from "@/components/ui/button"
import { SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar"
import { MerchantAccountPermanenceNotice } from "@/components/merchant/merchant-account-permanence-notice"
import { useApp } from "@/context/AppProvider"
import { MERCHANT_ONBOARDING_PATH } from "@/lib/merchant-onboarding"
import { cn } from "@/lib/utils"

/**
 * The one-time merchant offer in the sidebar, for somebody who signed up on the
 * mobile app and has now arrived on the web app.
 *
 * The mobile signup never asks which kind of account this is — it registers a
 * personal one — so these people are the only ones who have never been put the
 * question. The web app puts it once, here, and from then on the option lives
 * in Settings where somebody can go looking for it rather than being asked
 * again on every sign-in.
 *
 * The stamp is written as soon as the offer renders, not when it is answered.
 * "Shown once" has to mean shown once even to somebody who ignores it and
 * closes the tab, or it would greet them again every sign-in — which is the
 * nagging this replaced.
 */
export function BecomeMerchantPrompt() {
  const { showWebMerchantPrompt, dismissWebMerchantPrompt, setOwnAccountType } = useApp()
  const router = useRouter()

  // Held locally because dismissWebMerchantPrompt clears the context flag the
  // moment it stamps. Without this the item would vanish out from under the
  // person on the same render it appeared.
  const [visible, setVisible] = useState(false)
  const [noticeOpen, setNoticeOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const stamped = useRef(false)

  useEffect(() => {
    if (!showWebMerchantPrompt || stamped.current) return
    stamped.current = true
    setVisible(true)
    void dismissWebMerchantPrompt()
  }, [showWebMerchantPrompt, dismissWebMerchantPrompt])

  if (!visible) return null

  const becomeMerchant = async () => {
    setBusy(true)
    setError("")
    try {
      await setOwnAccountType("merchant")
      setNoticeOpen(false)
      setVisible(false)
      router.push(MERCHANT_ONBOARDING_PATH)
    } catch (switchError) {
      setError(
        switchError instanceof Error
          ? switchError.message
          : "Unable to switch this account right now.",
      )
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <SidebarMenuItem>
        <SidebarMenuButton asChild tooltip="Become a merchant">
          <Button
            variant="ghost"
            className={cn(
              "w-full justify-start rounded-md text-[#eb6c6c] transition-colors hover:bg-[#eb6c6c] hover:text-white",
            )}
            onClick={() => setNoticeOpen(true)}
          >
            <Store className="mr-2 h-4 w-4" />
            <span>Become a merchant</span>
          </Button>
        </SidebarMenuButton>
      </SidebarMenuItem>

      <MerchantAccountPermanenceNotice
        open={noticeOpen}
        onOpenChange={(next) => {
          setNoticeOpen(next)
          if (!next) {
            // Declining takes the offer away for good. It is still in Settings,
            // and that is where somebody who changes their mind should find it.
            setVisible(false)
            setError("")
          }
        }}
        onConfirm={() => void becomeMerchant()}
        busy={busy}
        confirmLabel={busy ? "Switching..." : "Become a merchant"}
      />

      {error && (
        <p className="px-2 pb-2 text-xs text-red-600 dark:text-red-300">{error}</p>
      )}
    </>
  )
}

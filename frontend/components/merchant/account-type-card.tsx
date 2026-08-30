"use client"

import { useCallback, useEffect, useState } from "react"
import { useRouter } from "next/navigation"
import { Loader2, Store, User } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { MerchantAccountPermanenceNotice } from "@/components/merchant/merchant-account-permanence-notice"
import { useApp, type MerchantRevertEligibility } from "@/context/AppProvider"
import { MERCHANT_ONBOARDING_PATH } from "@/lib/merchant-onboarding"

/**
 * Where the merchant choice lives from now on.
 *
 * The navbar used to carry a standing "become a merchant" button; it is a
 * settings decision, made once, and a permanent one past a certain point, so it
 * belongs on the screen people go to when they mean to change something rather
 * than in the furniture of every page.
 *
 * Which way round it can be turned is the server's answer, not this screen's.
 * The client's copy of the location list is whatever the last profile fetch
 * returned, and an admin approving a listing between that fetch and this click
 * is exactly the case where the two disagree — so the way back is offered on a
 * fresh read and the switch itself is refused server-side regardless.
 */
export function AccountTypeCard() {
  const { user, setOwnAccountType, getMerchantRevertEligibility } = useApp()
  const router = useRouter()

  const [eligibility, setEligibility] = useState<MerchantRevertEligibility | null>(null)
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [noticeOpen, setNoticeOpen] = useState(false)

  const isMerchant = user?.accountType === "merchant"

  const refreshEligibility = useCallback(async () => {
    if (!isMerchant) {
      setEligibility(null)
      return
    }
    setLoading(true)
    try {
      setEligibility(await getMerchantRevertEligibility())
    } catch {
      // Left null, which shows nothing rather than an offer that might be
      // wrong. The switch is refused server-side either way.
      setEligibility(null)
    } finally {
      setLoading(false)
    }
  }, [getMerchantRevertEligibility, isMerchant])

  useEffect(() => {
    void refreshEligibility()
  }, [refreshEligibility])

  const becomeMerchant = async () => {
    setBusy(true)
    setError("")
    try {
      await setOwnAccountType("merchant")
      setNoticeOpen(false)
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

  const revertToPersonal = async () => {
    setBusy(true)
    setError("")
    try {
      await setOwnAccountType("regular")
    } catch (switchError) {
      setError(
        switchError instanceof Error
          ? switchError.message
          : "Unable to switch this account right now.",
      )
      // The refusal came with counts attached; re-reading them is what makes
      // the explanation below match the reason it was refused.
      void refreshEligibility()
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card className="mt-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-black dark:text-white">
          {isMerchant ? <Store className="h-5 w-5" /> : <User className="h-5 w-5" />}
          Account Type
        </CardTitle>
        <CardDescription>
          {isMerchant
            ? "This is a merchant account. You can list locations on the SFLuv map."
            : "This is a personal account. You spend and receive SFLuv in your own wallet."}
        </CardDescription>
      </CardHeader>

      <CardContent className="space-y-4">
        {!isMerchant && (
          <>
            <p className="text-sm leading-relaxed text-muted-foreground">
              Run a business that wants to accept SFLuv? Switching to a merchant account lets you
              apply to put your locations on the map. You can switch back while you have no
              locations approved and nothing waiting to be reviewed — after that it is permanent.
            </p>
            <Button
              className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
              disabled={busy}
              onClick={() => setNoticeOpen(true)}
            >
              Become a merchant
            </Button>
          </>
        )}

        {isMerchant && loading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Checking your locations...
          </div>
        )}

        {isMerchant && !loading && eligibility?.can_revert && (
          <>
            <p className="text-sm leading-relaxed text-muted-foreground">
              You have no locations on the map and nothing waiting to be reviewed, so this account
              can still go back to being a personal one. Once any location of yours is approved,
              it stays a merchant account for good.
            </p>
            <Button variant="outline" disabled={busy} onClick={() => void revertToPersonal()}>
              {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Switch back to a personal account
            </Button>
          </>
        )}

        {isMerchant && !loading && eligibility && !eligibility.can_revert && (
          <p className="text-sm leading-relaxed text-muted-foreground">
            {eligibility.approved_locations > 0
              ? "You have a location on the SFLuv map, so this stays a merchant account. An approved location is public and takes payments, which cannot be unwound into a personal account."
              : "You have an application waiting to be reviewed. Cancel it from your locations if you want to go back to a personal account."}
          </p>
        )}

        {error && (
          <p className="rounded-md border border-red-400/40 bg-red-50 px-4 py-3 text-sm text-red-800 dark:bg-red-500/10 dark:text-red-200">
            {error}
          </p>
        )}
      </CardContent>

      <MerchantAccountPermanenceNotice
        open={noticeOpen}
        onOpenChange={(next) => {
          setNoticeOpen(next)
          if (!next) setError("")
        }}
        onConfirm={() => void becomeMerchant()}
        busy={busy}
        confirmLabel={busy ? "Switching..." : "Become a merchant"}
      />
    </Card>
  )
}

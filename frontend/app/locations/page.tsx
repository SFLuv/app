"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useRouter } from "next/navigation"
import { ArrowDownLeft, ArrowLeftRight, Check, Copy, RefreshCw, Send, Store, Wallet } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { LocationApprovalFlow } from "@/components/merchant/location-approval-flow"
import { CancelLocationApplication } from "@/components/merchant/cancel-location-application"
import { ReceiveCryptoModal } from "@/components/wallets/receive-crypto-modal"
import { SendCryptoModal } from "@/components/wallets/send-crypto-modal"
import { UnwrapModal } from "@/components/wallets/unwrap-modal"
import { WalletBalanceCard } from "@/components/wallets/wallet-balance-card"
import { useApp } from "@/context/AppProvider"
import { useUnwrapEnabled } from "@/context/ChainConfigProvider"
import { useToast } from "@/hooks/use-toast"
import { ensureGooglePlacesScript, hasGoogleMapsPlaces } from "@/lib/google-places"
import {
  getLocationApplicationStatus,
  MERCHANT_ONBOARDING_PATH,
  MERCHANT_SUPPORT_EMAIL,
} from "@/lib/merchant-onboarding"

/**
 * Where a merchant manages the till a shop is paid into — the slot Connected
 * Wallets occupies for everyone else.
 *
 * A location's wallet is one of the merchant's own smart accounts, derived at
 * its own index when the listing was created, so it arrives through the same
 * `wallets` list as any other and the send/receive components work on it
 * unchanged. Nothing here mints or moves a wallet; the backend glues one to the
 * location at creation and this only ever reads which.
 */
export default function LocationsPage() {
  const router = useRouter()
  const { toast } = useToast()
  const unwrapEnabled = useUnwrapEnabled()
  const {
    user,
    userLocations,
    wallets,
    walletsStatus,
    status,
    refreshUserRecord,
    refreshWallets,
    merchantOnboardingRequired,
  } = useApp()

  const [selectedLocationId, setSelectedLocationId] = useState<number | null>(null)
  const [locationCountBeforeApply, setLocationCountBeforeApply] = useState<number | null>(null)
  const [balance, setBalance] = useState<number | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [addressCopied, setAddressCopied] = useState(false)
  const [showSendModal, setShowSendModal] = useState(false)
  const [showReceiveModal, setShowReceiveModal] = useState(false)
  const [showUnwrapModal, setShowUnwrapModal] = useState(false)
  const [showTipSendModal, setShowTipSendModal] = useState(false)
  const [showTipReceiveModal, setShowTipReceiveModal] = useState(false)
  const [tipBalance, setTipBalance] = useState<number | null>(null)
  const [applying, setApplying] = useState(false)
  const [googleReady, setGoogleReady] = useState(false)
  const [googleLoadError, setGoogleLoadError] = useState<string | null>(null)

  const isMerchantAccount = user?.accountType === "merchant" || user?.isMerchant === true

  const sortedLocations = useMemo(
    // Oldest first: a merchant's original shop is the one they think of as
    // theirs, so it is the one the page opens on.
    () => [...userLocations].sort((a, b) => a.id - b.id),
    [userLocations],
  )

  const selectedLocation = useMemo(
    () => sortedLocations.find((location) => location.id === selectedLocationId) ?? sortedLocations[0],
    [selectedLocationId, sortedLocations],
  )

  const paymentAddress = (selectedLocation?.pay_to_address || "").trim()

  const locationWallet = useMemo(() => {
    if (paymentAddress === "") return undefined
    return wallets.find((wallet) => wallet.address?.toLowerCase() === paymentAddress.toLowerCase())
  }, [paymentAddress, wallets])

  // The tipping account, when the location has one that is not just the till
  // under another name.
  const tipAddress = useMemo(() => {
    const raw = (selectedLocation?.tip_to_address || "").trim()
    if (raw === "" || raw.toLowerCase() === paymentAddress.toLowerCase()) return ""
    return raw
  }, [paymentAddress, selectedLocation])

  const tipWallet = useMemo(() => {
    if (tipAddress === "") return undefined
    return wallets.find((wallet) => wallet.address?.toLowerCase() === tipAddress.toLowerCase())
  }, [tipAddress, wallets])

  // The session's wallet list is built at sign-in, so a till minted after that
  // — a location approved mid-session, most commonly — is missing from it
  // until something rebuilds the list. The personal-wallets page used to be
  // that something, purely by accident of its own mount effect; this page has
  // to be able to heal itself. Once per address, not in a loop: an address
  // whose wallet cannot be built would otherwise refetch forever.
  const walletsHealRequestedRef = useRef<Set<string>>(new Set())
  useEffect(() => {
    if (status !== "authenticated" || walletsStatus === "loading") return
    const missing = [paymentAddress, tipAddress].filter(
      (address) =>
        address !== "" &&
        !wallets.some((wallet) => wallet.address?.toLowerCase() === address.toLowerCase()),
    )
    const fresh = missing.filter((address) => !walletsHealRequestedRef.current.has(address))
    if (fresh.length === 0) return
    fresh.forEach((address) => walletsHealRequestedRef.current.add(address))
    void refreshWallets()
  }, [paymentAddress, refreshWallets, status, tipAddress, wallets, walletsStatus])

  // Tip balance follows the tipping wallet the way the main balance follows
  // the till: blanked on switch so one shop's tips never show under another.
  useEffect(() => {
    let cancelled = false
    setTipBalance(null)
    if (!tipWallet) return
    tipWallet
      .getSFLUVBalanceFormatted()
      .then((value) => {
        if (!cancelled) setTipBalance(value)
      })
      .catch(console.error)
    return () => {
      cancelled = true
    }
  }, [tipWallet])

  // Showing the shop they just applied for is the only acknowledgement the form
  // leaves behind — it closes itself on success, and the page would otherwise
  // sit on the previous location as though nothing had happened. Waiting on the
  // count to grow rather than on the save returning: the refetch that carries
  // the new listing lands a render or two later, and switching before it does
  // would just re-select the shop we were already on.
  useEffect(() => {
    if (locationCountBeforeApply === null) return
    if (sortedLocations.length <= locationCountBeforeApply) return
    setSelectedLocationId(sortedLocations[sortedLocations.length - 1].id)
    setLocationCountBeforeApply(null)
  }, [locationCountBeforeApply, sortedLocations])

  // Regular accounts have no locations to manage. A merchant with none stays
  // here and is shown the setup view below — bouncing them to the form was what
  // made "no locations yet" indistinguishable from "you must fill this in now".
  useEffect(() => {
    if (status !== "authenticated") return
    if (!isMerchantAccount) router.replace("/wallets")
  }, [isMerchantAccount, router, status])

  useEffect(() => {
    if (!applying || googleReady) return
    let mounted = true

    const load = async () => {
      if (typeof window === "undefined") return
      if (hasGoogleMapsPlaces()) {
        setGoogleReady(true)
        return
      }
      await ensureGooglePlacesScript()
      if (mounted) setGoogleReady(true)
    }

    load().catch((error) => {
      if (!mounted) return
      console.error(error)
      setGoogleLoadError("Failed to load Google Places search. Please refresh and try again.")
    })

    return () => {
      mounted = false
    }
  }, [applying, googleReady])

  const updateBalance = useCallback(async () => {
    if (!locationWallet) {
      setBalance(null)
      return
    }
    try {
      setBalance(await locationWallet.getSFLUVBalanceFormatted())
    } catch (error) {
      console.error(error)
    }
  }, [locationWallet])

  // The balance belongs to whichever till is on screen, so switching locations
  // has to blank it rather than leave the previous shop's takings showing under
  // the new shop's name.
  useEffect(() => {
    setBalance(null)
    void updateBalance()
  }, [updateBalance])

  const handleRefresh = async () => {
    setRefreshing(true)
    try {
      await updateBalance()
    } finally {
      setRefreshing(false)
    }
  }

  const handleCopyAddress = async () => {
    if (paymentAddress === "") return
    try {
      await navigator.clipboard.writeText(paymentAddress)
      setAddressCopied(true)
      toast({ title: "Address copied", description: "This location's wallet address is on your clipboard." })
      setTimeout(() => setAddressCopied(false), 2000)
    } catch (error) {
      console.error(error)
    }
  }

  if (status === "loading" || walletsStatus === "loading") {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="h-12 w-12 animate-spin rounded-full border-b-2 border-t-2 border-[#eb6c6c]" />
      </div>
    )
  }

  // The merchant hub in its empty state: an account that has said it is a
  // merchant but has nothing listed, either because it has never applied or
  // because it withdrew the one application it had. This is where Cancel on the
  // form lands, so it has to be a real screen rather than a redirect back into
  // the form the merchant just left.
  if (sortedLocations.length === 0) {
    return (
      <div className="mx-auto w-full max-w-2xl px-3 pb-6 pt-2 sm:px-6">
        <h1 className="text-2xl font-bold text-black dark:text-white sm:text-3xl">Locations</h1>
        <Card className="mt-6 border-border/70">
          <CardContent className="space-y-5 p-6 text-center sm:p-8">
            <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-[#eb6c6c]/10">
              <Store className="h-6 w-6 text-[#eb6c6c]" />
            </div>
            <div className="space-y-2">
              <h2 className="text-xl font-semibold text-black dark:text-white">
                Set up your merchant account
              </h2>
              {/* Kept as text: it is the reason sending and receiving are
                  refused, and somebody who never hovers has no way to find that
                  out. It says nothing once a listing exists. */}
              {merchantOnboardingRequired && (
                <p className="text-sm text-muted-foreground">
                  Sending and receiving stay switched off until you list a business.
                </p>
              )}
            </div>
            <Button
              className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
              onClick={() => router.push(MERCHANT_ONBOARDING_PATH)}
            >
              Start application
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (!selectedLocation) return null

  const applicationStatus = getLocationApplicationStatus(selectedLocation.approval)
  const addressSummary = [selectedLocation.street, selectedLocation.city, selectedLocation.state]
    .filter(Boolean)
    .join(", ")
  const shortAddress =
    paymentAddress.length > 12
      ? `${paymentAddress.slice(0, 6)}...${paymentAddress.slice(-4)}`
      : paymentAddress

  // The stepper takes the whole screen, so it is returned instead of the page
  // rather than rendered inside it.
  if (applying) {
    if (googleLoadError) {
      return (
        <Card className="mx-auto mt-6 max-w-2xl border-[#eb6c6c]/40 bg-[#eb6c6c]/5">
          <CardContent className="p-6 text-sm text-[#8f2e2e]">{googleLoadError}</CardContent>
        </Card>
      )
    }
    if (!googleReady) {
      return (
        <div className="flex min-h-[60vh] items-center justify-center">
          <div className="h-12 w-12 animate-spin rounded-full border-b-2 border-t-2 border-[#eb6c6c]" />
        </div>
      )
    }

    return (
      <LocationApprovalFlow
        // Carried from the shop they started with rather than whichever one the
        // chooser happens to be on: the business and the person we ring about it
        // are the same across all of them, so a second application should not
        // read differently depending on what was on screen when it was opened.
        prefillFrom={sortedLocations[0]}
        onCancel={() => setApplying(false)}
        onFinish={() => setApplying(false)}
        onSubmitted={() => {
          setLocationCountBeforeApply(sortedLocations.length)
          void (async () => {
            await refreshUserRecord()
            // Wallets are minted at approval, not now, so the new listing has
            // none yet. Refreshed anyway because the same read is what picks up
            // an approval that landed while this form was open.
            await refreshWallets()
          })()
        }}
      />
    )
  }

  return (
    <div className="mx-auto w-full max-w-2xl space-y-4 px-3 pb-6 pt-2 sm:space-y-6 sm:px-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-black dark:text-white sm:text-3xl">Locations</h1>
        </div>
        <div className="flex items-center gap-2">
          <Button
            className="bg-[#eb6c6c] hover:bg-[#d55c5c]"
            onClick={() => setApplying(true)}
            disabled={applying}
          >
            <Store className="mr-2 h-4 w-4" />
            Add another location
          </Button>
          <Button variant="ghost" size="sm" onClick={handleRefresh} disabled={refreshing || !locationWallet}>
            <RefreshCw className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`} />
          </Button>
        </div>
      </div>

      {/* One shop needs no chooser — the page is already about it. */}
      {sortedLocations.length > 1 && (
        <div className="space-y-2">
          <Label htmlFor="location-select" className="text-black dark:text-white">
            Managing
          </Label>
          <Select
            value={String(selectedLocation.id)}
            onValueChange={(value) => setSelectedLocationId(Number(value))}
          >
            <SelectTrigger id="location-select" className="bg-secondary text-black dark:text-white">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {sortedLocations.map((location) => (
                <SelectItem key={location.id} value={String(location.id)}>
                  {location.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      <Card className="border-border/70 bg-card/90 shadow-sm">
        <CardHeader className="gap-3">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0 space-y-1">
              <CardTitle className="truncate text-black dark:text-white">{selectedLocation.name}</CardTitle>
              {addressSummary && <CardDescription className="break-words">{addressSummary}</CardDescription>}
            </div>
            <Badge
              variant={
                applicationStatus === "approved"
                  ? "success"
                  : applicationStatus === "rejected"
                    ? "destructive"
                    : "warning"
              }
              className="self-start"
            >
              {applicationStatus === "approved"
                ? "On the map"
                : applicationStatus === "rejected"
                  ? "Not approved"
                  : "Awaiting approval"}
            </Badge>
          </div>

          {/* While the listing is under review the wallet is deliberately not
              mentioned at all — no address, no balance, no send surface. A
              merchant mid-onboarding cannot use any of it yet, and showing it
              only raises questions the approval email answers. */}
          {applicationStatus === "pending" && (
            <div className="space-y-3">
              {/* Kept as text: it says why the wallet and payment surfaces
                  below are missing, and an explanation nobody can see until
                  they hover is no explanation. */}
              <p className="text-sm text-muted-foreground">
                Not on the map or taking payments until this listing is approved.
              </p>
              {/* Also the one thing standing between this account and a personal
                  one, if that is what the merchant is really after. */}
              <CancelLocationApplication location={selectedLocation} />
            </div>
          )}
          {applicationStatus === "rejected" && (
            <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              Not approved, so it takes no payments.{" "}
              {/* The setup page turns a regular account away, so a merchant who
                  predates account types is sent to a human instead of a door
                  that closes on them. */}
              {user?.accountType === "merchant" && (
                <>
                  Correct it on your{" "}
                  <button
                    type="button"
                    className="font-semibold text-foreground underline underline-offset-4"
                    onClick={() => router.push(MERCHANT_ONBOARDING_PATH)}
                  >
                    merchant setup page
                  </button>
                  , then email{" "}
                </>
              )}
              {user?.accountType !== "merchant" && "Email "}
              <a
                className="font-semibold text-foreground underline underline-offset-4"
                href={`mailto:${MERCHANT_SUPPORT_EMAIL}`}
              >
                {MERCHANT_SUPPORT_EMAIL}
              </a>{" "}
              to ask for another review.
            </p>
            {/* A refused application still blocks a revert to a personal
                account, so it needs a way out of its own rather than only a
                route back into review. */}
            <CancelLocationApplication location={selectedLocation} />
            </div>
          )}
        </CardHeader>

        {applicationStatus !== "pending" && (
        <CardContent className="space-y-4">
          {paymentAddress === "" ? (
            <div className="rounded-lg border bg-muted/25 px-4 py-5 text-sm text-muted-foreground">
              No wallet yet. Wallets are created at approval. Email {MERCHANT_SUPPORT_EMAIL} if
              this stays.
            </div>
          ) : (
            <div className="flex flex-wrap items-center gap-2">
              <span className="rounded-md bg-muted/50 px-2 py-1 font-mono text-xs text-gray-600 dark:text-gray-400">
                {shortAddress}
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 w-7 p-0 text-muted-foreground hover:text-foreground"
                onClick={handleCopyAddress}
                aria-label="Copy this location's wallet address"
              >
                {addressCopied ? <Check className="h-3.5 w-3.5 text-green-600" /> : <Copy className="h-3.5 w-3.5" />}
              </Button>
            </div>
          )}

          {paymentAddress !== "" && !locationWallet && (
            <div className="rounded-lg border bg-muted/25 px-4 py-5 text-sm text-muted-foreground">
              This wallet could not be opened, so sending is unavailable. Your balance is
              untouched. Reload the page, or email {MERCHANT_SUPPORT_EMAIL}.
            </div>
          )}

          {locationWallet && (
            <>
              <WalletBalanceCard balance={balance || 0} showBalance />

              <div className={`grid gap-3 ${unwrapEnabled ? "grid-cols-3" : "grid-cols-2"}`}>
                <Button
                  onClick={() => setShowSendModal(true)}
                  className="h-14 flex-col gap-1.5 text-sm sm:h-16"
                >
                  <Send className="h-4 w-4 sm:h-5 sm:w-5" />
                  <span>Send</span>
                </Button>
                <Button
                  variant="outline"
                  onClick={() => setShowReceiveModal(true)}
                  className="h-14 flex-col gap-1.5 text-sm hover:bg-primary/65 sm:h-16"
                >
                  <ArrowDownLeft className="h-4 w-4 sm:h-5 sm:w-5" />
                  <span>Receive</span>
                </Button>
                {/* Server-flagged, and off by default. See useUnwrapEnabled. */}
                {unwrapEnabled && (
                  <Button
                    variant="outline"
                    onClick={() => setShowUnwrapModal(true)}
                    className="h-14 flex-col gap-1.5 text-sm hover:bg-primary/65 sm:h-16"
                  >
                    <ArrowLeftRight className="h-4 w-4 sm:h-5 sm:w-5" />
                    <span>Unwrap</span>
                  </Button>
                )}
              </div>

              {tipWallet && (
                <div className="space-y-3 rounded-lg border bg-muted/20 p-4">
                  <p className="text-sm font-semibold text-foreground">Tipping account</p>
                  <WalletBalanceCard balance={tipBalance || 0} showBalance />
                  <div className="grid grid-cols-2 gap-3">
                    <Button
                      onClick={() => setShowTipSendModal(true)}
                      className="h-12 flex-col gap-1 text-sm"
                    >
                      {/* The paper plane's glyph fills ~85% of its 24-unit
                          canvas, so sizing the box sizes the icon. */}
                      <Send style={{ width: 24, height: 24 }} />
                      <span>Send</span>
                    </Button>
                    <Button
                      variant="outline"
                      onClick={() => setShowTipReceiveModal(true)}
                      className="h-12 flex-col gap-1 text-sm hover:bg-primary/65"
                    >
                      {/* The corner arrow's glyph spans only units 7–17 of
                          its 24-unit canvas — 42% ink, the rest padding — so
                          growing the box mostly grew invisible margin. Cropping
                          the viewBox to the glyph makes the box we size the
                          arrow we see, and 24px here paints about the same as
                          the plane at 24px. */}
                      <ArrowDownLeft viewBox="5 5 14 14" style={{ width: 24, height: 24 }} />
                      <span>Receive</span>
                    </Button>
                  </div>
                </div>
              )}
              {tipAddress !== "" && !tipWallet && (
                <p className="text-sm text-muted-foreground">
                  This tipping account could not be opened. Reload the page.
                </p>
              )}
            </>
          )}
        </CardContent>
        )}
      </Card>

      {locationWallet && (
        <>
          <SendCryptoModal
            open={showSendModal}
            onOpenChange={setShowSendModal}
            wallet={locationWallet}
            balance={balance || 0}
          />
          <ReceiveCryptoModal
            open={showReceiveModal}
            onOpenChange={setShowReceiveModal}
            wallet={locationWallet}
          />
          {unwrapEnabled && (
            <UnwrapModal
              open={showUnwrapModal}
              onOpenChange={setShowUnwrapModal}
              wallet={locationWallet}
              balance={balance}
              defaultDestination={user?.paypalEthAddress}
              onSuccess={updateBalance}
            />
          )}
        </>
      )}

      {tipWallet && (
        <>
          <SendCryptoModal
            open={showTipSendModal}
            onOpenChange={setShowTipSendModal}
            wallet={tipWallet}
            balance={tipBalance || 0}
          />
          <ReceiveCryptoModal
            open={showTipReceiveModal}
            onOpenChange={setShowTipReceiveModal}
            wallet={tipWallet}
          />
        </>
      )}
    </div>
  )
}

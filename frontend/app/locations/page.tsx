"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { ArrowDownLeft, ArrowLeftRight, Check, Copy, RefreshCw, Send, Store, Wallet } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { MerchantApprovalForm } from "@/components/merchant/merchant-approval-form"
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
  const { user, userLocations, wallets, walletsStatus, status, refreshUserRecord, refreshWallets } =
    useApp()

  const [selectedLocationId, setSelectedLocationId] = useState<number | null>(null)
  const [locationCountBeforeApply, setLocationCountBeforeApply] = useState<number | null>(null)
  const [balance, setBalance] = useState<number | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [addressCopied, setAddressCopied] = useState(false)
  const [showSendModal, setShowSendModal] = useState(false)
  const [showReceiveModal, setShowReceiveModal] = useState(false)
  const [showUnwrapModal, setShowUnwrapModal] = useState(false)
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

  // Regular accounts have no locations to manage, and a merchant with none has
  // an onboarding form waiting rather than an empty page.
  useEffect(() => {
    if (status !== "authenticated") return
    if (!isMerchantAccount) {
      router.replace("/wallets")
      return
    }
    if (userLocations.length === 0) router.replace(MERCHANT_ONBOARDING_PATH)
  }, [isMerchantAccount, router, status, userLocations.length])

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

  if (!selectedLocation) return null

  const applicationStatus = getLocationApplicationStatus(selectedLocation.approval)
  const hasApprovedLocation = userLocations.some(
    (location) => getLocationApplicationStatus(location.approval) === "approved",
  )
  const addressSummary = [selectedLocation.street, selectedLocation.city, selectedLocation.state]
    .filter(Boolean)
    .join(", ")
  const shortAddress =
    paymentAddress.length > 12
      ? `${paymentAddress.slice(0, 6)}...${paymentAddress.slice(-4)}`
      : paymentAddress

  return (
    <div className="mx-auto w-full max-w-2xl space-y-4 px-3 pb-6 pt-2 sm:space-y-6 sm:px-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-black dark:text-white sm:text-3xl">Locations</h1>
          <p className="mt-1 text-sm text-gray-600 dark:text-gray-400 sm:text-base">
            The wallet each of your shops is paid into
          </p>
        </div>
        <Button variant="ghost" size="sm" onClick={handleRefresh} disabled={refreshing || !locationWallet}>
          <RefreshCw className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`} />
        </Button>
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
            <p className="text-sm leading-relaxed text-muted-foreground">
              This location will not appear on the SFLuv map or take customer payments until our
              team approves the listing. We will email you when it has been reviewed.
            </p>
          )}
          {applicationStatus === "rejected" && (
            <p className="text-sm leading-relaxed text-muted-foreground">
              This listing was not approved, so it takes no payments.{" "}
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
          )}
        </CardHeader>

        {applicationStatus !== "pending" && (
        <CardContent className="space-y-4">
          {paymentAddress === "" ? (
            <div className="rounded-lg border bg-muted/25 px-4 py-5 text-sm text-muted-foreground">
              No wallet is attached to this location yet. One is normally minted with the listing,
              and approval assigns one if that did not happen. Email {MERCHANT_SUPPORT_EMAIL} if it
              stays this way.
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
              We could not open this location&apos;s wallet in this session, so sending from it is
              unavailable here. Anything already at the address is untouched — reload the page, and
              email {MERCHANT_SUPPORT_EMAIL} if it keeps happening.
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
            </>
          )}
        </CardContent>
        )}
      </Card>

      <div className="flex flex-col gap-3 sm:flex-row">
        {!applying && (
          <Button className="w-full bg-[#eb6c6c] hover:bg-[#d55c5c] sm:w-auto" onClick={() => setApplying(true)}>
            <Store className="mr-2 h-4 w-4" />
            Add another location
          </Button>
        )}
        {hasApprovedLocation && (
          <Button variant="outline" className="w-full sm:w-auto" onClick={() => router.push("/wallets")}>
            <Wallet className="mr-2 h-4 w-4" />
            Your personal wallets
          </Button>
        )}
      </div>

      {applying && (
        <div className="space-y-3">
          <p className="text-sm text-muted-foreground">
            A second shop is a separate listing with its own wallet, and needs its own approval
            before it goes on the map.
          </p>

          {googleLoadError ? (
            <Card className="border-[#eb6c6c]/40 bg-[#eb6c6c]/5">
              <CardContent className="p-6 text-sm text-[#8f2e2e]">{googleLoadError}</CardContent>
            </Card>
          ) : !googleReady ? (
            <div className="flex min-h-[200px] items-center justify-center">
              <div className="h-12 w-12 animate-spin rounded-full border-b-2 border-t-2 border-[#eb6c6c]" />
            </div>
          ) : (
            <>
              <Button variant="ghost" onClick={() => setApplying(false)}>
                Cancel
              </Button>
              <MerchantApprovalForm
                // Carried from the shop they started with rather than whichever
                // one the chooser happens to be on: the business and the person
                // we ring about it are the same across all of them, so a second
                // application should not read differently depending on what was
                // on screen when it was opened.
                prefillFrom={sortedLocations[0]}
                // Staying here is the point: the merchant came to manage tills
                // and the new one appears in the chooser as soon as it lands.
                redirectAfterCreate={null}
                onSaved={() => {
                  setApplying(false)
                  setLocationCountBeforeApply(sortedLocations.length)
                  void (async () => {
                    await refreshUserRecord()
                    // The till is minted server-side as the listing is created,
                    // so the wallet list has to be read again or the new shop
                    // would show an address with nothing behind it.
                    await refreshWallets()
                  })()
                }}
              />
            </>
          )}
        </div>
      )}

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
    </div>
  )
}

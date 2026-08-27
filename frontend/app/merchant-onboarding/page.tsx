"use client"

import { useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useApp } from "@/context/AppProvider"
import { MerchantApprovalForm } from "@/components/merchant/merchant-approval-form"
import { LocationApplicationStatusCard } from "@/components/merchant/location-application-status"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { ensureGooglePlacesScript, hasGoogleMapsPlaces } from "@/lib/google-places"
import {
  getLocationApplicationStatus,
  MERCHANT_SUPPORT_EMAIL,
} from "@/lib/merchant-onboarding"
import { AuthedLocation } from "@/types/location"

/**
 * A merchant's first screen, and the one they are handed back to until a shop
 * of theirs is listed.
 *
 * It is a page rather than an overlay on purpose: the product owner wanted a
 * new merchant able to look around — the map especially — so this has to be
 * somewhere they can navigate away from and come back to, not something that
 * takes the app away from them.
 */
export default function MerchantOnboardingPage() {
  const { status, user, userLocations, login, merchantOnboardingRequired, refreshUserRecord } =
    useApp()
  const router = useRouter()
  const [googleReady, setGoogleReady] = useState(false)
  const [googleLoadError, setGoogleLoadError] = useState<string | null>(null)
  const [editingLocationId, setEditingLocationId] = useState<number | null>(null)
  const [startingAnotherApplication, setStartingAnotherApplication] = useState(false)

  useEffect(() => {
    let mounted = true

    const ensureGoogleScript = async () => {
      if (typeof window === "undefined") return
      if (hasGoogleMapsPlaces()) {
        setGoogleReady(true)
        return
      }

      await ensureGooglePlacesScript()
      if (mounted) setGoogleReady(true)
    }

    ensureGoogleScript().catch((error) => {
      if (!mounted) return
      console.error(error)
      setGoogleLoadError("Failed to load Google Places search. Please refresh and try again.")
    })

    return () => {
      mounted = false
    }
  }, [])

  // Nothing here applies to a regular account. Sending them away rather than
  // showing an apply form is the point of the new signup question: merchant is
  // something you chose at the start, not something you convert into later.
  useEffect(() => {
    if (status !== "authenticated") return
    if (user?.accountType === "merchant") return
    router.replace("/map")
  }, [router, status, user?.accountType])

  const sortedUserLocations = useMemo(
    () => [...userLocations].sort((a, b) => b.id - a.id),
    [userLocations],
  )
  const rejectedLocations = useMemo(
    () =>
      sortedUserLocations.filter(
        (location) => getLocationApplicationStatus(location.approval) === "rejected",
      ),
    [sortedUserLocations],
  )
  const hasLiveApplication = useMemo(
    () =>
      sortedUserLocations.some(
        (location) => getLocationApplicationStatus(location.approval) !== "rejected",
      ),
    [sortedUserLocations],
  )
  const editingLocation: AuthedLocation | undefined = useMemo(
    () => sortedUserLocations.find((location) => location.id === editingLocationId),
    [editingLocationId, sortedUserLocations],
  )

  if (status === "loading") {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="h-12 w-12 animate-spin rounded-full border-b-2 border-t-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  if (status === "unauthenticated") {
    return (
      <Card className="mx-auto max-w-2xl border-[#eb6c6c]/40 bg-[#eb6c6c]/5">
        <CardContent className="space-y-4 p-6 text-center">
          <p className="text-sm text-[#8f2e2e]">
            Log in to finish setting up your merchant account.
          </p>
          <Button size="lg" className="bg-[#eb6c6c] hover:bg-[#d55c5c]" onClick={() => login()}>
            Create Account / Log In
          </Button>
        </CardContent>
      </Card>
    )
  }

  // The newest listing that was not rejected, else the newest of any kind:
  // whichever best describes the business is what a second shop inherits.
  const prefillSource = startingAnotherApplication
    ? (sortedUserLocations.find(
        (location) => getLocationApplicationStatus(location.approval) !== "rejected",
      ) ?? sortedUserLocations[0])
    : undefined

  const showForm =
    editingLocation !== undefined ||
    sortedUserLocations.length === 0 ||
    startingAnotherApplication
  // Only a new application needs the Places picker, so an edit does not wait on
  // a script it will never call.
  const needsPlacePicker = showForm && editingLocation === undefined

  return (
    <div className="mx-auto w-full max-w-3xl space-y-5 px-3 pb-6 pt-2 sm:space-y-6 sm:px-0">
      <div className="space-y-1">
        <h1 className="text-2xl font-bold text-black dark:text-white sm:text-3xl">
          Set up your merchant account
        </h1>
        {/* The write gate lifts as soon as a listing exists, approved or not, so
            a merchant reading a rejection is no longer held to reads — telling
            them otherwise would be the app describing a restriction that is not
            there. */}
        <p className="text-sm text-muted-foreground sm:text-base">
          {merchantOnboardingRequired
            ? "List your business so SFLuv can give it a till wallet and put it on the map. Until then you can look around, but sending, receiving and everything else stays switched off."
            : "Your business is not on the SFLuv map yet. Here is where your application stands."}
        </p>
      </div>

      {sortedUserLocations.length > 0 && (
        <div className="space-y-4 sm:space-y-5">
          {sortedUserLocations.map((location) => {
            const isRejected = getLocationApplicationStatus(location.approval) === "rejected"

            return (
              <LocationApplicationStatusCard key={location.id} location={location}>
                {isRejected && editingLocationId !== location.id && (
                  <div className="space-y-3">
                    <p className="text-sm leading-relaxed text-gray-600 dark:text-gray-400">
                      Correct your details and save them. Saving does not put the listing back in
                      the review queue on its own — email{" "}
                      <a
                        className="font-semibold text-foreground underline underline-offset-4"
                        href={`mailto:${MERCHANT_SUPPORT_EMAIL}`}
                      >
                        {MERCHANT_SUPPORT_EMAIL}
                      </a>{" "}
                      once your corrections are saved and ask for another review.
                    </p>
                    <Button
                      variant="outline"
                      className="border-[#eb6c6c] bg-secondary text-[#eb6c6c] hover:bg-[#eb6c6c] hover:text-white"
                      onClick={() => setEditingLocationId(location.id)}
                    >
                      Edit this application
                    </Button>
                  </div>
                )}
              </LocationApplicationStatusCard>
            )
          })}
        </div>
      )}

      {needsPlacePicker && googleLoadError && (
        <Card className="border-[#eb6c6c]/40 bg-[#eb6c6c]/5">
          <CardContent className="p-6 text-sm text-[#8f2e2e]">{googleLoadError}</CardContent>
        </Card>
      )}

      {needsPlacePicker && !googleLoadError && !googleReady && (
        <div className="flex min-h-[200px] items-center justify-center">
          <div className="h-12 w-12 animate-spin rounded-full border-b-2 border-t-2 border-[#eb6c6c]"></div>
        </div>
      )}

      {showForm && (!needsPlacePicker || (googleReady && !googleLoadError)) && (
        <div className="space-y-3">
          {editingLocation !== undefined && (
            <Button variant="ghost" onClick={() => setEditingLocationId(null)}>
              Cancel editing
            </Button>
          )}
          {startingAnotherApplication && editingLocation === undefined && (
            <Button variant="ghost" onClick={() => setStartingAnotherApplication(false)}>
              Back to your applications
            </Button>
          )}
          <MerchantApprovalForm
            key={
              editingLocation?.id ??
              (startingAnotherApplication ? "another-application" : "new-application")
            }
            existingLocation={editingLocation}
            prefillFrom={editingLocation === undefined ? prefillSource : undefined}
            onSaved={() => {
              setEditingLocationId(null)
              setStartingAnotherApplication(false)
              void refreshUserRecord()
            }}
          />
        </div>
      )}

      {!showForm && hasLiveApplication && (
        <p className="text-sm text-muted-foreground">
          Your application is with the SFLuv team. We will email you when it has been reviewed.
        </p>
      )}

      {!showForm && (
        <Card className="border-border/70">
          <CardContent className="flex flex-col gap-3 p-5 sm:flex-row sm:items-center sm:justify-between">
            <div className="space-y-1">
              <p className="text-sm font-semibold text-foreground">Have another location?</p>
              <p className="text-sm leading-6 text-muted-foreground">
                Each location gets its own listing and till wallet. We will carry your business
                details over so you only fill in what is different.
              </p>
            </div>
            <Button
              className="shrink-0 bg-[#eb6c6c] hover:bg-[#d55c5c]"
              onClick={() => setStartingAnotherApplication(true)}
            >
              Start its application
            </Button>
          </CardContent>
        </Card>
      )}

      {!showForm && !hasLiveApplication && rejectedLocations.length > 0 && (
        <p className="text-sm text-muted-foreground">
          Pick an application above to correct it, or email{" "}
          <a
            className="font-semibold text-foreground underline underline-offset-4"
            href={`mailto:${MERCHANT_SUPPORT_EMAIL}`}
          >
            {MERCHANT_SUPPORT_EMAIL}
          </a>{" "}
          if you need a hand.
        </p>
      )}
    </div>
  )
}

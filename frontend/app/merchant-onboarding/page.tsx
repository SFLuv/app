"use client"

import { useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"

import { useApp } from "@/context/AppProvider"
import { LocationApprovalFlow } from "@/components/merchant/location-approval-flow"
import { LocationApplicationStatusCard } from "@/components/merchant/location-application-status"
import { CancelLocationApplication } from "@/components/merchant/cancel-location-application"
import { MerchantApprovalForm } from "@/components/merchant/merchant-approval-form"
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
 *
 * Three views, and which one is showing is the whole of the state: the stepper
 * for a new application, the confirmation once one has gone in, and the list of
 * where existing applications stand.
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

  // Nothing here applies to a personal account. Sending them away rather than
  // showing an apply form is the point of the signup question: merchant is
  // something you choose, and the settings screen is where you change your mind.
  useEffect(() => {
    if (status !== "authenticated") return
    if (user?.accountType === "merchant") return
    router.replace("/map")
  }, [router, status, user?.accountType])

  const sortedUserLocations = useMemo(
    () => [...userLocations].sort((a, b) => b.id - a.id),
    [userLocations],
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
  // whichever best describes the business is what a second location inherits.
  const prefillSource =
    sortedUserLocations.find(
      (location) => getLocationApplicationStatus(location.approval) !== "rejected",
    ) ?? sortedUserLocations[0]

  // A new application takes the whole screen; editing an existing one stays in
  // the app's ordinary layout, because it is a correction to a record rather
  // than a flow with a beginning and an end.
  const applying = editingLocation === undefined &&
    (sortedUserLocations.length === 0 || startingAnotherApplication)
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
          <div className="h-12 w-12 animate-spin rounded-full border-b-2 border-t-2 border-[#eb6c6c]"></div>
        </div>
      )
    }

    return (
      <LocationApprovalFlow
        prefillFrom={prefillSource}
        // Always offered, and it always goes somewhere real. Locations is the
        // merchant hub: with shops listed it shows them, and with none it shows
        // the setup view this form was reached from. There is no wall any more,
        // so a navigation is a genuine exit rather than a round trip.
        onCancel={
          startingAnotherApplication
            ? () => setStartingAnotherApplication(false)
            : () => router.push("/locations")
        }
        onFinish={() => {
          setStartingAnotherApplication(false)
          // Same reasoning as the Cancel target above: a merchant who has just
          // applied does have a location now, so /locations is reachable.
          router.push("/locations")
        }}
        onSubmitted={() => void refreshUserRecord()}
      />
    )
  }

  return (
    <div className="mx-auto w-full max-w-3xl space-y-5 px-3 pb-6 pt-2 sm:space-y-6 sm:px-0">
      <div className="space-y-1">
        <h1 className="text-2xl font-bold text-black dark:text-white sm:text-3xl">
          Set up your merchant account
        </h1>
        {/* Kept, and only in the gated case. It is not a subtitle describing
            the page — it is the reason the rest of the app is switched off, and
            somebody who does not read it has no way to find that out. The write
            gate lifts as soon as a listing exists, approved or not, so it says
            nothing to a merchant who has already applied. */}
        {merchantOnboardingRequired && (
          <p className="text-sm text-muted-foreground">
            Sending and receiving stay switched off until you list a business.
          </p>
        )}
      </div>

      {sortedUserLocations.length > 0 && (
        <div className="space-y-4 sm:space-y-5">
          {sortedUserLocations.map((location) => {
            const applicationStatus = getLocationApplicationStatus(location.approval)

            return (
              <LocationApplicationStatusCard key={location.id} location={location}>
                {/* No status gate here: the component shows itself for anything
                    not yet approved, which now includes a rejected application
                    — that blocks a revert to a personal account too, so it needs
                    the same way out. */}
                <CancelLocationApplication location={location} />
                {applicationStatus === "rejected" && editingLocationId !== location.id && (
                  <div className="space-y-3">
                    {/* Not a tooltip: saving does not re-queue the listing, and
                        a merchant who never hovers would sit waiting on a review
                        nobody has been asked for. */}
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      Saving does not re-queue this listing. Email{" "}
                      <a
                        className="font-semibold text-foreground underline underline-offset-4"
                        href={`mailto:${MERCHANT_SUPPORT_EMAIL}`}
                      >
                        {MERCHANT_SUPPORT_EMAIL}
                      </a>{" "}
                      to ask for another review.
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

      {editingLocation !== undefined && (
        <div className="space-y-3">
          <Button variant="ghost" onClick={() => setEditingLocationId(null)}>
            Cancel editing
          </Button>

          {/* Editing keeps the older single-sheet form. It edits a record that
              already exists — the place is fixed, PUT /locations does not accept
              a new one — so a stepper built around picking a location would be
              three screens of questions with the first one answered already. */}
          <MerchantApprovalForm
            key={editingLocation.id}
            existingLocation={editingLocation}
            onSaved={() => {
              setEditingLocationId(null)
              void refreshUserRecord()
            }}
          />
        </div>
      )}

      {/* Also the landing state for a gated merchant who cancelled out of the
          form: with nothing listed there is no "another" location, so the card
          offers the first one instead of asking a question with no subject. */}
      {editingLocation === undefined && (
        <Card className="border-border/70">
          <CardContent className="flex flex-col gap-3 p-5 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm font-semibold text-foreground">
              {sortedUserLocations.length === 0
                ? "Ready to list your business?"
                : "Have another location?"}
            </p>
            <Button
              className="shrink-0 bg-[#eb6c6c] hover:bg-[#d55c5c]"
              onClick={() => setStartingAnotherApplication(sortedUserLocations.length > 0)}
            >
              {sortedUserLocations.length === 0
                ? "Start application"
                : "Apply for another location"}
            </Button>
          </CardContent>
        </Card>
      )}

    </div>
  )
}

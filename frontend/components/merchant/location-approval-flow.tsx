"use client"

import { useEffect, useState } from "react"
import { CheckCircle2, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { LocationApprovalForm } from "@/components/merchant/location-approval-form"
import type { AuthedLocation } from "@/types/location"

/** Survives the remount that clearing the onboarding wall causes. */
const SUBMITTED_FLAG_KEY = "sfluv_location_application_submitted"

const readSubmittedFlag = (): boolean => {
  if (typeof window === "undefined") return false
  try {
    return window.sessionStorage.getItem(SUBMITTED_FLAG_KEY) === "1"
  } catch {
    // Private modes and blocked site data throw on access rather than
    // returning null. A confirmation that does not survive a remount is worse
    // than nothing, but it is not worth a crash.
    return false
  }
}

const writeSubmittedFlag = (value: boolean) => {
  if (typeof window === "undefined") return
  try {
    if (value) window.sessionStorage.setItem(SUBMITTED_FLAG_KEY, "1")
    else window.sessionStorage.removeItem(SUBMITTED_FLAG_KEY)
  } catch {
    // As above.
  }
}

/**
 * The Location Approval Form as a screen of its own.
 *
 * Fixed over the app rather than laid out inside it. The stepper is a single
 * errand with three sections and a confirmation, and the sidebar and top bar
 * beside it are navigation to places the merchant is not going — on a laptop
 * they cost about a fifth of the width, which is a column of form the merchant
 * has to scroll for instead.
 *
 * Covering the chrome rather than asking the layout to hide it keeps the
 * decision local. A context flag read by the layout would have to be set from
 * inside a render, and every host of this flow would have to remember to unset
 * it; an overlay is correct wherever it is mounted, including in place of the
 * whole app for a merchant still behind the onboarding wall.
 *
 * The way out is deliberately narrow: a small Cancel, and — once an
 * application is in — the two things a merchant plausibly wants next. Applying
 * for another stays in here, because that is the same errand again.
 */
export function LocationApprovalFlow({
  prefillFrom,
  onCancel,
  onFinish,
  onSubmitted,
}: {
  /** An earlier listing whose shared answers start the form off. */
  prefillFrom?: AuthedLocation
  /**
   * Leaves without applying. Omitted for a merchant who has not listed
   * anything yet: the app behind this is switched off for them, so a Cancel
   * would lead to a screen that only tells them to come back here.
   */
  onCancel?: () => void
  /** "Return to app" — the deliberate exit, once something has been submitted. */
  onFinish: () => void
  /** Fired per accepted application, so the host can refresh its records. */
  onSubmitted?: (locationId: number) => void
}) {
  // Persisted, not merely held in state, and the reason is the most important
  // application there is: a merchant's first.
  //
  // Listing a shop clears the onboarding wall, and the wall renders this screen
  // *in place of* the whole app — so the moment the profile refresh lands, the
  // same page is remounted at its ordinary position in the tree and every piece
  // of local state goes with it. Plain state meant the confirmation appeared
  // and then vanished under the merchant a second later, replaced by a list.
  // Session storage outlives that remount; both exits clear it.
  const [submitted, setSubmitted] = useState(() => readSubmittedFlag())
  // Bumped to start a second application from scratch. Remounting the form is
  // what clears the place selection, the crop and every answer that belonged to
  // the shop just applied for.
  const [attempt, setAttempt] = useState(0)

  // The page underneath is still a scrolling document, and a wheel over a fixed
  // overlay that does not itself overflow is handed to it — so the form appeared
  // to scroll with nothing below it, revealing the blank tail of a page it had
  // covered. Locked for as long as the flow is mounted, and restored to whatever
  // was there before rather than to a hardcoded "visible".
  useEffect(() => {
    const previous = document.body.style.overflow
    document.body.style.overflow = "hidden"
    return () => {
      document.body.style.overflow = previous
    }
  }, [])

  const markSubmitted = (value: boolean) => {
    setSubmitted(value)
    writeSubmittedFlag(value)
  }

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto bg-background">
      {/* Pinned to the viewport, not laid out above the form.
          `position: fixed` resolves against the viewport even inside this
          scrolling overlay (no ancestor sets a transform), so the way out stays
          in the same place whether the merchant is at the top of step one or
          scrolled to the bottom of an expanded week of opening hours. Laid out
          in the flow, it scrolled off and the only remaining exit was Back,
          Back, Back.

          The gradient is a fade rather than a bar: something has to mask the
          form passing underneath, and a solid strip with an edge would read as
          the top bar this screen exists to remove. The wrapper takes no pointer
          events so the faded region does not swallow clicks meant for the form
          beneath it; only the button itself is clickable. */}
      {onCancel && (
        <div className="pointer-events-none fixed inset-x-0 top-0 z-10 flex justify-end bg-gradient-to-b from-background via-background to-transparent p-3 pb-8 sm:justify-start sm:p-4 sm:pb-10">
          {/* Right on a phone, left on a desktop. A thumb reaches the near top
              corner and a cursor starts at the far one, and on a phone the left
              of that strip is also where the OS puts its own back affordance. */}
          <Button
            variant="ghost"
            size="sm"
            className="pointer-events-auto text-muted-foreground"
            onClick={() => {
              markSubmitted(false)
              onCancel()
            }}
          >
            <X className="mr-1.5 h-4 w-4" />
            Cancel
          </Button>
        </div>
      )}

      {/* min-h-full plus `my-auto` on the child, rather than `items-center` on
          this flex parent: auto margins centre a short form and collapse to
          nothing once it is taller than the viewport, whereas centring a tall
          child in an overflow container pushes its top above the scrollable
          area, where it cannot be reached.

          The top padding is clearance for the pinned Cancel, so the card can
          never sit under it on a narrow screen. */}
      <div className="flex min-h-full w-full flex-col">
        <div
          className={`mx-auto my-auto w-full max-w-3xl px-4 pb-10 sm:px-6 ${
            onCancel ? "pt-16 sm:pt-20" : "pt-6"
          }`}
        >
          {submitted ? (
            /* No top padding of its own — the container centres it, and the
               confirmation is short enough that it always lands centred. */
            <div className="mx-auto max-w-md text-center">
              <div className="mx-auto mb-5 flex h-14 w-14 items-center justify-center rounded-full bg-green-100 dark:bg-green-500/15">
                <CheckCircle2 className="h-7 w-7 text-green-600 dark:text-green-400" />
              </div>
              <h1 className="text-2xl font-bold text-black dark:text-white">
                Your application has been submitted successfully
              </h1>
              <div className="mt-6 flex flex-col gap-3 sm:flex-row sm:justify-center">
                <Button
                  className="bg-[#eb6c6c] hover:bg-[#d55c5c] sm:w-52"
                  onClick={() => {
                    markSubmitted(false)
                    setAttempt((current) => current + 1)
                  }}
                >
                  Apply for another location
                </Button>
                <Button
                  variant="outline"
                  className="sm:w-52"
                  onClick={() => {
                    markSubmitted(false)
                    onFinish()
                  }}
                >
                  Return to app
                </Button>
              </div>
            </div>
          ) : (
            <LocationApprovalForm
              key={attempt}
              prefillFrom={prefillFrom}
              onSubmitted={(locationId) => {
                markSubmitted(true)
                onSubmitted?.(locationId)
              }}
            />
          )}
        </div>
      </div>
    </div>
  )
}

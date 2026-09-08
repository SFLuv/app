import { AuthedLocation } from "@/types/location"

/**
 * The screen a merchant is sent to on sign-in until they have listed a shop.
 * Shared rather than typed out at each call site so the redirect, the banner
 * that returns them to it, and the page's own "am I already here?" check can
 * never disagree about where it lives.
 */
export const MERCHANT_ONBOARDING_PATH = "/merchant-onboarding"

/** Who to ask when a listing needs a second look. */
export const MERCHANT_SUPPORT_EMAIL = "techsupport@sfluv.org"

export type LocationApplicationStatus = "approved" | "pending" | "rejected"

/**
 * `approval` is a tri-state: true approved, false rejected, null still queued.
 * Reading it as a boolean would file every unreviewed application as rejected.
 */
export const getLocationApplicationStatus = (
  approval?: boolean | null,
): LocationApplicationStatus => {
  if (approval === true) return "approved"
  if (approval === false) return "rejected"
  return "pending"
}

/**
 * A merchant whose applications have all come back rejected. The write gate has
 * already lifted for them — it clears the moment a location exists, approved or
 * not — so nothing else in the app would tell them why their shop never
 * appeared on the map.
 */
export const hasOnlyRejectedApplications = (
  locations: AuthedLocation[],
): boolean =>
  locations.length > 0 &&
  locations.every((location) => location.approval === false)

import { EMAIL_OPT_IN_POLICY_PATH, PRIVACY_POLICY_PATH } from "@/lib/policies"

/**
 * Screens that deliberately render without the app's chrome: single-purpose
 * flows, the two ways out of an account, the policy texts, and anything
 * embedded with ?sidebar=false.
 *
 * Shared rather than restated so anything else pinned over the app — the
 * merchant onboarding banner, for one — vanishes on exactly the screens the
 * sidebar does. A second copy of this list would drift, and the way it fails is
 * a fixed bar sitting on top of the button somebody came to that page to press.
 */
export function isChromeFreeRoute(
  pathname: string,
  search?: Pick<URLSearchParams, "get"> | null,
): boolean {
  return (
    pathname === "/faucet/redeem" ||
    pathname === "/update" ||
    pathname === "/mcp/authorize" ||
    pathname === "/delete-account" ||
    pathname === "/recovery" ||
    pathname.startsWith("/photos/") ||
    pathname.startsWith(PRIVACY_POLICY_PATH) ||
    pathname.startsWith(EMAIL_OPT_IN_POLICY_PATH) ||
    search?.get("sidebar") === "false"
  )
}

export const PRIVACY_POLICY_PATH = "/privacy-policy"
export const EMAIL_OPT_IN_POLICY_PATH = "/email-opt-in-policy"
export const PRIVACY_POLICY_LAST_UPDATED = "April 15, 2026"
export const EMAIL_OPT_IN_POLICY_LAST_UPDATED = "April 15, 2026"
export const POLICY_RETURN_TO_PARAM = "returnTo"

export function normalizePolicyReturnTo(rawValue: string | null): string | null {
  if (!rawValue) return null
  const trimmed = rawValue.trim()
  if (!trimmed.startsWith("/")) return null
  if (trimmed.startsWith("//")) return null
  return trimmed
}

export function buildPolicyReturnTo(
  pathname: string,
  searchParams?: Pick<URLSearchParams, "toString"> | null,
): string {
  const normalizedPathname = normalizePolicyReturnTo(pathname) || "/map"
  const query = searchParams?.toString().trim() || ""
  return query ? `${normalizedPathname}?${query}` : normalizedPathname
}

export function buildPolicyPageHref(
  policyPath: string,
  returnTo: string,
): string {
  const normalizedReturnTo = normalizePolicyReturnTo(returnTo)
  if (!normalizedReturnTo) {
    return policyPath
  }

  const params = new URLSearchParams()
  params.set(POLICY_RETURN_TO_PARAM, normalizedReturnTo)
  return `${policyPath}?${params.toString()}`
}

const DEFAULT_CW_BASE_URL = "https://app.citizenwallet.xyz"
const DEFAULT_CW_ALIAS = "wallet.berachain.sfluv.org"
const DEFAULT_APP_ORIGIN = "https://app.sfluv.org"

const UUID_EXACT_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const UUID_IN_TEXT_PATTERN = /[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}/i

type LegacyRedeemConfig = {
  appOrigin: string
  cwAlias: string
  cwBaseUrl: string
}

const parseLegacyRedeemConfig = (pre: string | undefined): LegacyRedeemConfig | null => {
  if (!pre) return null

  try {
    const deepLinkUrl = new URL(pre)
    const hashQuery = deepLinkUrl.hash.startsWith("#/?")
      ? deepLinkUrl.hash.slice(3)
      : deepLinkUrl.hash.startsWith("#")
        ? deepLinkUrl.hash.slice(1)
        : ""

    if (!hashQuery) return null

    const hashParams = new URLSearchParams(hashQuery)
    const pluginTarget = hashParams.get("plugin")
    if (!pluginTarget) return null

    const pluginUrl = new URL(pluginTarget)

    return {
      appOrigin: pluginUrl.origin,
      cwAlias: hashParams.get("alias") || DEFAULT_CW_ALIAS,
      cwBaseUrl: `${deepLinkUrl.origin}${deepLinkUrl.pathname}`.replace(/\/+$/, ""),
    }
  } catch {
    return null
  }
}

export const normalizeRedeemCode = (rawCode: string | null | undefined): string | null => {
  if (!rawCode) return null

  let code = rawCode.trim()
  if (!code) return null

  try {
    code = decodeURIComponent(code)
  } catch {
    // keep raw value when percent-decoding fails
  }

  code = code.replace(/\s+/g, "")

  if (UUID_EXACT_PATTERN.test(code)) {
    return code.toLowerCase()
  }

  const withTrailing26Trimmed = code.endsWith("26") ? code.slice(0, -2) : ""
  if (withTrailing26Trimmed && UUID_EXACT_PATTERN.test(withTrailing26Trimmed)) {
    return withTrailing26Trimmed.toLowerCase()
  }

  const uuidMatch = code.match(UUID_IN_TEXT_PATTERN)
  if (uuidMatch) {
    return uuidMatch[0].toLowerCase()
  }

  return code.toLowerCase()
}

export const buildEventRedeemQrValue = (code: string): string => {
  const trimmedCode = code.trim()
  const legacyPre = process.env.NEXT_PUBLIC_APP_REDEEM_URL_PRE?.trim()

  const legacyConfig = parseLegacyRedeemConfig(legacyPre)

  const appOrigin = legacyConfig?.appOrigin || DEFAULT_APP_ORIGIN
  const cwAlias = legacyConfig?.cwAlias || DEFAULT_CW_ALIAS
  const cwBaseUrl = legacyConfig?.cwBaseUrl || DEFAULT_CW_BASE_URL

  // Keep the base app endpoint flow; middleware uses page=redeem for redirect.
  const redeemTarget = new URL("/", appOrigin)
  redeemTarget.searchParams.set("code", trimmedCode)
  redeemTarget.searchParams.set("page", "redeem")

  return `${cwBaseUrl}/#/?dl=plugin&alias=${encodeURIComponent(cwAlias)}&plugin=${encodeURIComponent(redeemTarget.toString())}`
}

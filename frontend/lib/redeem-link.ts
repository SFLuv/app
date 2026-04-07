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
  const redeemQuery = new URLSearchParams()
  redeemQuery.set("code", trimmedCode)
  redeemQuery.set("page", "redeem")
  const redeemTarget = `${appOrigin}?${redeemQuery.toString()}`

  return `${cwBaseUrl}/#/?dl=plugin&alias=${encodeURIComponent(cwAlias)}&plugin=${encodeURIComponent(redeemTarget)}`
}

export interface MerchantSendQrParams {
  to: string
  tipTo?: string | null
}

const HEX_ADDRESS_PATTERN = /^0x[0-9a-fA-F]{40}$/

const hexAddressToBytes = (address: string): Uint8Array => {
  const hex = address.slice(2)
  const bytes = new Uint8Array(20)
  for (let i = 0; i < 20; i++) {
    bytes[i] = parseInt(hex.substr(i * 2, 2), 16)
  }
  return bytes
}

const bytesToHexAddress = (bytes: Uint8Array): string => {
  let hex = "0x"
  for (let i = 0; i < bytes.length; i++) {
    hex += bytes[i].toString(16).padStart(2, "0")
  }
  return hex
}

const bytesToBase64Url = (bytes: Uint8Array): string => {
  let binary = ""
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  const base64 = typeof btoa === "function"
    ? btoa(binary)
    : Buffer.from(binary, "binary").toString("base64")
  return base64.replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "")
}

const base64UrlToBytes = (b64url: string): Uint8Array => {
  let base64 = b64url.replace(/-/g, "+").replace(/_/g, "/")
  while (base64.length % 4 !== 0) base64 += "="
  const binary = typeof atob === "function"
    ? atob(base64)
    : Buffer.from(base64, "base64").toString("binary")
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes
}

// Encode a 0x... hex address as a compact base64url string (27 chars vs 42).
export const encodeAddressToBase64Url = (address: string): string => {
  if (!HEX_ADDRESS_PATTERN.test(address)) {
    throw new Error("Invalid hex address for base64url encoding")
  }
  return bytesToBase64Url(hexAddressToBytes(address))
}

// Decode a base64url-encoded 20-byte address back to its 0x... hex form.
// Returns null if the value is not a valid 20-byte base64url blob.
export const decodeBase64UrlAddress = (encoded: string): string | null => {
  try {
    const bytes = base64UrlToBytes(encoded)
    if (bytes.length !== 20) return null
    return bytesToHexAddress(bytes)
  } catch {
    return null
  }
}

export const buildMerchantSendQrValue = ({ to, tipTo }: MerchantSendQrParams): string => {
  const trimmedTo = to.trim()
  const trimmedTipTo = (tipTo || "").trim()
  const legacyConfig = parseLegacyRedeemConfig(
    process.env.NEXT_PUBLIC_APP_REDEEM_URL_PRE?.trim(),
  )

  const appOrigin = legacyConfig?.appOrigin || DEFAULT_APP_ORIGIN
  const cwAlias = legacyConfig?.cwAlias || DEFAULT_CW_ALIAS

  // Produce a native sendtoUrl-format URL on our own domain. Citizen Wallet's
  // Dart QR scanner (`parseQRFormat` in lib/utils/qr.dart) classifies ANY
  // http(s) URL containing `sendto=` as `sendtoUrl` — no host check — and
  // routes it to the native send screen with recipient, alias, and tipTo
  // prefilled. The plugin/sigAuth in-app-browser flow cannot be used here
  // because `ConnectedWebViewModal.handleDisplaySendActionModal` silently
  // bails out when `amount` is absent, and `ConnectedWebViewSendModal`
  // renders the amount read-only, which is incompatible with merchant QRs
  // where the customer decides the amount at checkout.
  //
  // For users scanning with a regular camera, the URL lands on our app; the
  // middleware picks up `p=r` and forwards to `/redirect`, which handles the
  // login/wallet-ensure fallback path.
  const parts = [
    `p=r`,
    `alias=${encodeURIComponent(cwAlias)}`,
    `sendto=${encodeURIComponent(`${trimmedTo}@${cwAlias}`)}`,
  ]
  if (trimmedTipTo && HEX_ADDRESS_PATTERN.test(trimmedTipTo)) {
    parts.push(`tipTo=${trimmedTipTo}`)
  }

  return `${appOrigin}/?${parts.join("&")}`
}

import { normalizeRedeemCode } from "@/lib/redeem-link"

const ETH_ADDRESS_PATTERN = /0x[a-fA-F0-9]{40}/
const UUID_IN_TEXT_PATTERN = /[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}/i

const REDEEM_PARAM_KEYS = [
  "code",
  "page",
  "sigAuthAccount",
  "sigAuthSignature",
  "sigAuthRedirect",
  "sigAuthExpiry",
] as const

const safeDecodeURIComponent = (value: string): string => {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

const toURL = (value: string): URL | null => {
  try {
    return new URL(value)
  } catch {
    return null
  }
}

const appendRedeemParams = (source: URLSearchParams, target: URLSearchParams) => {
  for (const key of REDEEM_PARAM_KEYS) {
    const value = source.get(key)
    if (value !== null && value !== "") {
      target.set(key, value)
    }
  }
}

const collectParamsFromURL = (url: URL): URLSearchParams[] => {
  const params: URLSearchParams[] = []
  if (url.search) {
    params.push(new URLSearchParams(url.search.slice(1)))
  }

  const rawHash = url.hash.startsWith("#/?")
    ? url.hash.slice(3)
    : url.hash.startsWith("#")
      ? url.hash.slice(1)
      : ""
  if (!rawHash) {
    return params
  }

  const hashParams = new URLSearchParams(rawHash)
  params.push(hashParams)

  const pluginTarget = hashParams.get("plugin")
  if (!pluginTarget) {
    return params
  }

  const decodedPlugin = safeDecodeURIComponent(pluginTarget)
  const pluginURL = toURL(decodedPlugin) || toURL(pluginTarget)
  if (!pluginURL) {
    return params
  }

  if (pluginURL.search) {
    params.push(new URLSearchParams(pluginURL.search.slice(1)))
  }
  return params
}

export const extractEthereumAddressFromPayload = (rawValue: string): string | null => {
  if (!rawValue) return null
  const trimmed = rawValue.trim()
  if (!trimmed) return null

  const directMatch = trimmed.match(ETH_ADDRESS_PATTERN)
  if (directMatch) {
    return directMatch[0]
  }

  const decoded = safeDecodeURIComponent(trimmed)
  const decodedMatch = decoded.match(ETH_ADDRESS_PATTERN)
  if (decodedMatch) {
    return decodedMatch[0]
  }

  return null
}

export const extractRedeemParamsFromPayload = (rawValue: string): URLSearchParams | null => {
  if (!rawValue) return null
  const trimmed = rawValue.trim()
  if (!trimmed) return null

  const combined = new URLSearchParams()
  let hasRedeemMarker = false
  let discoveredCode: string | null = null

  const inspectParams = (params: URLSearchParams) => {
    appendRedeemParams(params, combined)
    if (params.get("page") === "redeem") {
      hasRedeemMarker = true
    }
    if (!discoveredCode) {
      const code = params.get("code")
      if (code) {
        discoveredCode = code
      }
    }
  }

  const directURL = toURL(trimmed)
  if (directURL) {
    const urlParams = collectParamsFromURL(directURL)
    for (const params of urlParams) {
      inspectParams(params)
    }
  }

  const decoded = safeDecodeURIComponent(trimmed)
  const decodedURL = toURL(decoded)
  if (decodedURL) {
    const decodedParams = collectParamsFromURL(decodedURL)
    for (const params of decodedParams) {
      inspectParams(params)
    }
  }

  const lowerValue = trimmed.toLowerCase()
  const lowerDecoded = decoded.toLowerCase()
  if (
    lowerValue.includes("page=redeem") ||
    lowerValue.includes("%26page%3dredeem") ||
    lowerDecoded.includes("page=redeem")
  ) {
    hasRedeemMarker = true
  }

  if (!discoveredCode) {
    const paramsCandidate = trimmed.startsWith("?") ? trimmed.slice(1) : trimmed
    const directParams = new URLSearchParams(paramsCandidate)
    const code = directParams.get("code")
    if (code) {
      discoveredCode = code
      inspectParams(directParams)
    }
  }

  if (!discoveredCode) {
    const uuidMatch = decoded.match(UUID_IN_TEXT_PATTERN) || trimmed.match(UUID_IN_TEXT_PATTERN)
    if (uuidMatch) {
      discoveredCode = uuidMatch[0]
      hasRedeemMarker = true
    }
  }

  if (!discoveredCode && !hasRedeemMarker) {
    return null
  }

  const normalizedCode = normalizeRedeemCode(discoveredCode)
  if (!normalizedCode) {
    return null
  }

  combined.set("code", normalizedCode)
  combined.delete("page")
  return combined
}

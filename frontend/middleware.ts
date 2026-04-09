import { NextRequest, NextResponse } from "next/server"
import { normalizeRedeemCode } from "@/lib/redeem-link"

const LOCALHOST_BACKEND_ORIGIN = "http://localhost:8080"

const parseEnvList = (value: string | undefined) =>
  (value || "")
    .split(",")
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0)

const normalizeOrigin = (value: string) => {
  try {
    return new URL(value).origin
  } catch {
    return value.trim()
  }
}

const isProduction = () => process.env.IN_PRODUCTION?.trim().toLowerCase() === "true"
const isCspReportOnly = () => process.env.NEXT_PUBLIC_CSP_REPORT_ONLY?.trim().toLowerCase() === "true"

const getBackendOrigin = () => {
  const configuredOrigin =
    process.env.NEXT_PUBLIC_BACKEND_URL?.trim() ||
    process.env.NEXT_PUBLIC_BACKEND_BASE_URL?.trim()

  if (configuredOrigin) {
    return normalizeOrigin(configuredOrigin)
  }

  return isProduction() ? "" : LOCALHOST_BACKEND_ORIGIN
}

const appendUnique = (values: string[], additions: string[]) => {
  for (const entry of additions) {
    if (entry && !values.includes(entry)) {
      values.push(entry)
    }
  }
}

const buildContentSecurityPolicy = (nonce: string) => {
  const production = isProduction()
  const backendOrigin = getBackendOrigin()

  const scriptSrc = [
    "'self'",
    `'nonce-${nonce}'`,
    "'strict-dynamic'",
    "https://challenges.cloudflare.com",
    "https://maps.googleapis.com",
  ]
  if (!production) {
    scriptSrc.push("'unsafe-eval'")
  }
  appendUnique(scriptSrc, parseEnvList(process.env.NEXT_PUBLIC_CSP_EXTRA_SCRIPT_SRC))

  const styleSrc = [
    "'self'",
    "'unsafe-inline'",
    "https://fonts.googleapis.com",
  ]
  appendUnique(styleSrc, parseEnvList(process.env.NEXT_PUBLIC_CSP_EXTRA_STYLE_SRC))

  const fontSrc = [
    "'self'",
    "data:",
    "https://fonts.gstatic.com",
  ]
  appendUnique(fontSrc, parseEnvList(process.env.NEXT_PUBLIC_CSP_EXTRA_FONT_SRC))

  const imgSrc = [
    "'self'",
    "blob:",
    "data:",
    "https:",
  ]
  appendUnique(imgSrc, parseEnvList(process.env.NEXT_PUBLIC_CSP_EXTRA_IMG_SRC))

  const connectSrc = [
    "'self'",
    "https://auth.privy.io",
    "https://api.privy.io",
    "https://*.rpc.privy.systems",
    "https://explorer-api.walletconnect.com",
    "https://maps.googleapis.com",
    "https://*.googleapis.com",
    "https://*.gstatic.com",
    "https://*.citizenwallet.xyz",
    "wss://relay.walletconnect.com",
    "wss://relay.walletconnect.org",
    "wss://www.walletlink.org",
    "wss://*.citizenwallet.xyz",
  ]
  if (backendOrigin) {
    appendUnique(connectSrc, [backendOrigin])
  }
  if (!production) {
    appendUnique(connectSrc, [
      "ws://localhost:3000",
      "ws://127.0.0.1:3000",
      "ws://localhost:8080",
      "ws://127.0.0.1:8080",
    ])
  }
  appendUnique(connectSrc, parseEnvList(process.env.NEXT_PUBLIC_CSP_EXTRA_CONNECT_SRC))

  const frameSrc = [
    "'self'",
    "https://auth.privy.io",
    "https://verify.walletconnect.com",
    "https://verify.walletconnect.org",
    "https://challenges.cloudflare.com",
  ]
  appendUnique(frameSrc, parseEnvList(process.env.NEXT_PUBLIC_CSP_EXTRA_FRAME_SRC))

  const workerSrc = [
    "'self'",
    "blob:",
  ]
  appendUnique(workerSrc, parseEnvList(process.env.NEXT_PUBLIC_CSP_EXTRA_WORKER_SRC))

  const directives = [
    "default-src 'self'",
    `script-src ${scriptSrc.join(" ")}`,
    `style-src ${styleSrc.join(" ")}`,
    `font-src ${fontSrc.join(" ")}`,
    `img-src ${imgSrc.join(" ")}`,
    `connect-src ${connectSrc.join(" ")}`,
    `child-src ${frameSrc.join(" ")}`,
    `frame-src ${frameSrc.join(" ")}`,
    `worker-src ${workerSrc.join(" ")}`,
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
    "manifest-src 'self'",
  ]

  if (production) {
    directives.push("upgrade-insecure-requests")
  }

  const reportUri = process.env.NEXT_PUBLIC_CSP_REPORT_URI?.trim()
  if (reportUri) {
    directives.push(`report-uri ${reportUri}`)
  }

  return directives.join("; ")
}

const applySecurityHeaders = (response: NextResponse, nonce: string) => {
  const cspHeaderName = isCspReportOnly()
    ? "Content-Security-Policy-Report-Only"
    : "Content-Security-Policy"
  const alternateCspHeaderName = isCspReportOnly()
    ? "Content-Security-Policy"
    : "Content-Security-Policy-Report-Only"

  response.headers.delete(alternateCspHeaderName)
  response.headers.set(cspHeaderName, buildContentSecurityPolicy(nonce))
  response.headers.set("Referrer-Policy", "strict-origin-when-cross-origin")
  response.headers.set("X-Content-Type-Options", "nosniff")
  response.headers.set("X-Frame-Options", "DENY")
  response.headers.set("Permissions-Policy", [
    "camera=(self)",
    "geolocation=(self)",
    "microphone=()",
    "payment=()",
    "usb=()",
    "serial=()",
    "accelerometer=()",
    "gyroscope=()",
    "browsing-topics=()",
  ].join(", "))
  response.headers.set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
  if (isProduction()) {
    response.headers.set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
  }
}

const hasEmbeddedRedeemPage = (rawCode: string | null) => {
  if (!rawCode) return false
  const lowered = rawCode.toLowerCase()
  return lowered.includes("&page=redeem") || lowered.includes("%26page%3dredeem")
}

const middleware = (request: NextRequest) => {
  const nonce = crypto.randomUUID().replace(/-/g, "")
  const search = request?.nextUrl?.search
  const params = new URLSearchParams(search)
  const pageParam = params.get("page")
  const pageAlias = params.get("p")
  const rawCode = params.get("code")

  const isRedirect =
    pageParam === "redirect" || pageAlias === "r" || pageAlias === "redirect"

  if (isRedirect) {
    params.delete("page")
    params.delete("p")
    const response = NextResponse.redirect(new URL(
      "/redirect?" + params.toString(),
      request.url
    ))
    applySecurityHeaders(response, nonce)
    return response
  }

  const isRedeem = pageParam === "redeem"
  const shouldRedirectRedeem = isRedeem || hasEmbeddedRedeemPage(rawCode)

  if (!shouldRedirectRedeem) {
    const requestHeaders = new Headers(request.headers)
    requestHeaders.set("x-nonce", nonce)
    const response = NextResponse.next({
      request: {
        headers: requestHeaders,
      },
    })
    applySecurityHeaders(response, nonce)
    return response
  }

  params.delete("page")
  const normalizedCode = normalizeRedeemCode(rawCode)
  if (normalizedCode) {
    params.set("code", normalizedCode)
  }

  const response = NextResponse.redirect(new URL(
    "/faucet/redeem?" + params.toString(),
    request.url
  ))
  applySecurityHeaders(response, nonce)
  return response
}

export default middleware

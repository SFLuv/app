import { NextRequest, NextResponse } from "next/server"
import { normalizeRedeemCode } from "@/lib/redeem-link"

const hasEmbeddedRedeemPage = (rawCode: string | null) => {
  if (!rawCode) return false
  const lowered = rawCode.toLowerCase()
  return lowered.includes("&page=redeem") || lowered.includes("%26page%3dredeem")
}

const middleware = (request: NextRequest) => {
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
    return NextResponse.redirect(new URL(
      "/redirect?" + params.toString(),
      request.url
    ))
  }

  const isRedeem = pageParam === "redeem"
  const shouldRedirectRedeem = isRedeem || hasEmbeddedRedeemPage(rawCode)

  if (!shouldRedirectRedeem) {
    return
  }

  params.delete("page")
  const normalizedCode = normalizeRedeemCode(rawCode)
  if (normalizedCode) {
    params.set("code", normalizedCode)
  }

  return NextResponse.redirect(new URL(
    "/faucet/redeem?" + params.toString(),
    request.url
  ))
}

export default middleware

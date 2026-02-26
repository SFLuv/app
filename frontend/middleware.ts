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
  const isRedirect = params.get("page") === "redeem"
  const rawCode = params.get("code")
  const shouldRedirectRedeem = isRedirect || hasEmbeddedRedeemPage(rawCode)

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

import { NextResponse } from "next/server"

const middleware = (request) => {
  const search = request?.nextUrl?.search
  const params = new URLSearchParams(search)
  const isRedirect = params.get("page")
  params.delete("page")
  switch (isRedirect) {
    case "redeem":
      return NextResponse.redirect(new URL(
        "/faucet/redeem?" + params.toString(),
        request.url
      ))
    default:
      return
  }
}

export default middleware
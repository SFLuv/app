import { NextResponse } from "next/server"

const DEFAULT_IOS_BUNDLE_ID = "org.sfluv.wallet"

export const revalidate = 3600

export function GET() {
  const appleTeamId = (process.env.APPLE_TEAM_ID || "").trim()
  const iosBundleId = (process.env.APPLE_IOS_BUNDLE_ID || DEFAULT_IOS_BUNDLE_ID).trim()
  const details = appleTeamId
    ? [
        {
          appID: `${appleTeamId}.${iosBundleId}`,
          paths: ["/", "/*"],
        },
      ]
    : []

  return NextResponse.json(
    {
      applinks: {
        apps: [],
        details,
      },
    },
    {
      headers: {
        "Content-Type": "application/json",
        "Cache-Control": "public, max-age=3600, must-revalidate",
      },
    }
  )
}

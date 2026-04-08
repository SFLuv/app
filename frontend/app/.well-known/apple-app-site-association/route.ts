import { NextResponse } from "next/server"

export const revalidate = 3600

// APPLE_UNIVERSAL_LINK_MODE controls which iOS bundle (if any) is advertised
// in the AASA file:
//   "0" → universal links disabled (no details emitted)
//   "1" → use APPLE_IOS_BUNDLE_ID_1
//   "2" → use APPLE_IOS_BUNDLE_ID_2
const resolveIosBundleId = (): string | null => {
  const mode = (process.env.APPLE_UNIVERSAL_LINK_MODE || "").trim()
  if (mode === "0") return null
  if (mode === "1") return (process.env.APPLE_IOS_BUNDLE_ID_1 || "").trim() || null
  if (mode === "2") return (process.env.APPLE_IOS_BUNDLE_ID_2 || "").trim() || null
  return null
}

export function GET() {
  const appleTeamId = (process.env.APPLE_TEAM_ID || "").trim()
  const iosBundleId = resolveIosBundleId()
  const details =
    appleTeamId && iosBundleId
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

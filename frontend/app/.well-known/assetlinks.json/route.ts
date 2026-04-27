const DEFAULT_ANDROID_PACKAGE = "org.sfluv.wallet"

const parseList = (value: string | undefined): string[] =>
  (value || "")
    .split(",")
    .map((entry) => entry.trim())
    .filter(Boolean)

export const dynamic = "force-dynamic"

export function GET() {
  const packageName =
    process.env.ANDROID_APP_LINK_PACKAGE ||
    process.env.ANDROID_PACKAGE_NAME ||
    process.env.NEXT_PUBLIC_ANDROID_PACKAGE_NAME ||
    DEFAULT_ANDROID_PACKAGE
  const fingerprints = parseList(process.env.ANDROID_APP_LINK_SHA256_CERT_FINGERPRINTS)

  const statements = fingerprints.length > 0
    ? [{
        relation: ["delegate_permission/common.handle_all_urls"],
        target: {
          namespace: "android_app",
          package_name: packageName,
          sha256_cert_fingerprints: fingerprints,
        },
      }]
    : []

  return Response.json(statements, {
    headers: {
      "content-type": "application/json",
      "cache-control": "public, max-age=3600",
    },
  })
}

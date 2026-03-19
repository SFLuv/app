import { GlobalCredentialType } from "@/types/workflow"

const DEFAULT_CREDENTIAL_LABELS: Record<string, string> = {
  dpw_certified: "DPW Certified",
  sfluv_verifier: "SFLuv Verifier",
}

const whitespacePattern = /\s+/g
const separatorPattern = /[_-]+/g

const humanizeCredentialValue = (value: string): string => {
  const normalized = value
    .trim()
    .replace(separatorPattern, " ")
    .replace(whitespacePattern, " ")

  if (!normalized) return value

  return normalized
    .split(" ")
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ")
}

export const buildCredentialLabelMap = (
  credentialTypes: GlobalCredentialType[] | null | undefined,
): Record<string, string> => {
  const labelMap: Record<string, string> = { ...DEFAULT_CREDENTIAL_LABELS }
  for (const credentialType of credentialTypes || []) {
    const value = credentialType.value.trim()
    const label = credentialType.label.trim()
    if (!value || !label) continue
    labelMap[value] = label
  }
  return labelMap
}

export const formatCredentialLabel = (
  credentialValue: string,
  labelMap?: Record<string, string>,
): string => {
  const value = credentialValue.trim()
  if (!value) return credentialValue

  if (labelMap && labelMap[value]) return labelMap[value]
  if (DEFAULT_CREDENTIAL_LABELS[value]) return DEFAULT_CREDENTIAL_LABELS[value]

  return humanizeCredentialValue(value)
}

export const buildCredentialBadgeDataUrl = (
  credentialType: Pick<GlobalCredentialType, "badge_content_type" | "badge_data_base64"> | null | undefined,
): string | null => {
  if (!credentialType) return null
  const contentType = credentialType.badge_content_type?.trim()
  const base64Payload = credentialType.badge_data_base64?.trim()
  if (!contentType || !base64Payload) return null
  return `data:${contentType};base64,${base64Payload}`
}

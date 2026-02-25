export const formatStatusLabel = (value?: string | null): string => {
  if (!value) return ""
  const normalized = value.trim().toLowerCase().replace(/\s+/g, "_")
  if (normalized === "paid_out") {
    return "Finalized"
  }
  return value
    .replace(/_/g, " ")
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .map((token) => token.charAt(0).toUpperCase() + token.slice(1))
    .join(" ")
}

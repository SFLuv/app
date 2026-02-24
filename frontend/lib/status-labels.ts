export const formatStatusLabel = (value?: string | null): string => {
  if (!value) return ""
  return value
    .replace(/_/g, " ")
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .map((token) => token.charAt(0).toUpperCase() + token.slice(1))
    .join(" ")
}

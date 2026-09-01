/**
 * Phone and email handling for merchant contact details.
 *
 * Deliberately lenient about what it accepts and strict about what it stores.
 * A merchant typing their own number uses whatever punctuation they think in —
 * `4155551234`, `415-555-1234`, `(415) 555 1234`, `+1 415 555 1234` — and
 * refusing any of those teaches nothing. All of them mean one number, so all of
 * them are accepted and normalised to one rendering.
 */

/** Digits only, so every accepted shape reduces to the same string. */
const digitsOf = (value: string): string => value.replace(/\D+/g, "")

/**
 * Reduces a typed number to its NANP significant digits, or null when it is not
 * a real one.
 *
 * Ten digits, or eleven led by a country code of 1. An area code or exchange
 * starting with 0 or 1 does not exist in the plan, which is what separates a
 * mistyped number from a valid one — without that check `1111111111` passes.
 */
export const normalizePhone = (value: string): string | null => {
  let digits = digitsOf(value)
  if (digits.length === 11 && digits.startsWith("1")) digits = digits.slice(1)
  if (digits.length !== 10) return null
  if (/^[01]/.test(digits)) return null
  if (/^[01]/.test(digits.slice(3))) return null
  return digits
}

/** `(415) 555-1234`, or the input unchanged when it is not a number we know. */
export const formatPhone = (value: string): string => {
  const digits = normalizePhone(value)
  if (!digits) return value.trim()
  return `(${digits.slice(0, 3)}) ${digits.slice(3, 6)}-${digits.slice(6)}`
}

export const isValidPhone = (value: string): boolean => normalizePhone(value) !== null

/**
 * Good enough for a contact field, and no stricter on purpose.
 *
 * The only thing worth catching here is a typo the merchant can see — a missing
 * @, a bare domain, a trailing comma. Whether the address receives mail is
 * settled by mail actually arriving, not by a regex, and the elaborate ones
 * reject valid addresses more often than they catch invalid ones.
 */
export const isValidEmail = (value: string): boolean => {
  const trimmed = value.trim()
  if (!trimmed || /\s/.test(trimmed)) return false
  const match = /^[^@]+@([^@.]+\.)+[A-Za-z]{2,}$/.test(trimmed)
  return match
}

export const normalizeEmail = (value: string): string => value.trim().toLowerCase()

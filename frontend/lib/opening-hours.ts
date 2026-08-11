import type { LocationDayHours } from "@/types/location"

/** Storage order: index 0 is Monday, matching location_hours.weekday. */
export const WEEKDAY_NAMES = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"]

/**
 * The merchants' timezone, not the viewer's.
 *
 * These are San Francisco businesses, so "today" means today where the shop is.
 * For someone standing outside it that is the same answer; for someone browsing
 * from elsewhere it is the useful one. Matches the timezone the nightly hours
 * sync runs on.
 */
const MERCHANT_TIME_ZONE = "America/Los_Angeles"

/** Today as a Monday-first index, or -1 if it cannot be determined. */
export const currentWeekdayIndex = (now: Date = new Date()): number => {
  try {
    const name = new Intl.DateTimeFormat("en-US", {
      timeZone: MERCHANT_TIME_ZONE,
      weekday: "long",
    }).format(now)
    return WEEKDAY_NAMES.indexOf(name)
  } catch {
    // An environment without that timezone should highlight nothing rather than
    // confidently bold the wrong day.
    return -1
  }
}

/**
 * Whether a stored hours line is today's.
 *
 * Matches on the "Monday: " prefix the line carries rather than its position,
 * because a location missing a day would shift every later entry and silently
 * bold the wrong one. Position is only the fallback for lines with no prefix.
 */
export const isTodayHoursLine = (line: string, index: number, today = currentWeekdayIndex()): boolean => {
  if (today < 0) return false

  const prefix = line.split(":")[0]?.trim().toLowerCase()
  const labelled = WEEKDAY_NAMES.findIndex((name) => name.toLowerCase() === prefix)
  if (labelled >= 0) return labelled === today

  return index === today
}

/**
 * Whether a merchant is open right now.
 *
 * Three answers, not two. "unknown" is the honest result for a listing whose
 * hours were never recorded, and it has to stay distinct from "closed": a map
 * that greys out every merchant Google never published hours for is telling
 * customers those shops are shut, which is a claim we cannot make.
 */
export type OpenState = "open" | "closed" | "unknown"

/** Minutes since midnight in the merchant's timezone, or -1 if undeterminable. */
export const currentMerchantMinutes = (now: Date = new Date()): number => {
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: MERCHANT_TIME_ZONE,
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).formatToParts(now)
    const hour = Number(parts.find((part) => part.type === "hour")?.value)
    const minute = Number(parts.find((part) => part.type === "minute")?.value)
    if (!Number.isFinite(hour) || !Number.isFinite(minute)) return -1
    // Some locales render midnight as hour 24 under hour12:false.
    return (hour % 24) * 60 + minute
  } catch {
    return -1
  }
}

const dayFor = (hours: LocationDayHours[], weekday: number): LocationDayHours | undefined =>
  hours.find((day) => day.weekday === weekday)

/**
 * Resolve a merchant's current open state from their structured week.
 *
 * A stretch whose close time is before its open time runs past midnight, which
 * is ordinary for bars and late kitchens — so yesterday's late shift is checked
 * as well as today's. That is the whole reason this cannot be a simple
 * "is now between open and close" test.
 */
export const getOpenState = (hours: LocationDayHours[] | undefined, now: Date = new Date()): OpenState => {
  if (!hours || hours.length === 0) return "unknown"

  const today = currentWeekdayIndex(now)
  const minutes = currentMerchantMinutes(now)
  if (today < 0 || minutes < 0) return "unknown"

  const todayHours = dayFor(hours, today)
  const yesterdayHours = dayFor(hours, (today + 6) % 7)

  for (const interval of todayHours?.intervals ?? []) {
    const { open_minute: open, close_minute: close } = interval
    if (close > open ? minutes >= open && minutes < close : minutes >= open) {
      return "open"
    }
  }

  // A stretch that started yesterday and has not closed yet.
  for (const interval of yesterdayHours?.intervals ?? []) {
    const { open_minute: open, close_minute: close } = interval
    if (close < open && minutes < close) return "open"
  }

  // Only claim "closed" for a day we actually know something about. An empty,
  // un-flagged day means the hours were never recorded.
  if (todayHours?.is_closed || (todayHours?.intervals?.length ?? 0) > 0) return "closed"

  return "unknown"
}

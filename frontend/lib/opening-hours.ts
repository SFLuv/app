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

"use client"

import { useEffect, useState } from "react"

import { getOpenState, type OpenState } from "@/lib/opening-hours"
import type { LocationDayHours } from "@/types/location"

/**
 * A clock that ticks once a minute, shared by everything that shows whether a
 * merchant is open.
 *
 * One timer per component would mean one per pin on a map of a hundred
 * merchants. Callers take this single value and derive their own state from it.
 *
 * Starts as null and only becomes a real time after mount: an open/closed
 * answer computed during server rendering can disagree with the one computed a
 * moment later in the browser, and React treats that as a hydration error.
 */
export function useMinuteTick(): Date | null {
  const [now, setNow] = useState<Date | null>(null)

  useEffect(() => {
    setNow(new Date())

    // Align to the next minute boundary so every card flips at the same moment
    // rather than drifting apart by however long each one took to mount.
    let interval: ReturnType<typeof setInterval> | undefined
    const msToNextMinute = 60_000 - (Date.now() % 60_000)
    const timeout = setTimeout(() => {
      setNow(new Date())
      interval = setInterval(() => setNow(new Date()), 60_000)
    }, msToNextMinute)

    return () => {
      clearTimeout(timeout)
      if (interval) clearInterval(interval)
    }
  }, [])

  return now
}

/** Open state for one merchant, recomputed as the shared clock ticks. */
export function useOpenState(hours: LocationDayHours[] | undefined): OpenState {
  const now = useMinuteTick()
  if (now === null) return "unknown"
  return getOpenState(hours, now)
}

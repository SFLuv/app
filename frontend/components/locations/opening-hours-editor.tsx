"use client"

import { Plus, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import type { LocationDayHours, LocationHoursInterval } from "@/types/location"

/** Storage order: index 0 is Monday, matching location_hours.weekday. */
export const WEEKDAY_NAMES = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"]

export type { LocationDayHours, LocationHoursInterval } from "@/types/location"

export const emptyWeek = (): LocationDayHours[] =>
  WEEKDAY_NAMES.map((_, weekday) => ({ weekday, is_closed: false, intervals: [] }))

/** "09:30" for a time input; empty when the day has no recorded time. */
export const minuteToTimeValue = (minute: number | null): string => {
  if (minute === null || minute === undefined) return ""
  const clamped = ((minute % 1440) + 1440) % 1440
  return `${String(Math.floor(clamped / 60)).padStart(2, "0")}:${String(clamped % 60).padStart(2, "0")}`
}

export const timeValueToMinute = (value: string): number | null => {
  const match = /^(\d{1,2}):(\d{2})$/.exec(value.trim())
  if (!match) return null
  const hour = Number(match[1])
  const minute = Number(match[2])
  if (hour > 23 || minute > 59) return null
  return hour * 60 + minute
}

/** Normalises whatever the API returned into a full Monday-first week. */
export const toWeek = (days: LocationDayHours[] | undefined | null): LocationDayHours[] => {
  const week = emptyWeek()
  for (const day of days ?? []) {
    if (day && day.weekday >= 0 && day.weekday < week.length) {
      week[day.weekday] = {
        weekday: day.weekday,
        is_closed: !!day.is_closed,
        intervals: (day.intervals ?? [])
          .filter((interval) => interval && interval.open_minute !== null && interval.close_minute !== null)
          .map((interval) => ({ open_minute: interval.open_minute, close_minute: interval.close_minute })),
      }
    }
  }
  return week
}

/** Stable signature for change detection, so a background refresh can tell a
 *  real update from an identical payload. */
export const weekSignature = (week: LocationDayHours[]): string =>
  week
    .map((day) => `${day.weekday}:${day.is_closed ? "x" : ""}:${day.intervals.map((i) => `${i.open_minute}-${i.close_minute}`).join("|")}`)
    .join(";")

/**
 * Returns the first reason this week cannot be saved, or null.
 *
 * Mirrors the server's rules so a merchant is told at the field rather than by
 * a rejected request. A close earlier than the open is allowed on purpose: that
 * is a day running past midnight, which is ordinary for bars and late kitchens.
 */
export const validateWeek = (week: LocationDayHours[]): string | null => {
  for (const day of week) {
    if (day.is_closed) continue
    const name = WEEKDAY_NAMES[day.weekday]

    for (const interval of day.intervals) {
      if (interval.open_minute === null || interval.close_minute === null) {
        return `${name} needs both an opening and a closing time.`
      }
      if (interval.open_minute === interval.close_minute) {
        return `${name} opens and closes at the same time.`
      }
    }

    // Same-day spans only: a stretch closing after midnight runs into the next
    // day and cannot overlap a later one here. Mirrors the server's rule.
    const sameDay = day.intervals.filter((interval) => interval.close_minute > interval.open_minute)
    for (let i = 0; i < sameDay.length; i += 1) {
      for (let j = i + 1; j < sameDay.length; j += 1) {
        if (sameDay[i].open_minute < sameDay[j].close_minute && sameDay[j].open_minute < sameDay[i].close_minute) {
          return `${name} has overlapping opening times.`
        }
      }
    }
  }
  return null
}

interface OpeningHoursEditorProps {
  week: LocationDayHours[]
  onChange: (week: LocationDayHours[]) => void
  manual: boolean
  onManualChange: (manual: boolean) => void
  disabled?: boolean
  /** ISO timestamp of the last successful Google sync, if any. */
  lastSyncedAt?: string | null
}

/**
 * Opening hours as real time inputs, plus the switch that decides whether the
 * nightly Google sync may overwrite them.
 *
 * Editing times does not flip the switch. Doing so automatically would mean a
 * merchant correcting one day silently opts out of updates forever, and a
 * merchant who wants Google to keep them current could never fix a single day.
 */
export function OpeningHoursEditor({
  week,
  onChange,
  manual,
  onManualChange,
  disabled = false,
  lastSyncedAt = null,
}: OpeningHoursEditorProps) {
  const updateDay = (weekday: number, patch: Partial<LocationDayHours>) => {
    onChange(week.map((day) => (day.weekday === weekday ? { ...day, ...patch } : day)))
  }

  return (
    <div className="space-y-4">
      <div className="flex items-start gap-3 rounded-lg border p-3">
        <Checkbox
          id="hours-manual"
          checked={manual}
          disabled={disabled}
          onCheckedChange={(checked) => onManualChange(checked === true)}
          className="mt-0.5"
        />
        <div className="space-y-1">
          <Label htmlFor="hours-manual" className="cursor-pointer font-medium">
            Set hours manually
          </Label>
          <p className="text-xs text-muted-foreground">
            {manual
              ? "These hours stay exactly as entered. The nightly check against Google will not change them."
              : "Hours refresh from Google each night. Anything entered here can be replaced by the next check."}
          </p>
          {!manual && lastSyncedAt && (
            <p className="text-xs text-muted-foreground">
              Last refreshed {new Date(lastSyncedAt).toLocaleString()}.
            </p>
          )}
        </div>
      </div>

      <div className="space-y-3">
        {week.map((day) => {
          const setIntervals = (intervals: LocationHoursInterval[]) =>
            updateDay(day.weekday, { intervals })

          return (
            <div key={day.weekday} className="rounded-lg border p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <Label className="text-sm font-medium">{WEEKDAY_NAMES[day.weekday]}</Label>
                <label className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={day.is_closed}
                    disabled={disabled}
                    onCheckedChange={(checked) =>
                      updateDay(day.weekday, {
                        is_closed: checked === true,
                        // Dropping the stretches on close keeps "closed" and "we
                        // never learned this day" from collapsing into each other.
                        ...(checked === true ? { intervals: [] } : {}),
                      })
                    }
                  />
                  Closed
                </label>
              </div>

              {!day.is_closed && (
                <div className="mt-2 space-y-2">
                  {day.intervals.map((interval, index) => (
                    <div
                      key={index}
                      className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)_auto] items-center gap-2"
                    >
                      <Input
                        type="time"
                        aria-label={`${WEEKDAY_NAMES[day.weekday]} opening time ${index + 1}`}
                        value={minuteToTimeValue(interval.open_minute)}
                        disabled={disabled}
                        onChange={(event) => {
                          const minute = timeValueToMinute(event.target.value)
                          setIntervals(
                            day.intervals.map((entry, position) =>
                              position === index ? { ...entry, open_minute: minute ?? 0 } : entry,
                            ),
                          )
                        }}
                      />
                      <span className="text-xs text-muted-foreground">to</span>
                      <Input
                        type="time"
                        aria-label={`${WEEKDAY_NAMES[day.weekday]} closing time ${index + 1}`}
                        value={minuteToTimeValue(interval.close_minute)}
                        disabled={disabled}
                        onChange={(event) => {
                          const minute = timeValueToMinute(event.target.value)
                          setIntervals(
                            day.intervals.map((entry, position) =>
                              position === index ? { ...entry, close_minute: minute ?? 0 } : entry,
                            ),
                          )
                        }}
                      />
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        disabled={disabled}
                        aria-label={`Remove ${WEEKDAY_NAMES[day.weekday]} stretch ${index + 1}`}
                        onClick={() => setIntervals(day.intervals.filter((_, position) => position !== index))}
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  ))}

                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={disabled}
                    onClick={() => {
                      // A second stretch starts after the last one ends, which is
                      // the shape of a split day and saves the common case a click.
                      const last = day.intervals[day.intervals.length - 1]
                      const open = last ? Math.min(last.close_minute + 60, 23 * 60) : 9 * 60
                      setIntervals([...day.intervals, { open_minute: open, close_minute: Math.min(open + 240, 23 * 60 + 59) }])
                    }}
                  >
                    <Plus className="mr-2 h-3.5 w-3.5" />
                    {day.intervals.length === 0 ? "Add hours" : "Add another stretch"}
                  </Button>
                </div>
              )}
            </div>
          )
        })}
      </div>

      <p className="text-xs text-muted-foreground">
        A day with no stretches is shown as &ldquo;hours not available&rdquo;, which is different from being
        closed. Add a second stretch for a split day, such as lunch and dinner. A closing time earlier than its
        opening time means that stretch runs past midnight.
      </p>
    </div>
  )
}

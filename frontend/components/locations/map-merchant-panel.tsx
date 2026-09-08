"use client"

import { useMemo, useState } from "react"
import { ChevronLeft, ChevronRight, Search } from "lucide-react"

import { Input } from "@/components/ui/input"
import { MerchantIcon, OpenStatusBadge } from "@/components/locations/merchant-pin"
import { getOpenState, type OpenState } from "@/lib/opening-hours"
import type { Location } from "@/types/location"
import type { UserLocation } from "@/types/merchant"
import { calculateDistance } from "@/utils/location"
import { cn } from "@/lib/utils"

/** Sort order for the list: open, then unknown, then closed. */
const openRank = (state: OpenState): number => (state === "open" ? 0 : state === "closed" ? 2 : 1)

interface MapMerchantPanelProps {
  locations: Location[]
  /** Shared clock, so panel rows and pins never disagree about who is open. */
  now: Date | null
  /** Opens the merchant's details, exactly as clicking their pin does. */
  onSelectLocation: (location: Location) => void
  /** Moves the map to a merchant without opening anything. */
  onFocusLocation: (location: Location) => void
  /** Sorts each open/closed band by how far away it is. */
  userLocation: UserLocation
  collapsed: boolean
  onCollapsedChange: (collapsed: boolean) => void
  /**
   * Whether the list is the chosen view on small screens, where it replaces the
   * map rather than sitting beside it.
   */
  mobileVisible: boolean
}

/**
 * The merchant list beside the map.
 *
 * Shares the map's already-filtered locations rather than fetching or filtering
 * again, so the count in the panel is always the count of pins on screen. Its
 * search narrows only the list: emptying the map as someone types would hide
 * the very pins they are trying to locate.
 */
export function MapMerchantPanel({
  locations,
  now,
  onSelectLocation,
  onFocusLocation,
  userLocation,
  collapsed,
  onCollapsedChange,
  mobileVisible,
}: MapMerchantPanelProps) {
  const [query, setQuery] = useState("")

  const matches = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const found =
      needle === ""
        ? locations
        : locations.filter((location) =>
            `${location.name} ${location.type} ${location.city} ${location.street}`
              .toLowerCase()
              .includes(needle),
          )

    // Open merchants first, shut ones last. Someone scanning this list is
    // deciding where to go now, and a closed shop is not an answer to that.
    // Merchants whose hours we never learned sit between the two rather than
    // being sunk with the closed: we have no grounds to rule them out.
    //
    // Nearest first within each band, because "open" alone still leaves a
    // list too long to read and distance is the next thing anyone asks.
    return [...found].sort((left, right) => {
      const byState =
        openRank(now === null ? "unknown" : getOpenState(left.hours, now)) -
        openRank(now === null ? "unknown" : getOpenState(right.hours, now))
      if (byState !== 0) return byState

      return (
        calculateDistance(userLocation.lat, userLocation.lng, left.lat, left.lng) -
        calculateDistance(userLocation.lat, userLocation.lng, right.lat, right.lng)
      )
    })
  }, [locations, now, query, userLocation.lat, userLocation.lng])

  return (
    <>
      {/*
        The collapsed rail is a desktop affordance only. On small screens the
        list IS the view, and the Map/List toggle above already does this job.
      */}
      <button
        type="button"
        onClick={() => onCollapsedChange(false)}
        className={cn(
          "hidden shrink-0 flex-col items-center gap-2 rounded-xl border border-border/60 bg-card px-2 py-3 text-muted-foreground transition-colors hover:text-foreground",
          collapsed ? "md:flex" : "md:hidden",
        )}
        aria-label="Show merchant list"
      >
        <ChevronLeft className="h-4 w-4" />
        <span className="text-xs font-medium [writing-mode:vertical-rl]">
          {locations.length} merchant{locations.length === 1 ? "" : "s"}
        </span>
      </button>

      <aside
        className={cn(
          "shrink-0 flex-col overflow-hidden rounded-xl border border-border/60 bg-card",
          mobileVisible ? "flex w-full" : "hidden",
          collapsed ? "md:hidden" : "md:flex md:w-[17rem] lg:w-[19rem] xl:w-[21rem]",
        )}
      >
        <div className="flex items-center gap-2 border-b border-border/60 p-2.5">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search merchants"
              className="h-9 rounded-lg border-border/60 bg-background pl-8 text-sm"
            />
          </div>
          <button
            type="button"
            onClick={() => onCollapsedChange(true)}
            className="hidden shrink-0 rounded-lg p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground md:block"
            aria-label="Hide merchant list"
          >
            <ChevronRight className="h-4 w-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {matches.length === 0 ? (
            <p className="p-4 text-sm text-muted-foreground">No merchants match that search.</p>
          ) : (
            <ul className="divide-y divide-border/50">
              {matches.map((location) => {
                const state = now === null ? "unknown" : getOpenState(location.hours, now)

                return (
                  <li key={location.id}>
                    <button
                      type="button"
                      // Hovering the row moves the map, clicking opens the card:
                      // browsing the list should not cost a click per merchant.
                      onMouseEnter={() => onFocusLocation(location)}
                      onFocus={() => onFocusLocation(location)}
                      onClick={() => onSelectLocation(location)}
                      className="flex w-full items-start gap-2.5 px-3 py-2.5 text-left transition-colors hover:bg-muted/60"
                    >
                      <div className="h-9 w-9 shrink-0 overflow-hidden rounded-lg border border-border/60">
                        <MerchantIcon name={location.name} iconUrl={location.icon_url} size={36} state={state} />
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium text-foreground">{location.name}</p>
                        <p className="truncate text-xs capitalize text-muted-foreground">
                          {[location.type, location.city].filter(Boolean).join(" • ")}
                        </p>
                        <OpenStatusBadge state={state} className="mt-1" />
                      </div>
                    </button>
                  </li>
                )
              })}
            </ul>
          )}
        </div>

        <p className="border-t border-border/60 px-3 py-2 text-xs text-muted-foreground">
          {matches.length} of {locations.length} shown
        </p>
      </aside>
    </>
  )
}

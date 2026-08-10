"use client"

import { memo, useCallback, useEffect, useMemo, useState } from "react"
import { Card, CardContent } from "@/components/ui/card"
import { List, Map as MapIcon } from "lucide-react"
import type { UserLocation } from "@/types/merchant"
import { AdvancedMarker, APIProvider, Map, useMap } from "@vis.gl/react-google-maps"
import { useLocation } from "@/context/LocationProvider"
import { Location } from "@/types/location"
import { GOOGLE_MAPS_API_KEY, MAP_CENTER, MAP_ID } from "@/lib/constants"
import { MerchantMapPin } from "@/components/locations/merchant-pin"
import { MapMerchantPanel } from "@/components/locations/map-merchant-panel"
import { useMinuteTick } from "@/hooks/use-open-state"
import { getOpenState } from "@/lib/opening-hours"
import { cn } from "@/lib/utils"

/**
 * Map pins.
 *
 * Declared at module scope, NOT inside MapView. A component defined in a render
 * body is a new component type on every render, so React unmounts and remounts
 * the entire pin tree each time — which is what made the icons flicker whenever
 * anything above re-rendered, including the background user-record poll.
 *
 * memo keeps it still even when the parent re-renders for unrelated reasons;
 * all three props are referentially stable at the call site.
 */
const PoiMarkers = memo(function PoiMarkers({
  locations,
  onSelectLocation,
  now,
}: {
  locations: Location[]
  onSelectLocation: (location: Location) => void
  now: Date | null
}) {
  return (
    <>
      {locations.map((currentLocation) => (
        <AdvancedMarker
          key={currentLocation.id}
          position={{ lat: currentLocation.lat, lng: currentLocation.lng }}
          clickable={true}
          onClick={() => onSelectLocation(currentLocation)}
          title={currentLocation.name}
        >
          <MerchantMapPin
            name={currentLocation.name}
            iconUrl={currentLocation.icon_url}
            state={now === null ? "unknown" : getOpenState(currentLocation.hours, now)}
          />
        </AdvancedMarker>
      ))}
    </>
  )
})

/**
 * Pans the map to whichever merchant the list is pointing at.
 *
 * A child of Map rather than a hook in MapView, because useMap only resolves
 * inside the map's own context. Renders nothing.
 */
const MapFocus = memo(function MapFocus({ target }: { target: Location | null }) {
  const map = useMap()

  useEffect(() => {
    if (!map || !target) return
    map.panTo({ lat: target.lat, lng: target.lng })
    // Only zoom in on someone already zoomed out; yanking the zoom back on
    // every hover would fight a user who deliberately zoomed in.
    if ((map.getZoom() ?? 0) < 15) map.setZoom(15)
  }, [map, target])

  return null
})

interface MapViewProps {
  locations: Location[]
  selectedLocationType: string
  onSelectLocation: (location: Location) => void
  userLocation: UserLocation
  setUserLocation: (userLocation: UserLocation) => void
}

export function MapView({
  locations,
  selectedLocationType,
  onSelectLocation,
  userLocation,
  setUserLocation: _setUserLocation,
}: MapViewProps) {
  const { mapLocationsStatus } = useLocation()
  const mapHeightClass = "h-[calc(100svh-320px)] sm:h-[calc(100svh-300px)]"
  // One clock for every pin and every row. Ticking it re-renders them once a
  // minute, which is what recolours a merchant the moment they shut.
  const now = useMinuteTick()
  const [panelCollapsed, setPanelCollapsed] = useState(false)
  const [focused, setFocused] = useState<Location | null>(null)
  // Only consulted below md, where the map and the list cannot share the width
  // and one has to give way to the other.
  const [mobileView, setMobileView] = useState<"map" | "list">("map")

  const filteredLocations = useMemo(() => {
    return (locations ?? []).filter((location) => {
      const locationType = (location.type || "").trim()
      return selectedLocationType === "All Locations" || locationType === selectedLocationType
    })
  }, [locations, selectedLocationType])

  // Stable identity so the memoised pins are not invalidated by a new closure
  // on every render of this component.
  const handleSelectLocation = useCallback(
    (location: Location) => onSelectLocation(location),
    [onSelectLocation],
  )

  const handleFocusLocation = useCallback((location: Location) => setFocused(location), [])

  if (mapLocationsStatus === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  const showListOnMobile = mobileView === "list"

  return (
    <div className="space-y-3 px-1 pt-1">
      {/*
        Below md the merchant list cannot sit beside the map without starving
        both, so it takes the map's place instead and this chooses between
        them. From md up the two are side by side and the toggle is redundant.
      */}
      <div className="relative grid grid-cols-2 rounded-lg bg-secondary p-1 md:hidden">
        <div
          className={cn(
            "absolute inset-y-1 left-1 w-[calc(50%-0.25rem)] rounded-md bg-[#eb6c6c] shadow-sm transition-transform duration-300 ease-out",
            showListOnMobile ? "translate-x-full" : "translate-x-0",
          )}
        />
        <button
          type="button"
          onClick={() => setMobileView("map")}
          className={cn(
            "relative z-10 inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium transition-colors",
            showListOnMobile ? "text-foreground/80 hover:text-foreground" : "text-white",
          )}
        >
          <MapIcon className="h-4 w-4" />
          Map View
        </button>
        <button
          type="button"
          onClick={() => setMobileView("list")}
          className={cn(
            "relative z-10 inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium transition-colors",
            showListOnMobile ? "text-white" : "text-foreground/80 hover:text-foreground",
          )}
        >
          <List className="h-4 w-4" />
          List View
        </button>
      </div>

      <Card className="mt-2 overflow-hidden rounded-2xl border shadow-sm">
        <CardContent className="overflow-hidden rounded-2xl p-2 sm:p-2.5">
          {/*
            Desktop splits the card: the map takes roughly two thirds and the
            merchant list the rest. Below md the list would leave the map too
            narrow to be a map, so it drops away and the map takes the width.
          */}
          <div className={`${mapHeightClass} flex max-h-[560px] min-h-[280px] w-full gap-2.5 sm:min-h-[340px]`}>
            <div
              className={cn(
                "h-full min-w-0 flex-1 overflow-hidden rounded-xl bg-muted/30",
                showListOnMobile && "hidden md:block",
              )}
            >
              <div className="h-full w-full overflow-hidden rounded-xl">
                <APIProvider apiKey={GOOGLE_MAPS_API_KEY}>
                  <Map
                    defaultZoom={12}
                    defaultCenter={MAP_CENTER}
                    mapId={MAP_ID}
                    gestureHandling="greedy"
                    className="h-full w-full"
                  >
                    <MapFocus target={focused} />
                  </Map>
                  <PoiMarkers locations={filteredLocations} onSelectLocation={handleSelectLocation} now={now} />
                </APIProvider>
              </div>
            </div>

            <MapMerchantPanel
              locations={filteredLocations}
              now={now}
              onSelectLocation={handleSelectLocation}
              onFocusLocation={handleFocusLocation}
              userLocation={userLocation}
              collapsed={panelCollapsed}
              onCollapsedChange={setPanelCollapsed}
              mobileVisible={showListOnMobile}
            />
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

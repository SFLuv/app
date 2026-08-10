"use client"

import { memo, useCallback, useEffect, useMemo, useState } from "react"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { SlidersHorizontal } from "lucide-react"
import type { UserLocation } from "@/types/merchant"
import { AdvancedMarker, APIProvider, Map, useMap } from "@vis.gl/react-google-maps"
import { useLocation } from "@/context/LocationProvider"
import { Location } from "@/types/location"
import { GOOGLE_MAPS_API_KEY, MAP_CENTER, MAP_ID } from "@/lib/constants"
import { MerchantMapPin } from "@/components/locations/merchant-pin"
import { MapMerchantPanel } from "@/components/locations/map-merchant-panel"
import { useMinuteTick } from "@/hooks/use-open-state"
import { getOpenState } from "@/lib/opening-hours"

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
  setSelectedLocationType: (type: string) => void
  onSelectLocation: (location: Location) => void
  userLocation: UserLocation
  setUserLocation: (userLocation: UserLocation) => void
}

export function MapView({
  locations,
  selectedLocationType,
  setSelectedLocationType,
  onSelectLocation,
  userLocation: _userLocation,
  setUserLocation: _setUserLocation,
}: MapViewProps) {
  const { mapLocationsStatus, locationTypes } = useLocation()
  const mapHeightClass = "h-[calc(100svh-320px)] sm:h-[calc(100svh-300px)]"
  // One clock for every pin and every row. Ticking it re-renders them once a
  // minute, which is what recolours a merchant the moment they shut.
  const now = useMinuteTick()
  const [panelCollapsed, setPanelCollapsed] = useState(false)
  const [focused, setFocused] = useState<Location | null>(null)

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

  return (
    <div className="space-y-4 px-1 pt-4 sm:pt-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Select value={selectedLocationType} onValueChange={setSelectedLocationType}>
            <SelectTrigger className="h-9 w-[180px] rounded-lg border-border/60 bg-background sm:w-[210px]">
              <div className="flex items-center gap-2">
                <SlidersHorizontal className="h-4 w-4 text-muted-foreground" />
                <SelectValue placeholder="Filter by type" />
              </div>
            </SelectTrigger>
            <SelectContent>
              {locationTypes.map(type => (
                <SelectItem key={type} value={type}>
                  {type}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {selectedLocationType !== "All Locations" ? (
            <Badge variant="outline" className="rounded-full border-border/70 bg-background px-3 py-1 text-xs font-medium">
              {selectedLocationType}
            </Badge>
          ) : null}
        </div>
        <div className="text-xs text-muted-foreground">
          {filteredLocations.length} location{filteredLocations.length === 1 ? "" : "s"}
        </div>
      </div>

      <Card className="mt-2 overflow-hidden rounded-2xl border shadow-sm">
        <CardContent className="overflow-hidden rounded-2xl p-2 sm:p-2.5">
          {/*
            Desktop splits the card: the map takes roughly two thirds and the
            merchant list the rest. Below md the list would leave the map too
            narrow to be a map, so it drops away and the map takes the width.
          */}
          <div className={`${mapHeightClass} flex max-h-[560px] min-h-[280px] w-full gap-2.5 sm:min-h-[340px]`}>
            <div className="h-full min-w-0 flex-1 overflow-hidden rounded-xl bg-muted/30">
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
              collapsed={panelCollapsed}
              onCollapsedChange={setPanelCollapsed}
            />
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

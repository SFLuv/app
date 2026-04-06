"use client"

import { useMemo, useState } from "react"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { SlidersHorizontal } from "lucide-react"
import type { UserLocation } from "@/types/merchant"
import { AdvancedMarker, APIProvider, Map, Pin } from "@vis.gl/react-google-maps"
import type { Marker } from "@googlemaps/markerclusterer"
import { useLocation } from "@/context/LocationProvider"
import { Location } from "@/types/location"
import { GOOGLE_MAPS_API_KEY, MAP_CENTER, MAP_ID } from "@/lib/constants"

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

  const PoiMarkers = (props: { locations: Location[] }) => {
    const [markers, setMarkers] = useState<{ [key: number]: Marker }>({})

    const setMarkerRef = (marker: Marker | null, key: number) => {
      if (marker && markers[key]) return
      if (!marker && !markers[key]) return

      setMarkers(prev => {
        if (marker) {
          return { ...prev, [key]: marker }
        } else {
          const nextMarkers = { ...prev }
          delete nextMarkers[key]
          return nextMarkers
        }
      })
    }

    return (
      <>
        {props.locations.map(currentLocation => (
          <AdvancedMarker
            key={currentLocation.id}
            position={
              {
                lat: currentLocation.lat,
                lng: currentLocation.lng,
              }
            }
            ref={(marker: Marker | null) => setMarkerRef(marker, currentLocation.id)}
            clickable={true}
            onClick={() => onSelectLocation(currentLocation)}
          >
            <Pin background="#eb6c6c" glyphColor="#111111" borderColor="#111111" />
          </AdvancedMarker>
        ))}
      </>
    )
  }

  const filteredLocations = useMemo(() => {
    return locations?.filter(location => {
      const locationType = (location.type || "").trim()
      return selectedLocationType === "All Locations" || locationType === selectedLocationType
    })
  }, [locations, selectedLocationType])

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
          <div className={`${mapHeightClass} max-h-[500px] min-h-[250px] w-full overflow-hidden rounded-xl bg-muted/30 sm:min-h-[310px]`}>
            <div className="h-full w-full overflow-hidden rounded-xl">
              <APIProvider apiKey={GOOGLE_MAPS_API_KEY}>
                <Map
                  defaultZoom={12}
                  defaultCenter={MAP_CENTER}
                  mapId={MAP_ID}
                  gestureHandling="greedy"
                  className="h-full w-full"
                />
                <PoiMarkers locations={filteredLocations ?? []} />
              </APIProvider>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

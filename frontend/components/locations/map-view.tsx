"use client"

import { useState, useEffect, useCallback, useRef, useMemo } from "react"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardContent } from "@/components/ui/card"
import { Search, MapPin } from "lucide-react"
import type { Merchant, UserLocation } from "@/types/merchant"
import type { GoogleMerchant } from "@/types/google-merchant"
import {AdvancedMarker, APIProvider, Map, MapCameraChangedEvent, Pin, useMap} from '@vis.gl/react-google-maps'
import type {Marker} from '@googlemaps/markerclusterer';
import { useLocation } from "@/context/LocationProvider"
import { Location } from "@/types/location"
import { GOOGLE_MAPS_API_KEY, MAP_CENTER, MAP_ID } from "@/lib/constants"

type Poi ={ key: string, location: google.maps.LatLngLiteral }
interface MapViewProps {
  locations: Location[]
  selectedLocationType: string
  setSelectedLocationType: (type: string) => void
  onSelectLocation: (location: Location) => void
  userLocation: UserLocation
  setUserLocation: (userlocation: UserLocation) => void
}

export function MapView({
  locations,
  selectedLocationType,
  setSelectedLocationType,
  onSelectLocation,
  userLocation,
  setUserLocation,
}: MapViewProps) {
  const [locationInput, setLocationInput] = useState(userLocation.address || "")
  const { mapLocationsStatus, locationTypes } = useLocation();
  const [searchQuery, setSearchQuery] = useState("")

  const PoiMarkers = (props: {locations: Location[]}) => {
    const [markers, setMarkers] = useState<{[key: number]: Marker}>({});

    const setMarkerRef = (marker: Marker | null, key: number) => {
      if (marker && markers[key]) return;
      if (!marker && !markers[key]) return;

      setMarkers(prev => {
        if (marker) {
          return {...prev, [key]: marker};
        } else {
          const newMarkers = {...prev};
          delete newMarkers[key];
          return newMarkers;
        }
      });
    };

    return (
      <>
        {props.locations.map( (currentLocation: Location) => (
          <AdvancedMarker
            key={currentLocation.id}
            position={
              {
              lat: currentLocation.lat,
              lng: currentLocation.lng
              }
            }
            ref={(marker: Marker | null) => setMarkerRef(marker, currentLocation.id)}
            clickable={true}
            onClick={() => onSelectLocation(currentLocation)}
            >
            <Pin background={'#eb6c6c'} glyphColor={'#000'} borderColor={'#000'} />
          </AdvancedMarker>
        ))}
      </>
    );
  };


  // Filter merchants by type
  const filteredLocations = useMemo(() => {
  return locations?.filter((location) => {
    const matchesSearch =
      searchQuery === "" ||
      location.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      location.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
      location.city.toLowerCase().includes(searchQuery.toLowerCase());

    const matchesType =
      selectedLocationType === "All Locations" ||
      location.type === selectedLocationType;

    return matchesType && matchesSearch;
  })
}, [selectedLocationType, searchQuery])


  if (mapLocationsStatus === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }


  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row gap-4">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 h-4 w-4" />
          <Input
            placeholder="Search merchants..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10 text-black dark:text-white bg-secondary"
          />
        </div>
        <Select value={selectedLocationType} onValueChange={setSelectedLocationType}>
          <SelectTrigger className="w-[180px] text-black dark:text-white bg-secondary">
            <SelectValue placeholder="Filter by type" />
          </SelectTrigger>
          <SelectContent>
            {locationTypes.map((type) => (
              <SelectItem key={type} value={type}>
                {type}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <Card className="border bg-card">
        <CardContent className="p-0">
          <div className="w-full h-[600px] bg-gray-100 dark:bg-gray-800 flex items-center justify-center p-4 rounded-lg">
            <APIProvider apiKey={GOOGLE_MAPS_API_KEY}>
                  <Map
                    defaultZoom={12}
                    defaultCenter={ MAP_CENTER }
                    mapId={ MAP_ID }
                  >
                </Map>
                <PoiMarkers locations={filteredLocations ?? []} />
              </APIProvider>
          </div>
        </CardContent>
      </Card>

      <div className="text-sm text-gray-500 dark:text-gray-400">
        Showing {filteredLocations.length} merchants
        {selectedLocationType !== "all" &&
          ` of type: ${locationTypes.find((t) => t === selectedLocationType)}`}
      </div>
    </div>
  )
}

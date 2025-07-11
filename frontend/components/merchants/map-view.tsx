"use client"

import { useState, useEffect, useCallback, useRef } from "react"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardContent } from "@/components/ui/card"
import { Search, MapPin } from "lucide-react"
import type { Merchant, UserLocation } from "@/types/merchant"
import type { GoogleMerchant } from "@/types/google-merchant"
import { merchantTypes, defaultLocation } from "@/data/mock-merchants"
import {AdvancedMarker, APIProvider, Map, MapCameraChangedEvent, Pin, useMap} from '@vis.gl/react-google-maps'
import type {Marker} from '@googlemaps/markerclusterer';

type Poi ={ key: string, location: google.maps.LatLngLiteral }
interface MapViewProps {
  merchants: Merchant[]
  selectedMerchantType: string
  setSelectedMerchantType: (type: string) => void
  onSelectMerchant: (merchant: Merchant) => void
  userLocation: UserLocation
  setUserLocation: (location: UserLocation) => void
}

export function MapView({
  merchants,
  selectedMerchantType,
  setSelectedMerchantType,
  onSelectMerchant,
  userLocation,
  setUserLocation,
}: MapViewProps) {
  const [locationInput, setLocationInput] = useState(userLocation.address || "")
  const [isMapLoaded, setIsMapLoaded] = useState(false)

  const PoiMarkers = (props: {merchants: Merchant[]}) => {
    const [markers, setMarkers] = useState<{[key: string]: Marker}>({});
    const map = useMap();

    const setMarkerRef = (marker: Marker | null, key: string) => {
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
        {props.merchants.map( (currentMerchant: Merchant) => (
          <AdvancedMarker
            key={currentMerchant.name}
            position={
              {
              lat: currentMerchant.address.coordinates.lat,
              lng: currentMerchant.address.coordinates.lng
              }
            }
            ref={(marker: Marker | null) => setMarkerRef(marker, currentMerchant.name)}
            clickable={true}
            onClick={() => onSelectMerchant(currentMerchant)}
            >
            <Pin background={'#eb6c6c'} glyphColor={'#000'} borderColor={'#000'} />
          </AdvancedMarker>
        ))}
      </>
    );
  };

  // Filter merchants by type
  const filteredMerchants = merchants.filter(
    (merchant) => selectedMerchantType === "all" || merchant.type === selectedMerchantType,
  )

  // Initialize Google Maps
  useEffect(() => {
    // This would be the actual implementation with a real API key
    // For now, we'll just simulate the map loading
    const loadMap = async () => {
            setIsMapLoaded(true)
    }
    loadMap()
  }, [])



  // Handle location search
  const handleLocationSearch = () => {
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row gap-4">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 h-4 w-4" />
          <Input
            placeholder="Enter an address..."
            value={locationInput}
            onChange={(e) => setLocationInput(e.target.value)}
            className="pl-10 text-black dark:text-white bg-secondary"
            onKeyDown={(e) => e.key === "Enter" && handleLocationSearch()}
          />
        </div>
        <Button onClick={handleLocationSearch} className="bg-[#eb6c6c] hover:bg-[#d55c5c]">
          <MapPin className="h-4 w-4 mr-2" />
          Go to Location
        </Button>
        <Select value={selectedMerchantType} onValueChange={setSelectedMerchantType}>
          <SelectTrigger className="w-[180px] text-black dark:text-white bg-secondary">
            <SelectValue placeholder="Filter by type" />
          </SelectTrigger>
          <SelectContent>
            {merchantTypes.map((type) => (
              <SelectItem key={type.value} value={type.value}>
                {type.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <Card className="border bg-card">
        <CardContent className="p-0">
          <div className="w-full h-[600px] bg-gray-100 dark:bg-gray-800 flex items-center justify-center p-4 rounded-lg">
            {!isMapLoaded ? (
              <div className="text-center">
                <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c] mb-4"></div>
                <p className="text-gray-500 dark:text-gray-400">Loading map...</p>
              </div>
            ) : (
              <APIProvider apiKey={'AIzaSyDushyc7TgeFyIlxbqiujHdydWDoVoHwNQ'}>
                    <Map
                      defaultZoom={12}
                      defaultCenter={ { lat: defaultLocation.lat, lng: defaultLocation.lng } }
                      mapId='5d823aa5e32225a021e19266'
                    >
                  </Map>
                  <PoiMarkers merchants={merchants} />
                </APIProvider>
            )}
          </div>
        </CardContent>
      </Card>

      <div className="text-sm text-gray-500 dark:text-gray-400">
        Showing {filteredMerchants.length} merchants
        {selectedMerchantType !== "all" &&
          ` of type: ${merchantTypes.find((t) => t.value === selectedMerchantType)?.label}`}
      </div>
    </div>
  )
}
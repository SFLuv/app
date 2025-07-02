"use client"

import { useState, useEffect, useRef } from "react"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardContent } from "@/components/ui/card"
import { Search, MapPin } from "lucide-react"
import type { Merchant, UserLocation } from "@/types/merchant"
import { merchantTypes, defaultLocation } from "@/data/mock-merchants"

// Declare the google variable
declare global {
  interface Window {
    google: any
  }
}

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
  const mapRef = useRef<HTMLDivElement>(null)
  const googleMapRef = useRef<google.maps.Map | null>(null)
  const markersRef = useRef<google.maps.Marker[]>([])

  // Filter merchants by type
  const filteredMerchants = merchants.filter(
    (merchant) => selectedMerchantType === "all" || merchant.type === selectedMerchantType,
  )

  // Initialize Google Maps
  useEffect(() => {
    // This would be the actual implementation with a real API key
    // For now, we'll just simulate the map loading
    const loadMap = async () => {
      try {
        // In a real implementation, we would load the Google Maps API here
        console.log("Loading Google Maps...")

        // Simulate map loading
        setTimeout(() => {
          if (mapRef.current) {
            // Create a mock map object for our simulation
            const mockMap = {
              setCenter: (location: { lat: number; lng: number }) => {
                console.log("Map center set to:", location)
              },
              setZoom: (zoom: number) => {
                console.log("Map zoom set to:", zoom)
              },
            } as unknown as google.maps.Map

            googleMapRef.current = mockMap
            setIsMapLoaded(true)

            // Initialize markers after map is loaded
            initializeMarkers()
          }
        }, 1000)
      } catch (error) {
        console.error("Error loading Google Maps:", error)
      }
    }

    loadMap()

    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Update markers when filtered merchants change
  useEffect(() => {
    if (isMapLoaded) {
      initializeMarkers()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filteredMerchants, isMapLoaded])

  // Initialize markers for merchants
  const initializeMarkers = () => {
    // Clear existing markers
    markersRef.current.forEach((marker) => {
      marker.setMap(null)
    })
    markersRef.current = []

    // In a real implementation, we would create actual Google Maps markers
    // For now, we'll just log the markers we would create
    filteredMerchants.forEach((merchant) => {
      console.log(
        `Creating marker for ${merchant.name} at ${merchant.address.coordinates.lat}, ${merchant.address.coordinates.lng}`,
      )

      // Mock marker creation
      const mockMarker = {
        setMap: (map: google.maps.Map | null) => {
          console.log(`Marker for ${merchant.name} ${map ? "added to" : "removed from"} map`)
        },
        addListener: (event: string, callback: () => void) => {
          console.log(`Added ${event} listener to marker for ${merchant.name}`)
          return { remove: () => {} }
        },
      } as unknown as google.maps.Marker

      markersRef.current.push(mockMarker)
    })
  }

  // Handle location search
  const handleLocationSearch = () => {
    if (!locationInput) return

    // In a real implementation, we would use the Google Maps Geocoding API
    // For now, we'll just simulate finding the location
    console.log(`Searching for location: ${locationInput}`)

    // Simulate geocoding result with a slight offset from the default location
    const newLocation = {
      lat: defaultLocation.lat + (Math.random() - 0.5) * 0.02,
      lng: defaultLocation.lng + (Math.random() - 0.5) * 0.02,
      address: locationInput,
    }

    setUserLocation(newLocation)

    // Center map on new location
    if (googleMapRef.current) {
      googleMapRef.current.setCenter(newLocation)
      googleMapRef.current.setZoom(14)
    }
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
          <div ref={mapRef} className="w-full h-[600px] bg-gray-100 dark:bg-gray-800 flex items-center justify-center">
            {!isMapLoaded ? (
              <div className="text-center">
                <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c] mb-4"></div>
                <p className="text-gray-500 dark:text-gray-400">Loading map...</p>
              </div>
            ) : (
              <div className="text-center">
                <MapPin className="h-12 w-12 text-[#eb6c6c] mx-auto mb-4" />
                <p className="text-gray-700 dark:text-gray-300">
                  Map would display here with {filteredMerchants.length} merchant pins
                </p>
                <p className="text-gray-500 dark:text-gray-400 text-sm mt-2">
                  (Google Maps API integration would be implemented here)
                </p>
              </div>
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

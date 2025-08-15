"use client"

import { useEffect, useState } from "react"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { MapView } from "@/components/locations/map-view"
import { ListView } from "@/components/locations/list-view"
import { LocationModal } from "@/components/locations/location-modal"
import { defaultLocation } from "@/data/mock-merchants"
import type { UserLocation } from "@/types/merchant"
import { useApp } from "@/context/AppProvider"
import { useLocation } from "@/context/LocationProvider"
import { Location } from "@/types/location"


export default function LocationMapPage() {
  const [activeTab, setActiveTab] = useState("map")
  const [selectedLocationType, setSelectedLocationType] = useState("all")
  const [selectedLocation, setSelectedLocation] = useState<Location | null>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [userLocation, setUserLocation] = useState<UserLocation>(defaultLocation)
  const { status } = useApp()
  const { getMapLocations, mapLocations } = useLocation()


  useEffect(() => {
    getMapLocations()
  },[])


  const handleSelectLocation = (location: Location) => {
    setSelectedLocation(location)
    setIsModalOpen(true)
  }

  const handleCloseModal = () => {
    setIsModalOpen(false)
    setSelectedLocation(null)
  }

  // will use similar function later when a user goes to add a new location
  /*function parseGoogleToMerchant(place_details: any): Merchant {
    const newMerchant : Merchant = {
        id: place_details?.id,
        name: place_details?.displayName?.text,
        description: place_details?.editorialSummary?.text,
        type: place_details?.primaryType,
        status: false,
        address: {
          street: place_details?.postalAddress?.addressLines[0],
          city: place_details?.postalAddress?.locality,
          state: place_details?.postalAddress?.administrativeArea,
          zip: place_details?.postalAddress?.postalCode,
          coordinates: {
            lat: place_details?.location?.latitude,
            lng: place_details?.location?.longitude,
          }
        },
        contactInfo: {
          phone: place_details?.nationalPhoneNumber,
          email: "",
          website: place_details?.websiteUri,
        },
        imageUrl: place_details.googleMapsLinks.photosUri,
        acceptsSFLuv: false,
        rating: place_details?.rating,
        opening_hours: place_details?.regularOpeningHours?.weekdayDescriptions,
        mapsPage: place_details?.googleMapsLinks.placesUri
    }
    return newMerchant
  }
  */

  if (status === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Location Map</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">Find locations that accept SFLuv in your area</p>
      </div>

      <Tabs defaultValue="map" value={activeTab} onValueChange={setActiveTab} className="w-full">
        <TabsList className="grid grid-cols-2 w-full mb-6 bg-secondary">
          <TabsTrigger value="map" className="text-black dark:text-white">
            Map View
          </TabsTrigger>
          <TabsTrigger value="list" className="text-black dark:text-white">
            List View
          </TabsTrigger>
        </TabsList>
        <TabsContent value="map">
          <MapView
            locations={mapLocations}
            selectedLocationType={selectedLocationType}
            setSelectedLocationType={setSelectedLocationType}
            onSelectLocation={handleSelectLocation}
            userLocation={userLocation}
            setUserLocation={setUserLocation}
          />
        </TabsContent>
        <TabsContent value="list">
          <ListView
            locations={mapLocations}
            selectedLocationType={selectedLocationType}
            setSelectedLocationType={setSelectedLocationType}
            onSelectLocation={handleSelectLocation}
            userLocation={userLocation}
            setUserLocation={setUserLocation}
          />
        </TabsContent>
      </Tabs>

      <LocationModal location={selectedLocation} isOpen={isModalOpen} onClose={handleCloseModal} />
    </div>
  )
  }

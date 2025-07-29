"use client"

import { useEffect, useState } from "react"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { MapView } from "@/components/merchants/map-view"
import { ListView } from "@/components/merchants/list-view"
import { MerchantModal } from "@/components/merchants/merchant-modal"
import { mockMerchants, defaultLocation } from "@/data/mock-merchants"
import { mockGoogleMerchants } from "@/data/mock-google-merchants"
import type { Merchant, UserLocation } from "@/types/merchant"
import { useApp } from "@/context/AppProvider"


export default function MerchantMapPage() {
  const [activeTab, setActiveTab] = useState("map")
  const [selectedMerchantType, setSelectedMerchantType] = useState("all")
  const [selectedMerchant, setSelectedMerchant] = useState<Merchant | null>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [userLocation, setUserLocation] = useState<UserLocation>(defaultLocation)
  const [merchants, setMerchants] = useState<Merchant[]>([])
  const { status } = useApp();

  useEffect(() => {
    loadMerchantData()
  }, [])


  const handleSelectMerchant = (merchant: Merchant) => {
    setSelectedMerchant(merchant)
    setIsModalOpen(true)
  }

  const handleCloseModal = () => {
    setIsModalOpen(false)
    setSelectedMerchant(null)
  }


  async function loadMerchantData() {
    let requests = []
    for (const merchant of mockGoogleMerchants) {
      const res = fetch(`https://places.googleapis.com/v1/places/${merchant.google_id}?fields=*&key=AIzaSyDushyc7TgeFyIlxbqiujHdydWDoVoHwNQ`);
      requests.push(res)

    }
    requests = await Promise.all(requests)
    let newMerchants: Merchant[] = []
    for (const res of requests) {
      if (!res.ok) {
        console.error("Failed to fetch data")
        continue
      }
      const data = await res.json();
      const tempMerchant = parseGoogleToMerchant(data)
      newMerchants.push(tempMerchant)
    }
    setMerchants(newMerchants)
  }

  function parseGoogleToMerchant(place_details: any): Merchant {
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
        <h1 className="text-3xl font-bold text-black dark:text-white">Merchant Map</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">Find merchants that accept SFLuv in your area</p>
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
            merchants={merchants}
            selectedMerchantType={selectedMerchantType}
            setSelectedMerchantType={setSelectedMerchantType}
            onSelectMerchant={handleSelectMerchant}
            userLocation={userLocation}
            setUserLocation={setUserLocation}
          />
        </TabsContent>
        <TabsContent value="list">
          <ListView
            merchants={merchants}
            selectedMerchantType={selectedMerchantType}
            setSelectedMerchantType={setSelectedMerchantType}
            onSelectMerchant={handleSelectMerchant}
            userLocation={userLocation}
            setUserLocation={setUserLocation}
          />
        </TabsContent>
      </Tabs>

      <MerchantModal merchant={selectedMerchant} isOpen={isModalOpen} onClose={handleCloseModal} />
    </div>
  )
  }
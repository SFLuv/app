"use client"

import { useState } from "react"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { MapView } from "@/components/merchants/map-view"
import { ListView } from "@/components/merchants/list-view"
import { MerchantModal } from "@/components/merchants/merchant-modal"
import { mockMerchants, defaultLocation } from "@/data/mock-merchants"
import type { Merchant, UserLocation } from "@/types/merchant"

export default function MerchantMapPage() {
  const [activeTab, setActiveTab] = useState("map")
  const [selectedMerchantType, setSelectedMerchantType] = useState("all")
  const [selectedMerchant, setSelectedMerchant] = useState<Merchant | null>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [userLocation, setUserLocation] = useState<UserLocation>(defaultLocation)

  const handleSelectMerchant = (merchant: Merchant) => {
    setSelectedMerchant(merchant)
    setIsModalOpen(true)
  }

  const handleCloseModal = () => {
    setIsModalOpen(false)
    setSelectedMerchant(null)
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
            merchants={mockMerchants}
            selectedMerchantType={selectedMerchantType}
            setSelectedMerchantType={setSelectedMerchantType}
            onSelectMerchant={handleSelectMerchant}
            userLocation={userLocation}
            setUserLocation={setUserLocation}
          />
        </TabsContent>
        <TabsContent value="list">
          <ListView
            merchants={mockMerchants}
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

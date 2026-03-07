"use client"

import { memo, useEffect, useRef, useState } from "react"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { MapView } from "@/components/locations/map-view"
import { ListView } from "@/components/locations/list-view"
import { LocationModal } from "@/components/locations/location-modal"
import { defaultLocation } from "@/data/mock-merchants"
import type { UserLocation } from "@/types/merchant"
import { useLocation } from "@/context/LocationProvider"
import { Location } from "@/types/location"
import { usePathname, useRouter, useSearchParams } from "next/navigation"


const LocationMapPageContent = memo(function LocationMapPageContent() {
  const search = useSearchParams()
  const pathname = usePathname()
  const router = useRouter()
  const tabFromQuery = search.get("tab")
  const activeTab = tabFromQuery === "list" ? "list" : "map"
  const [selectedLocationType, setSelectedLocationType] = useState("All Locations")
  const [selectedLocation, setSelectedLocation] = useState<Location | null>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [userLocation, setUserLocation] = useState<UserLocation>(defaultLocation)
  const { mapLocations, mapLocationsStatus, getMapLocations } = useLocation()
  const previousTabRef = useRef(activeTab)

  useEffect(() => {
    if (previousTabRef.current !== activeTab) {
      previousTabRef.current = activeTab
      void getMapLocations()
    }
  }, [activeTab, getMapLocations])

  const handleTabChange = (value: string) => {
    if (value !== "map" && value !== "list") return
    if (value === activeTab) return
    const params = new URLSearchParams(search.toString())
    params.set("tab", value)
    const nextQuery = params.toString()
    if (nextQuery !== search.toString()) {
      router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, { scroll: false })
    }
  }

  const handleSelectLocation = (location: Location) => {
    setSelectedLocation(location)
    setIsModalOpen(true)
  }

  const handleCloseModal = () => {
    setIsModalOpen(false)
    setSelectedLocation(null)
  }

  if (mapLocationsStatus === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }


  return (
    <div className={`space-y-6 ${search.get("sidebar") === "false" ? "p-5" : ""}`}>
      <div>
        <h1 className="text-3xl font-bold text-black dark:text-white">Location Map</h1>
        <p className="text-gray-600 dark:text-gray-400 mt-1">Find locations that accept SFLuv in your area</p>
      </div>

      <Tabs value={activeTab} onValueChange={handleTabChange} className="w-full">
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
})

export default function LocationMapPage() {
  return <LocationMapPageContent />
}

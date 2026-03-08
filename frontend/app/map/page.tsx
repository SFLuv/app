"use client"

import { memo, useEffect, useRef, useState } from "react"
import { Tabs, TabsContent } from "@/components/ui/tabs"
import { MapView } from "@/components/locations/map-view"
import { ListView } from "@/components/locations/list-view"
import { LocationModal } from "@/components/locations/location-modal"
import { defaultLocation } from "@/data/mock-merchants"
import type { UserLocation } from "@/types/merchant"
import { useLocation } from "@/context/LocationProvider"
import { Location } from "@/types/location"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { List, Map as MapIcon } from "lucide-react"
import { cn } from "@/lib/utils"

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
  const [isInitialLoading, setIsInitialLoading] = useState(true)
  const { mapLocations, getMapLocations } = useLocation()
  const previousTabRef = useRef(activeTab)

  useEffect(() => {
    let isMounted = true
    setIsInitialLoading(true)
    void getMapLocations().finally(() => {
      if (isMounted) {
        setIsInitialLoading(false)
      }
    })
    return () => {
      isMounted = false
    }
  }, [getMapLocations])

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

  if (isInitialLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }


  return (
    <div className={`mx-auto w-full max-w-6xl space-y-3 pt-4 sm:space-y-4 sm:pt-5 ${search.get("sidebar") === "false" ? "p-4 sm:p-6" : ""}`}>
      <section className="px-1">
        <h1 className="text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">Merchant Map</h1>
        <p className="mt-1 text-sm text-muted-foreground sm:text-base">Places that accept SFLuv.</p>
      </section>

      <div className="w-full px-1 sm:w-[340px]">
        <div className="relative grid grid-cols-2 rounded-lg bg-secondary p-1">
          <div
            className={cn(
              "absolute inset-y-1 left-1 w-[calc(50%-0.25rem)] rounded-md bg-[#eb6c6c] shadow-sm transition-transform duration-300 ease-out",
              activeTab === "map" ? "translate-x-0" : "translate-x-full",
            )}
          />
          <button
            type="button"
            className={cn(
              "relative z-10 inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium transition-colors",
              activeTab === "map" ? "text-white" : "text-foreground/80 hover:text-foreground",
            )}
            onClick={() => handleTabChange("map")}
          >
            <MapIcon className="h-4 w-4" />
            Map View
          </button>
          <button
            type="button"
            className={cn(
              "relative z-10 inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium transition-colors",
              activeTab === "list" ? "text-white" : "text-foreground/80 hover:text-foreground",
            )}
            onClick={() => handleTabChange("list")}
          >
            <List className="h-4 w-4" />
            List View
          </button>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={handleTabChange} className="w-full">
        <TabsContent value="map" className="mt-2">
          <MapView
            locations={mapLocations}
            selectedLocationType={selectedLocationType}
            setSelectedLocationType={setSelectedLocationType}
            onSelectLocation={handleSelectLocation}
            userLocation={userLocation}
            setUserLocation={setUserLocation}
          />
        </TabsContent>

        <TabsContent value="list" className="mt-2">
          <div className="rounded-2xl border bg-card/90 p-3 shadow-sm sm:p-4">
            <ListView
              locations={mapLocations}
              selectedLocationType={selectedLocationType}
              setSelectedLocationType={setSelectedLocationType}
              onSelectLocation={handleSelectLocation}
              userLocation={userLocation}
              setUserLocation={setUserLocation}
            />
          </div>
        </TabsContent>
      </Tabs>

      <LocationModal location={selectedLocation} isOpen={isModalOpen} onClose={handleCloseModal} />
    </div>
  )
})

export default function LocationMapPage() {
  return <LocationMapPageContent />
}

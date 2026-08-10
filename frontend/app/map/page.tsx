"use client"

import { memo, useCallback, useEffect, useState } from "react"
import { MapView } from "@/components/locations/map-view"
import { LocationModal } from "@/components/locations/location-modal"
import { defaultLocation } from "@/data/mock-merchants"
import type { UserLocation } from "@/types/merchant"
import { useLocation } from "@/context/LocationProvider"
import { useApp } from "@/context/AppProvider"
import { Location } from "@/types/location"
import { useRouter, useSearchParams } from "next/navigation"
import { buildMerchantRedirectPath } from "@/lib/redeem-link"
import { isAddress } from "viem"

const LocationMapPageContent = memo(function LocationMapPageContent() {
  const search = useSearchParams()
  const router = useRouter()
  const [selectedLocationType, setSelectedLocationType] = useState("All Locations")
  const [selectedLocation, setSelectedLocation] = useState<Location | null>(null)
  const [isModalOpen, setIsModalOpen] = useState(false)
  const [userLocation, setUserLocation] = useState<UserLocation>(defaultLocation)
  const [isInitialLoading, setIsInitialLoading] = useState(true)
  const { mapLocations, getMapLocations } = useLocation()
  const { status } = useApp()
  const isPayEnabled = status === "authenticated"

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

  // Stable so the memoised map pins are not invalidated on every render of this
  // page — an unstable callback here would undo the flicker fix in MapView.
  const handleSelectLocation = useCallback((location: Location) => {
    setSelectedLocation(location)
    setIsModalOpen(true)
  }, [])

  const handleCloseModal = () => {
    setIsModalOpen(false)
    setSelectedLocation(null)
  }

  const handlePayLocation = (location: Location) => {
    if (!isPayEnabled) return
    const payToAddress = (location.pay_to_address || "").trim()
    if (!isAddress(payToAddress)) return

    handleCloseModal()
    router.push(buildMerchantRedirectPath({
      to: payToAddress,
      tipTo: location.tip_to_address || null,
      locationId: location.id,
    }))
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

      {/*
        No view switcher: the map carries its own searchable merchant list
        beside it, which is what the List View tab existed to provide.
      */}
      <MapView
        locations={mapLocations}
        selectedLocationType={selectedLocationType}
        setSelectedLocationType={setSelectedLocationType}
        onSelectLocation={handleSelectLocation}
        userLocation={userLocation}
        setUserLocation={setUserLocation}
      />

      <LocationModal
        location={selectedLocation}
        isOpen={isModalOpen}
        onClose={handleCloseModal}
        isPayEnabled={isPayEnabled}
        onPayLocation={handlePayLocation}
      />
    </div>
  )
})

export default function LocationMapPage() {
  return <LocationMapPageContent />
}

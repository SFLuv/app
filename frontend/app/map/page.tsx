"use client"

import { memo, useCallback, useEffect, useState } from "react"
import { MapView } from "@/components/locations/map-view"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { SlidersHorizontal } from "lucide-react"
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
  // Starts at the city centre and upgrades to the real position if the browser
  // will give one. The merchant list sorts by distance from here, so an unknown
  // position sorts from downtown rather than refusing to sort at all.
  const [userLocation, setUserLocation] = useState<UserLocation>(defaultLocation)

  useEffect(() => {
    if (typeof navigator === "undefined" || !navigator.geolocation) return
    let cancelled = false
    navigator.geolocation.getCurrentPosition(
      (position) => {
        if (cancelled) return
        setUserLocation({ lat: position.coords.latitude, lng: position.coords.longitude })
      },
      // Denied or unavailable is not an error worth surfacing: the default
      // stands and the list is still ordered sensibly.
      () => undefined,
      { maximumAge: 300_000, timeout: 10_000 },
    )
    return () => {
      cancelled = true
    }
  }, [])
  const [isInitialLoading, setIsInitialLoading] = useState(true)
  const { mapLocations, getMapLocations, locationTypes } = useLocation()
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
      {/*
        The type filter sits on the title line rather than over the map: it
        acts on the whole page, and the running count it used to sit beside is
        now the "n of n shown" line in the map's own merchant list.
      */}
      <section className="flex flex-wrap items-start justify-between gap-3 px-1">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">Merchant Map</h1>
          <p className="mt-1 text-sm text-muted-foreground sm:text-base">Places that accept SFLuv.</p>
        </div>

        <Select value={selectedLocationType} onValueChange={setSelectedLocationType}>
          <SelectTrigger className="h-9 w-[180px] rounded-lg border-border/60 bg-background sm:w-[210px]">
            <div className="flex items-center gap-2">
              <SlidersHorizontal className="h-4 w-4 text-muted-foreground" />
              <SelectValue placeholder="Filter by type" />
            </div>
          </SelectTrigger>
          <SelectContent align="end">
            {locationTypes.map((type) => (
              <SelectItem key={type} value={type}>
                {type}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </section>

      {/*
        No view switcher: the map carries its own searchable merchant list
        beside it, which is what the List View tab existed to provide.
      */}
      <MapView
        locations={mapLocations}
        selectedLocationType={selectedLocationType}
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

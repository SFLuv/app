"use client"

import { useEffect, useMemo, useState } from "react"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardContent } from "@/components/ui/card"
import { Search, MapPin, Star, Phone, SlidersHorizontal } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import type { UserLocation } from "@/types/merchant"
import { calculateDistance, formatDistance } from "@/utils/location"
import { Pagination } from "@/components/opportunities/pagination"
import { useLocation } from "@/context/LocationProvider"
import { Location } from "@/types/location"

interface ListViewProps {
  locations: Location[]
  selectedLocationType: string
  setSelectedLocationType: (type: string) => void
  onSelectLocation: (location: Location) => void
  userLocation: UserLocation
  setUserLocation: (userlocation: UserLocation) => void
}

export function ListView({
  locations,
  selectedLocationType,
  setSelectedLocationType,
  onSelectLocation,
  userLocation,
}: ListViewProps) {
  const [currentPage, setCurrentPage] = useState(1)
  const [searchQuery, setSearchQuery] = useState("")
  const ITEMS_PER_PAGE = 5
  const { mapLocationsStatus, locationTypes } = useLocation()

  const filteredLocations = useMemo(
    () =>
      locations
        ?.filter(
          location =>
            (selectedLocationType === "All Locations" || (location.type || "").trim() === selectedLocationType) &&
            (searchQuery === "" ||
              location.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
              location.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
              location.city.toLowerCase().includes(searchQuery.toLowerCase())),
        )
        .map(location => ({
          ...location,
          distance: calculateDistance(userLocation.lat, userLocation.lng, location.lat, location.lng),
        }))
        .sort((a, b) => a.distance - b.distance),
    [locations, searchQuery, selectedLocationType, userLocation.lat, userLocation.lng],
  )

  const totalPages = Math.ceil(filteredLocations.length / ITEMS_PER_PAGE)
  const paginatedLocations = filteredLocations.slice((currentPage - 1) * ITEMS_PER_PAGE, currentPage * ITEMS_PER_PAGE)

  useEffect(() => {
    setCurrentPage(1)
  }, [searchQuery, selectedLocationType])

  useEffect(() => {
    if (currentPage > totalPages && totalPages > 0) {
      setCurrentPage(totalPages)
    }
  }, [currentPage, totalPages])

  const renderStars = (rating: number) => {
    return Array(5)
      .fill(0)
      .map((_, i) => (
        <Star
          key={i}
          className={`h-3.5 w-3.5 ${i < Math.floor(rating) ? "fill-yellow-400 text-yellow-400" : "text-muted-foreground/30"}`}
        />
      ))
  }

  if (mapLocationsStatus === "loading") {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-[#eb6c6c]"></div>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="rounded-xl border bg-muted/25 p-3 sm:p-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Search locations by name, city, or description"
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              className="h-10 rounded-lg border-border/60 bg-background pl-10 text-foreground"
            />
          </div>
          <Select value={selectedLocationType} onValueChange={setSelectedLocationType}>
            <SelectTrigger className="h-10 w-full rounded-lg border-border/60 bg-background sm:w-[220px]">
              <div className="flex items-center gap-2">
                <SlidersHorizontal className="h-4 w-4 text-muted-foreground" />
                <SelectValue placeholder="Filter by type" />
              </div>
            </SelectTrigger>
            <SelectContent>
              {locationTypes.map(type => (
                <SelectItem key={type} value={type}>
                  {type}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Badge variant="outline" className="rounded-full border-border/70 bg-background px-3 py-1 text-xs font-medium">
            {filteredLocations.length} result{filteredLocations.length === 1 ? "" : "s"}
          </Badge>
          {selectedLocationType !== "All Locations" ? (
            <Badge variant="outline" className="rounded-full border-border/70 bg-background px-3 py-1 text-xs font-medium">
              {selectedLocationType}
            </Badge>
          ) : null}
        </div>
      </div>

      <div className="space-y-4">
        {paginatedLocations.length === 0 ? (
          <div className="rounded-xl border border-dashed px-6 py-12 text-center">
            <h3 className="text-xl font-medium text-foreground">No locations found</h3>
            <p className="mt-2 text-muted-foreground">
              Try adjusting your search or filters to find locations
            </p>
          </div>
        ) : (
          paginatedLocations.map(location => (
            <Card
              key={location.google_id}
              className="cursor-pointer overflow-hidden rounded-xl border-border/70 transition-all hover:-translate-y-0.5 hover:shadow-md"
              onClick={() => onSelectLocation(location)}
            >
              <CardContent className="p-4 sm:p-5">
                <div className="flex h-full flex-col justify-between space-y-4">
                  <div className="space-y-3">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0 space-y-2">
                        <h3 className="truncate text-lg font-semibold text-foreground sm:text-xl">{location.name}</h3>
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge variant="outline" className="rounded-full bg-muted px-2.5 py-0.5 text-xs font-medium">
                            {location.type.charAt(0).toUpperCase() + location.type.slice(1)}
                          </Badge>
                          <div className="flex items-center">
                            {renderStars(location.rating)}
                            <span className="ml-1 text-xs text-muted-foreground">{location.rating.toFixed(1)}</span>
                          </div>
                        </div>
                      </div>
                      <Badge className="rounded-full bg-[#eb6c6c] px-2.5 py-0.5 text-xs font-semibold text-white">
                        {formatDistance(location.distance)}
                      </Badge>
                    </div>

                    <p className="line-clamp-2 text-sm text-muted-foreground sm:text-[0.95rem]">{location.description}</p>
                  </div>

                  <div className="grid gap-2 text-sm text-muted-foreground sm:grid-cols-2">
                    <div className="flex items-center">
                      <MapPin className="mr-2 h-4 w-4 text-[#eb6c6c]" />
                      <span className="line-clamp-1">
                        {location.street}, {location.city}
                      </span>
                    </div>
                    <div className="flex items-center">
                      <Phone className="mr-2 h-4 w-4 text-[#eb6c6c]" />
                      <span className="line-clamp-1">{location.phone || "Not available"}</span>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          ))
        )}
      </div>

      {totalPages > 1 && (
        <div className="mt-6 rounded-xl border bg-muted/25 px-2 py-3 sm:px-3">
          <Pagination currentPage={currentPage} totalPages={totalPages} onPageChange={setCurrentPage} />
        </div>
      )}

      <div className="px-1 text-sm text-muted-foreground">
        Showing {paginatedLocations.length} of {filteredLocations.length} location
        {filteredLocations.length === 1 ? "" : "s"}
        {selectedLocationType !== "All Locations" ? ` in ${selectedLocationType}` : ""}
      </div>
    </div>
  )
}

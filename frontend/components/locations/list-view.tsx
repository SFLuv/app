"use client"

import { useState } from "react"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardContent } from "@/components/ui/card"
import { Search, MapPin, Star, Phone, Clock } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import Image from "next/image"
import type { Merchant, UserLocation } from "@/types/merchant"
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
  setUserLocation,
}: ListViewProps) {
  const [locationInput, setLocationInput] = useState(userLocation.address || "")
  const [currentPage, setCurrentPage] = useState(1)
  const [searchQuery, setSearchQuery] = useState("")
  const ITEMS_PER_PAGE = 5
  const { mapLocationsStatus, locationTypes } = useLocation();
  console.log(locationTypes)

  // Filter merchants by type and search query
  const filteredLocations = locations?.filter(
      (location) =>
        (selectedLocationType === "All Locations" || location.type === selectedLocationType) &&
        (searchQuery === "" ||
          location.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
          location.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
          location.city.toLowerCase().includes(searchQuery.toLowerCase())),
    )
    .map((location) => ({
      ...location,
      distance: calculateDistance(
        userLocation.lat,
        userLocation.lng,
        location.lat,
        location.lng,
      ),
    }))
    .sort((a, b) => a.distance - b.distance)

  // Calculate pagination
  const totalPages = Math.ceil(filteredLocations.length / ITEMS_PER_PAGE)
  const paginatedLocations = filteredLocations.slice((currentPage - 1) * ITEMS_PER_PAGE, currentPage * ITEMS_PER_PAGE)

  // Handle location search
  const handleLocationSearch = () => {
    if (!locationInput) return

    // In a real implementation, we would use the Google Maps Geocoding API
    // For now, we'll just simulate finding the location
    console.log(`Searching for location: ${locationInput}`)

    // Simulate geocoding result with a slight offset from the current location
    const newLocation = {
      lat: userLocation.lat + (Math.random() - 0.5) * 0.02,
      lng: userLocation.lng + (Math.random() - 0.5) * 0.02,
      address: locationInput,
    }

    setUserLocation(newLocation)
  }

  // Render stars for ratings
  const renderStars = (rating: number) => {
    return Array(5)
      .fill(0)
      .map((_, i) => (
        <Star
          key={i}
          className={`h-3 w-3 ${i < Math.floor(rating) ? "text-yellow-400 fill-yellow-400" : "text-gray-300"}`}
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
      <div className="flex flex-col sm:flex-row gap-4">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 h-4 w-4" />
          <Input
            placeholder="Search merchants..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10 text-black dark:text-white bg-secondary"
          />
        </div>
        <div className="relative flex-1">
          <MapPin className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 h-4 w-4" />
          <Input
            placeholder="Enter your location..."
            value={locationInput}
            onChange={(e) => setLocationInput(e.target.value)}
            className="pl-10 text-black dark:text-white bg-secondary"
            onKeyDown={(e) => e.key === "Enter" && handleLocationSearch()}
          />
        </div>
        <Button onClick={handleLocationSearch} className="bg-[#eb6c6c] hover:bg-[#d55c5c]">
          Search
        </Button>
        <Select value={selectedLocationType} onValueChange={setSelectedLocationType}>
          <SelectTrigger className="w-[180px] text-black dark:text-white bg-secondary">
            <SelectValue placeholder="Filter by type" />
          </SelectTrigger>
          <SelectContent>
            {locationTypes.map((type) => (
              <SelectItem key={type} value={type}>
                {type}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-4">
        {paginatedLocations.length === 0 ? (
          <div className="text-center py-12">
            <h3 className="text-xl font-medium text-black dark:text-white">No locations found</h3>
            <p className="text-gray-600 dark:text-gray-400 mt-2">
              Try adjusting your search or filters to find locations
            </p>
          </div>
        ) : (
          paginatedLocations.map((location) => (
            <Card
              key={location.name}
              className="overflow-hidden cursor-pointer hover:shadow-md transition-shadow"
              onClick={() => onSelectLocation(location)}
            >
              <div className="flex flex-col md:flex-row">
                <div className="relative h-48 md:h-auto md:w-48 flex-shrink-0">
                  <Image
                    src={location.image_url || "/placeholder.svg?height=200&width=200"}
                    alt={location.name}
                    fill
                    className="object-cover"
                  />
                </div>
                <CardContent className="flex-1 p-4">
                  <div className="flex flex-col h-full justify-between">
                    <div>
                      <div className="flex justify-between items-start">
                        <div>
                          <h3 className="text-xl font-semibold text-black dark:text-white">{location.name}</h3>
                          <div className="flex items-center mt-1 mb-2">
                            <Badge variant="outline" className="mr-2 bg-secondary text-black dark:text-white">
                              {location.type.charAt(0).toUpperCase() + location.type.slice(1)}
                            </Badge>
                            <div className="flex items-center">
                              {renderStars(location.rating)}
                              <span className="ml-1 text-sm text-gray-600 dark:text-gray-400">
                                {location.rating.toFixed(1)}
                              </span>
                            </div>
                          </div>
                        </div>
                        <Badge className="bg-[#eb6c6c]">{formatDistance(location.distance)}</Badge>
                      </div>
                      <p className="text-gray-600 dark:text-gray-300 line-clamp-2 mb-4">{location.description}</p>
                    </div>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-sm">
                      <div className="flex items-center text-gray-600 dark:text-gray-300">
                        <MapPin className="h-4 w-4 mr-2 text-[#eb6c6c]" />
                        <span>
                          {location.street}, {location.city}
                        </span>
                      </div>
                      <div className="flex items-center text-gray-600 dark:text-gray-300">
                        <Phone className="h-4 w-4 mr-2 text-[#eb6c6c]" />
                        <span>{location.phone || "Not Available"}</span>
                      </div>
                    </div>
                  </div>
                </CardContent>
              </div>
            </Card>
          ))
        )}
      </div>

      {totalPages > 1 && (
        <div className="mt-8">
          <Pagination currentPage={currentPage} totalPages={totalPages} onPageChange={setCurrentPage} />
        </div>
      )}

      <div className="text-sm text-gray-500 dark:text-gray-400">
        Showing {paginatedLocations.length} of {filteredLocations.length} merchants
        {selectedLocationType !== "all" &&
          ` of type: ${locationTypes.find((t) => t === selectedLocationType)}`}
      </div>
    </div>
  )
}

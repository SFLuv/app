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
import { merchantTypes } from "@/data/mock-merchants"
import { calculateDistance, formatDistance } from "@/utils/location"
import { Pagination } from "@/components/opportunities/pagination"
import { merchantTypeLabels } from "@/types/merchant"

interface ListViewProps {
  merchants: Merchant[]
  selectedMerchantType: string
  setSelectedMerchantType: (type: string) => void
  onSelectMerchant: (merchant: Merchant) => void
  userLocation: UserLocation
  setUserLocation: (location: UserLocation) => void
}

export function ListView({
  merchants,
  selectedMerchantType,
  setSelectedMerchantType,
  onSelectMerchant,
  userLocation,
  setUserLocation,
}: ListViewProps) {
  const [locationInput, setLocationInput] = useState(userLocation.address || "")
  const [currentPage, setCurrentPage] = useState(1)
  const [searchQuery, setSearchQuery] = useState("")
  const ITEMS_PER_PAGE = 5

  // Filter merchants by type and search query
  const filteredMerchants = merchants
    .filter(
      (merchant) =>
        (selectedMerchantType === "all" || merchant.type === selectedMerchantType) &&
        (searchQuery === "" ||
          merchant.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
          merchant.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
          merchant.address.city.toLowerCase().includes(searchQuery.toLowerCase())),
    )
    .map((merchant) => ({
      ...merchant,
      distance: calculateDistance(
        userLocation.lat,
        userLocation.lng,
        merchant.address.coordinates.lat,
        merchant.address.coordinates.lng,
      ),
    }))
    .sort((a, b) => a.distance - b.distance)

  // Calculate pagination
  const totalPages = Math.ceil(filteredMerchants.length / ITEMS_PER_PAGE)
  const paginatedMerchants = filteredMerchants.slice((currentPage - 1) * ITEMS_PER_PAGE, currentPage * ITEMS_PER_PAGE)

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
          Update Location
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

      <div className="space-y-4">
        {paginatedMerchants.length === 0 ? (
          <div className="text-center py-12">
            <h3 className="text-xl font-medium text-black dark:text-white">No merchants found</h3>
            <p className="text-gray-600 dark:text-gray-400 mt-2">
              Try adjusting your search or filters to find merchants
            </p>
          </div>
        ) : (
          paginatedMerchants.map((merchant) => (
            <Card
              key={merchant.name}
              className="overflow-hidden cursor-pointer hover:shadow-md transition-shadow"
              onClick={() => onSelectMerchant(merchant)}
            >
              <div className="flex flex-col md:flex-row">
                <div className="relative h-48 md:h-auto md:w-48 flex-shrink-0">
                  <Image
                    src={merchant.imageUrl || "/placeholder.svg?height=200&width=200"}
                    alt={merchant.name}
                    fill
                    className="object-cover"
                  />
                </div>
                <CardContent className="flex-1 p-4">
                  <div className="flex flex-col h-full justify-between">
                    <div>
                      <div className="flex justify-between items-start">
                        <div>
                          <h3 className="text-xl font-semibold text-black dark:text-white">{merchant.name}</h3>
                          <div className="flex items-center mt-1 mb-2">
                            <Badge variant="outline" className="mr-2 bg-secondary text-black dark:text-white">
                              {merchant.type.charAt(0).toUpperCase() + merchant.type.slice(1)}
                            </Badge>
                            <div className="flex items-center">
                              {renderStars(merchant.rating)}
                              <span className="ml-1 text-sm text-gray-600 dark:text-gray-400">
                                {merchant.rating.toFixed(1)}
                              </span>
                            </div>
                          </div>
                        </div>
                        <Badge className="bg-[#eb6c6c]">{formatDistance(merchant.distance)}</Badge>
                      </div>
                      <p className="text-gray-600 dark:text-gray-300 line-clamp-2 mb-4">{merchant.description}</p>
                    </div>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-2 text-sm">
                      <div className="flex items-center text-gray-600 dark:text-gray-300">
                        <MapPin className="h-4 w-4 mr-2 text-[#eb6c6c]" />
                        <span>
                          {merchant.address.street}, {merchant.address.city}
                        </span>
                      </div>
                      <div className="flex items-center text-gray-600 dark:text-gray-300">
                        <Phone className="h-4 w-4 mr-2 text-[#eb6c6c]" />
                        <span>{merchant.contactInfo.phone}</span>
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
        Showing {paginatedMerchants.length} of {filteredMerchants.length} merchants
        {selectedMerchantType !== "all" &&
          ` of type: ${merchantTypes.find((t) => t.value === selectedMerchantType)?.label}`}
      </div>
    </div>
  )
}
